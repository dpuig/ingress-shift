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
		// A failed dispatch, signature check, etc. is a runtime outcome, not
		// a command-line usage mistake — don't dump the full flag reference
		// on every such error. main() prints the error itself.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newKeygenCmd())
	cmd.AddCommand(newVerifyCmd())

	return cmd
}
