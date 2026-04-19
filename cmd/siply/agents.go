// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/agents"
	"siply.dev/siply/internal/skills"
)

var agentNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// globalAgentsDir returns the path to the global agent configs directory.
func globalAgentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("agents: cannot determine home directory: %w", err)
	}
	return agents.GlobalDir(home), nil
}

// newAgentsCmd creates the root `siply agents` command group.
func newAgentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Manage installed agent configurations",
	}
	cmd.AddCommand(newAgentsListCmd())
	cmd.AddCommand(newAgentsCreateCmd())
	return cmd
}

// newAgentsCreateCmd creates `siply agents create <name>`.
func newAgentsCreateCmd() *cobra.Command {
	var (
		global      bool
		description string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Scaffold a new agent configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			targetDir, err := resolveAgentCreateDir(name, global)
			if err != nil {
				return err
			}
			return executeAgentsCreate(cmd, name, description, targetDir)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "scaffold agent config to the global ~/.siply/agents/ directory")
	cmd.Flags().StringVar(&description, "description", "A custom agent configuration", "description of the agent config")
	return cmd
}

// resolveAgentCreateDir determines the target directory for the new agent config.
func resolveAgentCreateDir(name string, global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("agents: get home dir: %w", err)
		}
		return filepath.Join(agents.GlobalDir(home), name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("agents: get working dir: %w", err)
	}
	return filepath.Join(cwd, ".siply", "agents", name), nil
}

// executeAgentsCreate scaffolds a new agent config at targetDir.
func executeAgentsCreate(cmd *cobra.Command, name, description, targetDir string) error {
	if err := validateAgentName(name); err != nil {
		return err
	}
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("agent config %q already exists at %s", name, targetDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("agents: check target dir: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("agents: create directory: %w", err)
	}
	if err := agents.ScaffoldAgentConfig(targetDir, name, description); err != nil {
		if rmErr := os.RemoveAll(targetDir); rmErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to clean up %s: %v\n", targetDir, rmErr)
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created agent config %q at %s\nActivate with agent.config: %s in .siply/config.yaml.\n", name, targetDir, name)
	return nil
}

// validateAgentName enforces the agent config naming rules.
func validateAgentName(name string) error {
	if !agentNameRegex.MatchString(name) {
		return fmt.Errorf("agent name must be lowercase letters, digits, or hyphens; 1-63 chars, starting with a letter")
	}
	// Reuse the reserved command list from skills — agent names share the same namespace.
	if skills.IsReservedCommand(name) {
		return fmt.Errorf("%w: %q", skills.ErrReservedCommand, name)
	}
	return nil
}

// newAgentsListCmd creates `siply agents list`.
func newAgentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed agent configurations (global + project)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeAgentsList(cmd)
		},
	}
}

func executeAgentsList(cmd *cobra.Command) error {
	globalDir, err := globalAgentsDir()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("agents: get working dir: %w", err)
	}
	projectDir := ""
	projectAgentsDir := filepath.Join(cwd, ".siply", "agents")
	if info, statErr := os.Stat(projectAgentsDir); statErr == nil && info.IsDir() {
		projectDir = projectAgentsDir
	}

	loader := agents.NewAgentConfigLoader(globalDir, projectDir)
	if err := loader.LoadAll(cmd.Context()); err != nil {
		return fmt.Errorf("agents: load: %w", err)
	}

	all := loader.List()
	if len(all) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No agent configurations installed.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSOURCE\tDESCRIPTION")
	for _, a := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", a.Name, a.Version, a.Source, a.Description)
	}
	return w.Flush()
}
