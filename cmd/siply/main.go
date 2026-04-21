// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/commands"
	"siply.dev/siply/internal/completion"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/profiles"
)

// completionFunc is the function signature used by Cobra's ValidArgsFunction.
type completionFunc = completion.CompletionFunc

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

	// Disable Cobra's built-in completion command — we provide our own.
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Register TUI-related persistent flags on root command.
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable color output")
	rootCmd.PersistentFlags().Bool("no-emoji", false, "Disable emoji in output")
	rootCmd.PersistentFlags().Bool("no-borders", false, "Disable border rendering")
	rootCmd.PersistentFlags().Bool("no-motion", false, "Disable animations/spinners")
	rootCmd.PersistentFlags().Bool("accessible", false, "Enable accessible mode (text headers, no animations)")
	rootCmd.PersistentFlags().Bool("low-bandwidth", false, "Optimize for low bandwidth (ASCII borders, no animations)")
	rootCmd.PersistentFlags().Bool("minimal", false, "Use minimal profile (no borders, single-line status)")
	rootCmd.PersistentFlags().Bool("standard", false, "Use standard profile (borders, full status bar, emoji)")

	// Build plugin name completion function (best-effort: nil registry is safe).
	pluginComplete := buildPluginCompletion()

	rootCmd.AddCommand(newRunCmd())
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newLoginAlias())
	rootCmd.AddCommand(newLogoutAlias())
	rootCmd.AddCommand(newStatusAlias())
	rootCmd.AddCommand(newProAlias())
	rootCmd.AddCommand(newWorkspacesCmd())
	rootCmd.AddCommand(newLockCmd())
	rootCmd.AddCommand(newInstallCmd())
	rootCmd.AddCommand(newPluginsCmd(pluginComplete))
	rootCmd.AddCommand(newTUICmd())
	rootCmd.AddCommand(newCheckCmd(pluginComplete))
	rootCmd.AddCommand(newUpdateCmd(pluginComplete))
	rootCmd.AddCommand(newRollbackCmd(pluginComplete))
	rootCmd.AddCommand(newPinCmd(pluginComplete))
	rootCmd.AddCommand(newUnpinCmd(pluginComplete))
	rootCmd.AddCommand(newCompletionCmd(rootCmd))
	rootCmd.AddCommand(newSkillsCmd())
	rootCmd.AddCommand(newAgentsCmd())
	rootCmd.AddCommand(newProfileCmd())
	rootCmd.AddCommand(newDevCmd())
	rootCmd.AddCommand(commands.NewMarketplaceCmd())

	runFirstRunIfNeeded(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// runFirstRunIfNeeded checks for first-run state and prompts the user to choose a profile.
func runFirstRunIfNeeded(_ *cobra.Command) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	if !profiles.IsFirstRun(home) {
		return
	}

	choice, err := profiles.RunFirstRunPrompt(context.Background(), os.Stdout, os.Stdin)
	if err != nil {
		return
	}

	if choice != "" {
		configPath := filepath.Join(home, ".siply", "config.yaml")
		cfg := profiles.TUIOnlyConfig(choice)
		if err := profiles.ApplyProfileConfig(&cfg, configPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to apply profile config: %v\n", err)
			return
		}
	}

	if err := profiles.WriteFirstRunMarker(home); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write first-run marker: %v\n", err)
	}
}

// buildPluginCompletion creates a plugin name completion function that
// lazily initializes the LocalRegistry on first invocation. This avoids
// disk I/O (reading manifests) on every CLI command — the registry is only
// needed when the shell actually requests tab completions.
func buildPluginCompletion() completionFunc {
	var (
		once     sync.Once
		registry *plugins.LocalRegistry
	)

	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		once.Do(func() {
			dir, err := pluginsRegistryDir()
			if err != nil {
				return
			}
			r := plugins.NewLocalRegistry(dir)
			if err := r.Init(context.Background()); err != nil {
				return
			}
			registry = r
		})

		return completion.PluginNameCompletionFunc(registry)(cmd, args, toComplete)
	}
}
