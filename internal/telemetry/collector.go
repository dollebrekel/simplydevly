// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

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
	mu        sync.Mutex
	steps     []core.StepTelemetry
	stepSeq   atomic.Int64
	started   atomic.Bool
	outputDir string
	sessionID string
	maxSteps  int
}

// Option configures a TelemetryCollector.
type Option func(*collector)

// WithOutputDir sets the directory for JSONL output files.
func WithOutputDir(dir string) Option {
	return func(c *collector) { c.outputDir = dir }
}

// WithMaxSteps sets the ring buffer capacity (default 1000).
func WithMaxSteps(n int) Option {
	return func(c *collector) { c.maxSteps = n }
}

// defaultOutputDir returns the default telemetry output directory.
func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".siply", "telemetry")
	}
	return filepath.Join(home, ".siply", "telemetry")
}

// NewTelemetryCollector creates a TelemetryCollector that records per-step
// metrics in memory and flushes to persistent storage on demand.
func NewTelemetryCollector(opts ...Option) core.TelemetryCollector {
	c := &collector{
		outputDir: defaultOutputDir(),
		maxSteps:  1000,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *collector) Init(_ context.Context) error {
	return nil
}

func (c *collector) Start(_ context.Context) error {
	c.sessionID = fmt.Sprintf("%d", time.Now().UnixNano())
	c.started.Store(true)
	slog.Debug("telemetry collector started", "session_id", c.sessionID)
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
// When the ring buffer is full (maxSteps), the oldest step is evicted.
func (c *collector) RecordStep(step core.StepTelemetry) error {
	if !c.started.Load() {
		return fmt.Errorf("telemetry: collector not running")
	}
	if step.StepID == "" {
		step.StepID = fmt.Sprintf("step-%d", c.stepSeq.Add(1))
	}
	c.mu.Lock()
	if c.maxSteps > 0 && len(c.steps) >= c.maxSteps {
		copy(c.steps, c.steps[1:])
		c.steps[len(c.steps)-1] = step
		c.mu.Unlock()
		slog.Warn("telemetry: ring buffer full, evicting oldest step", "max", c.maxSteps)
	} else {
		c.steps = append(c.steps, step)
		c.mu.Unlock()
	}
	return nil
}

// Flush writes all accumulated step telemetry to JSONL files.
// Uses atomic write pattern: write to temp file, then rename.
func (c *collector) Flush(_ context.Context) error {
	c.mu.Lock()
	steps := c.steps
	c.steps = nil
	c.mu.Unlock()

	if len(steps) == 0 {
		return nil
	}

	if err := os.MkdirAll(c.outputDir, 0700); err != nil {
		return fmt.Errorf("telemetry: create output dir: %w", err)
	}

	tmpPath := filepath.Join(c.outputDir, c.sessionID+".jsonl.tmp")
	finalPath := filepath.Join(c.outputDir, c.sessionID+".jsonl")

	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("telemetry: create temp file: %w", err)
	}

	enc := json.NewEncoder(f)
	for _, step := range steps {
		if err := enc.Encode(step); err != nil {
			f.Close()          //nolint:errcheck
			os.Remove(tmpPath) //nolint:errcheck
			return fmt.Errorf("telemetry: encode step: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("telemetry: close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath) //nolint:errcheck
		return fmt.Errorf("telemetry: rename to final: %w", err)
	}

	slog.Debug("telemetry flushed to file", "path", finalPath, "steps", len(steps))
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
