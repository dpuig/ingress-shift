package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/dpuig/ingress-shift/src/orchestrator/pkg/certificate"
	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func newVerifyCmd() *cobra.Command {
	var certPath string
	var publicKeyPath string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify a remediation certificate's signature",
		Long: `Verify checks that a remediation certificate has not been tampered with
since it was signed. If --public-key is given, the certificate is checked
against that specific trusted key rather than merely the key embedded in
the certificate itself — use this when verifying a certificate someone
else sent you.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(certPath)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", certPath, err)
			}

			var doc sign.Document
			if err := json.Unmarshal(raw, &doc); err != nil {
				return fmt.Errorf("failed to parse %s as a signed certificate: %w", certPath, err)
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

			cert, err := certificate.Verify(&doc)
			if err != nil {
				return fmt.Errorf("verification FAILED: %w", err)
			}

			fmt.Printf("Signature VALID (public key: %s)\n", doc.PublicKey)
			fmt.Printf("HTTPRoute: %s (%s -> %s)\n", cert.HTTPRoute, cert.IncumbentName, cert.CandidateName)
			fmt.Printf("Migration completed: %v, %d stages recorded\n", cert.Completed, len(cert.Stages))

			return nil
		},
	}

	cmd.Flags().StringVar(&certPath, "certificate", "remediation-certificate.json", "Path to the signed JSON remediation certificate")
	cmd.Flags().StringVar(&publicKeyPath, "public-key", "", "Path to a trusted public key to verify against (if unset, only checks internal consistency)")

	return cmd
}
