// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/workspace"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install from lockfile for reproducible setups",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeInstall(cmd)
		},
	}
}

func executeInstall(cmd *cobra.Command) error {
	ctx := cmd.Context()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("lockfile: cannot determine home directory: %w", err)
	}
	siplyDir := filepath.Join(home, ".siply")

	wsMgr := workspace.NewManager(siplyDir)
	if err := wsMgr.Init(ctx); err != nil {
		return fmt.Errorf("lockfile: workspace init: %w", err)
	}

	// Detect active workspace so ConfigDir() returns the correct path.
	if _, err := wsMgr.Detect(ctx); err != nil {
		// Non-fatal: fall through to cwd-based path if no workspace detected.
		_ = err
	}

	projectDir := wsMgr.ConfigDir()
	if projectDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("lockfile: cannot determine working directory: %w", err)
		}
		projectDir = filepath.Join(cwd, ".siply")
	}

	lockPath := filepath.Join(projectDir, "config.lock")

	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("lockfile: not found at %s — run 'siply lock' first", lockPath)
		}
		return fmt.Errorf("lockfile: failed to read %s: %w", lockPath, err)
	}

	lf, err := config.ParseLockfile(data)
	if err != nil {
		return err
	}

	// Config values are applied automatically by the config loader's layer 3
	// when it reads the lockfile. The loader is initialized with the lockfile path.
	loader := config.NewLoader(config.LoaderOptions{
		GlobalDir:  siplyDir,
		ProjectDir: projectDir,
	})
	if err := loader.Init(ctx); err != nil {
		return fmt.Errorf("lockfile: config init: %w", err)
	}
	defer loader.Stop(ctx) //nolint:errcheck

	fmt.Printf("Config applied from lockfile: %s\n", lockPath)

	// Plugin loading from lockfile — load installed Tier 1 plugins automatically.
	if len(lf.Plugins) > 0 {
		registryDir := filepath.Join(siplyDir, "plugins")
		registry := plugins.NewLocalRegistry(registryDir)
		if err := registry.Init(ctx); err != nil {
			return fmt.Errorf("lockfile: plugin registry init: %w", err)
		}

		merger := config.NewPluginConfigMerger(loader)
		tier1Loader := plugins.NewTier1Loader(registry, merger)

		for _, p := range lf.Plugins {
			switch p.Tier {
			case 1:
				if err := tier1Loader.Load(ctx, p.Name); err != nil {
					fmt.Printf("  plugin: %s (version=%s) — load failed: %v\n", p.Name, p.Version, err)
				} else {
					fmt.Printf("  plugin: %s (version=%s) — loaded (tier 1)\n", p.Name, p.Version)
				}
			case 3:
				fmt.Printf("  plugin: %s (version=%s) — available (tier 3, lazy-loaded)\n", p.Name, p.Version)
			default:
				fmt.Printf("  plugin: %s (version=%s, tier %d) — not yet available\n", p.Name, p.Version, p.Tier)
			}
		}
	}

	return nil
}
