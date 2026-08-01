package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dpuig/ingress-shift/src/analyzer/pkg"
)

var (
	kubeConfigPath string
	contextNames   []string
	namespace      string
	allNamespaces  bool
	outputFormat   string
	verbose        bool
)

// NewRootCmd creates the root command for the analyzer
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		// Cobra derives the command name (used in "help for %s", "version
		// for %s", etc.) from the first whitespace-delimited word of Use, so
		// this must stay a single token — the `kubectl` invocation form is
		// shown in Long/Example instead, not baked into Use itself.
		Use:     "ingress-shift-analyzer",
		Short:   "Analyze Ingress resources for Gateway API migration",
		Version: version,
		Long: `Ingress Shift Analyzer is a kubectl plugin that analyzes Ingress resources
and their annotations to determine migration complexity to Gateway API.

It enumerates Ingress resources across all contexts and namespaces,
maps every annotation against a maintained knowledge base,
flags classes that break naive translation, and emits a scored report
with percentage translatable, list of manual interventions with effort estimate,
and recommendation for target controller.

Once installed via krew, invoke it as a kubectl plugin:
  kubectl ingress-shift-analyzer -A`,
		Example: `  kubectl ingress-shift-analyzer -A
  kubectl ingress-shift-analyzer -n my-namespace -o json
  kubectl ingress-shift-analyzer -A --context prod-us --context prod-eu`,
		RunE: runRootCmd,
		// A cluster/analysis failure is a runtime outcome, not a command-line
		// usage mistake — don't dump the full flag reference on every such
		// error. main() prints the error itself.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Add flags
	cmd.Flags().StringVar(&kubeConfigPath, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().StringSliceVar(&contextNames, "context", nil, "Kubeconfig context(s) to analyze (default: all contexts in the kubeconfig)")
	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Analyze a specific namespace (default: \"default\"; ignored if --all-namespaces is set)")
	cmd.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "Analyze all namespaces")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table, json, yaml)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	// Add completion for output formats
	cmd.ValidArgs = []string{"table", "json", "yaml"}
	return cmd
}

// runRootCmd executes the root command
func runRootCmd(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	configPath := resolveKubeConfigPath(kubeConfigPath)

	restConfigs, err := pkg.LoadContextConfigs(configPath, contextNames)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig contexts: %w", err)
	}

	if allNamespaces && namespace != "" {
		return fmt.Errorf("--namespace and --all-namespaces are mutually exclusive")
	}

	targetNamespace := namespace
	if !allNamespaces && targetNamespace == "" {
		targetNamespace = "default"
	}

	perContext := make(map[string]*pkg.AnalysisReport, len(restConfigs))
	failedContexts := make(map[string]error)
	for name, restConfig := range restConfigs {
		client, err := pkg.NewKubernetesClient(restConfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping context %q: failed to create kubernetes client: %v\n", name, err)
			failedContexts[name] = err
			continue
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "Analyzing context %q...\n", name)
		}

		report, err := pkg.AnalyzeIngressResources(ctx, client, targetNamespace, verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping context %q: failed to analyze ingress resources: %v\n", name, err)
			failedContexts[name] = err
			continue
		}
		perContext[name] = report
	}

	if len(perContext) == 0 {
		return fmt.Errorf("failed to analyze any context (%d attempted, all failed)", len(failedContexts))
	}

	report := pkg.MergeReports(perContext, failedContexts)

	// Output based on format
	switch outputFormat {
	case "json":
		err = report.ToJSON(os.Stdout)
	case "yaml":
		err = report.ToYAML(os.Stdout)
	default:
		// Default to table output
		report.PrintTable(os.Stdout)
	}

	if err != nil {
		return fmt.Errorf("failed to output report: %w", err)
	}

	return nil
}

// resolveKubeConfigPath returns the explicit path if set, otherwise the
// default ~/.kube/config location if it exists, otherwise an empty string
// (signaling in-cluster config to LoadContextConfigs).
func resolveKubeConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	defaultPath := filepath.Join(homeDir, ".kube", "config")
	if fileExists(defaultPath) {
		return defaultPath
	}

	return ""
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
