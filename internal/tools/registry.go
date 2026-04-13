// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"siply.dev/siply/internal/core"
)

// Tool is the interface that individual tools implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Destructive() bool
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// Registry implements core.ToolExecutor by dispatching to registered tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	perm  core.PermissionEvaluator
}

// NewRegistry creates a Registry with the given permission evaluator.
func NewRegistry(perm core.PermissionEvaluator) *Registry {
	if perm == nil {
		panic("tools: NewRegistry requires a non-nil PermissionEvaluator")
	}
	return &Registry{
		tools: make(map[string]Tool),
		perm:  perm,
	}
}

// Register adds a tool to the registry. Returns an error if a tool with
// the same name is already registered.
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = tool
	return nil
}

// Execute looks up a tool by name, checks permissions, and runs it.
func (r *Registry) Execute(ctx context.Context, req core.ToolRequest) (core.ToolResponse, error) {
	r.mu.RLock()
	tool, exists := r.tools[req.Name]
	r.mu.RUnlock()

	if !exists {
		return core.ToolResponse{}, core.ErrToolNotFound
	}

	// Build action for permission evaluation.
	action := core.Action{
		Source:      req.Source,
		Tool:        req.Name,
		Target:      extractTarget(req),
		Destructive: tool.Destructive(),
	}

	verdict, err := r.perm.EvaluateAction(ctx, action)
	if err != nil {
		return core.ToolResponse{}, fmt.Errorf("evaluating permission: %w", err)
	}

	switch verdict {
	case core.Deny:
		return core.ToolResponse{IsError: true, Output: "permission denied"}, core.ErrPermissionDenied
	case core.Ask:
		return core.ToolResponse{IsError: true, Output: "confirmation required"}, nil
	case core.Allow:
		// proceed
	default:
		return core.ToolResponse{}, fmt.Errorf("unknown permission verdict: %d", verdict)
	}

	start := time.Now()
	output, execErr := tool.Execute(ctx, req.Input)
	duration := time.Since(start)

	resp := core.ToolResponse{
		Output:   output,
		IsError:  execErr != nil,
		Duration: duration,
	}
	return resp, execErr
}

// ListTools returns tool definitions for all registered tools.
func (r *Registry) ListTools() []core.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]core.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, core.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}

	// Sort alphabetically for deterministic ordering across all providers.
	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})

	return defs
}

// GetTool returns the definition of a single tool by name.
func (r *Registry) GetTool(name string) (core.ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, exists := r.tools[name]
	if !exists {
		return core.ToolDefinition{}, core.ErrToolNotFound
	}
	return core.ToolDefinition{
		Name:        t.Name(),
		Description: t.Description(),
		InputSchema: t.InputSchema(),
	}, nil
}

// Init registers the built-in tools.
func (r *Registry) Init(_ context.Context) error {
	builtins := []Tool{
		&FileReadTool{},
		&FileWriteTool{},
		&FileEditTool{},
		&BashTool{},
		&SearchTool{},
		&WebTool{},
	}
	for _, t := range builtins {
		if err := r.Register(t); err != nil {
			return fmt.Errorf("registering built-in tool %q: %w", t.Name(), err)
		}
	}
	return nil
}

// Start is a no-op for the tool registry.
func (r *Registry) Start(_ context.Context) error { return nil }

// Stop is a no-op for the tool registry.
func (r *Registry) Stop(_ context.Context) error { return nil }

// Health returns nil — the registry is always healthy.
func (r *Registry) Health() error { return nil }

// extractTarget pulls a human-readable target from the tool request input.
// For file tools this is the path; for bash, the command; for web, the URL.
func extractTarget(req core.ToolRequest) string {
	var generic struct {
		Path    string `json:"path"`
		Command string `json:"command"`
		URL     string `json:"url"`
		Pattern string `json:"pattern"`
	}
	if err := json.Unmarshal(req.Input, &generic); err != nil {
		return ""
	}
	switch {
	case generic.Path != "":
		return generic.Path
	case generic.Command != "":
		return generic.Command
	case generic.URL != "":
		return generic.URL
	case generic.Pattern != "":
		return generic.Pattern
	default:
		return ""
	}
}
