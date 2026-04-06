// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show account info and current plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeStatus(cmd)
		},
	}
}

func executeStatus(cmd *cobra.Command) error {
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

	status := validator.Validate()

	tierName := "Free"
	if status.Tier == core.TierPro {
		tierName = "Pro"
	}

	fmt.Printf("Plan: %s\n", tierName)

	if !status.LoggedIn {
		fmt.Println("Account: not logged in")
		fmt.Println("\nRun `siply login` to sign in.")
		return nil
	}

	fmt.Printf("Account: %s (%s)\n", status.DisplayName, status.AccountEmail)
	fmt.Printf("Provider: %s\n", licensing.ProviderDisplayName(status.AuthProvider))
	if status.GitHubUser != "" {
		fmt.Printf("GitHub: @%s\n", status.GitHubUser)
	}
	fmt.Printf("Instance: %s\n", status.InstanceID)

	return nil
}
