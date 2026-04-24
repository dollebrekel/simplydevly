// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// testProvider is a mock core.Provider for routing tests.
type testProvider struct {
	name       string
	queryCalls []core.QueryRequest
	caps       core.ProviderCapabilities
	mu         sync.Mutex
}

func (p *testProvider) Init(_ context.Context) error  { return nil }
func (p *testProvider) Start(_ context.Context) error { return nil }
func (p *testProvider) Stop(_ context.Context) error  { return nil }
func (p *testProvider) Health() error                 { return nil }

func (p *testProvider) Capabilities() core.ProviderCapabilities { return p.caps }

func (p *testProvider) Query(_ context.Context, req core.QueryRequest) (<-chan core.StreamEvent, error) {
	p.mu.Lock()
	p.queryCalls = append(p.queryCalls, req)
	p.mu.Unlock()

	ch := make(chan core.StreamEvent)
	close(ch)
	return ch, nil
}

func (p *testProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queryCalls)
}

func (p *testProvider) lastRequest() core.QueryRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.queryCalls[len(p.queryCalls)-1]
}

// testEventBus records published events.
type testEventBus struct {
	events []core.Event
	mu     sync.Mutex
}

func (b *testEventBus) Init(_ context.Context) error  { return nil }
func (b *testEventBus) Start(_ context.Context) error { return nil }
func (b *testEventBus) Stop(_ context.Context) error  { return nil }
func (b *testEventBus) Health() error                 { return nil }

func (b *testEventBus) Publish(_ context.Context, ev core.Event) error {
	b.mu.Lock()
	b.events = append(b.events, ev)
	b.mu.Unlock()
	return nil
}

func (b *testEventBus) Subscribe(_ string, _ core.EventHandler) func() {
	return func() {}
}

func (b *testEventBus) SubscribeChan(_ string) (<-chan core.Event, func()) {
	ch := make(chan core.Event)
	close(ch)
	return ch, func() {}
}

func (b *testEventBus) routingEvents() []*RoutingDecisionEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []*RoutingDecisionEvent
	for _, ev := range b.events {
		if re, ok := ev.(*RoutingDecisionEvent); ok {
			result = append(result, re)
		}
	}
	return result
}

func TestRoutingProvider_SingleProviderBypass(t *testing.T) {
	cheap := &testProvider{name: "cheap"}
	bus := &testEventBus{}

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers:       map[string]core.Provider{"cheap": cheap},
		Policy:          NewConfigPolicy(RoutingConfig{Enabled: true}),
		DefaultProvider: "cheap",
		EventBus:        bus,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{"task.category": "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, 1, cheap.callCount())
	// No routing events should be published on bypass.
	assert.Empty(t, bus.routingEvents())
}

func TestRoutingProvider_DualProviderRouting(t *testing.T) {
	primary := &testProvider{name: "primary"}
	preprocess := &testProvider{name: "preprocess"}
	bus := &testEventBus{}

	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryPreprocess, Provider: "preprocess", Model: "llama3.2"},
			{Category: CategoryPrimary, Provider: "primary"},
		},
		DefaultProvider: "primary",
		Enabled:         true,
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":    primary,
			"preprocess": preprocess,
		},
		Policy:          policy,
		DefaultProvider: "primary",
		EventBus:        bus,
	})

	ctx := context.Background()

	// Preprocess call should go to preprocess provider.
	req := core.QueryRequest{Hints: map[string]string{"task.category": "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, preprocess.callCount())
	assert.Equal(t, 0, primary.callCount())
	assert.Equal(t, "llama3.2", preprocess.lastRequest().Model)

	// Primary call should go to primary provider.
	req = core.QueryRequest{Hints: map[string]string{"task.category": "primary"}}
	_, err = rp.Query(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, primary.callCount())

	// Verify routing events.
	events := bus.routingEvents()
	assert.Len(t, events, 2)
	assert.Equal(t, "preprocess", events[0].SelectedProvider)
	assert.Equal(t, "llama3.2", events[0].SelectedModel)
	assert.Equal(t, "primary", events[1].SelectedProvider)
}

func TestRoutingProvider_UnknownProviderFallsBack(t *testing.T) {
	primary := &testProvider{name: "primary"}
	bus := &testEventBus{}

	// Policy that returns a provider name not in the map.
	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryPreprocess, Provider: "nonexistent"},
		},
		DefaultProvider: "primary",
		Enabled:         true,
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":  primary,
			"fallback": &testProvider{name: "fallback"},
		},
		Policy:          policy,
		DefaultProvider: "primary",
		EventBus:        bus,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{"task.category": "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, 1, primary.callCount())
}

func TestRoutingProvider_NilHintsUseDefault(t *testing.T) {
	primary := &testProvider{name: "primary"}
	preprocess := &testProvider{name: "preprocess"}
	bus := &testEventBus{}

	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryPreprocess, Provider: "preprocess"},
		},
		DefaultProvider: "primary",
		Enabled:         true,
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":    primary,
			"preprocess": preprocess,
		},
		Policy:          policy,
		DefaultProvider: "primary",
		EventBus:        bus,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: nil}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, 1, primary.callCount())
	assert.Equal(t, 0, preprocess.callCount())
}

func TestRoutingProvider_NilPolicyBypass(t *testing.T) {
	primary := &testProvider{name: "primary"}
	preprocess := &testProvider{name: "preprocess"}

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":    primary,
			"preprocess": preprocess,
		},
		Policy:          nil, // nil policy → bypass
		DefaultProvider: "primary",
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{"task.category": "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	// Should go to default since bypass is active.
	assert.Equal(t, 1, primary.callCount())
	assert.Equal(t, 0, preprocess.callCount())
}

