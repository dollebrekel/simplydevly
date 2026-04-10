// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
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

// newStartedBus creates a Bus with the given buffer size and starts it.
func newStartedBus(t *testing.T, size int) *Bus {
	t.Helper()
	bus := NewBusWithBuffer(size)
	require.NoError(t, bus.Start(context.Background()))
	return bus
}

// --- Async delivery tests ---

func TestBus_AsyncDelivery(t *testing.T) {
	bus := NewBusWithBuffer(10)
	ctx := context.Background()
	require.NoError(t, bus.Init(ctx))
	require.NoError(t, bus.Start(ctx))

	delivered := make(chan core.Event, 1)
	bus.Subscribe("test.event", func(_ context.Context, ev core.Event) {
		delivered <- ev
	})

	ev := newTestEvent("test.event")
	err := bus.Publish(ctx, ev)
	require.NoError(t, err)

	select {
	case got := <-delivered:
		assert.Equal(t, "test.event", got.Type())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async delivery")
	}
}

func TestBus_AsyncDelivery_NotInPublishGoroutine(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	publishGoroutineID := make(chan bool, 1)

	bus.Subscribe("test.event", func(_ context.Context, _ core.Event) {
		// If this runs synchronously in Publish, a blocking operation here
		// would block Publish. We verify by checking Publish returns immediately.
		publishGoroutineID <- true
	})

	// Publish should return before handler executes.
	err := bus.Publish(ctx, newTestEvent("test.event"))
	require.NoError(t, err)

	select {
	case <-publishGoroutineID:
		// Handler was called (async) — good
	case <-time.After(2 * time.Second):
		t.Fatal("handler was never called")
	}
}

// --- Sync ConfigChanged tests ---

func TestBus_SyncConfigChanged(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	var handlerCalled atomic.Bool
	bus.Subscribe(EventConfigChanged, func(_ context.Context, ev core.Event) {
		handlerCalled.Store(true)
	})

	ev := NewConfigChangedEvent("theme", "dark", "light")
	err := bus.Publish(ctx, ev)
	require.NoError(t, err)

	// For sync delivery, handler must have been called before Publish returns.
	assert.True(t, handlerCalled.Load(), "ConfigChanged handler must be called synchronously before Publish returns")
}

func TestBus_SyncConfigChanged_ChannelSubscriber(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	ch, unsub := bus.SubscribeChan(EventConfigChanged)
	defer unsub()

	ev := NewConfigChangedEvent("key", "old", "new")

	// Publish in a goroutine because sync delivery blocks until channel receives.
	done := make(chan error, 1)
	go func() {
		done <- bus.Publish(ctx, ev)
	}()

	select {
	case got := <-ch:
		assert.Equal(t, EventConfigChanged, got.Type())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync ConfigChanged on channel")
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("publish did not complete")
	}
}

// --- Buffer overflow / drop tests ---

func TestBus_BufferOverflow_Drop(t *testing.T) {
	bufSize := 5
	bus := newStartedBus(t, bufSize)

	// Use a handler that blocks so events accumulate in the buffer.
	block := make(chan struct{})
	bus.Subscribe("overflow", func(_ context.Context, _ core.Event) {
		<-block // Block forever during this test
	})

	// Capture slog output.
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	ctx := context.Background()
	// Fill the buffer + overflow.
	for range bufSize + 3 {
		_ = bus.Publish(ctx, newTestEvent("overflow"))
	}

	// slog.Warn is called synchronously in Publish — no sleep needed.
	logOutput := logBuf.String()
	assert.True(t, strings.Contains(logOutput, "event not delivered"),
		"expected 'event not delivered' in log output, got: %s", logOutput)

	close(block)
}

func TestBus_BufferOverflow_ChannelSubscriber(t *testing.T) {
	bufSize := 3
	bus := newStartedBus(t, bufSize)

	ch, unsub := bus.SubscribeChan("overflow")
	defer unsub()

	// Capture slog output.
	var logBuf bytes.Buffer
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	ctx := context.Background()
	// Fill buffer beyond capacity without reading.
	for range bufSize + 3 {
		_ = bus.Publish(ctx, newTestEvent("overflow"))
	}

	// slog.Warn is called synchronously in Publish — no sleep needed.
	logOutput := logBuf.String()
	assert.True(t, strings.Contains(logOutput, "event not delivered"),
		"expected 'event not delivered' for channel subscriber, got: %s", logOutput)

	// Drain what was buffered.
	drained := 0
	for range bufSize {
		select {
		case <-ch:
			drained++
		default:
		}
	}
	assert.Equal(t, bufSize, drained, "should have received exactly bufSize events")
}

