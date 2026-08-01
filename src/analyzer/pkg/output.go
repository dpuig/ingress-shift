package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"gopkg.in/yaml.v2"
)

// ToJSON outputs the report in JSON format
func (r *AnalysisReport) ToJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// ToYAML outputs the report in YAML format
func (r *AnalysisReport) ToYAML(w io.Writer) error {
	output, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("failed to marshal report to YAML: %w", err)
	}
	_, err = w.Write(output)
	return err
}

// PrintTable prints the report in a formatted table
func (r *AnalysisReport) PrintTable(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Ingress Shift Analysis Report\n")
	_, _ = fmt.Fprintf(w, "============================\n\n")

	if len(r.Contexts) > 0 {
		_, _ = fmt.Fprintf(w, "Contexts analyzed:\n")
		for _, c := range r.Contexts {
			if c.Error != "" {
				_, _ = fmt.Fprintf(w, "  %s: ERROR: %s\n", c.Context, c.Error)
			} else {
				_, _ = fmt.Fprintf(w, "  %s: %d ingress resources\n", c.Context, c.TotalIngresses)
			}
		}
		_, _ = fmt.Fprintf(w, "\n")
	}

	// Summary statistics
	_, _ = fmt.Fprintf(w, "Summary:\n")
	_, _ = fmt.Fprintf(w, "  Total Ingress Resources: %d\n", r.TotalIngresses)
	_, _ = fmt.Fprintf(w, "  Translatable Annotation Classes: %d\n", r.Translatable)
	_, _ = fmt.Fprintf(w, "  Requires Manual Intervention: %d\n", r.NeedsManualIntervention)
	_, _ = fmt.Fprintf(w, "  No Gateway API Equivalent: %d\n", r.NoEquivalent)
	_, _ = fmt.Fprintf(w, "  Percentage Directly Translatable: %.1f%%\n", r.PercentTranslatable)
	_, _ = fmt.Fprintf(w, "  Complexity Score: %.1f%%\n\n", r.ComplexityScore)

	if r.ControllerRecommendation != nil {
		_, _ = fmt.Fprintf(w, "Recommended Target Controller: %s\n", r.ControllerRecommendation.Controller)
		for _, reason := range r.ControllerRecommendation.Reasoning {
			_, _ = fmt.Fprintf(w, "  - %s\n", reason)
		}
		_, _ = fmt.Fprintf(w, "\n")
	}

	// Recommendations
	_, _ = fmt.Fprintf(w, "Recommendations:\n")
	for _, rec := range r.Recommendations {
		_, _ = fmt.Fprintf(w, "  - %s\n", rec)
	}
	_, _ = fmt.Fprintf(w, "\n")

	if len(r.ManualInterventions) > 0 {
		_, _ = fmt.Fprintf(w, "Manual Interventions (effort estimate):\n")
		writer := tabwriter.NewWriter(w, 0, 0, 3, ' ', tabwriter.AlignRight)
		_, _ = fmt.Fprintf(writer, "ANNOTATION\tCOUNT\tEFFORT\tREASON\n")
		for _, mi := range r.ManualInterventions {
			_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\t%s\n", mi.Annotation, mi.Count, mi.Effort, mi.Reason)
		}
		_ = writer.Flush()
		_, _ = fmt.Fprintf(w, "\n")
	}

	// Annotation Classes
	if len(r.AnnotationClasses) > 0 {
		_, _ = fmt.Fprintf(w, "Annotation Classes:\n")

		// Sort annotation classes by count (descending)
		sort.Slice(r.AnnotationClasses, func(i, j int) bool {
			return r.AnnotationClasses[i].Count > r.AnnotationClasses[j].Count
		})

		// Create a tab writer for formatted output
		writer := tabwriter.NewWriter(w, 0, 0, 3, ' ', tabwriter.AlignRight)
		_, _ = fmt.Fprintf(writer, "ANNOTATION\tCOUNT\tTRANSLATABLE\tREQUIRES EXTENSION\tNO EQUIVALENT\n")
		_, _ = fmt.Fprintf(writer, "----------\t-----\t------------\t------------------\t-------------\n")

		for _, class := range r.AnnotationClasses {
			translatable := "No"
			if class.IsTranslatable {
				translatable = "Yes"
			}

			requiresExtension := "No"
			if class.RequiresExtension {
				requiresExtension = "Yes"
			}

			noEquivalent := "No"
			if class.NoEquivalent {
				noEquivalent = "Yes"
			}

			_, _ = fmt.Fprintf(writer, "%s\t%d\t%s\t%s\t%s\n",
				class.Name,
				class.Count,
				translatable,
				requiresExtension,
				noEquivalent)
		}
		_ = writer.Flush()
		_, _ = fmt.Fprintf(w, "\n")
	} else {
		_, _ = fmt.Fprintf(w, "No annotation classes found.\n\n")
	}

	// Detailed annotation information
	if len(r.AnnotationClasses) > 0 {
		_, _ = fmt.Fprintf(w, "Detailed Annotation Usage:\n")

		// Sort annotation classes by count (descending)
		sort.Slice(r.AnnotationClasses, func(i, j int) bool {
			return r.AnnotationClasses[i].Count > r.AnnotationClasses[j].Count
		})

		for _, class := range r.AnnotationClasses {
			if len(class.Annotations) > 0 {
				_, _ = fmt.Fprintf(w, "\n  %s (%d occurrences):\n", class.Name, class.Count)
				_, _ = fmt.Fprintf(w, "    Description: %s\n", class.Description)

				if class.IsTranslatable {
					_, _ = fmt.Fprintf(w, "    Translatable: Yes\n")
				} else if class.RequiresExtension {
					_, _ = fmt.Fprintf(w, "    Requires Extension: Yes\n")
				} else if class.NoEquivalent {
					_, _ = fmt.Fprintf(w, "    No Gateway API Equivalent: Yes\n")
				}

				_, _ = fmt.Fprintf(w, "    Sample Usage:\n")
				for _, ann := range class.Annotations {
					_, _ = fmt.Fprintf(w, "      %s/%s: %s = %s\n",
						ann.Namespace,
						ann.Resource,
						ann.Name,
						ann.Value)
				}
			}
		}
	}
}

