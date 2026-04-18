// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateCmd_Registered(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()

	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "rate" {
			found = true
			assert.Equal(t, "rate <name> <score>", sub.Use)
			break
		}
	}
	assert.True(t, found, "expected 'rate' subcommand")
}

func TestRateCmd_InvalidScore(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"rate", "test-item", "7"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rating must be between 1 and 5")
}
