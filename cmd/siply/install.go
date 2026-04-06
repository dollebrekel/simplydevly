// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/config"
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

	// TODO(epic6): compare lockfile plugin versions against available versions and warn on mismatch (AC#4).
	// Plugin installation — stub until PluginRegistry is implemented (Epic 6).
	if len(lf.Plugins) > 0 {
		slog.Info("Plugin installation not yet available — config applied from lockfile")
		for _, p := range lf.Plugins {
			fmt.Printf("  plugin: %s (version=%s) — skipped, plugin system not yet available\n", p.Name, p.Version)
		}
	}

	return nil
}