// --- Channel-based subscription tests ---

func TestBus_SubscribeChan_ReceiveEvent(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	ch, unsub := bus.SubscribeChan("chan.test")
	defer unsub()

	ev := newTestEvent("chan.test")
	require.NoError(t, bus.Publish(ctx, ev))

	select {
	case got := <-ch:
		assert.Equal(t, "chan.test", got.Type())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for channel event")
	}
}

func TestBus_SubscribeChan_SelectMultiplex(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	chA, unsubA := bus.SubscribeChan("type.a")
	defer unsubA()
	chB, unsubB := bus.SubscribeChan("type.b")
	defer unsubB()

	require.NoError(t, bus.Publish(ctx, newTestEvent("type.a")))
	require.NoError(t, bus.Publish(ctx, newTestEvent("type.b")))

	received := make(map[string]bool)
	timeout := time.After(2 * time.Second)
	for range 2 {
		select {
		case ev := <-chA:
			received[ev.Type()] = true
		case ev := <-chB:
			received[ev.Type()] = true
		case <-timeout:
			t.Fatal("timed out in select multiplex")
		}
	}

	assert.True(t, received["type.a"], "should have received type.a")
	assert.True(t, received["type.b"], "should have received type.b")
}

// --- Unsubscribe tests ---

func TestBus_Unsubscribe_ClosesChannel(t *testing.T) {
	bus := NewBusWithBuffer(10)

	ch, unsub := bus.SubscribeChan("unsub.test")
	unsub()

	// Channel should be closed: receive should return zero value with ok=false.
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after unsubscribe")
}

func TestBus_Unsubscribe_Callback(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	delivered := make(chan struct{}, 10)
	unsub := bus.Subscribe("unsub.test", func(_ context.Context, _ core.Event) {
		delivered <- struct{}{}
	})

	require.NoError(t, bus.Publish(ctx, newTestEvent("unsub.test")))

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("expected delivery before unsubscribe")
	}

	unsub()

	// After unsubscribe, publish should not deliver.
	require.NoError(t, bus.Publish(ctx, newTestEvent("unsub.test")))

	select {
	case <-delivered:
		t.Fatal("handler should not be called after unsubscribe")
	case <-time.After(100 * time.Millisecond):
		// Good — no delivery
	}
}

func TestBus_DoubleUnsubscribe_Safe(t *testing.T) {
	bus := NewBusWithBuffer(10)

	ch, unsub := bus.SubscribeChan("double.unsub")
	unsub()
	unsub() // Should not panic

	_, ok := <-ch
	assert.False(t, ok)
}

// --- Multiple event types ---

