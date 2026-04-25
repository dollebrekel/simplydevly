// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/checkpoint"
)

func newCheckpointCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Inspect and manage session checkpoints",
	}

	cmd.AddCommand(newCheckpointListCmd())
	cmd.AddCommand(newCheckpointShowCmd())
	cmd.AddCommand(newCheckpointExportCmd())
	cmd.AddCommand(newCheckpointCleanCmd())

	return cmd
}

func newCheckpointListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [session-id]",
		Short: "List checkpoints for a session",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, sessionID, err := bootstrapCheckpointManager(args)
			if err != nil {
				return err
			}
			defer mgr.Close()

			metas, err := mgr.List(sessionID)
			if err != nil {
				return fmt.Errorf("list checkpoints: %w", err)
			}

			if len(metas) == 0 {
				fmt.Println("No checkpoints found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "Step\tTime\tTool\tMessages\tSize")
			fmt.Fprintln(w, "----\t----\t----\t--------\t----")
			for _, m := range metas {
				fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\n",
					m.StepNumber,
					m.Timestamp.Format("15:04:05"),
					m.ToolName,
					m.MessageCount,
					formatBytes(m.DiskSize),
				)
			}
			return w.Flush()
		},
	}
}

func newCheckpointShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <step> [session-id]",
		Short: "Show checkpoint details with truncated messages",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			step, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid step number: %w", err)
			}

			var sessionArgs []string
			if len(args) > 1 {
				sessionArgs = []string{args[1]}
			}
			mgr, sessionID, err := bootstrapCheckpointManager(sessionArgs)
			if err != nil {
				return err
			}
			defer mgr.Close()

			cp, err := mgr.Load(sessionID, step)
			if err != nil {
				return fmt.Errorf("load checkpoint: %w", err)
			}

			fmt.Printf("Step %d — %s\n", cp.StepNumber, cp.Timestamp.Format("2006-01-02 15:04:05"))
			fmt.Printf("Tool: %s\n", cp.ToolName)
			fmt.Printf("Messages: %d\n", len(cp.Messages))
			fmt.Printf("Files tracked: %d\n", len(cp.FileHashes))
			fmt.Printf("Disk size: %s\n\n", formatBytes(cp.DiskSize))

			for i, msg := range cp.Messages {
				content := msg.Content
				if utf8.RuneCountInString(content) > 500 {
					content = string([]rune(content)[:500]) + "..."
				}
				fmt.Printf("[%d] %s: %s\n", i, msg.Role, content)
			}
			return nil
		},
	}
}

func newCheckpointExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export <step> [session-id]",
		Short: "Export checkpoint as JSON to stdout",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			step, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid step number: %w", err)
			}

			var sessionArgs []string
			if len(args) > 1 {
				sessionArgs = []string{args[1]}
			}
			mgr, sessionID, err := bootstrapCheckpointManager(sessionArgs)
			if err != nil {
				return err
			}
			defer mgr.Close()

			cp, err := mgr.Load(sessionID, step)
			if err != nil {
				return fmt.Errorf("load checkpoint: %w", err)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(cp)
		},
	}
}

func newCheckpointCleanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Prune old checkpoint data to free disk space",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			baseDir := filepath.Join(home, ".siply", "checkpoints")

			before := dirSizeTotal(baseDir)
			mgr := checkpoint.NewManager(baseDir, "")
			if err := mgr.Prune(100 * 1024 * 1024); err != nil {
				mgr.Close()
				return fmt.Errorf("prune: %w", err)
			}
			mgr.Close()
			after := dirSizeTotal(baseDir)

			freed := before - after
			if freed < 0 {
				freed = 0
			}
			fmt.Printf("Pruned checkpoints: freed %s (was %s, now %s)\n",
				formatBytes(freed), formatBytes(before), formatBytes(after))
			return nil
		},
	}
}

func bootstrapCheckpointManager(args []string) (*checkpoint.Manager, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	baseDir := filepath.Join(home, ".siply", "checkpoints")

	sessionID := ""
	if len(args) > 0 {
		if strings.ContainsAny(args[0], "/\\") || strings.Contains(args[0], "..") {
			return nil, "", fmt.Errorf("invalid session ID: %q", args[0])
		}
		sessionID = args[0]
	} else {
		sessionID, err = latestSessionDir(baseDir)
		if err != nil {
			return nil, "", fmt.Errorf("no sessions found: %w", err)
		}
	}

	mgr := checkpoint.NewManager(baseDir, sessionID)
	return mgr, sessionID, nil
}

func latestSessionDir(baseDir string) (string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", err
	}
	var latest string
	var latestTime int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UnixNano() > latestTime {
			latestTime = info.ModTime().UnixNano()
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", fmt.Errorf("no session directories found")
	}
	return latest, nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func dirSizeTotal(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}
