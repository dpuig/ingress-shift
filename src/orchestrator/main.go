package main

import (
	"fmt"
	"os"

	"github.com/dpuig/ingress-shift/src/orchestrator/cmd"
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