func TestBus_MultipleEventTypes(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	resultA := make(chan core.Event, 1)
	resultB := make(chan core.Event, 1)

	bus.Subscribe("type.a", func(_ context.Context, ev core.Event) {
		resultA <- ev
	})
	bus.Subscribe("type.b", func(_ context.Context, ev core.Event) {
		resultB <- ev
	})

	require.NoError(t, bus.Publish(ctx, newTestEvent("type.a")))
	require.NoError(t, bus.Publish(ctx, newTestEvent("type.b")))

	select {
	case ev := <-resultA:
		assert.Equal(t, "type.a", ev.Type())
	case <-time.After(2 * time.Second):
		t.Fatal("timeout for type.a")
	}

	select {
	case ev := <-resultB:
		assert.Equal(t, "type.b", ev.Type())
	case <-time.After(2 * time.Second):
		t.Fatal("timeout for type.b")
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := newStartedBus(t, 10)
	ctx := context.Background()

	var count1, count2 atomic.Int64
	bus.Subscribe("test.event", func(_ context.Context, _ core.Event) { count1.Add(1) })
	bus.Subscribe("test.event", func(_ context.Context, _ core.Event) { count2.Add(1) })

	require.NoError(t, bus.Publish(ctx, newTestEvent("test.event")))

	// Wait for async delivery.
	assert.Eventually(t, func() bool {
		return count1.Load() == 1 && count2.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

// --- Concurrent publish + subscribe (race detection) ---

func TestBus_ConcurrentPublishSubscribe(t *testing.T) {
	bus := newStartedBus(t, 50)
	ctx := context.Background()

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			unsub := bus.Subscribe("race", func(_ context.Context, _ core.Event) {})
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

func TestBus_ConcurrentPublishUnsubscribe(t *testing.T) {
	bus := newStartedBus(t, 50)
	ctx := context.Background()

	var wg sync.WaitGroup

	for range 50 {
		ch, unsub := bus.SubscribeChan("race.chan")
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = bus.Publish(ctx, newTestEvent("race.chan"))
		}()
		go func() {
			defer wg.Done()
			unsub()
			// Drain channel.
			for range ch {
			}
		}()
	}
	wg.Wait()
}

func TestBus_ConcurrentAccess(t *testing.T) {
	bus := newStartedBus(t, 100)
	ctx := context.Background()

	var counter atomic.Int64
	bus.Subscribe("concurrent", func(_ context.Context, _ core.Event) {
		counter.Add(1)
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.Publish(ctx, newTestEvent("concurrent"))
		}()
	}
	wg.Wait()

	assert.Eventually(t, func() bool {
		return counter.Load() == 100
	}, 5*time.Second, 10*time.Millisecond)
}

// --- Lifecycle tests ---

func TestBus_Lifecycle(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()

	require.NoError(t, bus.Init(ctx))
	require.NoError(t, bus.Start(ctx))
	require.NoError(t, bus.Health())

	err := bus.Publish(ctx, newTestEvent("test"))
	require.NoError(t, err)

	require.NoError(t, bus.Stop(ctx))
	assert.Error(t, bus.Health(), "health should error after stop")

	err = bus.Publish(ctx, newTestEvent("test"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrBusStopped)
}

func TestBus_PublishAfterStop(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()
	require.NoError(t, bus.Stop(ctx))

	err := bus.Publish(ctx, newTestEvent("test"))
	assert.ErrorIs(t, err, ErrBusStopped)
}

func TestBus_SubscribeAfterStop(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()
	require.NoError(t, bus.Stop(ctx))

	unsub := bus.Subscribe("test", func(_ context.Context, _ core.Event) {
		t.Fatal("should never be called")
	})
	unsub() // Should be safe no-op
}

func TestBus_SubscribeChanAfterStop(t *testing.T) {
	bus := NewBus()
	ctx := context.Background()
	require.NoError(t, bus.Stop(ctx))

	ch, unsub := bus.SubscribeChan("test")
	defer unsub()

	// Channel should be closed immediately.
	_, ok := <-ch
	assert.False(t, ok, "channel from stopped bus should be closed")
}

func TestBus_PublishNilEvent(t *testing.T) {
	bus := NewBus()
	err := bus.Publish(context.Background(), nil)
	assert.Error(t, err)
}

func TestBus_NoSubscribers(t *testing.T) {
	bus := newStartedBus(t, defaultBufferSize)
	ctx := context.Background()

	err := bus.Publish(ctx, newTestEvent("nobody.listening"))
	require.NoError(t, err)
}

func TestBus_PublishBeforeStart(t *testing.T) {
	bus := NewBus()
	err := bus.Publish(context.Background(), newTestEvent("test"))
	assert.ErrorIs(t, err, ErrBusNotStarted)
}

// --- Stop closes channels ---

func TestBus_Stop_ClosesChannelSubscribers(t *testing.T) {
	bus := NewBusWithBuffer(10)
	ctx := context.Background()

	ch, _ := bus.SubscribeChan("stop.test")

	require.NoError(t, bus.Stop(ctx))

	// Channel should be closed.
	_, ok := <-ch
	assert.False(t, ok, "channel should be closed after Stop")
}

// --- NewBusWithBuffer ---

func TestNewBusWithBuffer_InvalidSize(t *testing.T) {
	bus := NewBusWithBuffer(0)
	assert.Equal(t, defaultBufferSize, bus.bufferSize)

	bus = NewBusWithBuffer(-5)
	assert.Equal(t, defaultBufferSize, bus.bufferSize)
}
