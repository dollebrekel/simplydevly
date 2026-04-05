package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
)

// stdinReader reads a line from stdin. Overridable for testing.
// Returns the trimmed input and whether reading succeeded.
var stdinReader = defaultStdinReader

func defaultStdinReader() (string, bool) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), true
	}
	return "", false
}

func newLoginCmd() *cobra.Command {
	var refreshRepos bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in with GitHub or Google",
		Long:  "Authenticate with your GitHub or Google account to link your identity for marketplace access.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if refreshRepos {
				return executeRefreshRepos(cmd)
			}
			return executeLogin(cmd)
		},
	}

	cmd.Flags().BoolVar(&refreshRepos, "refresh-repos", false, "Re-run repo discovery without full re-auth")

	return cmd
}

func executeLogin(cmd *cobra.Command) error {
	provider, skipped, err := licensing.SelectProvider("Sign in to Simply Devly")
	if err != nil {
		return err
	}
	if skipped {
		fmt.Println("Skipped. You can sign in later with `siply login`.")
		return nil
	}

	configDir, err := defaultConfigDir()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
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

	fmt.Println()
	fmt.Println("Opening browser for authentication...")

	status, err := validator.Login(ctx, provider)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("✓ Signed in as %s (%s)\n", status.DisplayName, licensing.ProviderDisplayName(status.AuthProvider))

	// Auto-discover repos for GitHub users (AC #4-#6).
	if provider == core.AuthGitHub {
		if err := runRepoDiscovery(ctx, validator); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: repo discovery failed: %v\n", err)
		}
	}

	return nil
}

// executeRefreshRepos re-runs discovery without full re-auth (AC #7).
func executeRefreshRepos(cmd *cobra.Command) error {
	configDir, err := defaultConfigDir()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
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

	// Check if logged in.
	status := validator.Validate()
	if !status.LoggedIn {
		return fmt.Errorf("not logged in — run `siply login` first")
	}
	if status.AuthProvider != licensing.ProviderName(core.AuthGitHub) {
		fmt.Println("Repo discovery requires GitHub login. Skipping.")
		return nil
	}

	return runRepoDiscovery(ctx, validator)
}

// runRepoDiscovery discovers repos, prompts user, and sets up workspaces.
func runRepoDiscovery(ctx context.Context, validator core.LicenseValidator) error {
	repos, err := validator.DiscoverRepos(ctx)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No matching repos found on this machine.")
		return nil
	}

	// Count repos that need setup (no existing .siply/).
	var needSetup []core.DiscoveredRepo
	for _, r := range repos {
		if !r.HasSiplyConfig {
			needSetup = append(needSetup, r)
		}
	}

	if len(needSetup) == 0 {
		fmt.Printf("Found %d repos — all already have siply configured.\n", len(repos))
		return nil
	}

	// Prompt (AC #4).
	fmt.Printf("\nFound %d repos on this machine (%d need setup). Set up siply workspaces? [Y/n] ", len(repos), len(needSetup))
	answer, ok := stdinReader()
	if !ok {
		// Non-interactive (EOF/pipe) — don't auto-accept.
		fmt.Println("Non-interactive mode detected. Skipping workspace setup.")
		return nil
	}
	if answer != "" && strings.ToLower(answer) != "y" {
		fmt.Println("Skipped workspace setup.")
		return nil
	}

	// Create workspaces (AC #5, #6).
	for _, r := range needSetup {
		if err := licensing.SetupWorkspace(r.LocalPath, r.Language); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", r.GitHubFullName, err)
			continue
		}
		fmt.Printf("  ✓ %s (%s)\n", r.GitHubFullName, r.LocalPath)
	}

	return nil
}

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".siply"), nil
}
