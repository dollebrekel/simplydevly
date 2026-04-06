// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"context"
	"encoding/json"
	"sync"

	"siply.dev/siply/internal/core"
)

// mockProvider implements core.Provider with configurable stream responses.
// Each call to Query consumes the next entry from Responses.
type mockProvider struct {
	Responses [][]core.StreamEvent // one entry per Query call
	Caps      core.ProviderCapabilities
	mu        sync.Mutex
	callIdx   int
}

func (m *mockProvider) Init(_ context.Context) error  { return nil }
func (m *mockProvider) Start(_ context.Context) error { return nil }
func (m *mockProvider) Stop(_ context.Context) error  { return nil }
func (m *mockProvider) Health() error                 { return nil }

func (m *mockProvider) Capabilities() core.ProviderCapabilities { return m.Caps }

func (m *mockProvider) Query(_ context.Context, _ core.QueryRequest) (<-chan core.StreamEvent, error) {
	m.mu.Lock()
	idx := m.callIdx
	m.callIdx++
	m.mu.Unlock()

	ch := make(chan core.StreamEvent, 100)
	go func() {
		defer close(ch)
		if idx < len(m.Responses) {
			for _, ev := range m.Responses[idx] {
				ch <- ev
			}
		}
	}()
	return ch, nil
}

// mockToolExecutor implements core.ToolExecutor with configurable responses.
type mockToolExecutor struct {
	// Responses maps tool name → response. If the name isn't found, returns a default.
	Responses map[string]mockToolResult
	// Tools is the list of available tool definitions.
	Tools []core.ToolDefinition
	// Calls records all Execute calls for assertion.
	Calls []core.ToolRequest
	mu    sync.Mutex
}

type mockToolResult struct {
	Response core.ToolResponse
	Err      error
}

func (m *mockToolExecutor) Init(_ context.Context) error  { return nil }
func (m *mockToolExecutor) Start(_ context.Context) error { return nil }
func (m *mockToolExecutor) Stop(_ context.Context) error  { return nil }
func (m *mockToolExecutor) Health() error                 { return nil }

func (m *mockToolExecutor) Execute(_ context.Context, req core.ToolRequest) (core.ToolResponse, error) {
	m.mu.Lock()
	m.Calls = append(m.Calls, req)
	m.mu.Unlock()

	if r, ok := m.Responses[req.Name]; ok {
		return r.Response, r.Err
	}
	return core.ToolResponse{Output: "default output"}, nil
}

func (m *mockToolExecutor) ListTools() []core.ToolDefinition {
	if m.Tools != nil {
		return m.Tools
	}
	return []core.ToolDefinition{}
}

func (m *mockToolExecutor) GetTool(name string) (core.ToolDefinition, error) {
	for _, t := range m.Tools {
		if t.Name == name {
			return t, nil
		}
	}
	return core.ToolDefinition{}, core.ErrToolNotFound
}

// mockEventBus implements core.EventBus and records all published events.
type mockEventBus struct {
	Events []core.Event
	mu     sync.Mutex
}

func (m *mockEventBus) Init(_ context.Context) error  { return nil }
func (m *mockEventBus) Start(_ context.Context) error { return nil }
func (m *mockEventBus) Stop(_ context.Context) error  { return nil }
func (m *mockEventBus) Health() error                 { return nil }

func (m *mockEventBus) Publish(_ context.Context, event core.Event) error {
	m.mu.Lock()
	m.Events = append(m.Events, event)
	m.mu.Unlock()
	return nil
}

func (m *mockEventBus) Subscribe(_ string, _ core.EventHandler) (unsubscribe func()) {
	return func() {}
}

// eventsOfType returns all recorded events matching the given type.
func (m *mockEventBus) eventsOfType(eventType string) []core.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []core.Event
	for _, ev := range m.Events {
		if ev.Type() == eventType {
			result = append(result, ev)
		}
	}
	return result
}

// mockTokenCounter implements core.TokenCounter with fixed return values.
type mockTokenCounter struct {
	FixedCount int
	FixedCost  float64
}

