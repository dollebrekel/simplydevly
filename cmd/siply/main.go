// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

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

	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newLoginCmd())
	rootCmd.AddCommand(newLogoutCmd())
	rootCmd.AddCommand(newProCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newWorkspacesCmd())
	rootCmd.AddCommand(newLockCmd())
	rootCmd.AddCommand(newInstallCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
