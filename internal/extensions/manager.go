// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package extensions

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/plugins"
)

const defaultCategory = "Extensions"

// builtinKeybinds lists keybinds reserved by the core TUI.
var builtinKeybinds = map[string]bool{
	"ctrl+c":     true,
	"ctrl+@":     true,
	"ctrl+space": true,
	"ctrl+b":     true,
	"tab":        true,
	"shift+tab":  true,
	"alt+left":   true,
	"alt+right":  true,
	"ctrl+]":     true,
	"ctrl+[":     true,
	"esc":        true,
	"q":          true,
}

// ContentProvider creates a ContentFunc for a plugin panel.
// The returned function is called on every render with the available panel dimensions.
type ContentProvider func(pluginName string) func(width, height int) string

// ActionProvider sends an action with payload to a plugin.
// Used for forwarding key presses and mouse clicks to plugin panels.
type ActionProvider func(pluginName, action string, payload []byte)

// Manager implements the ExtensionRegistration and Lifecycle interfaces.
// It coordinates panel, menu item, and keybinding registration for plugins.
type Manager struct {
	registrations   map[string][]core.Registration
	panelRegistry   core.PanelRegistry
	eventBus        core.EventBus
	pluginsDir      string
	contentProvider ContentProvider
	actionProvider  ActionProvider
	mu              sync.RWMutex
	started         bool

	unsubLoaded  func()
	unsubCrashed func()
}

// NewManager creates a new ExtensionManager with the given dependencies.
// pluginsDir is the path to ~/.siply/plugins/ for manifest-based auto-registration.
func NewManager(pr core.PanelRegistry, eb core.EventBus, pluginsDir string) *Manager {
	return &Manager{
		registrations: make(map[string][]core.Registration),
		panelRegistry: pr,
		eventBus:      eb,
		pluginsDir:    pluginsDir,
	}
}

// SetContentProvider sets a factory that creates ContentFunc closures for plugin panels.
func (m *Manager) SetContentProvider(cp ContentProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contentProvider = cp
}

// SetActionProvider sets a function for sending actions to plugin panels.
func (m *Manager) SetActionProvider(ap ActionProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionProvider = ap
}

// SendAction forwards an action to a plugin. Thread-safe.
func (m *Manager) SendAction(pluginName, action string, payload []byte) {
	m.mu.RLock()
	ap := m.actionProvider
	m.mu.RUnlock()
	if ap != nil {
		ap(pluginName, action, payload)
	}
}

// RegisterPanel validates a PanelConfig and delegates to PanelRegistry.
func (m *Manager) RegisterPanel(cfg core.PanelConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("%w: panel name is empty", core.ErrExtensionAlreadyRegistered)
	}
	if cfg.PluginName == "" {
		return fmt.Errorf("%w: panel plugin name is empty", core.ErrExtensionAlreadyRegistered)
	}
	switch cfg.Position {
	case core.PanelLeft, core.PanelRight, core.PanelBottom:
	default:
		return fmt.Errorf("%w: invalid panel position %d", core.ErrExtensionAlreadyRegistered, cfg.Position)
	}

	m.mu.Lock()
	for _, reg := range m.registrations[cfg.PluginName] {
		if reg.Kind == core.RegistrationPanel {
			if pc, ok := reg.Details.(core.PanelConfig); ok && pc.Name == cfg.Name {
				m.mu.Unlock()
				return fmt.Errorf("%w: panel %q for plugin %q", core.ErrExtensionAlreadyRegistered, cfg.Name, cfg.PluginName)
			}
		}
	}
	m.registrations[cfg.PluginName] = append(m.registrations[cfg.PluginName], core.Registration{
		Kind:       core.RegistrationPanel,
		PluginName: cfg.PluginName,
		Details:    cfg,
	})
	m.mu.Unlock()

	if err := m.panelRegistry.Register(cfg); err != nil {
		m.mu.Lock()
		regs := m.registrations[cfg.PluginName]
		for i, reg := range regs {
			if reg.Kind == core.RegistrationPanel {
				if pc, ok := reg.Details.(core.PanelConfig); ok && pc.Name == cfg.Name {
					m.registrations[cfg.PluginName] = append(regs[:i], regs[i+1:]...)
					break
				}
			}
		}
		m.mu.Unlock()
		return fmt.Errorf("extension: register panel: %w", err)
	}

	return nil
}

