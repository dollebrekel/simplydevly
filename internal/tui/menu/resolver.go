// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
)

// ResolvedKeybinding represents a single keybinding after four-layer merge resolution.
type ResolvedKeybinding struct {
	Key        string
	Action     string
	Source     string // "system", "plugin:<name>", "global", "project"
	PluginName string
	IsForced   bool
	OverrideOf string // source that was overridden, empty if none
}

// KeybindingResolver merges keybindings from four layers:
// system → plugin → global → project (with force-global override).
type KeybindingResolver struct {
	mu      sync.RWMutex
	system  []KeyBindingCategory
	plugins []core.Keybinding
	global  *config.KeybindingConfig
	project *config.KeybindingConfig
}

// NewKeybindingResolver creates a resolver with all four layers.
func NewKeybindingResolver(
	system []KeyBindingCategory,
	plugins []core.Keybinding,
	global *config.KeybindingConfig,
	project *config.KeybindingConfig,
) *KeybindingResolver {
	return &KeybindingResolver{
		system:  system,
		plugins: plugins,
		global:  global,
		project: project,
	}
}

// SetPlugins updates the plugin keybindings layer. Thread-safe.
func (r *KeybindingResolver) SetPlugins(plugins []core.Keybinding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = plugins
}

// Resolve merges all four layers and returns the resolved keybinding set.
func (r *KeybindingResolver) Resolve() []ResolvedKeybinding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resolveLocked()
}

// ActionForKey returns the resolved action for a normalized key.
func (r *KeybindingResolver) ActionForKey(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key = strings.ToLower(strings.TrimSpace(key))
	for _, rb := range r.resolveLocked() {
		if rb.Key == key {
			return rb.Action, true
		}
	}
	return "", false
}

// resolveLocked performs the merge. Caller must hold r.mu (read or write).
func (r *KeybindingResolver) resolveLocked() []ResolvedKeybinding {
	byKey := make(map[string]ResolvedKeybinding)
	var order []string

	for _, cat := range r.system {
		for _, kb := range cat.Bindings {
			key := strings.ToLower(kb.Key)
			if _, exists := byKey[key]; !exists {
				order = append(order, key)
			}
			byKey[key] = ResolvedKeybinding{
				Key:    key,
				Action: kb.Action,
				Source: "system",
			}
		}
	}

	for _, kb := range r.plugins {
		key := strings.ToLower(kb.Key)
		src := fmt.Sprintf("plugin:%s", kb.PluginName)
		existing, exists := byKey[key]
		rb := ResolvedKeybinding{
			Key:        key,
			Action:     kb.Description,
			Source:     src,
			PluginName: kb.PluginName,
		}
		if exists {
			rb.OverrideOf = existing.Source
		}
		byKey[key] = rb
		if !exists {
			order = append(order, key)
		}
	}

	forceKeys := make(map[string]bool)
	if r.global != nil {
		for _, kb := range r.global.Keybindings {
			key := strings.ToLower(kb.Key)
			existing, exists := byKey[key]
			rb := ResolvedKeybinding{
				Key:        key,
				Action:     kb.Action,
				Source:     "global",
				IsForced:   kb.Force,
				PluginName: existing.PluginName,
			}
			if exists {
				rb.OverrideOf = existing.Source
			}
			byKey[key] = rb
			if !exists {
				order = append(order, key)
			}
			if kb.Force {
				forceKeys[key] = true
			}
		}
	}

	if r.project != nil {
		for _, kb := range r.project.Keybindings {
			key := strings.ToLower(kb.Key)
			if forceKeys[key] {
				continue
			}
			existing, exists := byKey[key]
			rb := ResolvedKeybinding{
				Key:        key,
				Action:     kb.Action,
				Source:     "project",
				PluginName: existing.PluginName,
			}
			if exists {
				rb.OverrideOf = existing.Source
			}
			byKey[key] = rb
			if !exists {
				order = append(order, key)
			}
		}
	}

	result := make([]ResolvedKeybinding, 0, len(order))
	for _, key := range order {
		result = append(result, byKey[key])
	}
	return result
}

// LogForceWarnings logs once for any force-global keys that block project overrides.
// Call once after construction or SetPlugins, not per-render.
func (r *KeybindingResolver) LogForceWarnings() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.global == nil || r.project == nil {
		return
	}
	forceKeys := make(map[string]bool)
	for _, kb := range r.global.Keybindings {
		if kb.Force {
			forceKeys[strings.ToLower(kb.Key)] = true
		}
	}
	for _, kb := range r.project.Keybindings {
		if forceKeys[strings.ToLower(kb.Key)] {
			slog.Warn("keybinding: force-global blocks project override", "key", strings.ToLower(kb.Key))
		}
	}
}

// ResolveToCategories groups resolved bindings into categories for Learn view display.
// System categories appear first (preserving original 5), then one category per plugin.
func (r *KeybindingResolver) ResolveToCategories() []KeyBindingCategory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resolved := r.resolveLocked()

	resolvedByKey := make(map[string]ResolvedKeybinding, len(resolved))
	for _, rb := range resolved {
		resolvedByKey[rb.Key] = rb
	}

	cats := make([]KeyBindingCategory, 0, len(r.system)+4)
	for _, sc := range r.system {
		cat := KeyBindingCategory{Name: sc.Name}
		for _, kb := range sc.Bindings {
			key := strings.ToLower(kb.Key)
			rb, ok := resolvedByKey[key]
			action := kb.Action
			if ok && rb.OverrideOf != "" {
				action = fmt.Sprintf("%s ⚙ (%s)", rb.Action, rb.Source)
			}
			cat.Bindings = append(cat.Bindings, KeyBinding{
				Key:      kb.Key,
				Action:   action,
				Category: sc.Name,
			})
		}
		cats = append(cats, cat)
	}

	pluginGroups := make(map[string][]ResolvedKeybinding)
	var pluginOrder []string
	for _, rb := range resolved {
		if rb.PluginName == "" {
			continue
		}
		if _, seen := pluginGroups[rb.PluginName]; !seen {
			pluginOrder = append(pluginOrder, rb.PluginName)
		}
		pluginGroups[rb.PluginName] = append(pluginGroups[rb.PluginName], rb)
	}

	for _, pn := range pluginOrder {
		cat := KeyBindingCategory{Name: pn}
		for _, rb := range pluginGroups[pn] {
			action := rb.Action
			if rb.OverrideOf != "" {
				action = fmt.Sprintf("%s ⚙ (%s)", rb.Action, rb.Source)
			}
			cat.Bindings = append(cat.Bindings, KeyBinding{
				Key:      rb.Key,
				Action:   action,
				Category: pn,
			})
		}
		cats = append(cats, cat)
	}

	return cats
}
