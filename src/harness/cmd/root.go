package cmd

import "github.com/spf13/cobra"

// NewRootCmd creates the root command for the shadow & diff harness.
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ingress-shift-harness",
		Short:   "Shadow & diff harness: validate a candidate Gateway API controller against production traffic",
		Version: version,
		Long: `Ingress Shift Harness mirrors live production traffic to both the
incumbent ingress controller and a candidate Gateway API controller, diffs
the responses, and produces a signed parity report — the artifact that lets
someone approve the production cutover.`,
	}

	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newKeygenCmd())
	cmd.AddCommand(newVerifyCmd())

	return cmd
}