// RegisterMenuItem validates a MenuItem and stores it in the registrations map.
func (m *Manager) RegisterMenuItem(item core.MenuItem) error {
	if item.Label == "" {
		return fmt.Errorf("%w: menu item label is empty", core.ErrMenuItemDuplicate)
	}
	if item.PluginName == "" {
		return fmt.Errorf("%w: menu item plugin name is empty", core.ErrMenuItemDuplicate)
	}

	if item.Category == "" {
		item.Category = defaultCategory
	}

	m.mu.Lock()
	for pn, regs := range m.registrations {
		for _, reg := range regs {
			if reg.Kind == core.RegistrationMenu {
				if mi, ok := reg.Details.(core.MenuItem); ok {
					if mi.Label == item.Label && mi.Category == item.Category {
						m.mu.Unlock()
						return fmt.Errorf("%w: label %q in category %q (plugin %q)", core.ErrMenuItemDuplicate, item.Label, item.Category, pn)
					}
				}
			}
		}
	}

	m.registrations[item.PluginName] = append(m.registrations[item.PluginName], core.Registration{
		Kind:       core.RegistrationMenu,
		PluginName: item.PluginName,
		Details:    item,
	})
	shouldPublish := m.eventBus != nil && m.started
	m.mu.Unlock()

	if shouldPublish {
		_ = m.eventBus.Publish(context.Background(), events.NewMenuChangedEvent())
	}

	return nil
}

// RegisterMenuItemForPlugin registers a menu item for a specific plugin.
func (m *Manager) RegisterMenuItemForPlugin(pluginName string, item core.MenuItem) error {
	if item.Label == "" {
		return fmt.Errorf("%w: menu item label is empty", core.ErrMenuItemDuplicate)
	}
	if pluginName == "" {
		return fmt.Errorf("%w: plugin name is empty", core.ErrMenuItemDuplicate)
	}

	item.PluginName = pluginName
	if item.Category == "" {
		item.Category = defaultCategory
	}

	m.mu.Lock()
	for pn, regs := range m.registrations {
		for _, reg := range regs {
			if reg.Kind == core.RegistrationMenu {
				if mi, ok := reg.Details.(core.MenuItem); ok {
					if mi.Label == item.Label && mi.Category == item.Category {
						m.mu.Unlock()
						return fmt.Errorf("%w: label %q in category %q (plugin %q)", core.ErrMenuItemDuplicate, item.Label, item.Category, pn)
					}
				}
			}
		}
	}

	m.registrations[pluginName] = append(m.registrations[pluginName], core.Registration{
		Kind:       core.RegistrationMenu,
		PluginName: pluginName,
		Details:    item,
	})
	shouldPublish := m.eventBus != nil && m.started
	m.mu.Unlock()

	if shouldPublish {
		_ = m.eventBus.Publish(context.Background(), events.NewMenuChangedEvent())
	}

	return nil
}

