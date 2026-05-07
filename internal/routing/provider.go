// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"siply.dev/siply/internal/core"
)

// RoutingProvider implements core.Provider by delegating queries to different
// providers based on a routing policy. If only one provider is configured or
// the policy is nil, it bypasses routing and delegates directly.
type RoutingProvider struct {
	providers       map[string]core.Provider
	policy          RoutingPolicy
	defaultProvider string
	eventBus        core.EventBus
	gate            core.FeatureGate
	local         bool
	pricing         map[string]core.ProviderPricing
}

// RoutingProviderConfig holds construction parameters for RoutingProvider.
type RoutingProviderConfig struct {
	Providers       map[string]core.Provider
	Policy          RoutingPolicy
	DefaultProvider string
	EventBus        core.EventBus
	Gate            core.FeatureGate
	Local         bool
	Pricing         map[string]core.ProviderPricing
}

// NewRoutingProvider creates a RoutingProvider from the given config.
func NewRoutingProvider(cfg RoutingProviderConfig) *RoutingProvider {
	return &RoutingProvider{
		providers:       cfg.Providers,
		policy:          cfg.Policy,
		defaultProvider: cfg.DefaultProvider,
		eventBus:        cfg.EventBus,
		gate:            cfg.Gate,
		local:         cfg.Local,
		pricing:         cfg.Pricing,
	}
}

// shouldBypass returns true if routing should be skipped.
func (r *RoutingProvider) shouldBypass() bool {
	return len(r.providers) <= 1 || r.policy == nil
}

// Init initializes all underlying providers. On partial failure, stops
// already-initialized providers before returning the error.
func (r *RoutingProvider) Init(ctx context.Context) error {
	var initialized []core.Provider
	for name, p := range r.providers {
		if err := p.Init(ctx); err != nil {
			// Rollback: stop already-initialized providers.
			for _, ip := range initialized {
				_ = ip.Stop(ctx)
			}
			return fmt.Errorf("routing: init provider %q: %w", name, err)
		}
		initialized = append(initialized, p)
	}
	return nil
}

// Start starts all underlying providers. On partial failure, stops
// already-started providers before returning the error.
func (r *RoutingProvider) Start(ctx context.Context) error {
	var started []core.Provider
	for name, p := range r.providers {
		if err := p.Start(ctx); err != nil {
			// Rollback: stop already-started providers.
			for _, sp := range started {
				_ = sp.Stop(ctx)
			}
			return fmt.Errorf("routing: start provider %q: %w", name, err)
		}
		started = append(started, p)
	}
	return nil
}

