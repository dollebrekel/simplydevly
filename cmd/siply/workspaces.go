// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/workspace"
)

func newWorkspacesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspaces",
		Short: "Manage workspaces",
	}
	cmd.AddCommand(newWorkspacesListCmd())
	cmd.AddCommand(newWorkspacesSwitchCmd())
	return cmd
}

func newWorkspacesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all known workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("workspace: cannot determine home directory: %w", err)
			}
			mgr := workspace.NewManager(filepath.Join(home, ".siply"))
			if err := mgr.Init(cmd.Context()); err != nil {
				return fmt.Errorf("workspace: init: %w", err)
			}
			list := mgr.List(cmd.Context())
			if len(list) == 0 {
				fmt.Println("No workspaces registered yet.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tROOT DIR\tGIT ROOT")
			for _, ws := range list {
				fmt.Fprintf(w, "%s\t%s\t%s\n", ws.Name, ws.RootDir, ws.GitRoot)
			}
			return w.Flush()
		},
	}
}

func newWorkspacesSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <name>",
		Short: "Switch to a different workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("workspace: cannot determine home directory: %w", err)
			}
			mgr := workspace.NewManager(filepath.Join(home, ".siply"))
			if err := mgr.Init(cmd.Context()); err != nil {
				return fmt.Errorf("workspace: init: %w", err)
			}
			ws, err := mgr.Switch(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("workspace: switch: %w", err)
			}
			fmt.Printf("Switched to workspace %q (%s)\n", ws.Name, ws.RootDir)
			return nil
		},
	}
}

