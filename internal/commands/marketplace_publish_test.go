// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishCmd_AuthGuardFiresFirst(t *testing.T) {
	dir := t.TempDir()

	cmd := NewMarketplaceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"publish", "--dry-run", dir})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Authentication required")
}

func TestPublishCmd_Registered(t *testing.T) {
	cmd := NewMarketplaceCmd()

	// Verify publish subcommand is registered.
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "publish" {
			found = true
			assert.Equal(t, "publish [directory]", sub.Use)
			assert.False(t, sub.HasSubCommands())

			dryRun := sub.Flags().Lookup("dry-run")
			assert.NotNil(t, dryRun, "expected --dry-run flag")
			break
		}
	}
	assert.True(t, found, "expected 'publish' subcommand")
}

func TestPublishCmd_HelpText(t *testing.T) {
	cmd := NewMarketplaceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"publish", "--help"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "Package and publish")
	assert.Contains(t, output, "--dry-run")
}
