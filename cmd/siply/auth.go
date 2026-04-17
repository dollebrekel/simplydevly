// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import "github.com/spf13/cobra"

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Long:  "Sign in, check status, and manage your siply account.",
	}

	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newProCmd())

	return cmd
}

func newLoginAlias() *cobra.Command {
	cmd := newLoginCmd()
	cmd.Hidden = true
	return cmd
}

func newLogoutAlias() *cobra.Command {
	cmd := newLogoutCmd()
	cmd.Hidden = true
	return cmd
}

func newStatusAlias() *cobra.Command {
	cmd := newStatusCmd()
	cmd.Hidden = true
	return cmd
}

func newProAlias() *cobra.Command {
	cmd := newProCmd()
	cmd.Hidden = true
	return cmd
}
