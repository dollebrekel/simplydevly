// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewCmd_Registered(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()

	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "review" {
			found = true
			assert.Equal(t, "review <name>", sub.Use)

			msgFlag := sub.Flags().Lookup("message")
			assert.NotNil(t, msgFlag, "expected --message flag")

			ratingFlag := sub.Flags().Lookup("rating")
			assert.NotNil(t, ratingFlag, "expected --rating flag")
			break
		}
	}
	assert.True(t, found, "expected 'review' subcommand")
}

func TestReviewCmd_MissingMessage(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"review", "test-item", "--rating", "4"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message")
}
