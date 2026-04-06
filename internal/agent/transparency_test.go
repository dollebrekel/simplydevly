// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransparencyLogger_LogToolExecution(t *testing.T) {
	events := &mockEventBus{}
	logger := NewTransparencyLogger(events)

	logger.LogToolExecution(context.Background(), &ToolExecutedEvent{
		ToolName: "file_read",
		ToolID:   "call-1",
		Output:   "contents",
		IsError:  false,
	})

	toolEvents := events.eventsOfType("tool.executed")
	require.Len(t, toolEvents, 1)

	ev := toolEvents[0].(*ToolExecutedEvent)
	assert.Equal(t, "file_read", ev.ToolName)
	assert.Equal(t, "call-1", ev.ToolID)
	assert.False(t, ev.IsError)
	assert.False(t, ev.Timestamp().IsZero())
}

func TestTransparencyLogger_LogQueryStart(t *testing.T) {
	events := &mockEventBus{}
	logger := NewTransparencyLogger(events)

	logger.LogQueryStart(context.Background(), 5)

	queryEvents := events.eventsOfType("agent.query_started")
	require.Len(t, queryEvents, 1)

	ev := queryEvents[0].(*QueryStartedEvent)
	assert.Equal(t, 5, ev.MessageCount)
}

func TestTransparencyLogger_LogQueryComplete(t *testing.T) {
	events := &mockEventBus{}
	logger := NewTransparencyLogger(events)

	logger.LogQueryComplete(context.Background(), 100, 50, 0.005)

	queryEvents := events.eventsOfType("agent.query_completed")
	require.Len(t, queryEvents, 1)

	ev := queryEvents[0].(*QueryCompletedEvent)
	assert.Equal(t, 100, ev.TokensIn)
	assert.Equal(t, 50, ev.TokensOut)
	assert.Equal(t, 0.005, ev.Cost)
}
