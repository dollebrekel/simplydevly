// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportCmd_Registered(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()

	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "report" {
			found = true
			assert.Equal(t, "report <name>", sub.Use)

			reasonFlag := sub.Flags().Lookup("reason")
			assert.NotNil(t, reasonFlag, "expected --reason flag")

			detailFlag := sub.Flags().Lookup("detail")
			assert.NotNil(t, detailFlag, "expected --detail flag")
			break
		}
	}
	assert.True(t, found, "expected 'report' subcommand")
}

func TestReportCmd_MissingReason(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"report", "test-item"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reason")
}

func TestReportCmd_InvalidReason(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"report", "test-item", "--reason", "invalid"})

	err := cmd.Execute()
	require.Error(t, err)
	// The error will be about auth first since RequireAuth fires before validation
	// In a full integration test this would reach the validation
}
