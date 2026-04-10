// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"github.com/spf13/cobra"
)

// newCompletionCmd creates the `siply completion` command with bash, zsh, and
// fish subcommands. rootCmd is required to generate correct completion scripts.
func newCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for siply.

To load completions:

Bash:
  $ source <(siply completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ siply completion bash > /etc/bash_completion.d/siply
  # macOS:
  $ siply completion bash > $(brew --prefix)/etc/bash_completion.d/siply

Zsh:
  $ source <(siply completion zsh)

  # To load completions for each session, execute once:
  $ siply completion zsh > "${fpath[1]}/_siply"

Fish:
  $ siply completion fish | source

  # To load completions for each session, execute once:
  $ siply completion fish > ~/.config/fish/completions/siply.fish
`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate Bash completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return rootCmd.GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate Zsh completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return rootCmd.GenZshCompletion(cmd.OutOrStdout())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate Fish completion script",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return rootCmd.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	})

	return cmd
}
