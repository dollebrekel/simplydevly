// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/plugins"
)

func newRollbackCmd(pluginComplete completionFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "rollback <name>",
		Short:             "Rollback a plugin to its previous version",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: pluginComplete,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeRollback(cmd, args[0])
		},
	}
	cmd.Flags().Bool("yes", false, "Skip confirmation prompt")
	return cmd
}

func executeRollback(cmd *cobra.Command, name string) error {
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("rollback: invalid plugin name %q", name)
	}

	ctx := cmd.Context()
	yes, _ := cmd.Flags().GetBool("yes")

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("rollback: cannot determine home directory: %w", err)
	}
	backupDir := filepath.Join(home, ".siply", "plugins", ".versions")

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("rollback: registry init: %w", err)
	}

	vm := plugins.NewVersionManager(registry, backupDir)
	if err := vm.LoadState(); err != nil {
		return fmt.Errorf("rollback: load state: %w", err)
	}

	previousVersion := vm.GetPrevious(name)
	if previousVersion == "" {
		previousVersion = "(from backup)"
	}

	if !yes {
		fmt.Printf("Rollback %s to version %s? [y/N] ", name, previousVersion)
		var response string
		if _, err := fmt.Scanln(&response); err != nil || (response != "y" && response != "Y") {
			fmt.Println("Rollback cancelled.")
			return nil
		}
	}

	if err := vm.Rollback(ctx, name); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}

	fmt.Printf("✓ Rolled back: %s to %s\n", name, previousVersion)
	return nil
}
