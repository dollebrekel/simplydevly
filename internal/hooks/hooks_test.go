// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package hooks

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// --- mock EventBus ---

type mockEventBus struct {
	events []core.Event
	mu     sync.Mutex
}

func (m *mockEventBus) Init(_ context.Context) error  { return nil }
func (m *mockEventBus) Start(_ context.Context) error { return nil }
func (m *mockEventBus) Stop(_ context.Context) error  { return nil }
func (m *mockEventBus) Health() error                 { return nil }

func (m *mockEventBus) Publish(_ context.Context, event core.Event) error {
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
	return nil
}

func (m *mockEventBus) Subscribe(_ string, _ core.EventHandler) (unsubscribe func()) {
	return func() {}
}

func (m *mockEventBus) SubscribeChan(_ string) (<-chan core.Event, func()) {
	ch := make(chan core.Event)
	close(ch)
	return ch, func() {}
}

func (m *mockEventBus) eventsOfType(t string) []core.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []core.Event
	for _, ev := range m.events {
		if ev.Type() == t {
			result = append(result, ev)
		}
	}
	return result
}

// --- helpers ---

func newTestHooks() (*agentHooks, *mockEventBus) {
	bus := &mockEventBus{}
	h := NewAgentHooks(bus).(*agentHooks)
	_ = h.Init(context.Background())
	return h, bus
}

// --- PreQuery tests ---

func TestRunPreQueryEmptyChain(t *testing.T) {
	h, _ := newTestHooks()

	msgs := []core.Message{{Role: "user", Content: "hello"}}
	result, err := h.RunPreQuery(context.Background(), msgs)

	require.NoError(t, err)
	assert.Equal(t, msgs, result)
}

func TestRunPreQueryPriorityOrder(t *testing.T) {
	h, _ := newTestHooks()

	var order []int

	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		order = append(order, 10)
		return msgs, nil
	}, core.HookConfig{Priority: 10, Timeout: 5 * time.Second})

	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		order = append(order, 20)
		return msgs, nil
	}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

	msgs := []core.Message{{Role: "user", Content: "test"}}
	_, err := h.RunPreQuery(context.Background(), msgs)

	require.NoError(t, err)
	assert.Equal(t, []int{10, 20}, order)
}

func TestRunPreQueryModifiesMessages(t *testing.T) {
	h, _ := newTestHooks()

	// First hook appends a system message.
	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		return append(msgs, core.Message{Role: "system", Content: "injected"}), nil
	}, core.HookConfig{Priority: 10, Timeout: 5 * time.Second})

	// Second hook sees the modified messages.
	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		// Verify we see the injected message.
		last := msgs[len(msgs)-1]
		return append(msgs, core.Message{Role: "system", Content: "saw:" + last.Content}), nil
	}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

	msgs := []core.Message{{Role: "user", Content: "original"}}
	result, err := h.RunPreQuery(context.Background(), msgs)

	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "original", result[0].Content)
	assert.Equal(t, "injected", result[1].Content)
	assert.Equal(t, "saw:injected", result[2].Content)
}

func TestRunPreQuerySkipOnFailure(t *testing.T) {
	h, _ := newTestHooks()

	// Failing hook with SkipOnFailure.
	h.OnPreQuery(func(_ context.Context, _ []core.Message) ([]core.Message, error) {
		return nil, errors.New("hook broken")
	}, core.HookConfig{Priority: 10, OnFailure: core.HookSkipOnFailure, Timeout: 5 * time.Second})

	// Second hook should still execute.
	var secondCalled bool
	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		secondCalled = true
		return msgs, nil
	}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

	msgs := []core.Message{{Role: "user", Content: "test"}}
	result, err := h.RunPreQuery(context.Background(), msgs)

	require.NoError(t, err)
	assert.True(t, secondCalled)
	assert.Equal(t, msgs, result) // original data passed through (failing hook skipped)
}

func TestRunPreQueryAbortOnFailure(t *testing.T) {
	h, _ := newTestHooks()

	h.OnPreQuery(func(_ context.Context, _ []core.Message) ([]core.Message, error) {
		return nil, errors.New("critical failure")
	}, core.HookConfig{Priority: 10, OnFailure: core.HookAbortOnFailure, Timeout: 5 * time.Second})

	var secondCalled bool
	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		secondCalled = true
		return msgs, nil
	}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

	msgs := []core.Message{{Role: "user", Content: "test"}}
	_, err := h.RunPreQuery(context.Background(), msgs)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "critical failure")
	assert.False(t, secondCalled) // chain stopped
}

func TestRunPreQueryTimeout(t *testing.T) {
	h, _ := newTestHooks()

	// Slow hook with very short timeout — SkipOnFailure.
	h.OnPreQuery(func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return msgs, nil
		}
	}, core.HookConfig{Priority: 10, OnFailure: core.HookSkipOnFailure, Timeout: 50 * time.Millisecond})

	var secondCalled bool
	h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		secondCalled = true
		return msgs, nil
	}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

	msgs := []core.Message{{Role: "user", Content: "test"}}
	result, err := h.RunPreQuery(context.Background(), msgs)

	require.NoError(t, err)
	assert.True(t, secondCalled)
	assert.Equal(t, msgs, result)
}

