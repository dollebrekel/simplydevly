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

	byKey := make(map[string]ResolvedKeybinding)
	var order []string

	for _, cat := range r.system {
		for _, kb := range cat.Bindings {
			key := strings.ToLower(kb.Key)
			byKey[key] = ResolvedKeybinding{
				Key:    key,
				Action: kb.Action,
				Source: "system",
			}
			order = append(order, key)
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
				Key:      key,
				Action:   kb.Action,
				Source:   "global",
				IsForced: kb.Force,
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
				slog.Warn("keybinding: force-global blocks project override", "key", key)
				continue
			}
			existing, exists := byKey[key]
			rb := ResolvedKeybinding{
				Key:    key,
				Action: kb.Action,
				Source: "project",
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

// ResolveToCategories groups resolved bindings into categories for Learn view display.
// System categories appear first (preserving original 5), then one category per plugin.
func (r *KeybindingResolver) ResolveToCategories() []KeyBindingCategory {
	resolved := r.Resolve()

	resolvedByKey := make(map[string]ResolvedKeybinding, len(resolved))
	for _, rb := range resolved {
		resolvedByKey[rb.Key] = rb
	}

	r.mu.RLock()
	systemCats := r.system
	r.mu.RUnlock()

	cats := make([]KeyBindingCategory, 0, len(systemCats)+4)
	for _, sc := range systemCats {
		cat := KeyBindingCategory{Name: sc.Name}
		for _, kb := range sc.Bindings {
			key := strings.ToLower(kb.Key)
			rb, ok := resolvedByKey[key]
			action := kb.Action
			if ok && rb.OverrideOf != "" {
				action = fmt.Sprintf("%s ⚙ (%s override)", rb.Action, rb.Source)
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
		if rb.Source != fmt.Sprintf("plugin:%s", rb.PluginName) {
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
			cat.Bindings = append(cat.Bindings, KeyBinding{
				Key:      rb.Key,
				Action:   rb.Action,
				Category: pn,
			})
		}
		cats = append(cats, cat)
	}

	return cats
}
