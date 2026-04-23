// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"context"
	"time"
)

// AgentHooks manages typed PreQuery + PreTool hooks with priority & failure modes.
type AgentHooks interface {
	Lifecycle
	// OnPreQuery registers a typed hook with configuration. Returns unregister function.
	OnPreQuery(hook PreQueryHook, config HookConfig) (unregister func())
	// OnPreTool registers a typed hook with configuration. Returns unregister function.
	OnPreTool(hook PreToolHook, config HookConfig) (unregister func())
	// RunPreQuery executes hook chain (called by agent loop).
	RunPreQuery(ctx context.Context, msgs []Message) ([]Message, error)
	// RunPreTool executes hook chain (called by agent loop).
	RunPreTool(ctx context.Context, call ToolCall) (ToolCall, error)
}

// PreQueryHook is a typed hook function for pre-query processing.
// PostQuery and PostTool are observe-only → use EventBus.Subscribe() instead.
type PreQueryHook func(ctx context.Context, msgs []Message) ([]Message, error)

// PreToolHook is a typed hook function for pre-tool processing.
type PreToolHook func(ctx context.Context, call ToolCall) (ToolCall, error)

// HookConfig configures hook execution behavior.
type HookConfig struct {
	Priority  int           // 0-100, lower = earlier in chain
	OnFailure FailureMode   // What to do when hook fails
	Timeout   time.Duration // Default: 15s PreQuery, 5s PreTool
}

// FailureMode determines hook failure behavior.
type FailureMode int

const (
	HookSkipOnFailure  FailureMode = iota // Best effort: log warning, continue with original data
	HookAbortOnFailure                    // Must succeed: propagate error, stop execution
)

// HookPoint identifies where in the agent loop a hook executes.
type HookPoint int

const (
	HookPreQuery HookPoint = iota
	HookPreTool
)