// --- PreTool tests (mirror PreQuery) ---

func TestRunPreToolSamePatterns(t *testing.T) {
	t.Run("EmptyChain", func(t *testing.T) {
		h, _ := newTestHooks()
		call := core.ToolCall{ToolID: "1", ToolName: "test", Input: []byte(`{}`)}
		result, err := h.RunPreTool(context.Background(), call)
		require.NoError(t, err)
		assert.Equal(t, call, result)
	})

	t.Run("PriorityOrder", func(t *testing.T) {
		h, _ := newTestHooks()
		var order []int

		h.OnPreTool(func(_ context.Context, call core.ToolCall) (core.ToolCall, error) {
			order = append(order, 10)
			return call, nil
		}, core.HookConfig{Priority: 10, Timeout: 5 * time.Second})

		h.OnPreTool(func(_ context.Context, call core.ToolCall) (core.ToolCall, error) {
			order = append(order, 20)
			return call, nil
		}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

		call := core.ToolCall{ToolID: "1", ToolName: "test", Input: []byte(`{}`)}
		_, err := h.RunPreTool(context.Background(), call)
		require.NoError(t, err)
		assert.Equal(t, []int{10, 20}, order)
	})

	t.Run("SkipOnFailure", func(t *testing.T) {
		h, _ := newTestHooks()

		h.OnPreTool(func(_ context.Context, _ core.ToolCall) (core.ToolCall, error) {
			return core.ToolCall{}, errors.New("broken")
		}, core.HookConfig{Priority: 10, OnFailure: core.HookSkipOnFailure, Timeout: 5 * time.Second})

		var secondCalled bool
		h.OnPreTool(func(_ context.Context, call core.ToolCall) (core.ToolCall, error) {
			secondCalled = true
			return call, nil
		}, core.HookConfig{Priority: 20, Timeout: 5 * time.Second})

		call := core.ToolCall{ToolID: "1", ToolName: "test", Input: []byte(`{}`)}
		result, err := h.RunPreTool(context.Background(), call)
		require.NoError(t, err)
		assert.True(t, secondCalled)
		assert.Equal(t, call, result)
	})

	t.Run("AbortOnFailure", func(t *testing.T) {
		h, _ := newTestHooks()

		h.OnPreTool(func(_ context.Context, _ core.ToolCall) (core.ToolCall, error) {
			return core.ToolCall{}, errors.New("abort")
		}, core.HookConfig{Priority: 10, OnFailure: core.HookAbortOnFailure, Timeout: 5 * time.Second})

		call := core.ToolCall{ToolID: "1", ToolName: "test", Input: []byte(`{}`)}
		_, err := h.RunPreTool(context.Background(), call)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "abort")
	})

	t.Run("Modifies", func(t *testing.T) {
		h, _ := newTestHooks()

		h.OnPreTool(func(_ context.Context, call core.ToolCall) (core.ToolCall, error) {
			call.ToolName = "modified"
			return call, nil
		}, core.HookConfig{Priority: 10, Timeout: 5 * time.Second})

		call := core.ToolCall{ToolID: "1", ToolName: "original", Input: []byte(`{}`)}
		result, err := h.RunPreTool(context.Background(), call)
		require.NoError(t, err)
		assert.Equal(t, "modified", result.ToolName)
	})
}

// --- Unregister test ---

func TestUnregisterRemovesHook(t *testing.T) {
	h, _ := newTestHooks()

	var called bool
	unregister := h.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		called = true
		return msgs, nil
	}, core.HookConfig{Priority: 10, Timeout: 5 * time.Second})

	// Unregister and verify hook is no longer called.
	unregister()

	msgs := []core.Message{{Role: "user", Content: "test"}}
	_, err := h.RunPreQuery(context.Background(), msgs)

	require.NoError(t, err)
	assert.False(t, called)
}

// --- HookFailedEvent test ---

func TestHookFailedEventPublished(t *testing.T) {
	h, bus := newTestHooks()

	h.OnPreQuery(func(_ context.Context, _ []core.Message) ([]core.Message, error) {
		return nil, errors.New("distillation offline")
	}, core.HookConfig{Priority: 10, OnFailure: core.HookSkipOnFailure, Timeout: 5 * time.Second})

	msgs := []core.Message{{Role: "user", Content: "test"}}
	_, err := h.RunPreQuery(context.Background(), msgs)
	require.NoError(t, err)

	// Verify HookFailedEvent was published.
	failedEvents := bus.eventsOfType("hook.failed")
	require.Len(t, failedEvents, 1)

	hfe, ok := failedEvents[0].(*HookFailedEvent)
	require.True(t, ok)
	assert.Equal(t, "PreQuery", hfe.Point)
	assert.Contains(t, hfe.Err, "distillation offline")
	assert.NotEmpty(t, hfe.Fallback)
	assert.NotEmpty(t, hfe.CostEffect)
}
