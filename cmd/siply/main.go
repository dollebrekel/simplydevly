package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "siply",
		Short:   "AI coding agent — terminal-native, extensible, transparent",
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
