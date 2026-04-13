// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStepTelemetry_JSONTags(t *testing.T) {
	ts := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	step := StepTelemetry{
		StepID:     "step-1",
		Timestamp:  ts,
		Provider:   "anthropic",
		Model:      "claude-opus-4",
		TokensIn:   1000,
		TokensOut:  500,
		CostUSD:    0.015,
		LatencyMS:  234,
		ToolCalls:  []string{"file_read", "bash"},
		StepType:   "query",
		LocalSaved: 0,
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	expectedKeys := []string{
		"step_id", "timestamp", "provider", "model",
		"tokens_in", "tokens_out", "cost_usd", "latency_ms",
		"tool_calls", "step_type", "local_saved",
	}

	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// Verify specific values roundtrip correctly.
	if m["step_id"] != "step-1" {
		t.Errorf("step_id = %v, want step-1", m["step_id"])
	}
	if m["provider"] != "anthropic" {
		t.Errorf("provider = %v, want anthropic", m["provider"])
	}
	if m["step_type"] != "query" {
		t.Errorf("step_type = %v, want query", m["step_type"])
	}

	// Verify roundtrip.
	var roundtrip StepTelemetry
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("Unmarshal roundtrip: %v", err)
	}
	if roundtrip.StepID != step.StepID {
		t.Errorf("roundtrip StepID = %q, want %q", roundtrip.StepID, step.StepID)
	}
	if roundtrip.TokensIn != step.TokensIn {
		t.Errorf("roundtrip TokensIn = %d, want %d", roundtrip.TokensIn, step.TokensIn)
	}
	if len(roundtrip.ToolCalls) != 2 {
		t.Errorf("roundtrip ToolCalls len = %d, want 2", len(roundtrip.ToolCalls))
	}
}

func TestStepTelemetry_ToolCallsOmitEmpty(t *testing.T) {
	step := StepTelemetry{
		StepID:   "step-2",
		StepType: "query",
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// tool_calls should be omitted when nil.
	if _, ok := m["tool_calls"]; ok {
		t.Error("tool_calls should be omitted when nil (omitempty)")
	}
}
