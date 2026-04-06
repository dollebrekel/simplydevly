// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

// TaskCategory classifies a query for routing purposes.
type TaskCategory string

const (
	// CategoryPreprocess routes to a cheaper/local model for analysis tasks.
	CategoryPreprocess TaskCategory = "preprocess"
	// CategoryPrimary routes to the main model for final execution.
	CategoryPrimary TaskCategory = "primary"
)

// RoutingRule maps a task category to a provider and optional model override.
type RoutingRule struct {
	Category TaskCategory
	Provider string
	Model    string // optional; "" means use provider default
}

// RoutingConfig holds the complete routing configuration.
type RoutingConfig struct {
	Rules           []RoutingRule
	DefaultProvider string
	Enabled         bool
}
