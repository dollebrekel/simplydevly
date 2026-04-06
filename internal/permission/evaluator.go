// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package permission

import (
	"context"
	"fmt"
	"sync"

	"siply.dev/siply/internal/core"
)

// Evaluator implements core.PermissionEvaluator by evaluating actions against
// mode-specific rules with a DENY → ASK → ALLOW evaluation chain.
type Evaluator struct {
	mu     sync.RWMutex
	config Config
}

// NewEvaluator creates a permission evaluator with the given configuration.
// Returns an error if the mode is invalid.
func NewEvaluator(cfg Config) (*Evaluator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Evaluator{config: cfg}, nil
}

// Init validates the evaluator configuration.
func (e *Evaluator) Init(_ context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Validate()
}

// Start is a no-op.
func (e *Evaluator) Start(_ context.Context) error { return nil }

// Stop is a no-op.
func (e *Evaluator) Stop(_ context.Context) error { return nil }

// Health always returns nil.
func (e *Evaluator) Health() error { return nil }

// EvaluateAction checks if the given action is allowed under the current mode.
func (e *Evaluator) EvaluateAction(_ context.Context, action core.Action) (core.ActionVerdict, error) {
	e.mu.RLock()
	mode := e.config.Mode
	e.mu.RUnlock()

	rules := rulesForMode(mode)
	verdict := evaluateRules(rules, action, mode)
	return verdict, nil
}

// EvaluateCapabilities is a stub — detailed implementation in Story 5.x.
func (e *Evaluator) EvaluateCapabilities(_ context.Context, _ core.PluginMeta) (core.CapabilityVerdict, error) {
	return core.CapabilityVerdict{}, nil
}

// SetMode changes the permission mode at runtime (thread-safe).
func (e *Evaluator) SetMode(mode Mode) error {
	if !mode.Valid() {
		return fmt.Errorf("permission: invalid mode %q", mode)
	}
	e.mu.Lock()
	e.config.Mode = mode
	e.mu.Unlock()
	return nil
}

// Mode returns the current permission mode.
func (e *Evaluator) Mode() Mode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Mode
}
