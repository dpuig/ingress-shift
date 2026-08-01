package pkg

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTableIncludesNewFields(t *testing.T) {
	report := &AnalysisReport{
		TotalIngresses:          2,
		Translatable:            1,
		NeedsManualIntervention: 1,
		PercentTranslatable:     50,
		ComplexityScore:         50,
		Recommendations:         []string{"Do the thing"},
		ControllerRecommendation: &ControllerRecommendation{
			Controller: "Envoy Gateway",
			Reasoning:  []string{"because reasons"},
		},
		ManualInterventions: []ManualIntervention{
			{Annotation: "nginx.ingress.kubernetes.io/limit-rps", Reason: "needs a rate limit policy", Effort: "medium", Count: 3},
		},
		Contexts: []ContextResult{
			{Context: "prod", TotalIngresses: 2},
		},
	}

	var buf bytes.Buffer
	report.PrintTable(&buf)
	out := buf.String()

	for _, want := range []string{
		"Contexts analyzed:",
		"prod: 2 ingress resources",
		"Percentage Directly Translatable: 50.0%",
		"Recommended Target Controller: Envoy Gateway",
		"because reasons",
		"Manual Interventions (effort estimate):",
		"nginx.ingress.kubernetes.io/limit-rps",
		"medium",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestToJSONAndToYAMLRoundTrip(t *testing.T) {
	report := &AnalysisReport{TotalIngresses: 1, ComplexityScore: 10}

	var jsonBuf bytes.Buffer
	if err := report.ToJSON(&jsonBuf); err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	if !strings.Contains(jsonBuf.String(), `"total_ingresses": 1`) {
		t.Errorf("expected JSON to contain total_ingresses, got: %s", jsonBuf.String())
	}

	var yamlBuf bytes.Buffer
	if err := report.ToYAML(&yamlBuf); err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}
	if !strings.Contains(yamlBuf.String(), "total_ingresses: 1") {
		t.Errorf("expected YAML to contain total_ingresses, got: %s", yamlBuf.String())
	}
}
