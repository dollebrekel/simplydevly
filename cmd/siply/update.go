// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/plugins"
)

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Update a plugin to the latest compatible version",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			source, _ := cmd.Flags().GetString("source")

			if all {
				return executeUpdateAll(cmd, source)
			}
			if len(args) == 0 {
				return fmt.Errorf("update: specify a plugin name or use --all")
			}
			return executeUpdate(cmd, args[0], source)
		},
	}
	cmd.Flags().Bool("all", false, "Update all plugins")
	cmd.Flags().String("source", "", "Local directory containing the updated plugin (Phase 1)")
	return cmd
}

func executeUpdate(cmd *cobra.Command, name, source string) error {
	if source == "" {
		return fmt.Errorf("update: --source is required in Phase 1 (no marketplace yet)")
	}

	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("update: cannot determine home directory: %w", err)
	}
	backupDir := filepath.Join(home, ".siply", "plugins", ".versions")

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("update: registry init: %w", err)
	}

	vm := plugins.NewVersionManager(registry, backupDir)
	if err := vm.LoadState(); err != nil {
		return fmt.Errorf("update: load state: %w", err)
	}

	fmt.Printf("Updating %s from %s...\n", name, source)
	if err := vm.Update(ctx, name, source); err != nil {
		return fmt.Errorf("update: %w", err)
	}

	fmt.Printf("✓ Updated: %s\n", name)
	return nil
}

func executeUpdateAll(cmd *cobra.Command, source string) error {
	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("update: cannot determine home directory: %w", err)
	}
	backupDir := filepath.Join(home, ".siply", "plugins", ".versions")

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("update: registry init: %w", err)
	}

	vm := plugins.NewVersionManager(registry, backupDir)
	if err := vm.LoadState(); err != nil {
		return fmt.Errorf("update: load state: %w", err)
	}

	// In Phase 1, --all with a single source updates plugins that match the source manifest name.
	if source != "" {
		m, err := plugins.LoadManifestFromDir(source)
		if err != nil {
			return fmt.Errorf("update: read source manifest: %w", err)
		}
		sources := map[string]string{m.Metadata.Name: source}
		results, err := vm.UpdateAll(ctx, sources)
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		printUpdateResults(results)
		return nil
	}

	// Without a source, just show what would be updated.
	fmt.Println("No --source specified. In Phase 1, use `siply update <name> --source <path>` for each plugin.")
	fmt.Println("Run `siply check` to see installed plugins.")
	return nil
}

func printUpdateResults(results []plugins.UpdateResult) {
	for _, r := range results {
		switch r.Status {
		case "updated":
			fmt.Printf("✓ %s: %s → %s\n", r.Name, r.From, r.To)
		case "skipped":
			fmt.Printf("⊘ %s: skipped (%v)\n", r.Name, r.Error)
		case "failed":
			fmt.Printf("✗ %s: failed (%v)\n", r.Name, r.Error)
		}
	}
}
