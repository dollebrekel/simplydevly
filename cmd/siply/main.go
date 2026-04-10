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

	// Register TUI-related persistent flags on root command.
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("no-emoji", false, "Disable emoji in output")
	rootCmd.PersistentFlags().Bool("no-borders", false, "Disable border rendering")
	rootCmd.PersistentFlags().Bool("no-motion", false, "Disable animations/spinners")
	rootCmd.PersistentFlags().Bool("accessible", false, "Enable accessible mode (text headers, no animations)")
	rootCmd.PersistentFlags().Bool("low-bandwidth", false, "Optimize for low bandwidth (ASCII borders, no animations)")
	rootCmd.PersistentFlags().Bool("minimal", false, "Use minimal profile (no borders, single-line status)")
	rootCmd.PersistentFlags().Bool("standard", false, "Use standard profile (borders, full status bar, emoji)")

	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newLoginCmd())
	rootCmd.AddCommand(newLogoutCmd())
	rootCmd.AddCommand(newProCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newWorkspacesCmd())
	rootCmd.AddCommand(newLockCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newPluginsCmd())
	rootCmd.AddCommand(newTUICmd())
	rootCmd.AddCommand(newCheckCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newRollbackCmd())
	rootCmd.AddCommand(newPinCmd())
	rootCmd.AddCommand(newUnpinCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
