// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package telemetry

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
)

func startCollector(t *testing.T, opts ...Option) *collector {
	t.Helper()
	tc := NewTelemetryCollector(opts...).(*collector)
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

func TestFlush_WritesValidJSONL(t *testing.T) {
	dir := t.TempDir()
	tc := startCollector(t, WithOutputDir(dir))

	ts := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if err := tc.RecordStep(core.StepTelemetry{
			StepType:  "query",
			Provider:  "anthropic",
			Model:     "claude-opus-4",
			TokensIn:  100 + i,
			TokensOut: 50,
			CostUSD:   0.003,
			LatencyMS: 200,
			Timestamp: ts,
		}); err != nil {
			t.Fatalf("RecordStep %d: %v", i, err)
		}
	}

	if err := tc.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Buffer should be empty after Flush.
	steps := tc.Steps()
	if len(steps) != 0 {
		t.Fatalf("got %d steps after Flush, want 0", len(steps))
	}

	// Find the JSONL file.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 JSONL file, got %d", len(matches))
	}

	// Verify each line is valid JSON that unmarshals to StepTelemetry.
	f, err := os.Open(matches[0])
	if err != nil {
		t.Fatalf("open JSONL: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var count int
	for scanner.Scan() {
		var step core.StepTelemetry
		if err := json.Unmarshal(scanner.Bytes(), &step); err != nil {
			t.Fatalf("line %d: unmarshal: %v", count, err)
		}
		if step.Provider != "anthropic" {
			t.Errorf("line %d: provider = %q, want anthropic", count, step.Provider)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	if count != 3 {
		t.Errorf("got %d lines, want 3", count)
	}
}

func TestFlush_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "telemetry")
	tc := startCollector(t, WithOutputDir(dir))

	if err := tc.RecordStep(core.StepTelemetry{StepType: "query"}); err != nil {
		t.Fatalf("RecordStep: %v", err)
	}

	if err := tc.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
	// Check permissions (unix).
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("directory perm = %o, want 0700", perm)
	}
}

func TestFlush_NoTempFileOnError(t *testing.T) {
	// Use a read-only directory to trigger write error.
	dir := t.TempDir()
	tc := startCollector(t, WithOutputDir(dir))

	if err := tc.RecordStep(core.StepTelemetry{StepType: "query"}); err != nil {
		t.Fatalf("RecordStep: %v", err)
	}

	// Make directory read-only to cause file creation error.
	os.Chmod(dir, 0500)
	t.Cleanup(func() { os.Chmod(dir, 0700) })

	err := tc.Flush(context.Background())
	if err == nil {
		t.Fatal("expected Flush error on read-only dir")
	}

	// No temp files should remain.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if len(matches) != 0 {
		t.Errorf("found %d temp files, want 0", len(matches))
	}
}

func TestFlush_EmptyBuffer(t *testing.T) {
	dir := t.TempDir()
	tc := startCollector(t, WithOutputDir(dir))

	// Flushing empty buffer should be a no-op.
	if err := tc.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(matches) != 0 {
		t.Errorf("expected 0 JSONL files for empty Flush, got %d", len(matches))
	}
}

func TestRingBuffer_EvictsOldest(t *testing.T) {
	tc := startCollector(t, WithMaxSteps(3))

	for i := 1; i <= 5; i++ {
		if err := tc.RecordStep(core.StepTelemetry{
			StepID:   fmt.Sprintf("s-%d", i),
			StepType: "query",
		}); err != nil {
			t.Fatalf("RecordStep %d: %v", i, err)
		}
	}

	steps := tc.Steps()
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3 (capped at maxSteps)", len(steps))
	}

	// Oldest steps (s-1, s-2) should be evicted; s-3, s-4, s-5 remain.
	if steps[0].StepID != "s-3" {
		t.Errorf("steps[0].StepID = %q, want s-3", steps[0].StepID)
	}
	if steps[1].StepID != "s-4" {
		t.Errorf("steps[1].StepID = %q, want s-4", steps[1].StepID)
	}
	if steps[2].StepID != "s-5" {
		t.Errorf("steps[2].StepID = %q, want s-5", steps[2].StepID)
	}
}

func TestRingBuffer_FlushWritesRetainedOnly(t *testing.T) {
	dir := t.TempDir()
	tc := startCollector(t, WithOutputDir(dir), WithMaxSteps(2))

	for i := 1; i <= 5; i++ {
		if err := tc.RecordStep(core.StepTelemetry{
			StepID:   fmt.Sprintf("s-%d", i),
			StepType: "query",
		}); err != nil {
			t.Fatalf("RecordStep %d: %v", i, err)
		}
	}

	if err := tc.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 JSONL file, got %d", len(matches))
	}

	f, _ := os.Open(matches[0])
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var count int
	for scanner.Scan() {
		count++
	}
	if count != 2 {
		t.Errorf("got %d lines in JSONL, want 2 (retained steps only)", count)
	}
}

func TestStart_SetsSessionID(t *testing.T) {
	tc := NewTelemetryCollector().(*collector)
	ctx := context.Background()
	tc.Init(ctx)
	tc.Start(ctx)
	defer tc.Stop(ctx)

	if tc.sessionID == "" {
		t.Error("sessionID should be set after Start()")
	}
}