// RegisterKeybinding validates a Keybinding and stores it.
func (m *Manager) RegisterKeybinding(kb core.Keybinding) error {
	if kb.Key == "" {
		return fmt.Errorf("%w: keybinding key is empty", core.ErrKeybindConflict)
	}
	if kb.PluginName == "" {
		return fmt.Errorf("%w: keybinding plugin name is empty", core.ErrKeybindConflict)
	}

	normalizedKey := strings.ToLower(kb.Key)
	kb.Key = normalizedKey

	if builtinKeybinds[normalizedKey] {
		return fmt.Errorf("%w: key %q is a built-in keybinding", core.ErrKeybindConflict, normalizedKey)
	}

	m.mu.Lock()
	for pluginName, regs := range m.registrations {
		for _, reg := range regs {
			if reg.Kind == core.RegistrationKeybind {
				if existingKB, ok := reg.Details.(core.Keybinding); ok && existingKB.Key == normalizedKey {
					m.mu.Unlock()
					return fmt.Errorf("%w: key %q already registered by plugin %q", core.ErrKeybindConflict, normalizedKey, pluginName)
				}
			}
		}
	}

	m.registrations[kb.PluginName] = append(m.registrations[kb.PluginName], core.Registration{
		Kind:       core.RegistrationKeybind,
		PluginName: kb.PluginName,
		Details:    kb,
	})
	shouldPublish := m.eventBus != nil && m.started
	m.mu.Unlock()

	if shouldPublish {
		_ = m.eventBus.Publish(context.Background(), events.NewKeybindChangedEvent())
	}

	return nil
}

// Registrations returns all registrations for a given plugin.
func (m *Manager) Registrations(pluginName string) []core.Registration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	regs := m.registrations[pluginName]
	if len(regs) == 0 {
		return nil
	}

	cp := make([]core.Registration, len(regs))
	copy(cp, regs)
	return cp
}

// AllMenuItems returns all registered menu items across all plugins.
func (m *Manager) AllMenuItems() []core.MenuItem {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var items []core.MenuItem
	for _, regs := range m.registrations {
		for _, reg := range regs {
			if reg.Kind == core.RegistrationMenu {
				if mi, ok := reg.Details.(core.MenuItem); ok {
					items = append(items, mi)
				}
			}
		}
	}
	return items
}

// AllKeybindings returns all registered keybindings across all plugins.
func (m *Manager) AllKeybindings() []core.Keybinding {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bindings []core.Keybinding
	for _, regs := range m.registrations {
		for _, reg := range regs {
			if reg.Kind == core.RegistrationKeybind {
				if kb, ok := reg.Details.(core.Keybinding); ok {
					bindings = append(bindings, kb)
				}
			}
		}
	}
	return bindings
}

// UnregisterAll removes all registrations for a plugin.
func (m *Manager) UnregisterAll(pluginName string) {
	m.mu.Lock()

	regs, exists := m.registrations[pluginName]
	if !exists {
		m.mu.Unlock()
		return
	}

	hadPanels := false
	hadMenus := false
	hadKeybinds := false

	for _, reg := range regs {
		switch reg.Kind {
		case core.RegistrationPanel:
			hadPanels = true
			if pc, ok := reg.Details.(core.PanelConfig); ok {
				if err := m.panelRegistry.Unregister(pc.Name); err != nil {
					slog.Warn("extension: failed to unregister panel", "panel", pc.Name, "plugin", pluginName, "error", err)
				}
			}
		case core.RegistrationMenu:
			hadMenus = true
		case core.RegistrationKeybind:
			hadKeybinds = true
		}
	}

	delete(m.registrations, pluginName)
	m.mu.Unlock()

	if m.eventBus != nil && m.started {
		if hadPanels || hadMenus {
			_ = m.eventBus.Publish(context.Background(), events.NewMenuChangedEvent())
		}
		if hadKeybinds {
			_ = m.eventBus.Publish(context.Background(), events.NewKeybindChangedEvent())
		}
	}
}

// Init subscribes to plugin lifecycle events.
func (m *Manager) Init(_ context.Context) error {
	if m.eventBus == nil {
		return nil
	}

	m.unsubLoaded = m.eventBus.Subscribe("plugin.loaded", m.handlePluginLoaded)
	m.unsubCrashed = m.eventBus.Subscribe("plugin.crashed", m.handlePluginCrashed)

	return nil
}

// Start marks the manager as started.
func (m *Manager) Start(_ context.Context) error {
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	return nil
}

