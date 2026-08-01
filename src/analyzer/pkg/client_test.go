package pkg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	})

	// Create a mock Kubernetes client with test data
	mockClient := &MockKubernetesClient{
		ingressList: []runtime.Object{
			&metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ingress-1",
					Namespace: "default",
					Annotations: map[string]string{
						"nginx.ingress.kubernetes.io/rewrite-target": "/",
						"nginx.ingress.kubernetes.io/ssl-redirect":   "true",
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
	assert.Equal(t, 2, report.Translatable)            // rewrite-target and ssl-redirect are translatable
	assert.Equal(t, 2, report.NeedsManualIntervention) // configuration-snippet and limit-rps require intervention
	assert.Equal(t, 0, report.NoEquivalent)

	// Verify that we have the expected annotation classes in the report
	annotationClassNames := make(map[string]bool)
	for _, class := range report.AnnotationClasses {
		annotationClassNames[class.Name] = true
	}

	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/rewrite-target"])
	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/ssl-redirect"])
	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/configuration-snippet"])
	assert.True(t, annotationClassNames["nginx.ingress.kubernetes.io/limit-rps"])

	// Verify complexity score
	assert.Greater(t, report.ComplexityScore, float64(0))
}

func TestGetAnnotationClass(t *testing.T) {
	// Test known annotations
	class := getAnnotationClass("nginx.ingress.kubernetes.io/rewrite-target")
	assert.Equal(t, "nginx.ingress.kubernetes.io/rewrite-target", class.Name)
	assert.Equal(t, "Rewrite target for URL rewriting", class.Description)
	assert.True(t, class.IsTranslatable)
	assert.False(t, class.RequiresExtension)
	assert.False(t, class.NoEquivalent)

	class = getAnnotationClass("nginx.ingress.kubernetes.io/limit-rps")
	assert.Equal(t, "nginx.ingress.kubernetes.io/limit-rps", class.Name)
	assert.Equal(t, "Rate limiting per second", class.Description)
	assert.False(t, class.IsTranslatable)
	assert.True(t, class.RequiresExtension)
	assert.False(t, class.NoEquivalent)

	// Test unknown annotation
	class = getAnnotationClass("unknown.annotation.test")
	assert.Equal(t, "unknown.annotation.test", class.Name)
	assert.Equal(t, "Unknown annotation", class.Description)
	assert.False(t, class.IsTranslatable)
	assert.False(t, class.RequiresExtension)
	assert.True(t, class.NoEquivalent)
}

func TestAddAnnotationClass(t *testing.T) {
	report := &AnalysisReport{
		TotalIngresses:          0,
		Translatable:            0,
		NeedsManualIntervention: 0,
		NoEquivalent:            0,
		AnnotationClasses:       make([]AnnotationClass, 0),
	}

	// Add a new annotation class
	report.addAnnotationClass("test.annotation", "Test annotation description", true, false, false)
	assert.Equal(t, 1, len(report.AnnotationClasses))
	assert.Equal(t, 1, report.Translatable)

	// Add the same annotation class again (should increment count)
	report.addAnnotationClass("test.annotation", "Test annotation description", true, false, false)
	assert.Equal(t, 1, len(report.AnnotationClasses))
	assert.Equal(t, 2, report.AnnotationClasses[0].Count)
}

func TestCalculateComplexityScore(t *testing.T) {
	report := &AnalysisReport{
		TotalIngresses:          10,
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

	// Check that recommendations are added based on complexity
	for _, rec := range report.Recommendations {
		if rec == "Moderate migration complexity. Plan for some manual interventions and controller extensions." {
			assert.True(t, true) // Found expected recommendation
			return
		}
	}
	assert.Fail(t, "Expected recommendation not found")
}
