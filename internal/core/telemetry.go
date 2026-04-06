// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"context"
	"time"
)

// TelemetryCollector handles per-step metrics recording for cost tracking
// and simply-bench deep hooks export.
//
// Design (Sprint Change Proposal 2026-04-05):
//   - Agent loop calls RecordStep directly after each query/tool execution
//   - Flush writes accumulated JSONL to ~/.siply/telemetry/ at session end
//   - No EventBus auto-subscription (explicit > implicit)
//   - No FeatureGate gating (Pro boundary is at simply-bench adapter level)
type TelemetryCollector interface {
	Lifecycle
	// RecordStep captures metrics for a single agent step.
	// Called by the agent loop after each provider query and tool execution.
	RecordStep(step StepTelemetry) error
	// Flush writes all accumulated step telemetry to persistent storage.
	// Called at session end. Context allows cancellation for long writes.
	Flush(ctx context.Context) error
}

// StepTelemetry captures metrics for a single agent step.
type StepTelemetry struct {
	StepID     string
	Timestamp  time.Time
	Provider   string // "anthropic", "openai", "ollama"
	Model      string // "claude-opus-4", "gpt-4o"
	TokensIn   int
	TokensOut  int
	CostUSD    float64
	LatencyMS  int64
	ToolCalls  []string // ["file_read", "bash", "file_edit"]
	StepType   string   // "query", "tool-execution", "context-prep"
	LocalSaved int      // Tokens saved by local processing (reported by K+C plugins)
}

// SessionTelemetry captures session-level aggregates.
type SessionTelemetry struct {
	SessionID      string
	TotalSteps     int
	TotalTokensIn  int
	TotalTokensOut int
	TotalCostUSD   float64
	TotalLatencyMS int64
	LocalSavedPct  float64 // "92% saved" — the marketing number
}
