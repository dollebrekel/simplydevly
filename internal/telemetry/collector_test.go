// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package telemetry

import (
	"context"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
)

func startCollector(t *testing.T) *collector {
	t.Helper()
	tc := NewTelemetryCollector().(*collector)
	ctx := context.Background()
	if err := tc.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := tc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { tc.Stop(ctx) })
	return tc
}

func TestRecordStep(t *testing.T) {
	tc := startCollector(t)

	step := core.StepTelemetry{
		StepType:  "query",
		TokensIn:  100,
		TokensOut: 50,
		CostUSD:   0.003,
	}

	if err := tc.RecordStep(step); err != nil {
		t.Fatalf("RecordStep: %v", err)
	}

	steps := tc.Steps()
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].TokensIn != 100 {
		t.Errorf("TokensIn = %d, want 100", steps[0].TokensIn)
	}
	if steps[0].LocalSaved != 0 {
		t.Errorf("LocalSaved = %d, want 0 (AC#8)", steps[0].LocalSaved)
	}
}

func TestRecordStepAutoID(t *testing.T) {
	tc := startCollector(t)

	// Record two steps without explicit StepID — should get unique auto-IDs.
	if err := tc.RecordStep(core.StepTelemetry{StepType: "query"}); err != nil {
		t.Fatalf("RecordStep 1: %v", err)
	}
	if err := tc.RecordStep(core.StepTelemetry{StepType: "tool-execution"}); err != nil {
		t.Fatalf("RecordStep 2: %v", err)
	}

	steps := tc.Steps()
	if len(steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(steps))
	}
	if steps[0].StepID == steps[1].StepID {
		t.Errorf("duplicate StepIDs: %q", steps[0].StepID)
	}
	if steps[0].StepID != "step-1" {
		t.Errorf("StepID = %q, want step-1", steps[0].StepID)
	}
}

func TestRecordStepPreservesExplicitID(t *testing.T) {
	tc := startCollector(t)

	if err := tc.RecordStep(core.StepTelemetry{StepID: "my-id", StepType: "query"}); err != nil {
		t.Fatalf("RecordStep: %v", err)
	}

	steps := tc.Steps()
	if steps[0].StepID != "my-id" {
		t.Errorf("StepID = %q, want my-id", steps[0].StepID)
	}
}

func TestRecordStepNotStarted(t *testing.T) {
	tc := NewTelemetryCollector()

	err := tc.RecordStep(core.StepTelemetry{StepType: "query"})
	if err == nil {
		t.Fatal("RecordStep should return error when collector not started")
	}
}

func TestFlush(t *testing.T) {
	tc := startCollector(t)

	// Record some steps.
	for i := 0; i < 5; i++ {
		if err := tc.RecordStep(core.StepTelemetry{
			StepType:  "query",
			TokensIn:  100,
			Timestamp: time.Now(),
		}); err != nil {
			t.Fatalf("RecordStep %d: %v", i, err)
		}
	}

	steps := tc.Steps()
	if len(steps) != 5 {
		t.Fatalf("got %d steps before flush, want 5", len(steps))
	}

	// Flush clears the buffer.
	if err := tc.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	steps = tc.Steps()
	if len(steps) != 0 {
		t.Fatalf("got %d steps after flush, want 0", len(steps))
	}
}

func TestLocalSavedAlwaysZero(t *testing.T) {
	tc := startCollector(t)

	// AC#8: LocalSaved should be 0 for all steps (no Pro plugins saving tokens yet).
	types := []string{"query", "tool-execution", "context-prep"}
	for _, st := range types {
		if err := tc.RecordStep(core.StepTelemetry{StepType: st}); err != nil {
			t.Fatalf("RecordStep(%s): %v", st, err)
		}
	}

	for i, s := range tc.Steps() {
		if s.LocalSaved != 0 {
			t.Errorf("step[%d].LocalSaved = %d, want 0", i, s.LocalSaved)
		}
	}
}

func TestHealthReflectsState(t *testing.T) {
	tc := NewTelemetryCollector()

	// Not started — Health should return error.
	if err := tc.Health(); err == nil {
		t.Error("Health() should return error before Start()")
	}

	ctx := context.Background()
	tc.Init(ctx)
	tc.Start(ctx)

	// Started — Health should pass.
	if err := tc.Health(); err != nil {
		t.Errorf("Health() after Start: %v", err)
	}

	tc.Stop(ctx)

	// Stopped — Health should return error.
	if err := tc.Health(); err == nil {
		t.Error("Health() should return error after Stop()")
	}
}

func TestLifecycle(t *testing.T) {
	tc := NewTelemetryCollector()
	ctx := context.Background()

	if err := tc.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := tc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := tc.Health(); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := tc.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
