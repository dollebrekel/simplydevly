// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReviewsCmd_Registered(t *testing.T) {
	t.Parallel()
	cmd := NewMarketplaceCmd()

	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "reviews" {
			found = true
			assert.Equal(t, "reviews <name>", sub.Use)

			pageFlag := sub.Flags().Lookup("page")
			assert.NotNil(t, pageFlag, "expected --page flag")

			jsonFlag := sub.Flags().Lookup("json")
			assert.NotNil(t, jsonFlag, "expected --json flag")
			break
		}
	}
	assert.True(t, found, "expected 'reviews' subcommand")
}
