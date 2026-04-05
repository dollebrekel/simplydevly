package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
)

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in with GitHub or Google",
		Long:  "Authenticate with your GitHub or Google account to link your identity for marketplace access.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLogin(cmd)
		},
	}
	return cmd
}

func executeLogin(cmd *cobra.Command) error {
	fmt.Println("Sign in to Simply Devly")
	fmt.Println()
	fmt.Println("  [1] GitHub (recommended)")
	fmt.Println("  [2] Google")
	fmt.Println("  [s] Skip")
	fmt.Println()
	fmt.Print("Choose: ")

	var choice string
	if _, err := fmt.Scanln(&choice); err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	var provider core.AuthProvider
	switch choice {
	case "1":
		provider = core.AuthGitHub
	case "2":
		provider = core.AuthGoogle
	case "s", "S":
		fmt.Println("Skipped. You can sign in later with `siply login`.")
		return nil
	default:
		return fmt.Errorf("invalid choice: %q", choice)
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
	return nil
}

func defaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".siply"), nil
}
