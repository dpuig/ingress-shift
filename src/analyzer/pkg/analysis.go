package pkg

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AnalysisReport represents the overall analysis report
type AnalysisReport struct {
	TotalIngresses          int               `json:"total_ingresses" yaml:"total_ingresses"`
	Translatable            int               `json:"translatable" yaml:"translatable"`
	NeedsManualIntervention int               `json:"needs_manual_intervention" yaml:"needs_manual_intervention"`
	NoEquivalent            int               `json:"no_equivalent" yaml:"no_equivalent"`
	ComplexityScore         float64           `json:"complexity_score" yaml:"complexity_score"`
	Recommendations         []string          `json:"recommendations" yaml:"recommendations"`
	AnnotationClasses       []AnnotationClass `json:"annotation_classes" yaml:"annotation_classes"`
}

// AnnotationClass represents a class of annotations
type AnnotationClass struct {
	Name              string       `json:"name" yaml:"name"`
	Description       string       `json:"description" yaml:"description"`
	Count             int          `json:"count" yaml:"count"`
	IsTranslatable    bool         `json:"is_translatable" yaml:"is_translatable"`
	RequiresExtension bool         `json:"requires_extension" yaml:"requires_extension"`
	NoEquivalent      bool         `json:"no_equivalent" yaml:"no_equivalent"`
	Annotations       []Annotation `json:"annotations" yaml:"annotations"`
}

// Annotation represents a single annotation
type Annotation struct {
	Name      string `json:"name" yaml:"name"`
	Value     string `json:"value" yaml:"value"`
	Namespace string `json:"namespace" yaml:"namespace"`
	Resource  string `json:"resource" yaml:"resource"`
}

// KubernetesClient interface for easier testing
type KubernetesClientInterface interface {
	listIngressesInNamespace(ctx context.Context, namespace string) ([]runtime.Object, error)
	listIngressesInAllNamespaces(ctx context.Context) ([]runtime.Object, error)
}

