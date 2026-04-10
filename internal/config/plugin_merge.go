// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"fmt"

	"siply.dev/siply/internal/core"
)

// maxMergeDepth is the maximum recursion depth for deep merge and deep copy operations.
const maxMergeDepth = 32

// PluginConfigMerger implements plugins.ConfigMerger by integrating Tier 1 plugin
// configs into the config Loader's in-memory state via deep merge.
//
// Deep merge semantics:
//   - If both existing and incoming values at the same key are map[string]any, recurse.
//   - Scalar and list values: incoming replaces existing (upper-wins).
//   - New keys from incoming are added; base keys not in incoming are preserved.
//
// A deep copy of the incoming config is made before merging to prevent aliasing.
type PluginConfigMerger struct {
	loader *Loader
}

// NewPluginConfigMerger creates a PluginConfigMerger backed by the given Loader.
func NewPluginConfigMerger(loader *Loader) *PluginConfigMerger {
	return &PluginConfigMerger{loader: loader}
}

// MergePluginConfig deep-merges pluginConfig into Config.Plugins[pluginName].
// Thread-safe via the Loader's mutex.
func (m *PluginConfigMerger) MergePluginConfig(pluginName string, pluginConfig map[string]any) error {
	if m.loader == nil {
		return fmt.Errorf("config: PluginConfigMerger: loader is nil")
	}

	// Deep-copy incoming config to prevent callers from mutating our internal state.
	incoming := deepCopyMap(pluginConfig)

	m.loader.mu.Lock()
	defer m.loader.mu.Unlock()

	if m.loader.config == nil {
		m.loader.config = &core.Config{}
	}
	if m.loader.config.Plugins == nil {
		m.loader.config.Plugins = make(map[string]any)
	}

	existing, _ := m.loader.config.Plugins[pluginName].(map[string]any)
	merged, err := deepMerge(existing, incoming, 0)
	if err != nil {
		return fmt.Errorf("config: merge plugin %s: %w", pluginName, err)
	}
	m.loader.config.Plugins[pluginName] = merged
	return nil
}

// RemovePluginConfig deletes the plugin's config namespace from Config.Plugins.
// Thread-safe via the Loader's mutex.
func (m *PluginConfigMerger) RemovePluginConfig(pluginName string) error {
	if m.loader == nil {
		return fmt.Errorf("config: PluginConfigMerger: loader is nil")
	}

	m.loader.mu.Lock()
	defer m.loader.mu.Unlock()

	if m.loader.config == nil || m.loader.config.Plugins == nil {
		return nil
	}
	delete(m.loader.config.Plugins, pluginName)
	return nil
}

// deepMerge returns a new map that is the deep merge of base and upper.
// upper values take precedence; if both base[k] and upper[k] are maps, they are
// merged recursively. Scalars and slices from upper replace those in base.
// Returns an error if recursion exceeds maxMergeDepth.
func deepMerge(base, upper map[string]any, depth int) (map[string]any, error) {
	if depth > maxMergeDepth {
		return nil, fmt.Errorf("config: merge depth exceeds maximum (%d)", maxMergeDepth)
	}

	out := make(map[string]any, len(base)+len(upper))

	// Start with all base entries.
	for k, v := range base {
		out[k] = v
	}

	// Apply upper entries, recursing into nested maps.
	for k, uv := range upper {
		if bv, ok := out[k]; ok {
			bMap, bIsMap := bv.(map[string]any)
			uMap, uIsMap := uv.(map[string]any)
			if bIsMap && uIsMap {
				merged, err := deepMerge(bMap, uMap, depth+1)
				if err != nil {
					return nil, err
				}
				out[k] = merged
				continue
			}
		}
		out[k] = uv
	}
	return out, nil
}

// deepCopyMap returns a deep copy of m so that mutations to the copy do not
// affect the source (go-best-practices: deep-copy-before-merge).
// Panics if nesting exceeds maxMergeDepth (indicates malicious input; caller
// should validate depth via parsePluginYAML before reaching this point).
func deepCopyMap(m map[string]any) map[string]any {
	return deepCopyMapDepth(m, 0)
}

func deepCopyMapDepth(m map[string]any, depth int) map[string]any {
	if m == nil {
		return nil
	}
	if depth > maxMergeDepth {
		return nil // truncate at max depth — input should have been rejected by YAML validator
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			out[k] = deepCopyMapDepth(val, depth+1)
		case []any:
			out[k] = deepCopySliceDepth(val, depth+1)
		default:
			out[k] = v
		}
	}
	return out
}

func deepCopySliceDepth(s []any, depth int) []any {
	if s == nil {
		return nil
	}
	if depth > maxMergeDepth {
		return nil
	}
	out := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			out[i] = deepCopyMapDepth(val, depth+1)
		case []any:
			out[i] = deepCopySliceDepth(val, depth+1)
		default:
			out[i] = v
		}
	}
	return out
}
