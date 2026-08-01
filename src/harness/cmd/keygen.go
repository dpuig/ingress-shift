package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dpuig/ingress-shift/src/shared/sign"
)

func newKeygenCmd() *cobra.Command {
	var outputPrefix string

	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Generate an ed25519 signing key pair for parity reports",
		Long: `Keygen generates a key pair used to sign parity reports so they can be
verified later (e.g. by a change advisory board). Keys are generated and
stored locally — no external KMS or signing service is involved, so this
works in air-gapped environments.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kp, err := sign.GenerateKeyPair()
			if err != nil {
				return fmt.Errorf("failed to generate key pair: %w", err)
			}

			privPath := outputPrefix + ".priv"
			pubPath := outputPrefix + ".pub"

			if err := kp.SavePrivateKey(privPath); err != nil {
				return fmt.Errorf("failed to save private key: %w", err)
			}
			if err := kp.SavePublicKey(pubPath); err != nil {
				return fmt.Errorf("failed to save public key: %w", err)
			}

			fmt.Printf("Private key: %s (keep this secret; pass to 'serve --signing-key')\n", privPath)
			fmt.Printf("Public key:  %s (share this with whoever needs to verify reports)\n", pubPath)
			fmt.Printf("Fingerprint: %s\n", kp.Fingerprint())

			return nil
		},
	}

	cmd.Flags().StringVar(&outputPrefix, "output-prefix", "harness-key", "Prefix for the generated .priv/.pub files")

	return cmd
}
