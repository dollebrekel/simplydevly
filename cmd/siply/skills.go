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

	"siply.dev/siply/internal/skills"
)

var skillNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// globalSkillsDir returns the path to the global skills registry directory.
func globalSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("skills: cannot determine home directory: %w", err)
	}
	return skills.GlobalDir(home), nil
}

// newSkillsCmd creates the root `siply skills` command group.
func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage installed skills",
	}
	cmd.AddCommand(newSkillsListCmd())
	cmd.AddCommand(newSkillsCreateCmd())
	return cmd
}

// newSkillsCreateCmd creates `siply skills create <name>`.
func newSkillsCreateCmd() *cobra.Command {
	var (
		global      bool
		description string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Scaffold a new custom skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			targetDir, err := resolveSkillCreateDir(name, global)
			if err != nil {
				return err
			}
			return executeSkillsCreate(cmd, name, description, targetDir)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "scaffold skill to the global ~/.siply/skills/ directory")
	cmd.Flags().StringVar(&description, "description", "A custom skill", "description of the skill")
	return cmd
}

// resolveSkillCreateDir determines the target directory for the new skill.
func resolveSkillCreateDir(name string, global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("skills: get home dir: %w", err)
		}
		return filepath.Join(skills.GlobalDir(home), name), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("skills: get working dir: %w", err)
	}
	return filepath.Join(cwd, ".siply", "skills", name), nil
}

// executeSkillsCreate scaffolds a new skill at targetDir.
func executeSkillsCreate(cmd *cobra.Command, name, description, targetDir string) error {
	if err := validateSkillName(name); err != nil {
		return err
	}
	if _, err := os.Stat(targetDir); err == nil {
		return fmt.Errorf("skill %q already exists at %s", name, targetDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("skills: check target dir: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("skills: create directory: %w", err)
	}
	if err := skills.ScaffoldSkill(targetDir, name, description); err != nil {
		os.RemoveAll(targetDir)
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created skill %q at %s\nInvoke with /%s in the REPL.\n", name, targetDir, name)
	return nil
}

// validateSkillName enforces the skill naming rules (AC#5).
func validateSkillName(name string) error {
	if !skillNameRegex.MatchString(name) {
		return fmt.Errorf("skill name must be lowercase letters, digits, or hyphens; 1-63 chars, starting with a letter")
	}
	if skills.IsReservedCommand(name) {
		return fmt.Errorf("%w: %q", skills.ErrReservedCommand, name)
	}
	return nil
}

// newSkillsListCmd creates `siply skills list`.
func newSkillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed skills (global + project)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeSkillsList(cmd)
		},
	}
}

func executeSkillsList(cmd *cobra.Command) error {
	globalDir, err := globalSkillsDir()
	if err != nil {
		return err
	}

	// Detect project-level skills dir (CWD/.siply/skills/).
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("skills: get working dir: %w", err)
	}
	projectDir := ""
	projectSkillsDir := filepath.Join(cwd, ".siply", "skills")
	if info, statErr := os.Stat(projectSkillsDir); statErr == nil && info.IsDir() {
		projectDir = projectSkillsDir
	}

	loader := skills.NewSkillLoader(globalDir, projectDir)
	if err := loader.LoadAll(cmd.Context()); err != nil {
		return fmt.Errorf("skills: load: %w", err)
	}

	all := loader.List()
	if len(all) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No skills installed.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSOURCE\tDESCRIPTION")
	for _, s := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Version, s.Source, s.Description)
	}
	return w.Flush()
}
