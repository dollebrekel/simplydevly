// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

const (
	builtinMinimal  = "minimal"
	builtinStandard = "standard"
)

// newProfileCmd creates the root `siply profile` command group.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage workspace profiles (save, share, list, install)",
	}
	cmd.AddCommand(newProfileSaveCmd())
	cmd.AddCommand(newProfileShareCmd())
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileInstallCmd())
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

// newProfileInstallCmd creates `siply profile install <name>`.
func newProfileInstallCmd() *cobra.Command {
	var (
		global  bool
		yes     bool
		project bool
	)
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a team profile (plugins, skills, agents + config)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeProfileInstall(cmd, args[0], global, yes, project)
		},
	}
	cmd.Flags().BoolVar(&global, "global", false, "Write config to global ~/.siply/config.yaml instead of project")
	cmd.Flags().BoolVar(&yes, "yes", false, "Overwrite all conflicts without prompting")
	cmd.Flags().BoolVar(&project, "project", false, "Install items to project .siply/ dirs")
	return cmd
}

func executeProfileInstall(cmd *cobra.Command, name string, global, yes, project bool) error {
	if !profileNameRegex.MatchString(name) {
		return fmt.Errorf("profile name must be lowercase letters, digits, or hyphens; 1-63 chars, starting with a letter")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	out := cmd.OutOrStdout()

	// Built-in TUI profiles — no marketplace lookup needed.
	if name == builtinMinimal || name == builtinStandard {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("profile install: get home dir: %w", err)
		}
		targetPath, err := profileConfigTarget(home, global)
		if err != nil {
			return err
		}
		tuiCfg := profiles.TUIOnlyConfig(name)
		if err := profiles.ApplyProfileConfig(&tuiCfg, targetPath); err != nil {
			return err
		}
		fmt.Fprintf(out, "✅ TUI profile set to %q\n", name)
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("profile install: get home dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("profile install: get working dir: %w", err)
	}

	// Load profile — try local dirs first, then marketplace.
	globalProfilesDir := profiles.GlobalDir(home)
	projectProfilesDir := ""
	if info, statErr := os.Stat(filepath.Join(cwd, ".siply", "profiles")); statErr == nil && info.IsDir() {
		projectProfilesDir = filepath.Join(cwd, ".siply", "profiles")
	}
	loader := profiles.NewProfileLoader(globalProfilesDir, projectProfilesDir)
	if err := loader.LoadAll(ctx); err != nil {
		return fmt.Errorf("profile install: load profiles: %w", err)
	}

	profile, err := loader.Get(name)
	if err != nil {
		return fmt.Errorf("profile install: profile %q not found locally and marketplace download is not yet supported — run 'siply marketplace install %s' first", name, name)
	}

	// Collect existing installed items.
	registryDir := filepath.Join(home, ".siply", "plugins")
	registry := plugins.NewLocalRegistry(registryDir)
	if initErr := registry.Init(ctx); initErr != nil {
		return fmt.Errorf("profile install: init plugin registry: %w", initErr)
	}
	existingPlugins, err := registry.List(ctx)
	if err != nil {
		return fmt.Errorf("profile install: list plugins: %w", err)
	}

	globalSkillsDir := skills.GlobalDir(home)
	skillLoader := skills.NewSkillLoader(globalSkillsDir, "")
	if err := skillLoader.LoadAll(ctx); err != nil {
		return fmt.Errorf("profile install: load skills: %w", err)
	}
	existingSkills := make([]profiles.ProfileItem, 0)
	for _, s := range skillLoader.List() {
		existingSkills = append(existingSkills, profiles.ProfileItem{Name: s.Name, Version: s.Version, Category: "skills"})
	}

	globalAgentsDir := agents.GlobalDir(home)
	agentLoader := agents.NewAgentConfigLoader(globalAgentsDir, "")
	if err := agentLoader.LoadAll(ctx); err != nil {
		return fmt.Errorf("profile install: load agents: %w", err)
	}
	existingAgents := make([]profiles.ProfileItem, 0)
	for _, a := range agentLoader.List() {
		existingAgents = append(existingAgents, profiles.ProfileItem{Name: a.Name, Version: a.Version, Category: "agents"})
	}

	// Build installer functions per category.
	pluginsTargetDir := registryDir
	skillsTargetDir := globalSkillsDir
	agentsTargetDir := globalAgentsDir
	if project {
		pluginsTargetDir = filepath.Join(cwd, ".siply", "plugins")
		skillsTargetDir = filepath.Join(cwd, ".siply", "skills")
		agentsTargetDir = filepath.Join(cwd, ".siply", "agents")
	}

	pluginInstaller := marketplaceItemInstaller(pluginsTargetDir)
	skillInstaller := marketplaceItemInstaller(skillsTargetDir)
	agentInstaller := marketplaceItemInstaller(agentsTargetDir)

	opts := profiles.InstallOptions{
		Profile:         profile,
		PluginInstaller: pluginInstaller,
		SkillInstaller:  skillInstaller,
		AgentInstaller:  agentInstaller,
		Existing: profiles.ExistingItems{
			Plugins: existingPlugins,
			Skills:  existingSkills,
			Agents:  existingAgents,
		},
		Force:  yes,
		Writer: out,
	}

	result, err := profiles.InstallProfile(ctx, opts)
	if err != nil {
		return err
	}

	// Show conflicts even in non-interactive mode (--yes).
	if yes && len(result.Conflicts) > 0 {
		fmt.Fprintln(out, "Conflicts detected (overwriting with --yes):")
		for _, c := range result.Conflicts {
			fmt.Fprintf(out, "  %s (%s): installed v%s → profile wants v%s\n", c.Name, c.Category, c.CurrentVersion, c.ProfileVersion)
		}
	}

	// Handle conflicts interactively.
	if result.NeedsConfirmation {
		fmt.Fprintln(out, "Conflicts detected:")
		for _, c := range result.Conflicts {
			fmt.Fprintf(out, "  %s (%s): installed v%s → profile wants v%s\n", c.Name, c.Category, c.CurrentVersion, c.ProfileVersion)
		}
		fmt.Fprint(out, "[S]kip all conflicts / [O]verwrite all / [Q]uit: ")

		scanner := bufio.NewScanner(cmd.InOrStdin())
		if scanner.Scan() {
			choice := strings.ToLower(strings.TrimSpace(scanner.Text()))
			switch choice {
			case "o":
				opts.Force = true
				result, err = profiles.InstallProfile(ctx, opts)
				if err != nil {
					return err
				}
			case "q":
				fmt.Fprintln(out, "Aborted.")
				return nil
			default:
				fmt.Fprintln(out, "Conflicts skipped.")
			}
		} else if scanErr := scanner.Err(); scanErr != nil {
			return fmt.Errorf("profile install: read conflict choice: %w", scanErr)
		}
	}

	// Apply config from profile.
	if profile.Config != nil {
		cfgLoader := config.NewLoader(config.LoaderOptions{})
		if initErr := cfgLoader.Init(ctx); initErr != nil {
			return fmt.Errorf("profile install: load config: %w", initErr)
		}
		current := cfgLoader.Config()
		changes := profiles.DiffConfig(current, profile.Config)
		if len(changes) > 0 {
			fmt.Fprintln(out, "Config changes:")
			for _, ch := range changes {
				fmt.Fprintf(out, "  %s: %s → %s\n", ch.Key, ch.OldValue, ch.NewValue)
			}

			applyConfig := true
			if !yes {
				fmt.Fprint(out, "Apply these config changes? [Y/n]: ")
				scanner := bufio.NewScanner(cmd.InOrStdin())
				if scanner.Scan() {
					answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
					if answer == "n" || answer == "no" {
						applyConfig = false
						fmt.Fprintln(out, "Config changes skipped.")
					}
				}
			}

			if applyConfig {
				targetPath, err := profileConfigTarget(home, global)
				if err != nil {
					return err
				}
				if err := profiles.ApplyProfileConfig(profile.Config, targetPath); err != nil {
					return fmt.Errorf("profile install: apply config: %w", err)
				}
			}
		}
	}

	installed := 0
	skipped := 0
	failed := 0
	if result != nil {
		installed = len(result.Installed)
		skipped = len(result.Skipped)
		failed = len(result.Failed)
		for _, f := range result.Failed {
			fmt.Fprintf(out, "  ✗ %s (%s): %v\n", f.Name, f.Category, f.Err)
		}
	}

	fmt.Fprintf(out, "✅ Installed %d items, skipped %d", installed, skipped)
	if failed > 0 {
		fmt.Fprintf(out, ", %d failed", failed)
	}
	fmt.Fprintln(out)

	if failed > 0 {
		return fmt.Errorf("profile install: %d item(s) failed to install", failed)
	}
	return nil
}

// profileConfigTarget returns the config file path for the given flags.
func profileConfigTarget(homeDir string, global bool) (string, error) {
	if global {
		return filepath.Join(homeDir, ".siply", "config.yaml"), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("profile: determine project directory: %w", err)
	}
	return filepath.Join(cwd, ".siply", "config.yaml"), nil
}

// marketplaceItemInstaller creates an InstallerFunc that requires items to be pre-downloaded
// via the marketplace. It advises the user to run `siply marketplace install` first.
func marketplaceItemInstaller(targetDir string) profiles.InstallerFunc {
	return func(ctx context.Context, name, version string) error {
		_ = marketplace.ErrNoDownloadURL // ensure import is used
		reg := plugins.NewLocalRegistry(targetDir)
		if err := reg.Init(ctx); err != nil {
			return fmt.Errorf("init registry at %s: %w", targetDir, err)
		}
		return fmt.Errorf("item %q v%s: run 'siply marketplace install %s' first", name, version, name)
	}
}
