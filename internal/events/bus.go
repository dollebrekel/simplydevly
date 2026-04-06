// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import (
	"context"
	"fmt"
	"sync"

	"siply.dev/siply/internal/core"
)

// Bus implements core.EventBus with thread-safe pub/sub.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]map[uint64]core.EventHandler
	nextID      uint64
	started     bool
	stopped     bool
}

// NewBus creates a new EventBus.
func NewBus() *Bus {
	return &Bus{
		subscribers: make(map[string]map[uint64]core.EventHandler),
	}
}

// Init validates the bus state.
func (b *Bus) Init(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return fmt.Errorf("events: bus has been stopped")
	}
	return nil
}

// Start marks the bus as ready to receive events.
func (b *Bus) Start(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return fmt.Errorf("events: bus has been stopped")
	}
	b.started = true
	return nil
}

// Stop marks the bus as stopped. Further Publish calls will return an error.
func (b *Bus) Stop(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = true
	b.started = false
	// Clear all subscribers.
	b.subscribers = make(map[string]map[uint64]core.EventHandler)
	return nil
}

// Health returns an error if the bus is stopped.
func (b *Bus) Health() error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.stopped {
		return fmt.Errorf("events: bus is stopped")
	}
	return nil
}

// Publish sends an event to all subscribers of the event's type.
// Handlers are called synchronously in registration order.
func (b *Bus) Publish(ctx context.Context, event core.Event) error {
	if event == nil {
		return fmt.Errorf("events: nil event")
	}

	b.mu.RLock()
	if b.stopped {
		b.mu.RUnlock()
		return fmt.Errorf("events: bus is stopped")
	}
	handlers := b.subscribers[event.Type()]
	// Copy handler slice under read lock to avoid holding lock during dispatch.
	sorted := make([]core.EventHandler, 0, len(handlers))
	for _, h := range handlers {
		sorted = append(sorted, h)
	}
	b.mu.RUnlock()

	for _, handler := range sorted {
		handler(ctx, event)
	}
	return nil
}

// Subscribe registers a handler for events of the given type.
// Returns an unsubscribe function that removes the handler.
func (b *Bus) Subscribe(eventType string, handler core.EventHandler) (unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[eventType] == nil {
		b.subscribers[eventType] = make(map[uint64]core.EventHandler)
	}

	id := b.nextID
	b.nextID++
	b.subscribers[eventType][id] = handler

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subscribers[eventType], id)
		if len(b.subscribers[eventType]) == 0 {
			delete(b.subscribers, eventType)
		}
	}
}
