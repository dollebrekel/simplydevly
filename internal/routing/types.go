// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

// TaskCategory classifies a query for routing purposes.
type TaskCategory string

const (
	CategoryPreprocess TaskCategory = "preprocess"
	CategoryPrimary    TaskCategory = "primary"
	CategoryReview     TaskCategory = "review"
	CategoryChat       TaskCategory = "chat"
	CategoryVision     TaskCategory = "vision"
)

// RequiredCapabilities describes what a query needs from its provider.
type RequiredCapabilities struct {
	Tools      bool
	Vision     bool
	MinContext int
}

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
