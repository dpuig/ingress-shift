package main

import (
	"fmt"
	"os"

	// Registers in-tree cloud provider auth plugins (GCP, Azure, OpenStack)
	// so kubeconfigs using the older `auth-provider:` format authenticate
	// correctly — recommended by krew's plugin development best practices
	// for any Go-based kubectl plugin.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/dpuig/ingress-shift/src/analyzer/cmd"
)

// Version is set via -ldflags "-X main.Version=..." at build time.
var Version = "dev"

func main() {
	rootCmd := cmd.NewRootCmd(Version)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
