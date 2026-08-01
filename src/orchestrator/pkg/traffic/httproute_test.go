package traffic

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newTestRoute() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":      "checkout",
				"namespace": "default",
			},
			"spec": map[string]any{
				"rules": []any{
					map[string]any{
						"backendRefs": []any{
							map[string]any{"name": "incumbent-svc", "weight": int64(100)},
							map[string]any{"name": "candidate-svc", "weight": int64(0)},
						},
					},
				},
			},
		},
	}
}

func newFakeClient(t *testing.T, objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		HTTPRouteGVR: "HTTPRouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

func TestSetWeightsUpdatesBothBackends(t *testing.T) {
	client := newFakeClient(t, newTestRoute())
	shifter := NewShifter(client, "default", "checkout")

	err := shifter.SetWeights(context.Background(), []BackendWeight{
		{Name: "incumbent-svc", Weight: 75},
		{Name: "candidate-svc", Weight: 25},
	})
	if err != nil {
		t.Fatalf("SetWeights failed: %v", err)
	}

	weights, err := shifter.CurrentWeights(context.Background())
	if err != nil {
		t.Fatalf("CurrentWeights failed: %v", err)
	}

	if weights["incumbent-svc"] != 75 {
		t.Errorf("expected incumbent-svc weight 75, got %d", weights["incumbent-svc"])
	}
	if weights["candidate-svc"] != 25 {
		t.Errorf("expected candidate-svc weight 25, got %d", weights["candidate-svc"])
	}
}

func TestSetWeightsRollbackIsSinglePatch(t *testing.T) {
	client := newFakeClient(t, newTestRoute())
	shifter := NewShifter(client, "default", "checkout")

	// Simulate mid-rollout state.
	if err := shifter.SetWeights(context.Background(), []BackendWeight{
		{Name: "incumbent-svc", Weight: 50},
		{Name: "candidate-svc", Weight: 50},
	}); err != nil {
		t.Fatalf("initial SetWeights failed: %v", err)
	}

	// Rollback: a single call reverting to 100% incumbent.
	if err := shifter.SetWeights(context.Background(), []BackendWeight{
		{Name: "incumbent-svc", Weight: 100},
		{Name: "candidate-svc", Weight: 0},
	}); err != nil {
		t.Fatalf("rollback SetWeights failed: %v", err)
	}

	weights, err := shifter.CurrentWeights(context.Background())
	if err != nil {
		t.Fatalf("CurrentWeights failed: %v", err)
	}
	if weights["incumbent-svc"] != 100 || weights["candidate-svc"] != 0 {
		t.Errorf("expected full rollback, got %+v", weights)
	}
}

func TestSetWeightsFailsForUnknownBackend(t *testing.T) {
	client := newFakeClient(t, newTestRoute())
	shifter := NewShifter(client, "default", "checkout")

	err := shifter.SetWeights(context.Background(), []BackendWeight{
		{Name: "does-not-exist", Weight: 100},
	})
	if err == nil {
		t.Fatal("expected an error for an unknown backend name")
	}
}

func TestParseStagesValid(t *testing.T) {
	stages, err := ParseStages("1,5,25,50,100")
	if err != nil {
		t.Fatalf("ParseStages failed: %v", err)
	}
	want := []int32{1, 5, 25, 50, 100}
	if len(stages) != len(want) {
		t.Fatalf("expected %d stages, got %d", len(want), len(stages))
	}
	for i := range want {
		if stages[i] != want[i] {
			t.Errorf("stage %d: expected %d, got %d", i, want[i], stages[i])
		}
	}
}

func TestParseStagesRequiresFinalStageOf100(t *testing.T) {
	_, err := ParseStages("1,5,25,50")
	if err == nil {
		t.Fatal("expected an error when the last stage isn't 100")
	}
}

func TestParseStagesRejectsOutOfRangeWeight(t *testing.T) {
	_, err := ParseStages("1,150,100")
	if err == nil {
		t.Fatal("expected an error for an out-of-range weight")
	}
}
