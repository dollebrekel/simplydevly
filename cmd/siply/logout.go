package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
)

func newLogoutCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Sign out and remove account credentials",
		Long:  "Remove your account credentials from this machine.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLogout(cmd)
		},
	}
	return cmd
}

func executeLogout(cmd *cobra.Command) error {
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

	if err := validator.Logout(); err != nil {
		return err
	}

	fmt.Println("✓ Signed out. Account credentials removed.")
	return nil
}
