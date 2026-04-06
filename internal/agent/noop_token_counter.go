// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import "siply.dev/siply/internal/core"

// NoopTokenCounter implements core.TokenCounter by passing through
// provider-reported values. Real token estimation is deferred to later stories.
type NoopTokenCounter struct{}

// Count returns 0 — the agent relies on provider-reported usage instead.
func (n *NoopTokenCounter) Count(_ string, _ string) (int, error) {
	return 0, nil
}

// EstimateCost returns 0.0 — real cost estimation is deferred.
func (n *NoopTokenCounter) EstimateCost(_ core.TokenUsage, _ string) (float64, error) {
	return 0.0, nil
}