func TestRoutingProvider_Capabilities_Union(t *testing.T) {
	p1 := &testProvider{caps: core.ProviderCapabilities{
		SupportsToolCalls: true,
		MaxContextTokens:  100000,
	}}
	p2 := &testProvider{caps: core.ProviderCapabilities{
		SupportsVision:   true,
		MaxContextTokens: 200000,
	}}

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{"p1": p1, "p2": p2},
	})

	caps := rp.Capabilities()
	assert.True(t, caps.SupportsToolCalls)
	assert.True(t, caps.SupportsVision)
	assert.Equal(t, 200000, caps.MaxContextTokens)
}

func TestRoutingProvider_Lifecycle(t *testing.T) {
	p1 := &testProvider{}
	p2 := &testProvider{}

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers:       map[string]core.Provider{"p1": p1, "p2": p2},
		DefaultProvider: "p1",
	})

	ctx := context.Background()
	require.NoError(t, rp.Init(ctx))
	require.NoError(t, rp.Start(ctx))
	require.NoError(t, rp.Health())
	require.NoError(t, rp.Stop(ctx))
}

func TestRoutingProvider_NoProviders(t *testing.T) {
	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{},
	})

	ctx := context.Background()
	_, err := rp.Query(ctx, core.QueryRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no providers configured")
}

// unhealthyProvider returns an error from Health().
type unhealthyProvider struct {
	testProvider
}

func (p *unhealthyProvider) Health() error {
	return fmt.Errorf("provider unreachable")
}

func TestRoutingProvider_HealthFallback(t *testing.T) {
	unhealthy := &unhealthyProvider{testProvider{name: "preferred"}}
	healthy := &testProvider{name: "fallback"}
	bus := &testEventBus{}

	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryPrimary, Provider: "preferred"},
		},
		DefaultProvider: "preferred",
		Enabled:         true,
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"preferred": unhealthy,
			"fallback":  healthy,
		},
		Policy:          policy,
		DefaultProvider: "preferred",
		EventBus:        bus,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{HintKeyCategory: "primary"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, 1, healthy.callCount())

	events := bus.routingEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "fallback", events[0].SelectedProvider)
	assert.Contains(t, events[0].Reason, "fallback: preferred unreachable")
}

func TestRoutingProvider_OfflineBypass(t *testing.T) {
	primary := &testProvider{name: "primary"}
	preprocess := &testProvider{name: "preprocess"}

	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryPreprocess, Provider: "preprocess"},
		},
		DefaultProvider: "primary",
		Enabled:         true,
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":    primary,
			"preprocess": preprocess,
		},
		Policy:          policy,
		DefaultProvider: "primary",
		Offline:         true,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{HintKeyCategory: "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, 1, primary.callCount())
	assert.Equal(t, 0, preprocess.callCount())
}

// testFeatureGate is a mock FeatureGate that can be configured to block features.
type testFeatureGate struct {
	blocked map[string]bool
}

func (g *testFeatureGate) Init(_ context.Context) error  { return nil }
func (g *testFeatureGate) Start(_ context.Context) error { return nil }
func (g *testFeatureGate) Stop(_ context.Context) error  { return nil }
func (g *testFeatureGate) Health() error                 { return nil }

func (g *testFeatureGate) Register(_ core.Feature) error { return nil }

func (g *testFeatureGate) Guard(_ context.Context, featureID string) error {
	if g.blocked[featureID] {
		return core.ErrFeatureGated
	}
	return nil
}

func (g *testFeatureGate) GuardWithFallback(_ context.Context, featureID string) (core.GateResult, error) {
	if g.blocked[featureID] {
		return core.GateResult{Allowed: false, FeatureID: featureID}, core.ErrFeatureGated
	}
	return core.GateResult{Allowed: true, FeatureID: featureID}, nil
}

func (g *testFeatureGate) List() []core.FeatureStatus { return nil }

func TestRoutingProvider_FeatureGateBlocked_CostPolicy(t *testing.T) {
	primary := &testProvider{name: "primary"}
	preprocess := &testProvider{name: "preprocess"}

	gate := &testFeatureGate{blocked: map[string]bool{"provider-arbitrage": true}}

	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"primary":    {InputPer1M: 3.00, OutputPer1M: 15.00},
			"preprocess": {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"primary":    {MaxContextTokens: 200000},
			"preprocess": {MaxContextTokens: 8192},
		},
		DefaultProvider: "primary",
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":    primary,
			"preprocess": preprocess,
		},
		Policy:          policy,
		DefaultProvider: "primary",
		Gate:            gate,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{HintKeyCategory: "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	// CostPolicy is gated — should fall back to default.
	assert.Equal(t, 1, primary.callCount())
	assert.Equal(t, 0, preprocess.callCount())
}

func TestRoutingProvider_FeatureGateAllowed_ConfigPolicy(t *testing.T) {
	primary := &testProvider{name: "primary"}
	preprocess := &testProvider{name: "preprocess"}

	gate := &testFeatureGate{blocked: map[string]bool{"provider-arbitrage": true}}

	// ConfigPolicy is NOT gated — Free users keep basic routing.
	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryPreprocess, Provider: "preprocess"},
		},
		DefaultProvider: "primary",
		Enabled:         true,
	})

	rp := NewRoutingProvider(RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"primary":    primary,
			"preprocess": preprocess,
		},
		Policy:          policy,
		DefaultProvider: "primary",
		Gate:            gate,
	})

	ctx := context.Background()
	req := core.QueryRequest{Hints: map[string]string{HintKeyCategory: "preprocess"}}
	_, err := rp.Query(ctx, req)
	require.NoError(t, err)

	// ConfigPolicy NOT gated — routing works even with gate blocking "provider-arbitrage".
	assert.Equal(t, 0, primary.callCount())
	assert.Equal(t, 1, preprocess.callCount())
}
