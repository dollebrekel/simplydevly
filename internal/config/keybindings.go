// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// KeybindingEntry represents a single keybinding override in a user config file.
type KeybindingEntry struct {
	Key    string `yaml:"key"`
	Action string `yaml:"action"`
	Force  bool   `yaml:"force,omitempty"`
}

// KeybindingConfig holds the parsed keybinding overrides from a YAML config file.
type KeybindingConfig struct {
	Keybindings []KeybindingEntry `yaml:"keybindings"`
}

// LoadKeybindingConfig reads and validates a keybindings.yaml file.
// Returns (nil, os.ErrNotExist) when the file does not exist.
// Uses KnownFields(false) for forward-compatibility with future config fields.
func LoadKeybindingConfig(path string) (*KeybindingConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("keybindings: stat %s: %w", path, err)
	}
	if info.Size() > maxConfigFileSize {
		return nil, fmt.Errorf("keybindings: file exceeds 1MB limit: %s (%d bytes)", path, info.Size())
	}
	if info.Size() == 0 {
		return &KeybindingConfig{}, nil
	}

	dec := yaml.NewDecoder(f)

	var cfg KeybindingConfig
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("keybindings: invalid YAML in %s: %w", path, err)
	}

	seen := make(map[string]struct{}, len(cfg.Keybindings))
	for i := range cfg.Keybindings {
		kb := &cfg.Keybindings[i]

		if kb.Key == "" {
			return nil, fmt.Errorf("keybindings: entry %d has empty key in %s", i, path)
		}
		if kb.Action == "" {
			return nil, fmt.Errorf("keybindings: entry %d has empty action in %s", i, path)
		}

		kb.Key = strings.ToLower(kb.Key)

		if _, dup := seen[kb.Key]; dup {
			return nil, fmt.Errorf("keybindings: duplicate key %q in %s", kb.Key, path)
		}
		seen[kb.Key] = struct{}{}
	}

	return &cfg, nil
}
