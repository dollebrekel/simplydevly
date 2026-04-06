// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestNoopTokenCounter_Count(t *testing.T) {
	tc := &NoopTokenCounter{}
	count, err := tc.Count("hello world", "claude-3-sonnet")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestNoopTokenCounter_EstimateCost(t *testing.T) {
	tc := &NoopTokenCounter{}
	cost, err := tc.EstimateCost(core.TokenUsage{InputTokens: 100, OutputTokens: 50}, "claude-3-sonnet")
	require.NoError(t, err)
	assert.Equal(t, 0.0, cost)
}

func TestNoopStatusCollector_Lifecycle(t *testing.T) {
	sc := &NoopStatusCollector{}
	ctx := context.Background()

	require.NoError(t, sc.Init(ctx))
	require.NoError(t, sc.Start(ctx))
	require.NoError(t, sc.Health())
	require.NoError(t, sc.Stop(ctx))
}

func TestNoopStatusCollector_PublishAndSnapshot(t *testing.T) {
	sc := &NoopStatusCollector{}

	sc.Publish(core.StatusUpdate{
		Source:    "agent",
		Metrics:   map[string]any{"tokens_in": 100},
		Timestamp: time.Now(),
	})
	sc.Publish(core.StatusUpdate{
		Source:    "agent",
		Metrics:   map[string]any{"tokens_in": 200},
		Timestamp: time.Now(),
	})

	snap := sc.Snapshot()
	assert.Len(t, snap, 1)
	assert.Equal(t, 200, snap["agent"].Metrics["tokens_in"])
}

func TestNoopStatusCollector_Subscribe(t *testing.T) {
	sc := &NoopStatusCollector{}
	ch, unsub := sc.Subscribe()
	assert.NotNil(t, ch)

	// Unsubscribe should be safe to call multiple times.
	unsub()
	unsub()
}
