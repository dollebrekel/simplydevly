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

func newPinCmd(pluginComplete completionFunc) *cobra.Command {
	return &cobra.Command{
		Use:               "pin <name>@<version>",
		Short:             "Pin a plugin to a specific version (skips auto-update)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: pluginComplete,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executePin(cmd, args[0])
		},
	}
}

func newUnpinCmd(pluginComplete completionFunc) *cobra.Command {
	return &cobra.Command{
		Use:               "unpin <name>",
		Short:             "Unpin a plugin, allowing updates again",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: pluginComplete,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeUnpin(cmd, args[0])
		},
	}
}

func executePin(cmd *cobra.Command, arg string) error {
	// Parse name@version format.
	parts := strings.SplitN(arg, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("pin: expected format <name>@<version>, got %q", arg)
	}
	name := parts[0]
	version := parts[1]

	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("pin: invalid plugin name %q", name)
	}

	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("pin: cannot determine home directory: %w", err)
	}
	backupDir := filepath.Join(home, ".siply", "plugins", ".versions")

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("pin: registry init: %w", err)
	}

	vm := plugins.NewVersionManager(registry, backupDir)
	if err := vm.LoadState(); err != nil {
		return fmt.Errorf("pin: load state: %w", err)
	}
	if err := vm.Pin(ctx, name, version); err != nil {
		return fmt.Errorf("pin: %w", err)
	}

	fmt.Printf("✓ Pinned: %s@%s\n", name, version)
	return nil
}

func executeUnpin(cmd *cobra.Command, name string) error {
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("unpin: invalid plugin name %q", name)
	}

	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unpin: cannot determine home directory: %w", err)
	}
	backupDir := filepath.Join(home, ".siply", "plugins", ".versions")

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("unpin: registry init: %w", err)
	}

	vm := plugins.NewVersionManager(registry, backupDir)
	if err := vm.LoadState(); err != nil {
		return fmt.Errorf("unpin: load state: %w", err)
	}
	if err := vm.Unpin(ctx, name); err != nil {
		return fmt.Errorf("unpin: %w", err)
	}

	fmt.Printf("✓ Unpinned: %s\n", name)
	return nil
}
