// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
)

type statusView struct {
	LoggedIn       bool    `json:"logged_in"`
	Plan           string  `json:"plan"`
	AuthProvider   string  `json:"auth_provider,omitempty"`
	DisplayName    string  `json:"display_name,omitempty"`
	AccountEmail   string  `json:"account_email,omitempty"`
	GitHubUser     string  `json:"github_user,omitempty"`
	RepoAccess     bool    `json:"repo_access"`
	TokenExpiresAt *string `json:"token_expires_at,omitempty"`
}

func newStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show account info and current plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeStatus(cmd, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output status as JSON")
	return cmd
}

func executeStatus(cmd *cobra.Command, jsonOutput bool) error {
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

	if jsonOutput {
		view := statusView{
			LoggedIn:     status.LoggedIn,
			Plan:         "Free",
			AuthProvider: status.AuthProvider,
			DisplayName:  status.DisplayName,
			AccountEmail: status.AccountEmail,
			GitHubUser:   status.GitHubUser,
			RepoAccess:   status.RepoAccess,
		}
		if status.Tier == core.TierPro {
			view.Plan = "Pro"
		}
		if !status.TokenExpiresAt.IsZero() {
			s := status.TokenExpiresAt.Format("2006-01-02T15:04:05Z07:00")
			view.TokenExpiresAt = &s
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(view)
	}

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
	if !status.TokenExpiresAt.IsZero() {
		fmt.Printf("Token expires: %s\n", status.TokenExpiresAt.Format("2006-01-02 15:04 MST"))
	}
	if !status.LastChecked.IsZero() {
		fmt.Printf("Last verified: %s\n", status.LastChecked.Format("2006-01-02 15:04 MST"))
	}
	if status.RepoAccess {
		fmt.Println("Repos discovered: yes")
	}

	return nil
}
