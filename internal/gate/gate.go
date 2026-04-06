// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package gate

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"siply.dev/siply/internal/core"
)

// featureGate implements core.FeatureGate as a stub that always allows access.
// When LicenseValidator is implemented (PB.6), Guard will check license tier.
type featureGate struct {
	mu       sync.RWMutex
	features map[string]core.Feature
	license  core.LicenseValidator
}

// NewFeatureGate creates a FeatureGate stub. The license parameter can be nil
// in stub mode; it will be used when real license enforcement is added.
func NewFeatureGate(license core.LicenseValidator) core.FeatureGate {
	return &featureGate{
		features: make(map[string]core.Feature),
		license:  license,
	}
}

// Init is a no-op for the stub feature gate.
func (g *featureGate) Init(_ context.Context) error { return nil }

// Start is a no-op for the stub feature gate.
func (g *featureGate) Start(_ context.Context) error { return nil }

// Stop is a no-op for the stub feature gate.
func (g *featureGate) Stop(_ context.Context) error { return nil }

// Health returns nil — the feature gate is always healthy.
func (g *featureGate) Health() error { return nil }

// Register stores a feature in the internal map.
func (g *featureGate) Register(feature core.Feature) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if feature.ID == "" {
		return fmt.Errorf("gate: feature ID must not be empty")
	}

	if _, exists := g.features[feature.ID]; exists {
		return fmt.Errorf("gate: feature %q already registered", feature.ID)
	}

	g.features[feature.ID] = feature
	slog.Debug("feature registered", "id", feature.ID, "tier", feature.Tier)
	return nil
}

// Guard always returns nil in stub mode — all features are allowed.
func (g *featureGate) Guard(_ context.Context, _ string) error {
	return nil
}

// GuardWithFallback always returns GateResult{Allowed: true} in stub mode.
func (g *featureGate) GuardWithFallback(_ context.Context, featureID string) (core.GateResult, error) {
	return core.GateResult{
		Allowed:   true,
		FeatureID: featureID,
	}, nil
}

// List returns all registered features with Available: true (stub mode).
func (g *featureGate) List() []core.FeatureStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()

	statuses := make([]core.FeatureStatus, 0, len(g.features))
	for _, f := range g.features {
		statuses = append(statuses, core.FeatureStatus{
			Feature:   f,
			Available: true,
			Loaded:    true,
		})
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].ID < statuses[j].ID
	})
	return statuses
}
