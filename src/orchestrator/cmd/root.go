package cmd

import "github.com/spf13/cobra"

// NewRootCmd creates the root command for the cutover orchestrator.
func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ingress-shift-orchestrator",
		Short:   "Safely migrate traffic from an ingress controller to a Gateway API controller",
		Version: version,
		Long: `Ingress Shift Orchestrator performs a staged, weighted traffic shift from
an incumbent ingress controller to a candidate Gateway API controller,
using Gateway API's native HTTPRoute backendRef weights. It watches
SLO/health checks during each bake period and rolls back to 100% incumbent
in a single patch the instant a breach is detected — automatically, in
seconds, not minutes.`,
		// A rolled-back cutover or an unreachable cluster is a runtime
		// outcome, not a command-line usage mistake — don't dump the full
		// flag reference on every such error. main() prints the error itself.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newVerifyCmd())

	return cmd
}
