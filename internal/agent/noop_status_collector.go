package agent

import (
	"context"
	"sync"

	"siply.dev/siply/internal/core"
)

// NoopStatusCollector implements core.StatusCollector with no-op behavior.
// Real TUI status display is deferred to Epic 4.
type NoopStatusCollector struct {
	mu      sync.Mutex
	updates []core.StatusUpdate
}

func (n *NoopStatusCollector) Init(_ context.Context) error  { return nil }
func (n *NoopStatusCollector) Start(_ context.Context) error { return nil }
func (n *NoopStatusCollector) Stop(_ context.Context) error  { return nil }
func (n *NoopStatusCollector) Health() error                 { return nil }

// Publish stores the update for Snapshot but otherwise does nothing.
func (n *NoopStatusCollector) Publish(update core.StatusUpdate) {
	n.mu.Lock()
	n.updates = append(n.updates, update)
	n.mu.Unlock()
}

// Subscribe returns a channel that never receives and a no-op unsubscribe.
func (n *NoopStatusCollector) Subscribe() (updates <-chan core.StatusUpdate, unsubscribe func()) {
	ch := make(chan core.StatusUpdate, 1)
	var once sync.Once
	return ch, func() { once.Do(func() { close(ch) }) }
}

// Snapshot returns the most recent status update per source.
func (n *NoopStatusCollector) Snapshot() map[string]core.StatusUpdate {
	n.mu.Lock()
	defer n.mu.Unlock()
	snap := make(map[string]core.StatusUpdate)
	for _, u := range n.updates {
		snap[u.Source] = u
	}
	return snap
}
