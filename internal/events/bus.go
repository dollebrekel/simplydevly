// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"siply.dev/siply/internal/core"
)

// ErrBusStopped is returned when publishing to a stopped bus.
var ErrBusStopped = errors.New("events: bus is stopped")

// ErrBusNotStarted is returned when publishing to a bus that has not been started.
var ErrBusNotStarted = errors.New("events: bus is not started")

// defaultBufferSize is the per-subscriber channel buffer size.
const defaultBufferSize = 100

// callbackSub holds state for a callback-based subscriber.
type callbackSub struct {
	handler   core.EventHandler
	ch        chan core.Event // async delivery channel
	done      chan struct{}   // stop signal for the delivery goroutine
	mu        sync.Mutex
	closed    bool
	handlerMu sync.Mutex // serializes handler invocations (async goroutine vs sync ConfigChanged)
}

// trySend attempts a non-blocking send. Returns false if closed or buffer full.
func (s *callbackSub) trySend(ev core.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- ev:
		return true
	default:
		return false
	}
}

// stop marks the subscriber as closed and stops the delivery goroutine.
// Closes both the done channel and the delivery channel to ensure the
// goroutine exits promptly regardless of which select arm it is in.
func (s *callbackSub) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.done)
	close(s.ch)
}

// channelSub holds state for a channel-based subscriber.
type channelSub struct {
	ch     chan core.Event // buffered output channel exposed to consumer
	mu     sync.Mutex
	closed bool
}

// trySend attempts a non-blocking send. Returns false if closed or buffer full.
func (s *channelSub) trySend(ev core.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- ev:
		return true
	default:
		return false
	}
}

// trySendSync attempts a blocking send with context cancellation.
// Returns false if closed or context canceled. The mutex is released
// before the blocking send to prevent deadlocks with concurrent closeCh.
func (s *channelSub) trySendSync(ctx context.Context, ev core.Event) bool {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return false
	}
	ch := s.ch
	s.mu.Unlock()

	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// closeCh closes the channel and marks the subscriber as closed.
func (s *channelSub) closeCh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// Bus implements core.EventBus with async channel-native delivery.
// It supports both callback-based (Subscribe) and channel-based (SubscribeChan)
// subscription models with configurable buffer sizes.
type Bus struct {
	mu         sync.RWMutex
	callbacks  map[string]map[uint64]*callbackSub
	channels   map[string]map[uint64]*channelSub
	nextID     uint64
	started    bool
	stopped    bool
	bufferSize int
	wg         sync.WaitGroup // tracks in-flight delivery goroutines
}

// NewBus creates a new EventBus with the default buffer size (100).
func NewBus() *Bus {
	return NewBusWithBuffer(defaultBufferSize)
}

// NewBusWithBuffer creates a new EventBus with a custom buffer size.
func NewBusWithBuffer(size int) *Bus {
	if size <= 0 {
		size = defaultBufferSize
	}
	return &Bus{
		callbacks:  make(map[string]map[uint64]*callbackSub),
		channels:   make(map[string]map[uint64]*channelSub),
		bufferSize: size,
	}
}

// Init validates the bus state.
func (b *Bus) Init(_ context.Context) error {
	if b == nil {
		return fmt.Errorf("events: bus: init: nil bus")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return fmt.Errorf("events: bus: init: %w", ErrBusStopped)
	}
	return nil
}

// Start marks the bus as ready to receive events.
func (b *Bus) Start(_ context.Context) error {
	if b == nil {
		return fmt.Errorf("events: bus: start: nil bus")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return fmt.Errorf("events: bus: start: %w", ErrBusStopped)
	}
	b.started = true
	return nil
}

// Stop gracefully shuts down the bus. All callback goroutines are stopped
// and all channel subscribers' channels are closed. Waits for in-flight
// delivery goroutines to finish (with context timeout).
func (b *Bus) Stop(ctx context.Context) error {
	if b == nil {
		return fmt.Errorf("events: bus: stop: nil bus")
	}
	b.mu.Lock()

	b.stopped = true
	b.started = false

	// Stop all callback delivery goroutines.
	for _, subs := range b.callbacks {
		for _, sub := range subs {
			sub.stop()
		}
	}

	// Close all channel subscriber channels.
	for _, subs := range b.channels {
		for _, sub := range subs {
			sub.closeCh()
		}
	}

	b.callbacks = make(map[string]map[uint64]*callbackSub)
	b.channels = make(map[string]map[uint64]*channelSub)
	b.mu.Unlock()

	// Wait for all delivery goroutines to finish.
	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("events: bus: stop: timed out waiting for delivery goroutines")
	}
	return nil
}

