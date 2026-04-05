package core

import "time"

// TelemetryCollector handles per-step metrics recording, simply-bench deep hooks export.
type TelemetryCollector interface {
	Lifecycle
	// RecordStep captures metrics for a single agent step.
	RecordStep(step StepTelemetry)
	// RecordSession captures session-level aggregates.
	RecordSession(session SessionTelemetry)
	// Export returns all telemetry for simply-bench consumption (Pro-gated).
	Export() []StepTelemetry
	// Subscribe allows real-time consumers (simply-bench deep hooks, Pro-gated).
	Subscribe() <-chan StepTelemetry
}

// StepTelemetry captures metrics for a single agent step.
type StepTelemetry struct {
	StepID    string
	Timestamp time.Time
	Provider  string   // "anthropic", "openai", "ollama"
	Model     string   // "claude-opus-4", "gpt-4o"
	TokensIn  int
	TokensOut int
	CostUSD   float64
	LatencyMS int64
	ToolCalls []string // ["file_read", "bash", "file_edit"]
	StepType  string   // "query", "tool-execution", "context-prep"
	LocalSaved int     // Tokens saved by local processing (reported by K+C plugins)
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
