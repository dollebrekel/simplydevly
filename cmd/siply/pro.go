package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
)

func newProCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pro",
		Short: "Manage your Pro subscription",
	}

	cmd.AddCommand(newProActivateCmd())
	cmd.AddCommand(newProDeactivateCmd())

	return cmd
}

func newProActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate",
		Short: "Activate Pro subscription",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			_, err = validator.ActivatePro(ctx)
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			return nil
		},
	}
}

func newProDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate",
		Short: "Deactivate Pro subscription",
		RunE: func(cmd *cobra.Command, args []string) error {
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

			err = validator.DeactivatePro()
			if err != nil {
				fmt.Println(err.Error())
				return nil
			}
			return nil
		},
	}
}
