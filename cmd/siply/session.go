// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type sessionDistillate struct {
	SessionID  string                    `json:"session_id"`
	Workspace  string                    `json:"workspace"`
	Timestamp  time.Time                 `json:"timestamp"`
	Model      string                    `json:"model"`
	TokenCount int                       `json:"token_count"`
	Content    sessionDistillateContent  `json:"content"`
}

type sessionDistillateContent struct {
	KeyDecisions []string          `json:"key_decisions"`
	ActiveFiles  []string          `json:"active_files"`
	CurrentTask  string            `json:"current_task"`
	Constraints  []string          `json:"constraints"`
	Patterns     []sessionPattern  `json:"patterns"`
}

type sessionPattern struct {
	Pattern    string `json:"pattern"`
	Confidence string `json:"confidence"`
}

func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage session intelligence distillates",
	}

	cmd.AddCommand(newSessionListCmd())
	cmd.AddCommand(newSessionShowCmd())
	cmd.AddCommand(newSessionClearCmd())

	return cmd
}

func newSessionListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all session distillates for the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			dir := sessionDir(cwd)
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No session distillates found for this workspace.")
					return nil
				}
				return fmt.Errorf("read distillate dir: %w", err)
			}

			found := false
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), "-distillate.json") {
					continue
				}
				data, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
				if readErr != nil {
					continue
				}
				var d sessionDistillate
				if json.Unmarshal(data, &d) != nil {
					continue
				}
				found = true
				decisions := ""
				if len(d.Content.KeyDecisions) > 0 {
					decisions = d.Content.KeyDecisions[0]
					if len(decisions) > 60 {
						decisions = decisions[:57] + "..."
					}
				}
				fmt.Printf("  %s  %s  %d tokens  %s\n",
					d.SessionID,
					d.Timestamp.Format("2006-01-02 15:04"),
					d.TokenCount,
					decisions,
				)
			}

			if !found {
				fmt.Println("No session distillates found for this workspace.")
			}
			return nil
		},
	}
}

func newSessionShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show the full distillate for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
				return fmt.Errorf("invalid session ID: %q", id)
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			path := filepath.Join(sessionDir(cwd), id+"-distillate.json")
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("no distillate found for session %q", args[0])
				}
				return fmt.Errorf("read distillate: %w", err)
			}

			var d sessionDistillate
			if err := json.Unmarshal(data, &d); err != nil {
				return fmt.Errorf("parse distillate: %w", err)
			}

			fmt.Printf("Session: %s\n", d.SessionID)
			fmt.Printf("Date:    %s\n", d.Timestamp.Format("2006-01-02 15:04:05"))
			fmt.Printf("Model:   %s\n", d.Model)
			fmt.Printf("Tokens:  %d\n\n", d.TokenCount)

			if d.Content.CurrentTask != "" {
				fmt.Printf("Current Task: %s\n\n", d.Content.CurrentTask)
			}
			if len(d.Content.KeyDecisions) > 0 {
				fmt.Println("Key Decisions:")
				for _, dec := range d.Content.KeyDecisions {
					fmt.Printf("  - %s\n", dec)
				}
				fmt.Println()
			}
			if len(d.Content.ActiveFiles) > 0 {
				fmt.Println("Active Files:")
				for _, f := range d.Content.ActiveFiles {
					fmt.Printf("  - %s\n", f)
				}
				fmt.Println()
			}
			if len(d.Content.Constraints) > 0 {
				fmt.Println("Constraints:")
				for _, c := range d.Content.Constraints {
					fmt.Printf("  - %s\n", c)
				}
				fmt.Println()
			}
			if len(d.Content.Patterns) > 0 {
				fmt.Println("Patterns:")
				for _, p := range d.Content.Patterns {
					fmt.Printf("  - [%s] %s\n", p.Confidence, p.Pattern)
				}
			}
			return nil
		},
	}
}

func newSessionClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove all session distillates for the current workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			dir := sessionDir(cwd)
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("clear distillates: %w", err)
			}
			fmt.Println("Session distillates cleared.")
			return nil
		},
	}
}

func sessionDir(workspacePath string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		abs = workspacePath
	}
	h := sha256.Sum256([]byte(abs))
	hash := fmt.Sprintf("%x", h[:6])
	return filepath.Join(home, ".siply", "sessions", hash)
}
