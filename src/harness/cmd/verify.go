package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dpuig/ingress-shift/src/harness/pkg/report"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func newVerifyCmd() *cobra.Command {
	var reportPath string
	var publicKeyPath string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a parity report's signature",
		Long: `Verify checks that a parity report has not been tampered with since it
was signed. If --public-key is given, the report is checked against that
specific trusted key rather than merely the key embedded in the report
itself — use this when verifying a report someone else sent you.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(reportPath)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", reportPath, err)
			}

			var doc sign.Document
			if err := json.Unmarshal(raw, &doc); err != nil {
				return fmt.Errorf("failed to parse %s as a signed report: %w", reportPath, err)
			}

			if publicKeyPath != "" {
				trustedKey, err := sign.LoadPublicKey(publicKeyPath)
				if err != nil {
					return fmt.Errorf("failed to load public key: %w", err)
				}
				if err := sign.VerifyAgainstTrustedKey(&doc, trustedKey); err != nil {
					return fmt.Errorf("verification FAILED: %w", err)
				}
			} else {
				fmt.Fprintln(os.Stderr, "Warning: no --public-key given; only checking internal signature consistency, not that this key is trusted.")
				if err := sign.Verify(&doc); err != nil {
					return fmt.Errorf("verification FAILED: %w", err)
				}
			}

			parityReport, err := report.Verify(&doc)
			if err != nil {
				return fmt.Errorf("verification FAILED: %w", err)
			}

			fmt.Printf("Signature VALID (public key: %s)\n", doc.PublicKey)
			fmt.Printf("Parity: %.2f%% over %d requests (soak window: %s)\n",
				parityReport.ParityPercent, parityReport.TotalRequests, parityReport.SoakWindow)

			return nil
		},
	}

	cmd.Flags().StringVar(&reportPath, "report", "parity-report.json", "Path to the signed JSON report")
	cmd.Flags().StringVar(&publicKeyPath, "public-key", "", "Path to a trusted public key to verify against (if unset, only checks internal consistency)")

	return cmd
}
