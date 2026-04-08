// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/workspace"
)

func newLockCmd() *cobra.Command {
	var verify bool
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "Generate or verify lockfile for reproducible setups",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLock(cmd, verify)
		},
	}
	cmd.Flags().BoolVar(&verify, "verify", false, "Verify current state matches lockfile")
	return cmd
}

func executeLock(cmd *cobra.Command, verify bool) error {
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

	loader := config.NewLoader(config.LoaderOptions{
		GlobalDir:    siplyDir,
		ProjectDir:   projectDir,
		SkipLockfile: !verify,
	})
	if err := loader.Init(ctx); err != nil {
		return fmt.Errorf("lockfile: config init: %w", err)
	}
	defer loader.Stop(ctx) //nolint:errcheck

	lockPath := filepath.Join(projectDir, "config.lock")

	if verify {
		result, err := config.VerifyLockfile(ctx, config.VerifyOptions{
			LockfilePath:   lockPath,
			ConfigResolver: loader,
		})
		if err != nil {
			return err
		}
		if result.Match {
			fmt.Println("Lockfile verified: all settings match")
			return nil
		}
		fmt.Printf("lockfile: verification failed — %d mismatches found\n", len(result.Diffs))
		for _, d := range result.Diffs {
			fmt.Printf("  %s: expected=%s actual=%s\n", d.Field, d.Expected, d.Actual)
		}
		return fmt.Errorf("lockfile: verification failed — %d mismatches found", len(result.Diffs))
	}

	lf, err := config.GenerateLockfile(ctx, config.GenerateOptions{
		ConfigResolver: loader,
	})
	if err != nil {
		return err
	}

	// Ensure directory exists.
	if err := os.MkdirAll(projectDir, 0700); err != nil {
		return fmt.Errorf("lockfile: failed to create directory %s: %w", projectDir, err)
	}

	if err := config.WriteLockfile(lockPath, lf); err != nil {
		return err
	}

	fmt.Printf("Lockfile written: %s\n", lockPath)
	fmt.Printf("  version: %s\n", lf.Version)
	fmt.Printf("  plugins: %d\n", len(lf.Plugins))
	return nil
}
