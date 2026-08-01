package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"ingress-shift-analyzer/src/analyzer/pkg"
)

var (
	kubeConfigPath string
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

	// Create Kubernetes client
	kubeClient, err := createKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Analyze ingress resources
	var namespace string
	if allNamespaces {
		namespace = ""
	} else {
		namespace = "default"
	}

	report, err := pkg.AnalyzeIngressResources(ctx, kubeClient, namespace, verbose)
	if err != nil {
		return fmt.Errorf("failed to analyze ingress resources: %w", err)
	}

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

// createKubernetesClient creates a Kubernetes client from the kubeconfig
func createKubernetesClient() (*pkg.KubernetesClient, error) {
	// Get the kubeconfig path
	configPath := kubeConfigPath
	if configPath == "" {
		// Try to get default kubeconfig path
		if homeDir, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(homeDir, ".kube", "config")
		}
	}

	var config *rest.Config
	var err error

	// Try to load from kubeconfig file first
	if configPath != "" && fileExists(configPath) {
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: configPath},
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
	} else {
		// Try to load in-cluster config
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes config: %w", err)
	}

	return pkg.NewKubernetesClient(config)
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
