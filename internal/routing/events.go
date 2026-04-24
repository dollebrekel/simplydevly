// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import "time"

// RoutingDecisionEvent is published when a routing decision is made.
type RoutingDecisionEvent struct {
	SelectedProvider  string
	SelectedModel     string
	Category          string
	Reason            string
	ProviderCount     int
	EstimatedTokens   int
	ProviderCost      float64
	DecisionLatencyMS int64
	ts                time.Time
}

// Type returns the event type identifier.
func (e *RoutingDecisionEvent) Type() string { return "routing.decision" }

// Timestamp returns when the event occurred.
func (e *RoutingDecisionEvent) Timestamp() time.Time {
	return e.ts
}

// NewRoutingDecisionEvent creates a RoutingDecisionEvent with the current time.
func NewRoutingDecisionEvent(provider, model, category, reason string) *RoutingDecisionEvent {
	return &RoutingDecisionEvent{
		SelectedProvider: provider,
		SelectedModel:    model,
		Category:         category,
		Reason:           reason,
		ts:               time.Now(),
	}
}
