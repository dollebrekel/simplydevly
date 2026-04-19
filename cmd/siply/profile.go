// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/agents"
	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/profiles"
	"siply.dev/siply/internal/skills"
)

var profileNameRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// newProfileCmd creates the root `siply profile` command group.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage workspace profiles (save, share, list)",
	}
	cmd.AddCommand(newProfileSaveCmd())
	cmd.AddCommand(newProfileShareCmd())
	cmd.AddCommand(newProfileListCmd())
	return cmd
}

// newProfileSaveCmd creates `siply profile save <name>`.
func newProfileSaveCmd() *cobra.Command {
	var (
		description string
		force       bool
	)
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save current workspace configuration as a named profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return executeProfileSave(cmd, name, description, force)
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "description of the profile")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing profile")
	return cmd
}

func executeProfileSave(cmd *cobra.Command, name, description string, force bool) error {
	if !profileNameRegex.MatchString(name) {
		return fmt.Errorf("profile name must be lowercase letters, digits, or hyphens; 1-63 chars, starting with a letter")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("profile: get home dir: %w", err)
	}
	targetDir := filepath.Join(profiles.GlobalDir(home), name)
	ctx := cmd.Context()

	registryDir := filepath.Join(home, ".siply", "plugins")
	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("profile: init plugin registry: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("profile: get working dir: %w", err)
	}

	globalSkillsDir := skills.GlobalDir(home)
	projectSkillsDir := ""
	if info, statErr := os.Stat(filepath.Join(cwd, ".siply", "skills")); statErr == nil && info.IsDir() {
		projectSkillsDir = filepath.Join(cwd, ".siply", "skills")
	}
	skillLoader := skills.NewSkillLoader(globalSkillsDir, projectSkillsDir)
	if err := skillLoader.LoadAll(ctx); err != nil {
		return fmt.Errorf("profile: load skills: %w", err)
	}

	globalAgentsDir := agents.GlobalDir(home)
	projectAgentsDir := ""
	if info, statErr := os.Stat(filepath.Join(cwd, ".siply", "agents")); statErr == nil && info.IsDir() {
		projectAgentsDir = filepath.Join(cwd, ".siply", "agents")
	}
	agentLoader := agents.NewAgentConfigLoader(globalAgentsDir, projectAgentsDir)
	if err := agentLoader.LoadAll(ctx); err != nil {
		return fmt.Errorf("profile: load agents: %w", err)
	}

	cfgLoader := config.NewLoader(config.LoaderOptions{})
	if err := cfgLoader.Init(ctx); err != nil {
		return fmt.Errorf("profile: load config: %w", err)
	}

	opts := profiles.SaveOptions{
		Name:           name,
		Description:    description,
		TargetDir:      targetDir,
		Force:          force,
		PluginRegistry: registry,
		SkillLoader:    skillLoader,
		AgentLoader:    agentLoader,
		ConfigResolver: cfgLoader,
	}

	if err := profiles.SaveProfile(ctx, opts); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Profile %q saved to %s\n", name, targetDir)
	return nil
}

// newProfileShareCmd creates `siply profile share <name>`.
func newProfileShareCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "share <name>",
		Short: "Publish a saved profile to the marketplace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeProfileShare(cmd, args[0], dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and package without uploading")
	return cmd
}

func executeProfileShare(cmd *cobra.Command, name string, dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("profile: get home dir: %w", err)
	}
	profileDir := filepath.Join(profiles.GlobalDir(home), name)
	configDir := filepath.Join(home, ".siply")

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return err
	}
	if err := bus.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = bus.Stop(ctx) }()

	validator := licensing.NewLicenseValidator(bus, configDir)
	if err := validator.Init(ctx); err != nil {
		return err
	}
	if err := validator.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = validator.Stop(ctx) }()

	if err := licensing.RequireAuth(validator); err != nil {
		return fmt.Errorf("Authentication required. Run 'siply auth login' first: %w", err)
	}

	// Pre-publish validation.
	result, err := marketplace.ValidateForPublish(profileDir)
	if err != nil {
		return err
	}

	// Package.
	fmt.Fprintln(cmd.OutOrStdout(), "Packaging...")
	archivePath, sha256hex, err := marketplace.PackageDir(profileDir)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if dryRun {
		info, _ := os.Stat(archivePath)
		var size int64
		if info != nil {
			size = info.Size()
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — %s v%s\n  Archive: %d bytes\n  SHA256: %s\n",
			result.Manifest.Metadata.Name, result.Manifest.Metadata.Version, size, sha256hex)
		return nil
	}

	token, err := licensing.AccountToken(validator)
	if err != nil {
		return fmt.Errorf("profile share: get account token: %w", err)
	}

	// Upload.
	fmt.Fprintln(cmd.OutOrStdout(), "Publishing...")
	owner, repo := marketplace.DefaultRepoConfig()
	client := marketplace.NewClient(marketplace.NewClientConfig{
		RepoOwner: owner,
		RepoName:  repo,
		Token:     token,
	})
	resp, err := client.Publish(ctx, marketplace.PublishRequest{
		Manifest:    result.Manifest.Metadata,
		ArchivePath: archivePath,
		SHA256:      sha256hex,
		ReadmeText:  result.Readme,
	})
	if err != nil {
		return err
	}

	if resp.URL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Published profile %s v%s. View at: %s\n", resp.Name, resp.Version, resp.URL)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Published profile %s v%s.\n", resp.Name, resp.Version)
	}
	return nil
}

// newProfileListCmd creates `siply profile list`.
func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved profiles (global + project)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeProfileList(cmd)
		},
	}
}

func executeProfileList(cmd *cobra.Command) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("profile: get home dir: %w", err)
	}
	globalDir := profiles.GlobalDir(home)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("profile: get working dir: %w", err)
	}
	projectDir := ""
	projectProfilesDir := filepath.Join(cwd, ".siply", "profiles")
	if info, statErr := os.Stat(projectProfilesDir); statErr == nil && info.IsDir() {
		projectDir = projectProfilesDir
	}

	loader := profiles.NewProfileLoader(globalDir, projectDir)
	if err := loader.LoadAll(cmd.Context()); err != nil {
		return fmt.Errorf("profile: load: %w", err)
	}

	all := loader.List()
	if len(all) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No profiles saved.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION\tITEMS\tSOURCE")
	for _, p := range all {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", p.Name, p.Version, p.Description, len(p.Items), p.Source)
	}
	return w.Flush()
}