// PrintDetailedReport prints a more detailed report
func (r *AnalysisReport) PrintDetailedReport(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Ingress Shift Detailed Analysis Report\n")
	_, _ = fmt.Fprintf(w, "======================================\n\n")

	// Overall summary
	_, _ = fmt.Fprintf(w, "Overall Summary:\n")
	_, _ = fmt.Fprintf(w, "  Total Ingress Resources: %d\n", r.TotalIngresses)
	_, _ = fmt.Fprintf(w, "  Complexity Score: %.1f%%\n", r.ComplexityScore)
	_, _ = fmt.Fprintf(w, "\n")

	// Recommendation section
	_, _ = fmt.Fprintf(w, "Recommendations:\n")
	for _, rec := range r.Recommendations {
		_, _ = fmt.Fprintf(w, "  - %s\n", rec)
	}
	_, _ = fmt.Fprintf(w, "\n")

	// Annotation class summary
	_, _ = fmt.Fprintf(w, "Annotation Classes Summary:\n")
	_, _ = fmt.Fprintf(w, "  Translatable: %d\n", r.Translatable)
	_, _ = fmt.Fprintf(w, "  Requires Manual Intervention: %d\n", r.NeedsManualIntervention)
	_, _ = fmt.Fprintf(w, "  No Gateway API Equivalent: %d\n", r.NoEquivalent)
	_, _ = fmt.Fprintf(w, "\n")

	// Detailed breakdown of annotation classes
	if len(r.AnnotationClasses) > 0 {
		_, _ = fmt.Fprintf(w, "Detailed Annotation Classes:\n")

		// Sort by count descending
		sort.Slice(r.AnnotationClasses, func(i, j int) bool {
			return r.AnnotationClasses[i].Count > r.AnnotationClasses[j].Count
		})

		for i, class := range r.AnnotationClasses {
			if i > 0 {
				_, _ = fmt.Fprintf(w, "\n")
			}

			_, _ = fmt.Fprintf(w, "  %s\n", class.Name)
			_, _ = fmt.Fprintf(w, "    Description: %s\n", class.Description)
			_, _ = fmt.Fprintf(w, "    Count: %d\n", class.Count)

			if class.IsTranslatable {
				_, _ = fmt.Fprintf(w, "    Translatable: Yes\n")
			} else if class.RequiresExtension {
				_, _ = fmt.Fprintf(w, "    Requires Extension: Yes\n")
			} else if class.NoEquivalent {
				_, _ = fmt.Fprintf(w, "    No Gateway API Equivalent: Yes\n")
			}

			if len(class.Annotations) > 0 {
				_, _ = fmt.Fprintf(w, "    Sample Usage:\n")
				for _, ann := range class.Annotations {
					_, _ = fmt.Fprintf(w, "      %s/%s: %s = %s\n",
						ann.Namespace,
						ann.Resource,
						ann.Name,
						ann.Value)
				}
			}
		}
	} else {
		_, _ = fmt.Fprintf(w, "No annotation classes found.\n")
	}
}

// PrintSimpleReport prints a simple, concise report
func (r *AnalysisReport) PrintSimpleReport(w io.Writer) {
	_, _ = fmt.Fprintf(w, "Ingress Shift Analysis Report\n")
	_, _ = fmt.Fprintf(w, "=============================\n")
	_, _ = fmt.Fprintf(w, "Total Ingress Resources: %d\n", r.TotalIngresses)
	_, _ = fmt.Fprintf(w, "Complexity Score: %.1f%%\n", r.ComplexityScore)
	_, _ = fmt.Fprintf(w, "\n")

	if len(r.Recommendations) > 0 {
		_, _ = fmt.Fprintf(w, "Recommendations:\n")
		for _, rec := range r.Recommendations {
			_, _ = fmt.Fprintf(w, "  - %s\n", rec)
		}
		_, _ = fmt.Fprintf(w, "\n")
	}

	if r.TotalIngresses > 0 {
		_, _ = fmt.Fprintf(w, "Annotation Usage Summary:\n")
		_, _ = fmt.Fprintf(w, "  Translatable: %d (%.1f%%)\n",
			r.Translatable,
			float64(r.Translatable)/float64(r.TotalIngresses)*100)
		_, _ = fmt.Fprintf(w, "  Requires Manual Intervention: %d (%.1f%%)\n",
			r.NeedsManualIntervention,
			float64(r.NeedsManualIntervention)/float64(r.TotalIngresses)*100)
		_, _ = fmt.Fprintf(w, "  No Gateway API Equivalent: %d (%.1f%%)\n",
			r.NoEquivalent,
			float64(r.NoEquivalent)/float64(r.TotalIngresses)*100)
	}
}
