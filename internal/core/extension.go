// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import "errors"

// RegistrationKind identifies the type of extension registration.
type RegistrationKind int

const (
	RegistrationPanel   RegistrationKind = iota
	RegistrationMenu
	RegistrationKeybind
)

// MenuItem represents a menu item registered by an extension.
type MenuItem struct {
	Label      string
	Icon       string
	Keybind    string
	Action     func()
	Category   string
	PluginName string
}

// Keybinding represents a keybinding registered by an extension.
type Keybinding struct {
	Key         string
	Description string
	Handler     func() error
	PluginName  string
}

// Registration is a union type representing a single extension registration.
type Registration struct {
	Kind       RegistrationKind
	PluginName string
	Details    any
}

// ExtensionRegistration defines the interface for registering extension components.
type ExtensionRegistration interface {
	RegisterPanel(cfg PanelConfig) error
	RegisterMenuItem(item MenuItem) error
	RegisterKeybinding(kb Keybinding) error
	Registrations(pluginName string) []Registration
}

// Sentinel errors for extension operations.
var (
	ErrExtensionAlreadyRegistered = errors.New("extension: already registered")
	ErrKeybindConflict            = errors.New("extension: keybind conflict")
	ErrMenuItemDuplicate          = errors.New("extension: duplicate menu item")
)
