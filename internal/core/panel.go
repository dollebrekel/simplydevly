// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

// PanelPosition identifies where a panel is docked in the TUI layout.
type PanelPosition int

const (
	PanelLeft   PanelPosition = iota
	PanelRight
	PanelBottom
)

// PanelConfig holds the registration parameters for a TUI panel.
type PanelConfig struct {
	Name       string
	Position   PanelPosition
	MinWidth   int
	MaxWidth   int
	Collapsible bool
	Keybind    string
	Icon       string
	MenuLabel  string
	OnActivate  func() error
	LazyInit    bool
	PluginName  string
	// ContentFunc provides simple string content for display-only panels.
	ContentFunc func() string
}

// PanelInfo combines a panel's config with its current runtime state.
type PanelInfo struct {
	Config   PanelConfig
	Active   bool
	Focused  bool
	Width    int
	TabIndex int
}

// PanelRegistry manages TUI panel registration and lifecycle.
type PanelRegistry interface {
	Lifecycle
	Register(cfg PanelConfig) error
	Unregister(name string) error
	Panel(name string) (PanelInfo, bool)
	Panels() []PanelInfo
	Activate(name string) error
	Deactivate(name string) error
}

// compile-time interface check is performed in the implementation package.
