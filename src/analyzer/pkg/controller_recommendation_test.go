package pkg

import "testing"

func TestRecommendController(t *testing.T) {
	tests := []struct {
		name       string
		categories map[string]int
		total      int
		complexity float64
		want       string
	}{
		{
			name:       "no ingresses yields no recommendation",
			categories: map[string]int{},
			total:      0,
			want:       "",
		},
		{
			name:       "snippet heavy favors envoy gateway",
			categories: map[string]int{"snippet": 5},
			total:      5,
			complexity: 80,
			want:       "Envoy Gateway",
		},
		{
			name:       "mtls heavy favors istio",
			categories: map[string]int{"mtls": 3},
			total:      3,
			complexity: 80,
			want:       "Istio",
		},
		{
			name:       "auth and rate-limit heavy favors kong",
			categories: map[string]int{"auth": 4, "rate-limit": 2},
			total:      6,
			complexity: 60,
			want:       "Kong Gateway",
		},
		{
			name:       "low complexity with no extension categories favors gke gateway",
			categories: map[string]int{},
			total:      1,
			complexity: 10,
			want:       "GKE Gateway",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &AnalysisReport{
				TotalIngresses:  tt.total,
				ComplexityScore: tt.complexity,
			}
			for category, count := range tt.categories {
				report.AnnotationClasses = append(report.AnnotationClasses, AnnotationClass{
					Name:              category + "-annotation",
					Category:          category,
					Count:             count,
					RequiresExtension: true,
				})
			}

			report.recommendController()

			if tt.want == "" {
				if report.ControllerRecommendation != nil {
					t.Fatalf("expected no recommendation, got %+v", report.ControllerRecommendation)
				}
				return
			}

			if report.ControllerRecommendation == nil {
				t.Fatalf("expected a recommendation, got nil")
			}
			if report.ControllerRecommendation.Controller != tt.want {
				t.Errorf("got controller %q, want %q", report.ControllerRecommendation.Controller, tt.want)
			}
			if len(report.ControllerRecommendation.Reasoning) == 0 {
				t.Error("expected non-empty reasoning")
			}
		})
	}
}
