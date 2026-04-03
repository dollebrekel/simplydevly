package core

import "context"

// PluginMeta holds metadata about an installed plugin.
type PluginMeta struct {
	Name         string
	Version      string
	Tier         int
	Capabilities []string
}

// PluginRegistry manages plugin installation and lifecycle.
type PluginRegistry interface {
	Lifecycle
	Install(ctx context.Context, source string) error
	Load(ctx context.Context, name string) error
	List() []PluginMeta
	Remove(ctx context.Context, name string) error
	DevMode(ctx context.Context, path string) error
}
