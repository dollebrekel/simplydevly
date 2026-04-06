// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "context"

// PluginMeta holds metadata about an installed plugin.
type PluginMeta struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Tier         int      `json:"tier"`         // 1=YAML, 2=Lua, 3=Go/gRPC — see plugin architecture docs
	Capabilities []string `json:"capabilities"`
}

// PluginRegistry manages plugin installation and lifecycle.
type PluginRegistry interface {
	Lifecycle
	Install(ctx context.Context, source string) error
	Load(ctx context.Context, name string) error
	List(ctx context.Context) ([]PluginMeta, error)
	Remove(ctx context.Context, name string) error
	DevMode(ctx context.Context, path string) error
}
