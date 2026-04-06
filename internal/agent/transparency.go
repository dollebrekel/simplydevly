// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"siply.dev/siply/internal/core"
)

// ToolExecutedEvent records a tool execution by the agent.
type ToolExecutedEvent struct {
	ToolName string
	ToolID   string
	Input    json.RawMessage
	Output   string
	IsError  bool
	Duration time.Duration
	ts       time.Time
}

func (e *ToolExecutedEvent) Type() string         { return "tool.executed" }
func (e *ToolExecutedEvent) Timestamp() time.Time { return e.ts }

// QueryStartedEvent is published when the agent begins a provider query.
type QueryStartedEvent struct {
	MessageCount int
	ts           time.Time
}

func (e *QueryStartedEvent) Type() string         { return "agent.query_started" }
func (e *QueryStartedEvent) Timestamp() time.Time { return e.ts }

// QueryCompletedEvent is published when a provider query finishes.
type QueryCompletedEvent struct {
	TokensIn  int
	TokensOut int
	Cost      float64
	ts        time.Time
}

func (e *QueryCompletedEvent) Type() string         { return "agent.query_completed" }
func (e *QueryCompletedEvent) Timestamp() time.Time { return e.ts }

// TransparencyLogger publishes agent actions to the EventBus and slog.
type TransparencyLogger struct {
	events core.EventBus
}

// NewTransparencyLogger creates a TransparencyLogger.
func NewTransparencyLogger(events core.EventBus) *TransparencyLogger {
	return &TransparencyLogger{events: events}
}

// LogToolExecution publishes a ToolExecutedEvent and logs it.
func (t *TransparencyLogger) LogToolExecution(ctx context.Context, ev *ToolExecutedEvent) {
	if t == nil || t.events == nil || ev == nil {
		return
	}
	ev.ts = time.Now()
	if err := t.events.Publish(ctx, ev); err != nil {
		slog.Warn("failed to publish tool execution event", "error", err, "tool_id", ev.ToolID)
	}
	slog.Info("tool executed",
		"tool", ev.ToolName,
		"tool_id", ev.ToolID,
		"duration", ev.Duration,
		"error", ev.IsError,
	)
}

// LogQueryStart publishes a QueryStartedEvent and logs it.
func (t *TransparencyLogger) LogQueryStart(ctx context.Context, messageCount int) {
	if t == nil || t.events == nil {
		return
	}
	ev := &QueryStartedEvent{
		MessageCount: messageCount,
		ts:           time.Now(),
	}
	if err := t.events.Publish(ctx, ev); err != nil {
		slog.Warn("failed to publish query started event", "error", err)
	}
	slog.Info("query started", "message_count", messageCount)
}

// LogQueryComplete publishes a QueryCompletedEvent and logs it.
func (t *TransparencyLogger) LogQueryComplete(ctx context.Context, tokensIn, tokensOut int, cost float64) {
	if t == nil || t.events == nil {
		return
	}
	ev := &QueryCompletedEvent{
		TokensIn:  tokensIn,
		TokensOut: tokensOut,
		Cost:      cost,
		ts:        time.Now(),
	}
	if err := t.events.Publish(ctx, ev); err != nil {
		slog.Warn("failed to publish query completed event", "error", err)
	}
	slog.Info("query completed",
		"tokens_in", tokensIn,
		"tokens_out", tokensOut,
		"cost", cost,
	)
}
