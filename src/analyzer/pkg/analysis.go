package pkg

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/dpuig/ingress-shift/src/analyzer/pkg/knowledgebase"
)

// AnalysisReport represents the overall analysis report
type AnalysisReport struct {
	TotalIngresses           int                       `json:"total_ingresses" yaml:"total_ingresses"`
	Translatable             int                       `json:"translatable" yaml:"translatable"`
	NeedsManualIntervention  int                       `json:"needs_manual_intervention" yaml:"needs_manual_intervention"`
	NoEquivalent             int                       `json:"no_equivalent" yaml:"no_equivalent"`
	PercentTranslatable      float64                   `json:"percent_translatable" yaml:"percent_translatable"`
	ComplexityScore          float64                   `json:"complexity_score" yaml:"complexity_score"`
	Recommendations          []string                  `json:"recommendations" yaml:"recommendations"`
	ControllerRecommendation *ControllerRecommendation `json:"controller_recommendation,omitempty" yaml:"controller_recommendation,omitempty"`
	ManualInterventions      []ManualIntervention      `json:"manual_interventions" yaml:"manual_interventions"`
	AnnotationClasses        []AnnotationClass         `json:"annotation_classes" yaml:"annotation_classes"`
	Contexts                 []ContextResult           `json:"contexts,omitempty" yaml:"contexts,omitempty"`
}

// ManualIntervention is a single knowledge-base-backed item requiring manual work,
// with an effort estimate, as required by PLAN.md Workstream A.
type ManualIntervention struct {
	Annotation string `json:"annotation" yaml:"annotation"`
	Reason     string `json:"reason" yaml:"reason"`
	Effort     string `json:"effort" yaml:"effort"`
	Count      int    `json:"count" yaml:"count"`
}

// ControllerRecommendation names a target Gateway API controller with stated reasoning.
type ControllerRecommendation struct {
	Controller string   `json:"controller" yaml:"controller"`
	Reasoning  []string `json:"reasoning" yaml:"reasoning"`
}

// ContextResult summarizes a single kubeconfig context's contribution to a
// multi-context run.
type ContextResult struct {
	Context        string `json:"context" yaml:"context"`
	TotalIngresses int    `json:"total_ingresses" yaml:"total_ingresses"`
	Error          string `json:"error,omitempty" yaml:"error,omitempty"`
}

// AnnotationClass represents a class of annotations
type AnnotationClass struct {
	Name                   string       `json:"name" yaml:"name"`
	Description            string       `json:"description" yaml:"description"`
	Category               string       `json:"category" yaml:"category"`
	Effort                 string       `json:"effort" yaml:"effort"`
	GatewayAPINote         string       `json:"gateway_api_note" yaml:"gateway_api_note"`
	Count                  int          `json:"count" yaml:"count"`
	IsTranslatable         bool         `json:"is_translatable" yaml:"is_translatable"`
	RequiresExtension      bool         `json:"requires_extension" yaml:"requires_extension"`
	NoEquivalent           bool         `json:"no_equivalent" yaml:"no_equivalent"`
	BreaksNaiveTranslation bool         `json:"breaks_naive_translation" yaml:"breaks_naive_translation"`
	Annotations            []Annotation `json:"annotations" yaml:"annotations"`
}

