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
	StepID                   string    `json:"step_id"`
	Timestamp                time.Time `json:"timestamp"`
	Provider                 string    `json:"provider"`                            // "anthropic", "openai", "ollama"
	Model                    string    `json:"model"`                               // "claude-opus-4", "gpt-4o"
	TokensIn                 int       `json:"tokens_in"`
	TokensOut                int       `json:"tokens_out"`
	CacheReadInputTokens     int       `json:"cache_read_input_tokens,omitempty"`   // Tokens read from cache
	CacheCreationInputTokens int       `json:"cache_creation_input_tokens,omitempty"` // Tokens written to cache
	CostUSD                  float64   `json:"cost_usd"`
	LatencyMS                int64     `json:"latency_ms"`
	ToolCalls                []string  `json:"tool_calls,omitempty"` // ["file_read", "bash", "file_edit"]
	StepType                 string    `json:"step_type"`            // "query", "tool-execution", "context-prep"
	LocalSaved               int       `json:"local_saved"`          // Tokens saved by local processing (reported by K+C plugins)
}

// SessionTelemetry captures session-level aggregates.
type SessionTelemetry struct {
	SessionID      string  `json:"session_id"`
	TotalSteps     int     `json:"total_steps"`
	TotalTokensIn  int     `json:"total_tokens_in"`
	TotalTokensOut int     `json:"total_tokens_out"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	TotalLatencyMS int64   `json:"total_latency_ms"`
	LocalSavedPct  float64 `json:"local_saved_pct"` // "92% saved" — the marketing number
}
