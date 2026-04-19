// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"errors"
	"fmt"
	"log/slog"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/core"
)

// Sentinel errors for profile operations.
var (
	ErrProfileNotFound = errors.New("profiles: not found")
	ErrNoProfile       = errors.New("profiles: no profile.yaml")
	ErrInvalidProfile  = errors.New("profiles: invalid profile")
)

// validCategories lists the valid item categories for a profile snapshot.
var validCategories = map[string]bool{
	"plugins": true,
	"skills":  true,
	"agents":  true,
}

// parseProfileConfig extracts a Profile from a parsed YAML document (map[string]any).
func parseProfileConfig(data map[string]any) (*Profile, error) {
	if data == nil {
		return nil, fmt.Errorf("%w: empty YAML document", ErrInvalidProfile)
	}

	p := &Profile{}

	rawItems, hasItems := data["items"]
	if !hasItems {
		return nil, fmt.Errorf("%w: items list is missing", ErrInvalidProfile)
	}
	items, err := parseProfileItems(rawItems)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: items list is empty", ErrInvalidProfile)
	}
	p.Items = items

	// Parse config section by re-marshaling through YAML into core.Config.
	if rawConfig, ok := data["config"]; ok && rawConfig != nil {
		configBytes, err := yaml.Marshal(rawConfig)
		if err != nil {
			return nil, fmt.Errorf("%w: marshal config section: %v", ErrInvalidProfile, err)
		}
		var cfg core.Config
		if err := yaml.Unmarshal(configBytes, &cfg); err != nil {
			return nil, fmt.Errorf("%w: unmarshal config section: %v", ErrInvalidProfile, err)
		}
		p.Config = &cfg
	}

	return p, nil
}

func parseProfileItems(raw any) ([]ProfileItem, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: 'items' must be a list, got %T", ErrInvalidProfile, raw)
	}

	items := make([]ProfileItem, 0, len(list))
	for i, elem := range list {
		m, ok := elem.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: items[%d] must be a mapping, got %T", ErrInvalidProfile, i, elem)
		}
		item, err := parseProfileItem(m, i)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func parseProfileItem(m map[string]any, idx int) (ProfileItem, error) {
	item := ProfileItem{}

	name, ok := m["name"].(string)
	if !ok || name == "" {
		return item, fmt.Errorf("%w: items[%d].name is required", ErrInvalidProfile, idx)
	}
	item.Name = name

	version, ok := m["version"].(string)
	if !ok || version == "" {
		return item, fmt.Errorf("%w: items[%d].version is required", ErrInvalidProfile, idx)
	}
	item.Version = version

	category, ok := m["category"].(string)
	if !ok || category == "" {
		return item, fmt.Errorf("%w: items[%d].category is required", ErrInvalidProfile, idx)
	}
	if !validCategories[category] {
		return item, fmt.Errorf("%w: items[%d].category must be plugins, skills, or agents; got %q", ErrInvalidProfile, idx, category)
	}
	item.Category = category

	if raw, exists := m["pinned"]; exists {
		if pinned, ok := raw.(bool); ok {
			item.Pinned = pinned
		} else {
			slog.Warn("profiles: items[%d].pinned has non-bool type, defaulting to false", "index", idx, "type", fmt.Sprintf("%T", raw))
		}
	}

	return item, nil
}