// Health returns an error if the bus is stopped.
func (b *Bus) Health() error {
	if b == nil {
		return fmt.Errorf("events: bus: health: nil bus")
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.stopped {
		return ErrBusStopped
	}
	return nil
}

// Publish sends an event to all subscribers of the event's type.
//
// For most events, delivery is asynchronous: the event is sent non-blocking
// to each subscriber's buffered channel. If a subscriber's buffer is full,
// the event is dropped with a slog.Warn.
//
// Exception: ConfigChanged events are delivered synchronously — callback
// handlers are called directly and channel subscribers receive via blocking send.
func (b *Bus) Publish(ctx context.Context, event core.Event) error {
	if b == nil {
		return fmt.Errorf("events: bus: publish: nil bus")
	}
	if event == nil {
		return fmt.Errorf("events: bus: publish: nil event")
	}

	b.mu.RLock()
	if b.stopped {
		b.mu.RUnlock()
		return ErrBusStopped
	}
	if !b.started {
		b.mu.RUnlock()
		return ErrBusNotStarted
	}

	eventType := event.Type()

	// Snapshot subscribers under read lock.
	cbSubs := make([]*callbackSub, 0, len(b.callbacks[eventType]))
	cbIDs := make([]uint64, 0, len(b.callbacks[eventType]))
	for id, sub := range b.callbacks[eventType] {
		cbSubs = append(cbSubs, sub)
		cbIDs = append(cbIDs, id)
	}

	chSubs := make([]*channelSub, 0, len(b.channels[eventType]))
	chIDs := make([]uint64, 0, len(b.channels[eventType]))
	for id, sub := range b.channels[eventType] {
		chSubs = append(chSubs, sub)
		chIDs = append(chIDs, id)
	}
	b.mu.RUnlock()

	if eventType == EventConfigChanged {
		return b.publishSync(ctx, event, cbSubs, chSubs, chIDs)
	}
	return b.publishAsync(event, cbSubs, cbIDs, chSubs, chIDs)
}

// publishAsync delivers events non-blocking to subscriber channels.
func (b *Bus) publishAsync(event core.Event, cbSubs []*callbackSub, cbIDs []uint64, chSubs []*channelSub, chIDs []uint64) error {
	eventType := event.Type()

	for i, sub := range cbSubs {
		if !sub.trySend(event) {
			slog.Warn("events: bus: event not delivered",
				"type", eventType, "subscriber_id", cbIDs[i])
		}
	}

	for i, sub := range chSubs {
		if !sub.trySend(event) {
			slog.Warn("events: bus: event not delivered",
				"type", eventType, "subscriber_id", chIDs[i])
		}
	}

	return nil
}

// publishSync delivers ConfigChanged events synchronously.
// Callback handlers are called directly. Channel subscribers receive via
// context-aware blocking send.
func (b *Bus) publishSync(ctx context.Context, event core.Event, cbSubs []*callbackSub, chSubs []*channelSub, chIDs []uint64) error {
	// Call callback handlers directly (synchronous). The handlerMu ensures
	// this is serialized with any in-flight async delivery in the goroutine.
	for _, sub := range cbSubs {
		sub.handlerMu.Lock()
		sub.handler(ctx, event)
		sub.handlerMu.Unlock()
	}

	// Blocking send to channel subscribers with context cancellation.
	for i, sub := range chSubs {
		if !sub.trySendSync(ctx, event) {
			slog.Warn("events: bus: sync delivery canceled or subscriber closed",
				"type", event.Type(), "subscriber_id", chIDs[i])
		}
	}

	return nil
}

// Subscribe registers a callback handler for events of the given type.
// Events are delivered asynchronously via a dedicated goroutine per subscriber.
// Returns an unsubscribe function that stops the goroutine and removes the handler.
func (b *Bus) Subscribe(eventType string, handler EventHandler) (unsubscribe func()) {
	if b == nil || handler == nil || eventType == "" {
		return func() {}
	}

	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return func() {}
	}

	if b.callbacks[eventType] == nil {
		b.callbacks[eventType] = make(map[uint64]*callbackSub)
	}

	id := b.nextID
	b.nextID++

	sub := &callbackSub{
		handler: handler,
		ch:      make(chan core.Event, b.bufferSize),
		done:    make(chan struct{}),
	}
	b.callbacks[eventType][id] = sub
	b.mu.Unlock()

	// Launch delivery goroutine.
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case ev, ok := <-sub.ch:
				if !ok {
					return
				}
				// NOTE: Async handlers receive context.Background() because the
				// original publish context may be canceled by the time the handler
				// runs. This is an inherent trade-off of async delivery.
				sub.handlerMu.Lock()
				handler(context.Background(), ev)
				sub.handlerMu.Unlock()
			case <-sub.done:
				return
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.callbacks[eventType], id)
			if len(b.callbacks[eventType]) == 0 {
				delete(b.callbacks, eventType)
			}
			b.mu.Unlock()

			// Stop the delivery goroutine.
			sub.stop()
		})
	}
}

// SubscribeChan returns a read-only event channel and an unsubscribe function.
// The channel is buffered (bufferSize). On unsubscribe, the subscriber is
// removed from the map and the channel is closed (safe for range/select).
func (b *Bus) SubscribeChan(eventType string) (<-chan core.Event, func()) {
	if b == nil || eventType == "" {
		ch := make(chan core.Event)
		close(ch)
		return ch, func() {}
	}

	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		ch := make(chan core.Event)
		close(ch)
		return ch, func() {}
	}

	if b.channels[eventType] == nil {
		b.channels[eventType] = make(map[uint64]*channelSub)
	}

	id := b.nextID
	b.nextID++

	sub := &channelSub{
		ch: make(chan core.Event, b.bufferSize),
	}
	b.channels[eventType][id] = sub
	b.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.channels[eventType], id)
			if len(b.channels[eventType]) == 0 {
				delete(b.channels, eventType)
			}
			b.mu.Unlock()

			// Close channel after removal — trySend checks closed flag
			// so no writes can happen after closeCh returns.
			sub.closeCh()
		})
	}

	return sub.ch, unsub
}

// EventHandler is a type alias for core.EventHandler to keep bus.go self-contained.
type EventHandler = core.EventHandler