// Stop stops all underlying providers. Attempts to stop every provider
// even if some fail, returning all errors joined.
func (r *RoutingProvider) Stop(ctx context.Context) error {
	var errs []error
	for name, p := range r.providers {
		if err := p.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("routing: stop provider %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Health returns the first error from any provider.
func (r *RoutingProvider) Health() error {
	for name, p := range r.providers {
		if err := p.Health(); err != nil {
			return fmt.Errorf("routing: provider %q unhealthy: %w", name, err)
		}
	}
	return nil
}

// Capabilities returns the union of all provider capabilities.
func (r *RoutingProvider) Capabilities() core.ProviderCapabilities {
	var caps core.ProviderCapabilities
	for _, p := range r.providers {
		c := p.Capabilities()
		caps.SupportsToolCalls = caps.SupportsToolCalls || c.SupportsToolCalls
		caps.SupportsThinking = caps.SupportsThinking || c.SupportsThinking
		caps.SupportsStreaming = caps.SupportsStreaming || c.SupportsStreaming
		caps.SupportsSystemPrompt = caps.SupportsSystemPrompt || c.SupportsSystemPrompt
		caps.SupportsVision = caps.SupportsVision || c.SupportsVision
		if c.MaxContextTokens > caps.MaxContextTokens {
			caps.MaxContextTokens = c.MaxContextTokens
		}
	}
	return caps
}

// Query routes the request to the appropriate provider based on hints.
func (r *RoutingProvider) Query(ctx context.Context, req core.QueryRequest) (<-chan core.StreamEvent, error) {
	// Local mode: skip routing entirely, use default provider.
	if r.local {
		p := r.getDefault()
		if p == nil {
			return nil, fmt.Errorf("routing: no providers configured")
		}
		return p.Query(ctx, req)
	}

	// Bypass: single provider or no policy.
	if r.shouldBypass() {
		p := r.getDefault()
		if p == nil {
			return nil, fmt.Errorf("routing: no providers configured")
		}
		return p.Query(ctx, req)
	}

	decisionStart := time.Now()

	// FeatureGate: only gate CostPolicy (Pro feature). ConfigPolicy stays available for all users.
	if _, isCost := r.policy.(*CostPolicy); isCost && r.gate != nil {
		if err := r.gate.Guard(ctx, "provider-arbitrage"); err != nil {
			p := r.getDefault()
			if p == nil {
				return nil, fmt.Errorf("routing: no providers configured")
			}
			return p.Query(ctx, req)
		}
	}

	// Route based on policy.
	sel := r.policy.Select(req.Hints)

	// Look up selected provider.
	target, ok := r.providers[sel.Provider]
	if !ok {
		target = r.getDefault()
		if target == nil {
			return nil, fmt.Errorf("routing: provider %q not found", sel.Provider)
		}
		sel.Provider = r.defaultProvider
		sel.Reason = "selected provider not found, falling back to default"
	}

	// Health check: if selected provider is unhealthy, find a fallback.
	if err := target.Health(); err != nil {
		fallback, fallbackName := r.findHealthyFallback(sel.Provider)
		if fallback != nil {
			originalProvider := sel.Provider
			sel.Provider = fallbackName
			sel.Reason = fmt.Sprintf("fallback: %s unreachable", originalProvider)
			target = fallback
		} else {
			slog.Warn("routing: all providers unhealthy, proceeding with selected",
				"provider", sel.Provider, "error", err)
		}
	}

	decisionLatency := time.Since(decisionStart)

	// Apply model override if specified.
	if sel.Model != "" {
		req.Model = sel.Model
	}

	// Publish routing decision event.
	if r.eventBus != nil {
		category := req.Hints[HintKeyCategory]
		ev := NewRoutingDecisionEvent(sel.Provider, sel.Model, category, sel.Reason)
		ev.ProviderCount = len(r.providers)
		ev.DecisionLatencyMS = decisionLatency.Milliseconds()
		if pricing, ok := r.pricing[sel.Provider]; ok {
			ev.ProviderCost = pricing.InputPer1M + pricing.OutputPer1M
		}
		_ = r.eventBus.Publish(ctx, ev)
	}

	return target.Query(ctx, req)
}

// findHealthyFallback returns the cheapest healthy provider (excluding skip).
// When pricing data is unavailable, falls back to deterministic alphabetical order.
func (r *RoutingProvider) findHealthyFallback(skip string) (core.Provider, string) {
	type fallbackCandidate struct {
		name     string
		provider core.Provider
		cost     float64
	}

	var candidates []fallbackCandidate
	for name, p := range r.providers {
		if name == skip {
			continue
		}
		if p.Health() != nil {
			continue
		}
		cost := math.MaxFloat64
		if pricing, ok := r.pricing[name]; ok {
			cost = pricing.InputPer1M + pricing.OutputPer1M
		}
		candidates = append(candidates, fallbackCandidate{name: name, provider: p, cost: cost})
	}

	if len(candidates) == 0 {
		return nil, ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].cost != candidates[j].cost {
			return candidates[i].cost < candidates[j].cost
		}
		return candidates[i].name < candidates[j].name
	})

	return candidates[0].provider, candidates[0].name
}

// getDefault returns the default provider, or the single provider if exactly one exists.
func (r *RoutingProvider) getDefault() core.Provider {
	if p, ok := r.providers[r.defaultProvider]; ok {
		return p
	}
	// If exactly one provider, return it regardless of name.
	if len(r.providers) == 1 {
		for _, p := range r.providers {
			return p
		}
	}
	return nil
}
