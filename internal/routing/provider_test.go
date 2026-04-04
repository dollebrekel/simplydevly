package routing

import (
	"context"
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
