// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"siply.dev/siply/internal/core"
)

// collector implements core.TelemetryCollector with in-memory step storage
// and Flush-based export to persistent storage.
//
// Design (Sprint Change Proposal 2026-04-05):
//   - Agent loop calls RecordStep directly (no EventBus auto-subscription)
//   - Flush writes accumulated JSONL at session end
//   - No FeatureGate gating (Pro boundary is at simply-bench adapter level)
type collector struct {
	mu      sync.Mutex
	steps   []core.StepTelemetry
	stepSeq atomic.Int64
	started atomic.Bool
}

// NewTelemetryCollector creates a TelemetryCollector that records per-step
// metrics in memory and flushes to persistent storage on demand.
func NewTelemetryCollector() core.TelemetryCollector {
	return &collector{}
}

func (c *collector) Init(_ context.Context) error {
	return nil
}

func (c *collector) Start(_ context.Context) error {
	c.started.Store(true)
	slog.Debug("telemetry collector started")
	return nil
}

func (c *collector) Stop(_ context.Context) error {
	c.started.Store(false)
	return nil
}

func (c *collector) Health() error {
	if !c.started.Load() {
		return fmt.Errorf("telemetry: collector not running")
	}
	return nil
}

// RecordStep appends a step to the in-memory slice.
// Uses an atomic counter for StepID uniqueness (no time.Now collisions).
func (c *collector) RecordStep(step core.StepTelemetry) error {
	if !c.started.Load() {
		return fmt.Errorf("telemetry: collector not running")
	}
	if step.StepID == "" {
		step.StepID = fmt.Sprintf("step-%d", c.stepSeq.Add(1))
	}
	c.mu.Lock()
	c.steps = append(c.steps, step)
	c.mu.Unlock()
	return nil
}

// Flush writes all accumulated step telemetry to persistent storage.
// Currently a stub that clears the in-memory buffer — JSONL file writing
// will be implemented when simply-bench deep hooks ship (Epic 7).
func (c *collector) Flush(_ context.Context) error {
	c.mu.Lock()
	count := len(c.steps)
	c.steps = nil
	c.mu.Unlock()
	slog.Debug("telemetry flushed", "steps", count)
	return nil
}

// Steps returns a copy of all recorded steps. Exported for testing only.
func (c *collector) Steps() []core.StepTelemetry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]core.StepTelemetry, len(c.steps))
	copy(out, c.steps)
	return out
}