// Annotation represents a single annotation
type Annotation struct {
	Name      string `json:"name" yaml:"name"`
	Value     string `json:"value" yaml:"value"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Resource  string `json:"resource" yaml:"resource"`
}

// KubernetesClientInterface for easier testing
type KubernetesClientInterface interface {
	listIngressesInNamespace(ctx context.Context, namespace string) ([]runtime.Object, error)
	listIngressesInAllNamespaces(ctx context.Context) ([]runtime.Object, error)
}

// AnalyzeIngressResources analyzes ingress resources from a single client/context
// and generates a fully finalized report (complexity score, recommendations, etc.
// already computed).
func AnalyzeIngressResources(ctx context.Context, client KubernetesClientInterface, namespace string, verbose bool) (*AnalysisReport, error) {
	var allObjects []runtime.Object
	var err error

	if namespace == "" {
		allObjects, err = client.listIngressesInAllNamespaces(ctx)
	} else {
		allObjects, err = client.listIngressesInNamespace(ctx, namespace)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list ingress resources: %w", err)
	}

	report := &AnalysisReport{
		TotalIngresses:    len(allObjects),
		AnnotationClasses: make([]AnnotationClass, 0),
	}

	if verbose {
		fmt.Printf("Found %d ingress resources\n", len(allObjects))
	}

	for _, obj := range allObjects {
		partialObj := obj.(*metav1.PartialObjectMetadata)

		for annotationName, annotationValue := range partialObj.Annotations {
			if verbose {
				fmt.Printf("Processing annotation: %s = %s\n", annotationName, annotationValue)
			}

			entry, found := knowledgebase.Lookup(annotationName)
			if !found {
				entry = knowledgebase.Unknown(annotationName)
			}

			report.addAnnotationClass(entry)

			for i := range report.AnnotationClasses {
				if report.AnnotationClasses[i].Name == annotationName {
					report.AnnotationClasses[i].Annotations = append(report.AnnotationClasses[i].Annotations, Annotation{
						Name:      annotationName,
						Value:     annotationValue,
						Namespace: partialObj.Namespace,
						Resource:  partialObj.Name,
					})
					break
				}
			}
		}
	}

	report.finalize()
	return report, nil
}

// MergeReports combines per-context reports (keyed by context name) into a
// single aggregate report, preserving a per-context breakdown for multi-context
// clusters. Annotation classes with the same name are summed rather than
// duplicated.
func MergeReports(perContext map[string]*AnalysisReport) *AnalysisReport {
	merged := &AnalysisReport{
		AnnotationClasses: make([]AnnotationClass, 0),
		Contexts:          make([]ContextResult, 0, len(perContext)),
	}

	contextNames := make([]string, 0, len(perContext))
	for name := range perContext {
		contextNames = append(contextNames, name)
	}
	sort.Strings(contextNames)

	for _, name := range contextNames {
		report := perContext[name]
		merged.Contexts = append(merged.Contexts, ContextResult{
			Context:        name,
			TotalIngresses: report.TotalIngresses,
		})
		merged.TotalIngresses += report.TotalIngresses

		for _, class := range report.AnnotationClasses {
			merged.addAnnotationClassRaw(class)
		}
	}

	merged.finalize()
	return merged
}

// addAnnotationClass adds or increments an annotation class from a knowledge base entry.
func (r *AnalysisReport) addAnnotationClass(entry knowledgebase.Entry) {
	for i := range r.AnnotationClasses {
		if r.AnnotationClasses[i].Name == entry.Name {
			r.AnnotationClasses[i].Count++
			return
		}
	}

	r.AnnotationClasses = append(r.AnnotationClasses, AnnotationClass{
		Name:                   entry.Name,
		Description:            entry.Description,
		Category:               entry.Category,
		Effort:                 string(entry.Effort),
		GatewayAPINote:         entry.GatewayAPINote,
		Count:                  1,
		IsTranslatable:         entry.IsTranslatable(),
		RequiresExtension:      entry.RequiresExtension(),
		NoEquivalent:           entry.NoEquivalent(),
		BreaksNaiveTranslation: entry.BreaksNaiveTranslation,
		Annotations:            make([]Annotation, 0),
	})
}

// addAnnotationClassRaw merges an already-built AnnotationClass (used when
// combining per-context reports, where classes may repeat across contexts).
func (r *AnalysisReport) addAnnotationClassRaw(class AnnotationClass) {
	for i := range r.AnnotationClasses {
		if r.AnnotationClasses[i].Name == class.Name {
			r.AnnotationClasses[i].Count += class.Count
			r.AnnotationClasses[i].Annotations = append(r.AnnotationClasses[i].Annotations, class.Annotations...)
			return
		}
	}
	r.AnnotationClasses = append(r.AnnotationClasses, class)
}

// finalize recomputes every derived field (statistics, complexity score,
// manual interventions, controller recommendation, and free-text
// recommendations) from AnnotationClasses and TotalIngresses. Called once
// AnnotationClasses is fully populated, whether from a single context or a
// merge of several.
func (r *AnalysisReport) finalize() {
	r.Translatable = 0
	r.NeedsManualIntervention = 0
	r.NoEquivalent = 0
	r.ManualInterventions = make([]ManualIntervention, 0)

	for _, class := range r.AnnotationClasses {
		switch {
		case class.IsTranslatable:
			r.Translatable++
		case class.RequiresExtension:
			r.NeedsManualIntervention++
			r.ManualInterventions = append(r.ManualInterventions, ManualIntervention{
				Annotation: class.Name,
				Reason:     class.GatewayAPINote,
				Effort:     class.Effort,
				Count:      class.Count,
			})
		case class.NoEquivalent:
			r.NoEquivalent++
			r.ManualInterventions = append(r.ManualInterventions, ManualIntervention{
				Annotation: class.Name,
				Reason:     class.GatewayAPINote,
				Effort:     class.Effort,
				Count:      class.Count,
			})
		}
	}

	sort.Slice(r.ManualInterventions, func(i, j int) bool {
		return r.ManualInterventions[i].Count > r.ManualInterventions[j].Count
	})

	r.calculateComplexityScore()
	r.calculatePercentTranslatable()
	r.recommendController()
	r.addRecommendations()
}

// calculateComplexityScore calculates the complexity score based on annotation usage
func (r *AnalysisReport) calculateComplexityScore() {
	total := r.Translatable + r.NeedsManualIntervention + r.NoEquivalent
	if total == 0 {
		r.ComplexityScore = 0
		return
	}

	nonTranslatable := r.NeedsManualIntervention + r.NoEquivalent
	r.ComplexityScore = float64(nonTranslatable) / float64(total) * 100
}

// calculatePercentTranslatable calculates the percentage of distinct annotation
// classes with a direct Gateway API equivalent, as required by PLAN.md's
// "percentage of resources directly translatable" deliverable.
func (r *AnalysisReport) calculatePercentTranslatable() {
	total := r.Translatable + r.NeedsManualIntervention + r.NoEquivalent
	if total == 0 {
		r.PercentTranslatable = 100
		return
	}
	r.PercentTranslatable = float64(r.Translatable) / float64(total) * 100
}

// addRecommendations adds recommendations based on the analysis results
func (r *AnalysisReport) addRecommendations() {
	r.Recommendations = make([]string, 0)

	if r.TotalIngresses == 0 {
		r.Recommendations = append(r.Recommendations, "No Ingress resources found in the cluster")
		return
	}

	switch {
	case r.ComplexityScore >= 75:
		r.Recommendations = append(r.Recommendations, "High migration complexity detected. Consider a phased approach with extensive manual intervention.")
		r.Recommendations = append(r.Recommendations, "Allocate significant time for custom controller development or extension work.")
	case r.ComplexityScore >= 50:
		r.Recommendations = append(r.Recommendations, "Moderate migration complexity. Plan for some manual interventions and controller extensions.")
		r.Recommendations = append(r.Recommendations, "Consider using a controller with broader annotation support.")
	case r.ComplexityScore >= 25:
		r.Recommendations = append(r.Recommendations, "Low to moderate migration complexity. Standard migration approach should work well.")
		r.Recommendations = append(r.Recommendations, "Ensure you have the right Gateway API controller installed.")
	default:
		r.Recommendations = append(r.Recommendations, "Low migration complexity. Consider a straightforward migration approach.")
		r.Recommendations = append(r.Recommendations, "Standard Gateway API controller should be sufficient for this workload.")
	}

	if r.NeedsManualIntervention > 0 {
		r.Recommendations = append(r.Recommendations, fmt.Sprintf("Found %d annotation classes requiring manual intervention or controller extensions", r.NeedsManualIntervention))
	}

	if r.NoEquivalent > 0 {
		r.Recommendations = append(r.Recommendations, fmt.Sprintf("Found %d annotation classes with no Gateway API equivalent", r.NoEquivalent))
		r.Recommendations = append(r.Recommendations, "These will require custom controller logic or workarounds")
	}
}

// controllerProfile is a named Gateway API controller and the annotation
// categories it's a particularly strong fit for, used by recommendController.
type controllerProfile struct {
	name      string
	strengths map[string]string // category -> reason
	fallback  string            // reasoning used when recommended as the general-purpose default
}

var controllerProfiles = []controllerProfile{
	{
		name: "Envoy Gateway",
		strengths: map[string]string{
			"snippet":  "its EnvoyFilter/WASM extension points give the broadest escape hatch for replicating custom NGINX config and auth snippets",
			"affinity": "BackendTrafficPolicy supports native session persistence configuration",
		},
		fallback: "it has the broadest Gateway API conformance and extension policy coverage of any controller in this table, making it the safest general-purpose default",
	},
	{
		name: "Istio",
		strengths: map[string]string{
			"mtls": "mTLS and client certificate verification are first-class, service-mesh-native features rather than bolt-on policies",
		},
	},
	{
		name: "Kong Gateway",
		strengths: map[string]string{
			"auth":       "its plugin ecosystem covers auth, external auth, and CORS natively without custom policy authoring",
			"rate-limit": "rate limiting plugins (local and distributed) are mature and directly configurable",
			"cors":       "CORS is a first-class, well-tested plugin rather than an experimental-channel filter",
		},
	},
	{
		name:      "GKE Gateway",
		strengths: map[string]string{},
		fallback:  "for a low-complexity workload with mostly core-translatable annotations, a fully managed controller minimizes operational overhead",
	},
}

// recommendController names a target Gateway API controller with stated
// reasoning, matched against which extension capabilities the cluster's
// annotations actually need.
func (r *AnalysisReport) recommendController() {
	if r.TotalIngresses == 0 {
		r.ControllerRecommendation = nil
		return
	}

	categoryCounts := make(map[string]int)
	for _, class := range r.AnnotationClasses {
		if class.RequiresExtension || class.NoEquivalent {
			categoryCounts[class.Category] += class.Count
		}
	}

	// Score each controller by how many of the cluster's needed categories it's a strong fit for.
	type scored struct {
		profile controllerProfile
		score   int
		reasons []string
	}
	var candidates []scored

	for _, profile := range controllerProfiles {
		s := scored{profile: profile}
		for category := range categoryCounts {
			if reason, ok := profile.strengths[category]; ok {
				s.score++
				s.reasons = append(s.reasons, fmt.Sprintf("%d uses of %q-category annotations: %s", categoryCounts[category], category, reason))
			}
		}
		sort.Strings(s.reasons)
		candidates = append(candidates, s)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	best := candidates[0]
	if best.score == 0 {
		// No category-specific strength matched; fall back to the lowest-complexity-appropriate default.
		if r.ComplexityScore < 25 {
			for _, c := range candidates {
				if c.profile.name == "GKE Gateway" {
					best = c
					break
				}
			}
		} else {
			for _, c := range candidates {
				if c.profile.name == "Envoy Gateway" {
					best = c
					break
				}
			}
		}
		r.ControllerRecommendation = &ControllerRecommendation{
			Controller: best.profile.name,
			Reasoning:  []string{best.profile.fallback},
		}
		return
	}

	r.ControllerRecommendation = &ControllerRecommendation{
		Controller: best.profile.name,
		Reasoning:  best.reasons,
	}
}
