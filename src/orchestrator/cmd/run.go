package cmd

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/certificate"
	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/rollout"
	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/slo"
	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/traffic"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func newRunCmd() *cobra.Command {
	var (
		kubeConfigPath  string
		kubeContext     string
		namespace       string
		httpRouteName   string
		incumbentSvc    string
		candidateSvc    string
		stagesCSV       string
		bakeDuration    time.Duration
		confirmDuration time.Duration
		checkInterval   time.Duration

		prometheusURL     string
		errorRateQuery    string
		errorRateMax      float64
		latencyQuery      string
		latencyMaxSeconds float64
		healthCheckURLs   []string

		signingKeyPath   string
		oldControllerTag string
		certOutput       string
		certMarkdown     string
		checklistOutput  string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Execute a staged cutover for one HTTPRoute",
		RunE: func(cmd *cobra.Command, args []string) error {
			if httpRouteName == "" || incumbentSvc == "" || candidateSvc == "" {
				return fmt.Errorf("--httproute, --incumbent-service, and --candidate-service are required")
			}

			stages, err := traffic.ParseStages(stagesCSV)
			if err != nil {
				return fmt.Errorf("invalid --stages: %w", err)
			}

			restConfig, err := resolveRestConfig(kubeConfigPath, kubeContext)
			if err != nil {
				return fmt.Errorf("failed to load kubeconfig: %w", err)
			}

			dynClient, err := dynamic.NewForConfig(restConfig)
			if err != nil {
				return fmt.Errorf("failed to create dynamic client: %w", err)
			}

			shifter := traffic.NewShifter(dynClient, namespace, httpRouteName)

			var checkers []slo.Checker
			if prometheusURL != "" && errorRateQuery != "" {
				checkers = append(checkers, &slo.PromQLCheck{
					CheckName:     "error-rate",
					PrometheusURL: prometheusURL,
					Query:         errorRateQuery,
					Threshold:     errorRateMax,
					Comparison:    slo.ComparisonGreaterThan,
				})
			}
			if prometheusURL != "" && latencyQuery != "" {
				checkers = append(checkers, &slo.PromQLCheck{
					CheckName:     "latency",
					PrometheusURL: prometheusURL,
					Query:         latencyQuery,
					Threshold:     latencyMaxSeconds,
					Comparison:    slo.ComparisonGreaterThan,
				})
			}
			for i, url := range healthCheckURLs {
				checkers = append(checkers, &slo.HTTPHealthCheck{
					CheckName:      fmt.Sprintf("health-check-%d", i+1),
					URL:            url,
					ExpectedStatus: 200,
				})
			}

			if len(checkers) == 0 {
				fmt.Fprintln(os.Stderr, "Warning: no SLO or health checks configured. The rollout will advance through every stage on a timer with no automated rollback trigger.")
			}

			rolloutCfg := rollout.Config{
				IncumbentName:   incumbentSvc,
				CandidateName:   candidateSvc,
				Stages:          stages,
				BakeDuration:    bakeDuration,
				ConfirmDuration: confirmDuration,
				CheckInterval:   checkInterval,
				Checkers:        checkers,
			}

			fmt.Fprintf(os.Stderr, "Starting staged cutover for %s/%s: %s -> %s, stages %v\n",
				namespace, httpRouteName, incumbentSvc, candidateSvc, stages)

			result, err := rollout.Run(cmd.Context(), shifter, rolloutCfg)
			if err != nil {
				return fmt.Errorf("rollout failed: %w", err)
			}

			if result.RolledBack {
				fmt.Fprintf(os.Stderr, "ROLLED BACK: %s\n", result.Stages[len(result.Stages)-1].BreachReason)
				printStageHistory(result)
				return fmt.Errorf("cutover rolled back — traffic is back at 100%% incumbent")
			}

			if !result.Completed {
				return fmt.Errorf("rollout ended without completing or rolling back — this should not happen")
			}

			fmt.Fprintln(os.Stderr, "Cutover completed successfully.")
			printStageHistory(result)

			var priv ed25519.PrivateKey
			if signingKeyPath != "" {
				loaded, err := sign.LoadPrivateKey(signingKeyPath)
				if err != nil {
					return fmt.Errorf("failed to load signing key: %w", err)
				}
				priv = loaded
			} else {
				kp, err := sign.GenerateKeyPair()
				if err != nil {
					return fmt.Errorf("failed to generate a signing key: %w", err)
				}
				priv = kp.PrivateKey
				fmt.Fprintf(os.Stderr, "No --signing-key given; generated an ephemeral key for this run.\nPublic key (save this to verify the certificate later): %s\n", kp.Fingerprint())
			}

			cert := certificate.BuildRemediationCertificate(namespace+"/"+httpRouteName, incumbentSvc, candidateSvc, result)
			doc, err := certificate.Sign(cert, priv)
			if err != nil {
				return fmt.Errorf("failed to sign remediation certificate: %w", err)
			}
			if err := writeJSON(certOutput, doc); err != nil {
				return err
			}
			if err := os.WriteFile(certMarkdown, []byte(cert.ToMarkdown()), 0o644); err != nil {
				return fmt.Errorf("failed to write %s: %w", certMarkdown, err)
			}

			checklist := certificate.BuildDecommissionChecklist(oldControllerTag)
			if err := os.WriteFile(checklistOutput, []byte(checklist.ToMarkdown()), 0o644); err != nil {
				return fmt.Errorf("failed to write %s: %w", checklistOutput, err)
			}

			_, _ = fmt.Fprintf(os.Stdout, "Remediation certificate: %s (signed) and %s (human-readable).\nDecommission checklist: %s\n",
				certOutput, certMarkdown, checklistOutput)

			return nil
		},
	}

	cmd.Flags().StringVar(&kubeConfigPath, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().StringVar(&kubeContext, "context", "", "Kubeconfig context to use (default: current context)")
	cmd.Flags().StringVar(&namespace, "namespace", "default", "Namespace of the target HTTPRoute")
	cmd.Flags().StringVar(&httpRouteName, "httproute", "", "Name of the target HTTPRoute (required)")
	cmd.Flags().StringVar(&incumbentSvc, "incumbent-service", "", "Name of the incumbent backendRef Service (required)")
	cmd.Flags().StringVar(&candidateSvc, "candidate-service", "", "Name of the candidate backendRef Service (required)")
	cmd.Flags().StringVar(&stagesCSV, "stages", "1,5,25,50,100", "Comma-separated candidate weight progression; must end in 100")
	cmd.Flags().DurationVar(&bakeDuration, "bake-duration", 15*time.Minute, "How long to hold and watch each stage before advancing")
	cmd.Flags().DurationVar(&confirmDuration, "confirm-duration", 1*time.Hour, "How long to hold and watch 100% before declaring the cutover complete")
	cmd.Flags().DurationVar(&checkInterval, "check-interval", 30*time.Second, "How often to poll SLO/health checks during a bake period")

	cmd.Flags().StringVar(&prometheusURL, "prometheus-url", "", "Base URL of the Prometheus instance to query for SLO checks")
	cmd.Flags().StringVar(&errorRateQuery, "error-rate-query", "", "PromQL query returning a scalar error rate")
	cmd.Flags().Float64Var(&errorRateMax, "error-rate-threshold", 0, "Roll back if the error-rate query result exceeds this value")
	cmd.Flags().StringVar(&latencyQuery, "latency-query", "", "PromQL query returning a scalar latency (seconds)")
	cmd.Flags().Float64Var(&latencyMaxSeconds, "latency-threshold-seconds", 0, "Roll back if the latency query result exceeds this many seconds")
	cmd.Flags().StringSliceVar(&healthCheckURLs, "health-check-url", nil, "HTTP endpoint(s) expected to return 200; roll back on failure (repeatable)")

	cmd.Flags().StringVar(&signingKeyPath, "signing-key", "", "Path to a hex-encoded ed25519 private key (generated with 'harness keygen'; the same key format works for both tools). If unset, an ephemeral key is generated for this run.")
	cmd.Flags().StringVar(&oldControllerTag, "old-controller-name", "ingress-nginx", "Name of the controller being decommissioned, for the checklist")
	cmd.Flags().StringVar(&certOutput, "certificate-output", "remediation-certificate.json", "Path to write the signed JSON remediation certificate")
	cmd.Flags().StringVar(&certMarkdown, "certificate-markdown-output", "remediation-certificate.md", "Path to write the human-readable remediation certificate")
	cmd.Flags().StringVar(&checklistOutput, "checklist-output", "decommission-checklist.md", "Path to write the decommission checklist")

	return cmd
}

func printStageHistory(result *rollout.Result) {
	for _, s := range result.Stages {
		fmt.Fprintf(os.Stderr, "  [%s] %d%% candidate: %s", s.StartedAt.Format(time.RFC3339), s.CandidateWeight, s.Outcome)
		if s.BreachReason != "" {
			fmt.Fprintf(os.Stderr, " (%s)", s.BreachReason)
		}
		fmt.Fprintln(os.Stderr)
	}
}

func resolveRestConfig(kubeconfigPath, context string) (*rest.Config, error) {
	path := kubeconfigPath
	if path == "" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(homeDir, ".kube", "config")
			if _, statErr := os.Stat(candidate); statErr == nil {
				path = candidate
			}
		}
	}

	if path == "" {
		return rest.InClusterConfig()
	}

	overrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		overrides,
	).ClientConfig()
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}
