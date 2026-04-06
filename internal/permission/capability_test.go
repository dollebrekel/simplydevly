// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package permission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

func TestEvaluator_EvaluateCapabilities_Stub(t *testing.T) {
	eval, err := NewEvaluator(DefaultConfig())
	require.NoError(t, err)

	verdict, err := eval.EvaluateCapabilities(context.Background(), core.PluginMeta{
		Name:    "test-plugin",
		Version: "1.0.0",
		Tier:    1,
	})
	assert.NoError(t, err)
	assert.Equal(t, core.CapabilityVerdict{}, verdict)
}
