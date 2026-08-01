// Package traffic patches a Gateway API HTTPRoute's weighted backendRefs to
// perform staged traffic shifts. It uses Gateway API's native weight field
// rather than a custom traffic-splitting layer, per SPEC.md's design
// decision. It talks to HTTPRoute as an unstructured resource via the
// dynamic client rather than depending on the generated gateway-api
// clientset, keeping the dependency footprint to what's already vendored
// through client-go.
package traffic

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// HTTPRouteGVR identifies the Gateway API HTTPRoute resource.
var HTTPRouteGVR = schema.GroupVersionResource{
	Group:    "gateway.networking.k8s.io",
	Version:  "v1",
	Resource: "httproutes",
}

// BackendWeight is one backendRef's target weight, addressed by the
// Kubernetes Service name it refers to.
type BackendWeight struct {
	Name   string
	Weight int32
}

// Shifter patches a single HTTPRoute's backendRefs[].weight fields to
// perform a traffic shift. It only ever touches the weight field on
// backendRefs already present in the route — it never adds, removes, or
// otherwise modifies the route.
type Shifter struct {
	client    dynamic.Interface
	namespace string
	name      string
}

// NewShifter builds a Shifter targeting one HTTPRoute.
func NewShifter(client dynamic.Interface, namespace, name string) *Shifter {
	return &Shifter{client: client, namespace: namespace, name: name}
}

// SetWeights patches the target HTTPRoute so every backendRef whose name
// matches a given BackendWeight gets that weight, across every rule in the
// route. Unmatched backendRefs are left untouched.
func (s *Shifter) SetWeights(ctx context.Context, weights []BackendWeight) error {
	route, err := s.client.Resource(HTTPRouteGVR).Namespace(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get HTTPRoute %s/%s: %w", s.namespace, s.name, err)
	}

	weightByName := make(map[string]int32, len(weights))
	for _, w := range weights {
		weightByName[w.Name] = w.Weight
	}

	rules, found, err := unstructured.NestedSlice(route.Object, "spec", "rules")
	if err != nil {
		return fmt.Errorf("failed to read spec.rules: %w", err)
	}
	if !found {
		return fmt.Errorf("HTTPRoute %s/%s has no spec.rules", s.namespace, s.name)
	}

	matched := make(map[string]bool, len(weights))

	for i, ruleRaw := range rules {
		rule, ok := ruleRaw.(map[string]any)
		if !ok {
			continue
		}

		backendRefs, found, err := unstructured.NestedSlice(rule, "backendRefs")
		if err != nil || !found {
			continue
		}

		for j, refRaw := range backendRefs {
			ref, ok := refRaw.(map[string]any)
			if !ok {
				continue
			}

			name, _, _ := unstructured.NestedString(ref, "name")
			weight, hasWeight := weightByName[name]
			if !hasWeight {
				continue
			}

			if err := unstructured.SetNestedField(ref, int64(weight), "weight"); err != nil {
				return fmt.Errorf("failed to set weight for backend %q: %w", name, err)
			}
			backendRefs[j] = ref
			matched[name] = true
		}

		if err := unstructured.SetNestedSlice(rule, backendRefs, "backendRefs"); err != nil {
			return fmt.Errorf("failed to update backendRefs: %w", err)
		}
		rules[i] = rule
	}

	for _, w := range weights {
		if !matched[w.Name] {
			return fmt.Errorf("backend %q not found in any rule of HTTPRoute %s/%s", w.Name, s.namespace, s.name)
		}
	}

	if err := unstructured.SetNestedSlice(route.Object, rules, "spec", "rules"); err != nil {
		return fmt.Errorf("failed to update spec.rules: %w", err)
	}

	_, err = s.client.Resource(HTTPRouteGVR).Namespace(s.namespace).Update(ctx, route, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update HTTPRoute %s/%s: %w", s.namespace, s.name, err)
	}
	return nil
}

// CurrentWeights reads back the current weight of every backendRef in the
// target HTTPRoute, keyed by Service name.
func (s *Shifter) CurrentWeights(ctx context.Context) (map[string]int32, error) {
	route, err := s.client.Resource(HTTPRouteGVR).Namespace(s.namespace).Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTPRoute %s/%s: %w", s.namespace, s.name, err)
	}

	rules, found, err := unstructured.NestedSlice(route.Object, "spec", "rules")
	if err != nil || !found {
		return nil, fmt.Errorf("HTTPRoute %s/%s has no spec.rules", s.namespace, s.name)
	}

	weights := make(map[string]int32)
	for _, ruleRaw := range rules {
		rule, ok := ruleRaw.(map[string]any)
		if !ok {
			continue
		}
		backendRefs, found, err := unstructured.NestedSlice(rule, "backendRefs")
		if err != nil || !found {
			continue
		}
		for _, refRaw := range backendRefs {
			ref, ok := refRaw.(map[string]any)
			if !ok {
				continue
			}
			name, _, _ := unstructured.NestedString(ref, "name")
			weight, found, _ := unstructured.NestedInt64(ref, "weight")
			if found {
				weights[name] = int32(weight)
			}
		}
	}

	return weights, nil
}

// ParseStages parses a comma-separated candidate weight progression like
// "1,5,25,50,100" into an ordered slice of weights.
func ParseStages(csv string) ([]int32, error) {
	var stages []int32
	start := 0
	for i := 0; i <= len(csv); i++ {
		if i == len(csv) || csv[i] == ',' {
			token := csv[start:i]
			start = i + 1
			if token == "" {
				continue
			}
			var val int32
			if _, err := fmt.Sscanf(token, "%d", &val); err != nil {
				return nil, fmt.Errorf("invalid stage weight %q: %w", token, err)
			}
			if val < 0 || val > 100 {
				return nil, fmt.Errorf("stage weight %d out of range [0,100]", val)
			}
			stages = append(stages, val)
		}
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("no stages parsed from %q", csv)
	}
	if stages[len(stages)-1] != 100 {
		return nil, fmt.Errorf("last stage must be 100 (full cutover), got %d", stages[len(stages)-1])
	}
	return stages, nil
}
