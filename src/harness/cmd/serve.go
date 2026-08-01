package cmd

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dpuig/ingress-shift/src/harness/pkg/diff"
	"github.com/dpuig/ingress-shift/src/harness/pkg/mirror"
	"github.com/dpuig/ingress-shift/src/harness/pkg/report"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func newServeCmd() *cobra.Command {
	var (
		listen              string
		incumbentURL        string
		candidateURL        string
		soakWindow          time.Duration
		backendTimeout      time.Duration
		outputJSON          string
		outputMarkdown      string
		signingKeyPath      string
		ignoredHeaders      []string
		significantHeaders  []string
		expectedDiffHeaders []string
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Listen for mirrored traffic and build a signed parity report",
		Long: `Serve starts an HTTP listener that receives mirrored production
requests (fed by your ingress controller's or Gateway API's request-mirror
feature), replays each one against the incumbent and candidate backends,
and diffs the responses. It never sits inline with production traffic — it
only ever receives a copy — so it can't affect production if it fails.

After --soak-window elapses (or on SIGINT/SIGTERM), it writes a signed
parity report and exits.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if incumbentURL == "" || candidateURL == "" {
				return fmt.Errorf("--incumbent-url and --candidate-url are required")
			}

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
				fmt.Fprintf(os.Stderr, "No --signing-key given; generated an ephemeral key for this run.\nPublic key (save this to verify the report later): %s\n", kp.Fingerprint())
			}

			cfg := diff.DefaultConfig()
			if len(ignoredHeaders) > 0 {
				cfg.IgnoredHeaders = ignoredHeaders
			}
			if len(significantHeaders) > 0 {
				cfg.SignificantHeaders = significantHeaders
			}
			cfg.ExpectedDiffHeaders = expectedDiffHeaders

			agg := report.NewAggregator()
			listener := &mirror.Listener{
				Dispatcher: mirror.NewDispatcher(
					mirror.Target{Name: "incumbent", BaseURL: strings.TrimRight(incumbentURL, "/"), Timeout: backendTimeout},
					mirror.Target{Name: "candidate", BaseURL: strings.TrimRight(candidateURL, "/"), Timeout: backendTimeout},
				),
				DiffConfig: cfg,
				Aggregator: agg,
			}

			srv := &http.Server{Addr: listen, Handler: listener}

			serverErrCh := make(chan error, 1)
			go func() {
				fmt.Fprintf(os.Stderr, "Listening for mirrored traffic on %s (soak window: %s)\n", listen, soakWindow)
				if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					serverErrCh <- err
				}
			}()

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			select {
			case <-time.After(soakWindow):
				fmt.Fprintln(os.Stderr, "Soak window elapsed, generating report...")
			case <-ctx.Done():
				fmt.Fprintln(os.Stderr, "Interrupted, generating report from data collected so far...")
			case err := <-serverErrCh:
				return fmt.Errorf("listener failed: %w", err)
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)

			parityReport := report.Build(agg.Snapshot(), soakWindow, incumbentURL, candidateURL)

			doc, err := report.Sign(parityReport, priv)
			if err != nil {
				return fmt.Errorf("failed to sign report: %w", err)
			}

			if err := writeJSON(outputJSON, doc); err != nil {
				return err
			}
			if err := os.WriteFile(outputMarkdown, []byte(parityReport.ToMarkdown()), 0o644); err != nil {
				return fmt.Errorf("failed to write markdown report to %s: %w", outputMarkdown, err)
			}

			_, _ = fmt.Fprintf(os.Stdout, "Parity: %.2f%% over %d requests. Report written to %s (signed) and %s (human-readable).\n",
				parityReport.ParityPercent, parityReport.TotalRequests, outputJSON, outputMarkdown)

			return nil
		},
	}

	cmd.Flags().StringVar(&listen, "listen", ":8443", "Address to listen for mirrored traffic on")
	cmd.Flags().StringVar(&incumbentURL, "incumbent-url", "", "Base URL of the incumbent ingress controller (required)")
	cmd.Flags().StringVar(&candidateURL, "candidate-url", "", "Base URL of the candidate Gateway API controller (required)")
	cmd.Flags().DurationVar(&soakWindow, "soak-window", 24*time.Hour, "How long to collect mirrored traffic before producing a report")
	cmd.Flags().DurationVar(&backendTimeout, "backend-timeout", 10*time.Second, "Timeout for each backend dispatch")
	cmd.Flags().StringVar(&outputJSON, "output", "parity-report.json", "Path to write the signed JSON report")
	cmd.Flags().StringVar(&outputMarkdown, "markdown-output", "parity-report.md", "Path to write the human-readable Markdown report")
	cmd.Flags().StringVar(&signingKeyPath, "signing-key", "", "Path to a hex-encoded ed25519 private key (generated with 'harness keygen'). If unset, an ephemeral key is generated for this run.")
	cmd.Flags().StringSliceVar(&ignoredHeaders, "ignored-headers", nil, "Headers to exclude from comparison entirely (default: infra-noise headers)")
	cmd.Flags().StringSliceVar(&significantHeaders, "significant-headers", nil, "Headers whose mismatch is classified as breaking (default: Content-Type, Location)")
	cmd.Flags().StringSliceVar(&expectedDiffHeaders, "expected-diff-headers", nil, "Headers known to differ intentionally; mismatches are classified as expected")

	return cmd
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
