// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"context"
	"time"
)

// Event represents a system event.
type Event interface {
	Type() string
	Timestamp() time.Time
}

// EventHandler processes events.
type EventHandler func(ctx context.Context, event Event)

// EventBus manages event publication and subscription.
type EventBus interface {
	Lifecycle
	Publish(ctx context.Context, event Event) error
	Subscribe(eventType string, handler EventHandler) (unsubscribe func())
	SubscribeChan(eventType string) (<-chan Event, func())
}
