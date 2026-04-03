package core

import "context"

// ActionVerdict represents the outcome of a permission evaluation.
type ActionVerdict int

const (
	Deny  ActionVerdict = iota
	Ask
	Allow
)

// Action describes a requested operation for permission evaluation.
type Action struct {
	Source      string
	Tool        string
	Target      string
	Destructive bool
}

// CapabilityVerdict holds the result of a plugin capability evaluation.
// Placeholder — detailed in Story 5.x.
type CapabilityVerdict struct{}

// PermissionEvaluator evaluates permissions for actions and plugin capabilities.
type PermissionEvaluator interface {
	Lifecycle
	EvaluateCapabilities(ctx context.Context, plugin PluginMeta) (CapabilityVerdict, error)
	EvaluateAction(ctx context.Context, action Action) (ActionVerdict, error)
}
