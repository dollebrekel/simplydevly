// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

// testEvent is a simple event for testing.
type testEvent struct {
	eventType string
	ts        time.Time
}

func (e *testEvent) Type() string         { return e.eventType }
func (e *testEvent) Timestamp() time.Time { return e.ts }

func newTestEvent(eventType string) *testEvent {
	return &testEvent{eventType: eventType, ts: time.Now()}
}

func TestBus_PublishSubscribe(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var received []core.Event
	bus.Subscribe("test.event", func(_ context.Context, ev core.Event) {
		received = append(received, ev)
	})

	ev := newTestEvent("test.event")
	err := bus.Publish(ctx, ev)
	require.NoError(t, err)
	assert.Len(t, received, 1)
	assert.Equal(t, "test.event", received[0].Type())
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var count1, count2 int
	bus.Subscribe("test.event", func(_ context.Context, _ core.Event) { count1++ })
	bus.Subscribe("test.event", func(_ context.Context, _ core.Event) { count2++ })

	err := bus.Publish(ctx, newTestEvent("test.event"))
	require.NoError(t, err)
	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
}

func TestBus_NoSubscribers(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	// Publishing with no subscribers should not error.
	err := bus.Publish(ctx, newTestEvent("test.event"))
	require.NoError(t, err)
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var count int
	unsub := bus.Subscribe("test.event", func(_ context.Context, _ core.Event) { count++ })

	err := bus.Publish(ctx, newTestEvent("test.event"))
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	unsub()

	err = bus.Publish(ctx, newTestEvent("test.event"))
	require.NoError(t, err)
	assert.Equal(t, 1, count, "handler should not be called after unsubscribe")
}

func TestBus_DifferentEventTypes(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var typeA, typeB int
	bus.Subscribe("type.a", func(_ context.Context, _ core.Event) { typeA++ })
	bus.Subscribe("type.b", func(_ context.Context, _ core.Event) { typeB++ })

	err := bus.Publish(ctx, newTestEvent("type.a"))
	require.NoError(t, err)
	assert.Equal(t, 1, typeA)
	assert.Equal(t, 0, typeB)
}

func TestBus_ConcurrentAccess(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var counter atomic.Int64
	bus.Subscribe("concurrent", func(_ context.Context, _ core.Event) {
		counter.Add(1)
	})

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = bus.Publish(ctx, newTestEvent("concurrent"))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int64(100), counter.Load())
}

func TestBus_ConcurrentSubscribePublish(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	var wg sync.WaitGroup

	// Concurrent subscribes and publishes.
	for i := range 50 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			unsub := bus.Subscribe("race", func(_ context.Context, _ core.Event) {})
			// Unsubscribe some to exercise cleanup.
			if n%2 == 0 {
				unsub()
			}
		}(i)
		go func() {
			defer wg.Done()
			_ = bus.Publish(ctx, newTestEvent("race"))
		}()
	}
	wg.Wait()
}

func TestBus_Lifecycle(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	require.NoError(t, bus.Init(ctx))
	require.NoError(t, bus.Start(ctx))
	require.NoError(t, bus.Health())

	// Publish works after start.
	err := bus.Publish(ctx, newTestEvent("test"))
	require.NoError(t, err)

	require.NoError(t, bus.Stop(ctx))
	assert.Error(t, bus.Health(), "health should error after stop")

	// Publish after stop should error.
	err = bus.Publish(ctx, newTestEvent("test"))
	assert.Error(t, err)
}

func TestBus_PublishNilEvent(t *testing.T) {
	bus := NewBus()
	err := bus.Publish(context.Background(), nil)
	assert.Error(t, err)
}

func TestBus_UnsubscribeCleanup(t *testing.T) {
	bus := NewBus()

	unsub1 := bus.Subscribe("cleanup", func(_ context.Context, _ core.Event) {})
	unsub2 := bus.Subscribe("cleanup", func(_ context.Context, _ core.Event) {})

	unsub1()
	unsub2()

	// Double unsubscribe should be safe.
	unsub1()

	bus.mu.RLock()
	_, exists := bus.subscribers["cleanup"]
	bus.mu.RUnlock()
	assert.False(t, exists, "event type should be cleaned up when all subscribers removed")
}
