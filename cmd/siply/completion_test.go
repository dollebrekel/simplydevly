// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "siply",
		Short: "test root command",
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run a task",
	})
	rootCmd.AddCommand(&cobra.Command{
		Use:   "plugins",
		Short: "Manage plugins",
	})
	return rootCmd
}

func TestCompletionBash(t *testing.T) {
	rootCmd := newTestRootCmd()
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "bash"})
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "bash", "output should contain bash completion markers")
}

func TestCompletionZsh(t *testing.T) {
	rootCmd := newTestRootCmd()
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "zsh"})
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "zsh", "output should contain zsh completion markers")
}

func TestCompletionFish(t *testing.T) {
	rootCmd := newTestRootCmd()
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"completion", "fish"})
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "fish", "output should contain fish completion markers")
}

func TestCompletionNoArgs_PrintsUsage(t *testing.T) {
	rootCmd := newTestRootCmd()
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"completion"})
	rootCmd.SilenceErrors = true

	err := rootCmd.Execute()
	// Parent command with no RunE prints usage (no error).
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "completion", "should print usage with completion info")
}
