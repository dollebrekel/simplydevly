// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/providers"
)

const offlineFeatureMsg = "This feature requires an internet connection and is unavailable in offline mode"

// isOfflineMode returns true if the --offline flag is set or SIPLY_OFFLINE is truthy.
func isOfflineMode(cmd *cobra.Command) bool {
	offline, err := cmd.Flags().GetBool("offline")
	if err != nil {
		return providers.IsOfflineEnv()
	}
	return offline || providers.IsOfflineEnv()
}

// withOfflineGuard wraps a command with a PersistentPreRunE that blocks
// execution in offline mode with a clear message.
func withOfflineGuard(cmd *cobra.Command) *cobra.Command {
	existing := cmd.PersistentPreRunE
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if isOfflineMode(cmd) {
			return fmt.Errorf("%s", offlineFeatureMsg)
		}
		if existing != nil {
			return existing(cmd, args)
		}
		return nil
	}
	return cmd
}
