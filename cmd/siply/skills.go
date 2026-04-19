// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/skills"
)

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
	return cmd
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
