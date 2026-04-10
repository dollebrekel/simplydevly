// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package completion

import (
	"context"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/core"
)

// CompletionFunc is the function signature for Cobra's ValidArgsFunction field.
type CompletionFunc func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

// PluginNameCompletionFunc returns a CompletionFunc that provides tab
// completion for installed plugin names. If registry is nil, the function
// returns empty completions without error.
func PluginNameCompletionFunc(registry core.PluginRegistry) CompletionFunc {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		if registry == nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		metas, err := registry.List(context.Background())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		names := make([]string, 0, len(metas))
		for _, m := range metas {
			names = append(names, m.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	}
}
