// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"siply.dev/siply/internal/core"
)

// registeredHook wraps a hook function with its configuration and identity.
type registeredPreQueryHook struct {
	id     uint64
	hook   core.PreQueryHook
	config core.HookConfig
}

type registeredPreToolHook struct {
	id     uint64
	hook   core.PreToolHook
	config core.HookConfig
}

// agentHooks implements core.AgentHooks with priority-ordered hook chains,
// timeout enforcement, and failure mode handling.
type agentHooks struct {
	bus core.EventBus

	mu          sync.RWMutex
	preQuery    []registeredPreQueryHook
	preTool     []registeredPreToolHook
	nextID      uint64
	initialized bool
}

// NewAgentHooks creates a new AgentHooks implementation.
// bus is used to publish HookFailedEvent when a SkipOnFailure hook fails.
func NewAgentHooks(bus core.EventBus) core.AgentHooks {
	return &agentHooks{bus: bus}
}

// Lifecycle methods.

func (h *agentHooks) Init(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.bus == nil {
		return fmt.Errorf("hooks: event bus is required")
	}
	h.initialized = true
	return nil
}

func (h *agentHooks) Start(_ context.Context) error { return nil }
func (h *agentHooks) Stop(_ context.Context) error  { return nil }
func (h *agentHooks) Health() error                 { return nil }

// OnPreQuery registers a PreQuery hook with priority ordering.
// Returns an unregister function that removes the hook from the chain.
func (h *agentHooks) OnPreQuery(hook core.PreQueryHook, config core.HookConfig) (unregister func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.nextID
	h.nextID++

	entry := registeredPreQueryHook{id: id, hook: hook, config: config}

	// Insert in priority order (lower = earlier).
	h.preQuery = append(h.preQuery, entry)
	sort.Slice(h.preQuery, func(i, j int) bool {
		return h.preQuery[i].config.Priority < h.preQuery[j].config.Priority
	})

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, e := range h.preQuery {
			if e.id == id {
				h.preQuery = append(h.preQuery[:i], h.preQuery[i+1:]...)
				return
			}
		}
	}
}

// OnPreTool registers a PreTool hook with priority ordering.
// Returns an unregister function that removes the hook from the chain.
func (h *agentHooks) OnPreTool(hook core.PreToolHook, config core.HookConfig) (unregister func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := h.nextID
	h.nextID++

	entry := registeredPreToolHook{id: id, hook: hook, config: config}

	h.preTool = append(h.preTool, entry)
	sort.Slice(h.preTool, func(i, j int) bool {
		return h.preTool[i].config.Priority < h.preTool[j].config.Priority
	})

	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for i, e := range h.preTool {
			if e.id == id {
				h.preTool = append(h.preTool[:i], h.preTool[i+1:]...)
				return
			}
		}
	}
}

// RunPreQuery executes the PreQuery hook chain in priority order.
// Each hook receives the (possibly modified) messages from the previous hook.
// Timeout is enforced per-hook via context.WithTimeout.
func (h *agentHooks) RunPreQuery(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
	// Snapshot under read lock.
	h.mu.RLock()
	snapshot := make([]registeredPreQueryHook, len(h.preQuery))
	copy(snapshot, h.preQuery)
	h.mu.RUnlock()

	current := msgs
	for _, entry := range snapshot {
		timeout := entry.config.Timeout
		if timeout <= 0 {
			timeout = 15 * time.Second // default PreQuery timeout
		}

		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		result, err := entry.hook(hookCtx, current)
		cancel()

		if err != nil {
			if entry.config.OnFailure == core.HookAbortOnFailure {
				return nil, fmt.Errorf("hooks: PreQuery hook failed (abort): %w", err)
			}
			// SkipOnFailure: log warning, publish event, continue with unmodified data.
			slog.Warn("PreQuery hook failed, skipping",
				"priority", entry.config.Priority,
				"hookID", entry.id,
				"error", err,
			)
			h.publishHookFailed(ctx, "PreQuery", entry.id, err)
			continue
		}
		current = result
	}
	return current, nil
}

// RunPreTool executes the PreTool hook chain in priority order.
// Each hook receives the (possibly modified) tool call from the previous hook.
// Timeout is enforced per-hook via context.WithTimeout.
func (h *agentHooks) RunPreTool(ctx context.Context, call core.ToolCall) (core.ToolCall, error) {
	// Snapshot under read lock.
	h.mu.RLock()
	snapshot := make([]registeredPreToolHook, len(h.preTool))
	copy(snapshot, h.preTool)
	h.mu.RUnlock()

	current := call
	for _, entry := range snapshot {
		timeout := entry.config.Timeout
		if timeout <= 0 {
			timeout = 5 * time.Second // default PreTool timeout
		}

		hookCtx, cancel := context.WithTimeout(ctx, timeout)
		result, err := entry.hook(hookCtx, current)
		cancel()

		if err != nil {
			if entry.config.OnFailure == core.HookAbortOnFailure {
				return core.ToolCall{}, fmt.Errorf("hooks: PreTool hook failed (abort): %w", err)
			}
			slog.Warn("PreTool hook failed, skipping",
				"priority", entry.config.Priority,
				"hookID", entry.id,
				"error", err,
			)
			h.publishHookFailed(ctx, "PreTool", entry.id, err)
			continue
		}
		current = result
	}
	return current, nil
}

// publishHookFailed publishes a HookFailedEvent to the EventBus.
func (h *agentHooks) publishHookFailed(ctx context.Context, point string, hookID uint64, hookErr error) {
	if h.bus == nil {
		return
	}
	event := &HookFailedEvent{
		HookName:   fmt.Sprintf("hook-%d", hookID),
		Point:      point,
		Err:        hookErr.Error(),
		Fallback:   "continuing with unmodified data",
		CostEffect: "potential increased token usage",
		Ts:         time.Now(),
	}
	if pubErr := h.bus.Publish(ctx, event); pubErr != nil {
		slog.Warn("failed to publish HookFailedEvent", "error", pubErr)
	}
}