// AnalyzeIngressResources analyzes ingress resources and generates a report
func AnalyzeIngressResources(ctx context.Context, client KubernetesClientInterface, namespace string, verbose bool) (*AnalysisReport, error) {
	var allObjects []runtime.Object
	var err error

	if namespace == "" {
		// Analyze all namespaces
		allObjects, err = client.listIngressesInAllNamespaces(ctx)
	} else {
		// Analyze specific namespace
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

	// Process each ingress resource
	for _, obj := range allObjects {
		partialObj := obj.(*metav1.PartialObjectMetadata)

		// Analyze annotations for this ingress
		for annotationName, annotationValue := range partialObj.Annotations {
			if verbose {
				fmt.Printf("Processing annotation: %s = %s\n", annotationName, annotationValue)
			}

			// Determine annotation class properties
			class := getAnnotationClass(annotationName)
			report.addAnnotationClass(annotationName, class.Description, class.IsTranslatable, class.RequiresExtension, class.NoEquivalent)

			// Add sample annotation to the class
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

	// Calculate complexity score and add recommendations
	report.calculateComplexityScore()
	report.addRecommendations()

	return report, nil
}

// getAnnotationClass returns the class information for a given annotation name
func getAnnotationClass(annotationName string) AnnotationClass {
	// Map of known annotations to their properties
	annotationMap := map[string]AnnotationClass{
		"nginx.ingress.kubernetes.io/auth-secret": {
			Name:              "nginx.ingress.kubernetes.io/auth-secret",
			Description:       "Basic auth secret reference",
			IsTranslatable:    true,
			RequiresExtension: false,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/auth-realm": {
			Name:              "nginx.ingress.kubernetes.io/auth-realm",
			Description:       "Basic auth realm",
			IsTranslatable:    true,
			RequiresExtension: false,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/rewrite-target": {
			Name:              "nginx.ingress.kubernetes.io/rewrite-target",
			Description:       "Rewrite target for URL rewriting",
			IsTranslatable:    true,
			RequiresExtension: false,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/ssl-redirect": {
			Name:              "nginx.ingress.kubernetes.io/ssl-redirect",
			Description:       "SSL redirect configuration",
			IsTranslatable:    true,
			RequiresExtension: false,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/proxy-body-size": {
			Name:              "nginx.ingress.kubernetes.io/proxy-body-size",
			Description:       "Proxy body size limit",
			IsTranslatable:    true,
			RequiresExtension: false,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/limit-rps": {
			Name:              "nginx.ingress.kubernetes.io/limit-rps",
			Description:       "Rate limiting per second",
			IsTranslatable:    false,
			RequiresExtension: true,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/configuration-snippet": {
			Name:              "nginx.ingress.kubernetes.io/configuration-snippet",
			Description:       "Custom NGINX configuration snippet",
			IsTranslatable:    false,
			RequiresExtension: true,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/session-affinity": {
			Name:              "nginx.ingress.kubernetes.io/session-affinity",
			Description:       "Session affinity configuration",
			IsTranslatable:    false,
			RequiresExtension: true,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/ssl-passthrough": {
			Name:              "nginx.ingress.kubernetes.io/ssl-passthrough",
			Description:       "SSL passthrough configuration",
			IsTranslatable:    false,
			RequiresExtension: true,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/certificate": {
			Name:              "nginx.ingress.kubernetes.io/certificate",
			Description:       "Custom certificate configuration",
			IsTranslatable:    false,
			RequiresExtension: true,
			NoEquivalent:      false,
		},
		"nginx.ingress.kubernetes.io/upstream-hash-by": {
			Name:              "nginx.ingress.kubernetes.io/upstream-hash-by",
			Description:       "Upstream hash by configuration",
			IsTranslatable:    false,
			RequiresExtension: true,
			NoEquivalent:      false,
		},
	}

	// Check if annotation is in our known map
	if class, exists := annotationMap[annotationName]; exists {
		return class
	}

	// Default for unknown annotations: no mapping exists in the knowledge base,
	// so it falls into "no equivalent" rather than "requires extension" (which
	// implies a known, extension-based translation path).
	return AnnotationClass{
		Name:              annotationName,
		Description:       "Unknown annotation",
		IsTranslatable:    false,
		RequiresExtension: false,
		NoEquivalent:      true,
	}
}

// addAnnotationClass adds an annotation class to the report or increments its count
func (r *AnalysisReport) addAnnotationClass(name, description string, isTranslatable, requiresExtension, noEquivalent bool) {
	// Check if this annotation class already exists
	for i := range r.AnnotationClasses {
		if r.AnnotationClasses[i].Name == name {
			r.AnnotationClasses[i].Count++

			// Update the class properties based on what we know now
			if !r.AnnotationClasses[i].IsTranslatable && isTranslatable {
				r.AnnotationClasses[i].IsTranslatable = isTranslatable
			}
			if !r.AnnotationClasses[i].RequiresExtension && requiresExtension {
				r.AnnotationClasses[i].RequiresExtension = requiresExtension
			}
			if !r.AnnotationClasses[i].NoEquivalent && noEquivalent {
				r.AnnotationClasses[i].NoEquivalent = noEquivalent
			}

			return
		}
	}

	// Create new annotation class
	newClass := AnnotationClass{
		Name:              name,
		Description:       description,
		Count:             1,
		IsTranslatable:    isTranslatable,
		RequiresExtension: requiresExtension,
		NoEquivalent:      noEquivalent,
		Annotations:       make([]Annotation, 0),
	}

	r.AnnotationClasses = append(r.AnnotationClasses, newClass)

	// Update statistics
	if isTranslatable {
		r.Translatable++
	} else if requiresExtension {
		r.NeedsManualIntervention++
	} else if noEquivalent {
		r.NoEquivalent++
	}
}

// calculateComplexityScore calculates the complexity score based on annotation usage
func (r *AnalysisReport) calculateComplexityScore() {
	if r.TotalIngresses == 0 {
		r.ComplexityScore = 0
		return
	}

	// Complexity is inversely proportional to translatable annotations
	// Higher number of non-translatable annotations increases complexity score
	nonTranslatable := r.NeedsManualIntervention + r.NoEquivalent
	r.ComplexityScore = float64(nonTranslatable) / float64(r.TotalIngresses) * 100
}

// addRecommendations adds recommendations based on the analysis results
func (r *AnalysisReport) addRecommendations() {
	r.Recommendations = make([]string, 0)

	if r.TotalIngresses == 0 {
		r.Recommendations = append(r.Recommendations, "No Ingress resources found in the cluster")
		return
	}

	// Add complexity-based recommendations
	if r.ComplexityScore >= 75 {
		r.Recommendations = append(r.Recommendations, "High migration complexity detected. Consider a phased approach with extensive manual intervention.")
		r.Recommendations = append(r.Recommendations, "Allocate significant time for custom controller development or extension work.")
	} else if r.ComplexityScore >= 50 {
		r.Recommendations = append(r.Recommendations, "Moderate migration complexity. Plan for some manual interventions and controller extensions.")
		r.Recommendations = append(r.Recommendations, "Consider using a controller with broader annotation support.")
	} else if r.ComplexityScore >= 25 {
		r.Recommendations = append(r.Recommendations, "Low to moderate migration complexity. Standard migration approach should work well.")
		r.Recommendations = append(r.Recommendations, "Ensure you have the right Gateway API controller installed.")
	} else {
		r.Recommendations = append(r.Recommendations, "Low migration complexity. Consider a straightforward migration approach.")
		r.Recommendations = append(r.Recommendations, "Standard Gateway API controller should be sufficient for this workload.")
	}

	// Add specific recommendations based on annotation usage
	if r.NeedsManualIntervention > 0 {
		r.Recommendations = append(r.Recommendations, fmt.Sprintf("Found %d annotations requiring manual intervention or controller extensions", r.NeedsManualIntervention))
	}

	if r.NoEquivalent > 0 {
		r.Recommendations = append(r.Recommendations, fmt.Sprintf("Found %d annotations with no Gateway API equivalent", r.NoEquivalent))
		r.Recommendations = append(r.Recommendations, "These will require custom controller logic or workarounds")
	}

	// Suggest target controller based on complexity
	if r.ComplexityScore >= 50 {
		r.Recommendations = append(r.Recommendations, "Consider using a more feature-rich Gateway API controller with extension capabilities")
	} else {
		r.Recommendations = append(r.Recommendations, "A standard Gateway API controller should be sufficient for this workload")
	}
}
