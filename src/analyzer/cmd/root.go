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
	allNamespaces  bool
	outputFormat   string
	verbose        bool
)

// NewRootCmd creates the root command for the analyzer
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ingress-shift-analyzer",
		Short:   "Analyze Ingress resources for Gateway API migration",
		Version: version,
		Long: `Ingress Shift Analyzer is a kubectl plugin that analyzes Ingress resources
and their annotations to determine migration complexity to Gateway API.

It enumerates Ingress resources across all contexts and namespaces,
maps every annotation against a maintained knowledge base,
flags classes that break naive translation, and emits a scored report
with percentage translatable, list of manual interventions with effort estimate,
and recommendation for target controller.`,
		RunE: runRootCmd,
	}

	// Add flags
	cmd.Flags().StringVar(&kubeConfigPath, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().StringSliceVar(&contextNames, "context", nil, "Kubeconfig context(s) to analyze (default: all contexts in the kubeconfig)")
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

	var namespace string
	if !allNamespaces {
		namespace = "default"
	}

	perContext := make(map[string]*pkg.AnalysisReport, len(restConfigs))
	for name, restConfig := range restConfigs {
		client, err := pkg.NewKubernetesClient(restConfig)
		if err != nil {
			return fmt.Errorf("failed to create kubernetes client for context %q: %w", name, err)
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "Analyzing context %q...\n", name)
		}

		report, err := pkg.AnalyzeIngressResources(ctx, client, namespace, verbose)
		if err != nil {
			return fmt.Errorf("failed to analyze ingress resources in context %q: %w", name, err)
		}
		perContext[name] = report
	}

	report := pkg.MergeReports(perContext)

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
