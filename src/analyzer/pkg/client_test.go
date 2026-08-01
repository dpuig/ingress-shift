package pkg

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/dpuig/ingress-shift/src/analyzer/pkg/knowledgebase"
)

// MockKubernetesClient is a mock implementation of KubernetesClientInterface for testing
type MockKubernetesClient struct {
	ingressList []runtime.Object
}

func (m *MockKubernetesClient) listIngressesInNamespace(ctx context.Context, namespace string) ([]runtime.Object, error) {
	return m.ingressList, nil
}

func (m *MockKubernetesClient) listIngressesInAllNamespaces(ctx context.Context) ([]runtime.Object, error) {
	return m.ingressList, nil
}

func TestAnalyzeIngressResources(t *testing.T) {
	t.Run("no ingresses", func(t *testing.T) {
		mockClient := &MockKubernetesClient{ingressList: []runtime.Object{}}

		report, err := AnalyzeIngressResources(context.Background(), mockClient, "", false)
		assert.NoError(t, err)
		assert.Equal(t, 0, report.TotalIngresses)
		assert.Equal(t, 0, report.Translatable)
		assert.Equal(t, 0, report.NeedsManualIntervention)
		assert.Equal(t, 0, report.NoEquivalent)
		assert.Contains(t, report.Recommendations, "No Ingress resources found in the cluster")
	})

	t.Run("unknown annotation counts as no equivalent", func(t *testing.T) {
		mockClient := &MockKubernetesClient{
			ingressList: []runtime.Object{
				&metav1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "custom-ingress",
						Namespace: "default",
						Annotations: map[string]string{
							"custom.annotation.example.com/test": "value",
						},
					},
				},
			},
		}

		report, err := AnalyzeIngressResources(context.Background(), mockClient, "default", false)
		assert.NoError(t, err)
		assert.Equal(t, 1, report.TotalIngresses)
		assert.Equal(t, 0, report.Translatable)
		assert.Equal(t, 0, report.NeedsManualIntervention)
		assert.Equal(t, 1, report.NoEquivalent)
		assert.Len(t, report.ManualInterventions, 1)
		assert.Equal(t, "high", report.ManualInterventions[0].Effort)
	})

	// Create a mock Kubernetes client with test data
	mockClient := &MockKubernetesClient{
		ingressList: []runtime.Object{
			&metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-1",
					Namespace: "default",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/ssl-redirect": "true",
						"nginx.ingress.kubernetes.io/app-root":     "/app",
					},
				},
			},
			&metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-2",
					Namespace: "production",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/configuration-snippet": "more_set_headers \"X-Forwarded-Proto: https\";",
						"nginx.ingress.kubernetes.io/limit-rps":             "10",
					},
				},
			},
		},
	}

	ctx := context.Background()
	report, err := AnalyzeIngressResources(ctx, mockClient, "default", false)

	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 2, report.TotalIngresses)
	assert.Equal(t, 2, report.Translatable)            // ssl-redirect and app-root are direct
	assert.Equal(t, 1, report.NeedsManualIntervention) // limit-rps is "extension"
	assert.Equal(t, 1, report.NoEquivalent)            // configuration-snippet is "none"
	assert.Equal(t, 50.0, report.PercentTranslatable)

	// Verify that we have the expected annotation classes in the report
	annotationClassNames := make(map[string]bool)
	for _, class := range report.AnnotationClasses {
		annotationClassNames[class.Name] = true
	}

	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/ssl-redirect"])
	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/app-root"])
	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/configuration-snippet"])
	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/limit-rps"])

	// Verify complexity score
	assert.Greater(t, report.ComplexityScore, float64(0))
	assert.NotNil(t, report.ControllerRecommendation)
}

func TestKnowledgeBaseLookup(t *testing.T) {
	entry, ok := knowledgebase.Lookup("nginx.ingress.kubernetes.io/app-root")
	assert.True(t, ok)
	assert.Equal(t, "nginx.ingress.kubernetes.io/app-root", entry.Name)
	assert.True(t, entry.IsTranslatable())

	entry, ok = knowledgebase.Lookup("nginx.ingress.kubernetes.io/limit-rps")
	assert.True(t, ok)
	assert.True(t, entry.RequiresExtension())
	assert.True(t, entry.BreaksNaiveTranslation)

	entry = knowledgebase.Unknown("unknown.annotation.test")
	assert.Equal(t, "unknown.annotation.test", entry.Name)
	assert.True(t, entry.NoEquivalent())
}

func TestMergeReports(t *testing.T) {
	ctxAClient := &MockKubernetesClient{
		ingressList: []runtime.Object{
			&metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/ssl-redirect": "true",
					},
				},
			},
		},
	}
	ctxBClient := &MockKubernetesClient{
		ingressList: []runtime.Object{
			&metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "b-ingress",
					Namespace: "default",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/ssl-redirect": "true",
					},
				},
			},
		},
	}

	ctx := context.Background()
	reportA, err := AnalyzeIngressResources(ctx, ctxAClient, "", false)
	assert.NoError(t, err)
	reportB, err := AnalyzeIngressResources(ctx, ctxBClient, "", false)
	assert.NoError(t, err)

	merged := MergeReports(map[string]*AnalysisReport{
		"cluster-a": reportA,
		"cluster-b": reportB,
	})

	assert.Equal(t, 2, merged.TotalIngresses)
	assert.Len(t, merged.Contexts, 2)
	assert.Len(t, merged.AnnotationClasses, 1)
	assert.Equal(t, 2, merged.AnnotationClasses[0].Count)
}

func TestCalculateComplexityScore(t *testing.T) {
	report := &AnalysisReport{
		Translatable:            5,
		NeedsManualIntervention: 3,
		NoEquivalent:            2,
	}

	report.calculateComplexityScore()
	assert.Equal(t, float64(50.0), report.ComplexityScore) // (3+2)/10 * 100 = 50
}

func TestAddRecommendations(t *testing.T) {
	report := &AnalysisReport{
		TotalIngresses:          10,
		Translatable:            5,
		NeedsManualIntervention: 3,
		NoEquivalent:            2,
		ComplexityScore:         50.0,
	}

	report.addRecommendations()
	assert.NotNil(t, report.Recommendations)
	assert.Greater(t, len(report.Recommendations), 0)

	found := slices.Contains(report.Recommendations, "Moderate migration complexity. Plan for some manual interventions and controller extensions.")
	assert.True(t, found, "expected recommendation not found")
}