func (m *mockTokenCounter) Count(_ string, _ string) (int, error) {
	return m.FixedCount, nil
}

func (m *mockTokenCounter) EstimateCost(_ core.TokenUsage, _ string) (float64, error) {
	return m.FixedCost, nil
}

// mockStatusCollector implements core.StatusCollector and records updates.
type mockStatusCollector struct {
	Updates []core.StatusUpdate
	mu      sync.Mutex
}

func (m *mockStatusCollector) Init(_ context.Context) error  { return nil }
func (m *mockStatusCollector) Start(_ context.Context) error { return nil }
func (m *mockStatusCollector) Stop(_ context.Context) error  { return nil }
func (m *mockStatusCollector) Health() error                 { return nil }

func (m *mockStatusCollector) Publish(update core.StatusUpdate) {
	m.mu.Lock()
	m.Updates = append(m.Updates, update)
	m.mu.Unlock()
}

func (m *mockStatusCollector) Subscribe() (updates <-chan core.StatusUpdate, unsubscribe func()) {
	ch := make(chan core.StatusUpdate, 100)
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch) }) }
}

func (m *mockStatusCollector) Snapshot() map[string]core.StatusUpdate {
	return map[string]core.StatusUpdate{}
}

// mockContextManager implements core.ContextManager.
type mockContextManager struct {
	ShouldCompactResult bool
	CompactCalled       bool
}

func (m *mockContextManager) Init(_ context.Context) error  { return nil }
func (m *mockContextManager) Start(_ context.Context) error { return nil }
func (m *mockContextManager) Stop(_ context.Context) error  { return nil }
func (m *mockContextManager) Health() error                 { return nil }

func (m *mockContextManager) ShouldCompact(_ []core.Message, _ int) bool {
	return m.ShouldCompactResult
}

func (m *mockContextManager) Compact(_ context.Context, messages []core.Message) ([]core.Message, error) {
	m.CompactCalled = true
	// Simple mock: just return first + last message.
	if len(messages) <= 2 {
		return messages, nil
	}
	return []core.Message{messages[0], messages[len(messages)-1]}, nil
}

// mockPermissionEvaluator implements core.PermissionEvaluator.
type mockPermissionEvaluator struct {
	Verdict core.ActionVerdict
}

func (m *mockPermissionEvaluator) Init(_ context.Context) error  { return nil }
func (m *mockPermissionEvaluator) Start(_ context.Context) error { return nil }
func (m *mockPermissionEvaluator) Stop(_ context.Context) error  { return nil }
func (m *mockPermissionEvaluator) Health() error                 { return nil }

func (m *mockPermissionEvaluator) EvaluateCapabilities(_ context.Context, _ core.PluginMeta) (core.CapabilityVerdict, error) {
	return core.CapabilityVerdict{}, nil
}

func (m *mockPermissionEvaluator) EvaluateAction(_ context.Context, _ core.Action) (core.ActionVerdict, error) {
	return m.Verdict, nil
}

// newTestDeps creates a standard set of mock dependencies for testing.
func newTestDeps() (AgentDeps, *mockProvider, *mockToolExecutor, *mockEventBus, *mockStatusCollector, *mockContextManager) {
	provider := &mockProvider{Caps: core.ProviderCapabilities{MaxContextTokens: 100000}}
	tools := &mockToolExecutor{Responses: map[string]mockToolResult{}}
	events := &mockEventBus{}
	tokens := &mockTokenCounter{}
	status := &mockStatusCollector{}
	ctxMgr := &mockContextManager{}
	perm := &mockPermissionEvaluator{Verdict: core.Allow}

	deps := AgentDeps{
		Provider: provider,
		Tools:    tools,
		Events:   events,
		Tokens:   tokens,
		Context:  ctxMgr,
		Status:   status,
		Perm:     perm,
	}

	return deps, provider, tools, events, status, ctxMgr
}

// jsonRaw is a helper to create json.RawMessage from a map.
func jsonRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
