// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/plugins"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check installed plugins for available updates",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeCheck(cmd)
		},
	}
}

func executeCheck(cmd *cobra.Command) error {
	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("check: cannot determine home directory: %w", err)
	}
	backupDir := filepath.Join(home, ".siply", "plugins", ".versions")

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("check: registry init: %w", err)
	}

	vm := plugins.NewVersionManager(registry, backupDir)
	if err := vm.LoadState(); err != nil {
		return fmt.Errorf("check: load state: %w", err)
	}
	infos, err := vm.Check(ctx)
	if err != nil {
		return fmt.Errorf("check: %w", err)
	}

	if len(infos) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tINSTALLED\tSTATUS")
	for _, info := range infos {
		status := "ok"
		if info.Pinned {
			status = "pinned"
		} else if !info.Compatible {
			status = "incompatible"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", info.Name, info.Current, status)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	return nil
}