// Stop unsubscribes from events and clears registrations.
func (m *Manager) Stop(_ context.Context) error {
	if m.unsubLoaded != nil {
		m.unsubLoaded()
	}
	if m.unsubCrashed != nil {
		m.unsubCrashed()
	}

	m.mu.Lock()
	m.started = false
	var panelsToUnregister []string
	for _, regs := range m.registrations {
		for _, reg := range regs {
			if reg.Kind == core.RegistrationPanel {
				if pc, ok := reg.Details.(core.PanelConfig); ok {
					panelsToUnregister = append(panelsToUnregister, pc.Name)
				}
			}
		}
	}
	m.registrations = make(map[string][]core.Registration)
	m.mu.Unlock()

	for _, name := range panelsToUnregister {
		_ = m.panelRegistry.Unregister(name)
	}

	return nil
}

// Health returns nil if the manager is operational.
func (m *Manager) Health() error {
	return nil
}

func (m *Manager) handlePluginLoaded(_ context.Context, event core.Event) {
	ev, ok := event.(*events.PluginLoadedEvent)
	if !ok {
		slog.Warn("extension: plugin loaded event has unexpected type", "type", event.Type())
		return
	}

	slog.Info("extension: plugin loaded, auto-registering extensions", "plugin", ev.Name, "tier", ev.Tier)

	if m.pluginsDir == "" {
		slog.Debug("extension: pluginsDir not set, skipping manifest-based auto-registration")
		return
	}

	manifest, err := plugins.LoadManifestFromDir(filepath.Join(m.pluginsDir, ev.Name))
	if err != nil {
		slog.Warn("extension: could not load manifest for auto-registration", "plugin", ev.Name, "error", err)
		return
	}

	if manifest.Spec.Extensions == nil {
		return
	}

	ext := manifest.Spec.Extensions

	for _, p := range ext.Panels {
		pos := core.PanelLeft
		switch p.Position {
		case "right":
			pos = core.PanelRight
		case "bottom":
			pos = core.PanelBottom
		}
		cfg := core.PanelConfig{
			Name:        p.Name,
			PluginName:  ev.Name,
			Position:    pos,
			MinWidth:    p.MinWidth,
			MaxWidth:    p.MaxWidth,
			Collapsible: p.Collapsible,
			Keybind:     p.Keybind,
			Icon:        p.Icon,
			MenuLabel:   p.MenuLabel,
		}
		m.mu.RLock()
		cp := m.contentProvider
		m.mu.RUnlock()
		if cp != nil {
			cfg.ContentFunc = cp(ev.Name)
		}
		if err := m.RegisterPanel(cfg); err != nil {
			slog.Warn("extension: auto-register panel failed", "panel", p.Name, "plugin", ev.Name, "error", err)
		}
	}

	for _, mi := range ext.MenuItems {
		item := core.MenuItem{
			Label:      mi.Label,
			Icon:       mi.Icon,
			Keybind:    mi.Keybind,
			Category:   mi.Category,
			PluginName: ev.Name,
		}
		if err := m.RegisterMenuItem(item); err != nil {
			slog.Warn("extension: auto-register menu item failed", "label", mi.Label, "plugin", ev.Name, "error", err)
		}
	}

	for _, kb := range ext.Keybinds {
		binding := core.Keybinding{
			Key:         kb.Key,
			Description: kb.Description,
			PluginName:  ev.Name,
		}
		if err := m.RegisterKeybinding(binding); err != nil {
			slog.Warn("extension: auto-register keybinding failed", "key", kb.Key, "plugin", ev.Name, "error", err)
		}
	}
}

func (m *Manager) handlePluginCrashed(_ context.Context, event core.Event) {
	var pluginName string

	if ce, ok := event.(*events.PluginCrashedEvent); ok {
		pluginName = ce.Name
	}

	if pluginName == "" {
		slog.Warn("extension: plugin crashed event has no plugin name", "type", event.Type())
		return
	}

	slog.Info("extension: cleaning up crashed plugin", "plugin", pluginName)
	m.UnregisterAll(pluginName)
}

// Compile-time interface checks.
var (
	_ core.ExtensionRegistration = (*Manager)(nil)
	_ core.Lifecycle             = (*Manager)(nil)
)
