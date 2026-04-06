// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "context"

// FeatureGate defines the Free/Pro boundary: registration + enforcement (UX boundary).
// The FeatureGate is a UX boundary, not a security boundary.
type FeatureGate interface {
	Lifecycle
	// Register a feature with its tier requirement.
	Register(feature Feature) error
	// Guard checks if a feature is available. Returns nil if allowed,
	// ErrFeatureGated if not.
	Guard(ctx context.Context, featureID string) error
	// GuardWithFallback returns display context for upgrade prompts (FR98).
	GuardWithFallback(ctx context.Context, featureID string) (GateResult, error)
	// List all registered features with their status.
	List() []FeatureStatus
}

// Feature describes a gated feature with its tier requirement.
type Feature struct {
	ID          string      // "context-distillation", "provider-arbitrage"
	Name        string      // Human-readable name
	Description string
	Tier        FeatureTier // Free, Pro
	PluginName  string      // Which plugin provides this
}

// FeatureTier indicates the subscription tier required for a feature.
type FeatureTier int

const (
	TierFree FeatureTier = iota
	TierPro
)

// FeatureStatus combines feature metadata with runtime availability.
type FeatureStatus struct {
	Feature
	Available bool   // true if license covers this tier
	Loaded    bool   // true if providing plugin is loaded
	Reason    string // "no license", "plugin not installed", ""
}

// GateResult provides context for upgrade prompts when a feature is gated.
type GateResult struct {
	Allowed     bool
	FeatureID   string
	Tier        FeatureTier
	FallbackMsg string // "Upgrade to Pro — siply pro activate"
	ShowUpgrade bool   // true = show contextual upgrade prompt (FR98)
}
