// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/providers"
)

const localFeatureMsg = "This feature requires cloud connectivity and is unavailable in local mode"

// isLocalMode returns true if the --local flag is set or SIPLY_LOCAL is truthy.
func isLocalMode(cmd *cobra.Command) bool {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return providers.IsLocalEnv()
	}
	return local || providers.IsLocalEnv()
}

// withLocalGuard wraps a command with a PersistentPreRunE that blocks
// execution in local mode with a clear message.
func withLocalGuard(cmd *cobra.Command) *cobra.Command {
	existing := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if isLocalMode(cmd) {
			return fmt.Errorf("%s", localFeatureMsg)
		}
		if existing != nil {
			return existing(cmd, args)
		}
		return nil
	}
	return cmd
}
