// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package telemetry

import (
	"context"

	"siply.dev/siply/internal/core"
)

// noopCollector is a TelemetryCollector that discards all data.
// Used when telemetry is not opted-in.
type noopCollector struct{}

// NewNoopCollector returns a TelemetryCollector that silently discards all data.
func NewNoopCollector() core.TelemetryCollector {
	return &noopCollector{}
}

func (n *noopCollector) Init(_ context.Context) error            { return nil }
func (n *noopCollector) Start(_ context.Context) error           { return nil }
func (n *noopCollector) Stop(_ context.Context) error            { return nil }
func (n *noopCollector) Health() error                           { return nil }
func (n *noopCollector) RecordStep(_ core.StepTelemetry) error   { return nil }
func (n *noopCollector) Flush(_ context.Context) error           { return nil }
