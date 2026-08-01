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
	fmt.Fprintf(w, "Ingress Shift Analysis Report\n")
	fmt.Fprintf(w, "============================\n\n")

	// Summary statistics
	fmt.Fprintf(w, "Summary:\n")
	fmt.Fprintf(w, "  Total Ingress Resources: %d\n", r.TotalIngresses)
	fmt.Fprintf(w, "  Translatable Annotations: %d\n", r.Translatable)
	fmt.Fprintf(w, "  Requires Manual Intervention: %d\n", r.NeedsManualIntervention)
	fmt.Fprintf(w, "  No Gateway API Equivalent: %d\n", r.NoEquivalent)
	fmt.Fprintf(w, "  Complexity Score: %.1f%%\n\n", r.ComplexityScore)

	// Recommendations
	fmt.Fprintf(w, "Recommendations:\n")
	for _, rec := range r.Recommendations {
		fmt.Fprintf(w, "  - %s\n", rec)
	}
	fmt.Fprintf(w, "\n")

	// Annotation Classes
	if len(r.AnnotationClasses) > 0 {
		fmt.Fprintf(w, "Annotation Classes:\n")

		// Sort annotation classes by count (descending)
		sort.Slice(r.AnnotationClasses, func(i, j int) bool {
			return r.AnnotationClasses[i].Count > r.AnnotationClasses[j].Count
		})

		// Create a tab writer for formatted output
		writer := tabwriter.NewWriter(w, 0, 0, 3, ' ', tabwriter.AlignRight)
		fmt.Fprintf(writer, "ANNOTATION\tCOUNT\tTRANSLATABLE\tREQUIRES EXTENSION\tNO EQUIVALENT\n")
		fmt.Fprintf(writer, "----------\t-----\t------------\t------------------\t-------------\n")

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

			fmt.Fprintf(writer, "%s\t%d\t%s\t%s\t%s\n",
				class.Name,
				class.Count,
				translatable,
				requiresExtension,
				noEquivalent)
		}
		writer.Flush()
		fmt.Fprintf(w, "\n")
	} else {
		fmt.Fprintf(w, "No annotation classes found.\n\n")
	}

	// Detailed annotation information
	if len(r.AnnotationClasses) > 0 {
		fmt.Fprintf(w, "Detailed Annotation Usage:\n")

		// Sort annotation classes by count (descending)
		sort.Slice(r.AnnotationClasses, func(i, j int) bool {
			return r.AnnotationClasses[i].Count > r.AnnotationClasses[j].Count
		})

		for _, class := range r.AnnotationClasses {
			if len(class.Annotations) > 0 {
				fmt.Fprintf(w, "\n  %s (%d occurrences):\n", class.Name, class.Count)
				fmt.Fprintf(w, "    Description: %s\n", class.Description)

				if class.IsTranslatable {
					fmt.Fprintf(w, "    Translatable: Yes\n")
				} else if class.RequiresExtension {
					fmt.Fprintf(w, "    Requires Extension: Yes\n")
				} else if class.NoEquivalent {
					fmt.Fprintf(w, "    No Gateway API Equivalent: Yes\n")
				}

				fmt.Fprintf(w, "    Sample Usage:\n")
				for _, ann := range class.Annotations {
					fmt.Fprintf(w, "      %s/%s: %s = %s\n",
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
	fmt.Fprintf(w, "Ingress Shift Detailed Analysis Report\n")
	fmt.Fprintf(w, "======================================\n\n")

	// Overall summary
	fmt.Fprintf(w, "Overall Summary:\n")
	fmt.Fprintf(w, "  Total Ingress Resources: %d\n", r.TotalIngresses)
	fmt.Fprintf(w, "  Complexity Score: %.1f%%\n", r.ComplexityScore)
	fmt.Fprintf(w, "\n")

	// Recommendation section
	fmt.Fprintf(w, "Recommendations:\n")
	for _, rec := range r.Recommendations {
		fmt.Fprintf(w, "  - %s\n", rec)
	}
	fmt.Fprintf(w, "\n")

	// Annotation class summary
	fmt.Fprintf(w, "Annotation Classes Summary:\n")
	fmt.Fprintf(w, "  Translatable: %d\n", r.Translatable)
	fmt.Fprintf(w, "  Requires Manual Intervention: %d\n", r.NeedsManualIntervention)
	fmt.Fprintf(w, "  No Gateway API Equivalent: %d\n", r.NoEquivalent)
	fmt.Fprintf(w, "\n")

	// Detailed breakdown of annotation classes
	if len(r.AnnotationClasses) > 0 {
		fmt.Fprintf(w, "Detailed Annotation Classes:\n")

		// Sort by count descending
		sort.Slice(r.AnnotationClasses, func(i, j int) bool {
			return r.AnnotationClasses[i].Count > r.AnnotationClasses[j].Count
		})

		for i, class := range r.AnnotationClasses {
			if i > 0 {
				fmt.Fprintf(w, "\n")
			}

			fmt.Fprintf(w, "  %s\n", class.Name)
			fmt.Fprintf(w, "    Description: %s\n", class.Description)
			fmt.Fprintf(w, "    Count: %d\n", class.Count)

			if class.IsTranslatable {
				fmt.Fprintf(w, "    Translatable: Yes\n")
			} else if class.RequiresExtension {
				fmt.Fprintf(w, "    Requires Extension: Yes\n")
			} else if class.NoEquivalent {
				fmt.Fprintf(w, "    No Gateway API Equivalent: Yes\n")
			}

			if len(class.Annotations) > 0 {
				fmt.Fprintf(w, "    Sample Usage:\n")
				for _, ann := range class.Annotations {
					fmt.Fprintf(w, "      %s/%s: %s = %s\n",
						ann.Namespace,
						ann.Resource,
						ann.Name,
						ann.Value)
				}
			}
		}
	} else {
		fmt.Fprintf(w, "No annotation classes found.\n")
	}
}

// PrintSimpleReport prints a simple, concise report
func (r *AnalysisReport) PrintSimpleReport(w io.Writer) {
	fmt.Fprintf(w, "Ingress Shift Analysis Report\n")
	fmt.Fprintf(w, "=============================\n")
	fmt.Fprintf(w, "Total Ingress Resources: %d\n", r.TotalIngresses)
	fmt.Fprintf(w, "Complexity Score: %.1f%%\n", r.ComplexityScore)
	fmt.Fprintf(w, "\n")

	if len(r.Recommendations) > 0 {
		fmt.Fprintf(w, "Recommendations:\n")
		for _, rec := range r.Recommendations {
			fmt.Fprintf(w, "  - %s\n", rec)
		}
		fmt.Fprintf(w, "\n")
	}

	if r.TotalIngresses > 0 {
		fmt.Fprintf(w, "Annotation Usage Summary:\n")
		fmt.Fprintf(w, "  Translatable: %d (%.1f%%)\n",
			r.Translatable,
			float64(r.Translatable)/float64(r.TotalIngresses)*100)
		fmt.Fprintf(w, "  Requires Manual Intervention: %d (%.1f%%)\n",
			r.NeedsManualIntervention,
			float64(r.NeedsManualIntervention)/float64(r.TotalIngresses)*100)
		fmt.Fprintf(w, "  No Gateway API Equivalent: %d (%.1f%%)\n",
			r.NoEquivalent,
			float64(r.NoEquivalent)/float64(r.TotalIngresses)*100)
	}
}
