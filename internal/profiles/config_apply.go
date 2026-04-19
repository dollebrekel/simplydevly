// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
)

// ConfigChange describes a single config key that differs between current and profile configs.
type ConfigChange struct {
	Key      string
	OldValue string
	NewValue string
}

// TUIOnlyConfig returns a core.Config that only sets the TUI.Profile field.
// Used by built-in profile install (minimal/standard) which only sets the TUI preset.
func TUIOnlyConfig(profileName string) core.Config {
	return core.Config{TUI: core.TUIConfig{Profile: profileName}}
}

// DiffConfig compares current and profile configs and returns the list of changes.
// Only non-zero profile fields that differ from current values are included.
func DiffConfig(current, profile *core.Config) []ConfigChange {
	if profile == nil {
		return nil
	}
	if current == nil {
		current = &core.Config{}
	}

	var changes []ConfigChange

	if profile.Provider.Default != "" && profile.Provider.Default != current.Provider.Default {
		changes = append(changes, ConfigChange{Key: "provider.default", OldValue: current.Provider.Default, NewValue: profile.Provider.Default})
	}
	if profile.Provider.Model != "" && profile.Provider.Model != current.Provider.Model {
		changes = append(changes, ConfigChange{Key: "provider.model", OldValue: current.Provider.Model, NewValue: profile.Provider.Model})
	}
	if profile.Routing.Enabled != nil && (current.Routing.Enabled == nil || *current.Routing.Enabled != *profile.Routing.Enabled) {
		old := ""
		if current.Routing.Enabled != nil {
			old = fmt.Sprintf("%v", *current.Routing.Enabled)
		}
		changes = append(changes, ConfigChange{Key: "routing.enabled", OldValue: old, NewValue: fmt.Sprintf("%v", *profile.Routing.Enabled)})
	}
	if profile.Routing.DefaultProvider != "" && profile.Routing.DefaultProvider != current.Routing.DefaultProvider {
		changes = append(changes, ConfigChange{Key: "routing.default_provider", OldValue: current.Routing.DefaultProvider, NewValue: profile.Routing.DefaultProvider})
	}
	if profile.Routing.PreprocessProvider != "" && profile.Routing.PreprocessProvider != current.Routing.PreprocessProvider {
		changes = append(changes, ConfigChange{Key: "routing.preprocess_provider", OldValue: current.Routing.PreprocessProvider, NewValue: profile.Routing.PreprocessProvider})
	}
	if profile.Routing.PreprocessModel != "" && profile.Routing.PreprocessModel != current.Routing.PreprocessModel {
		changes = append(changes, ConfigChange{Key: "routing.preprocess_model", OldValue: current.Routing.PreprocessModel, NewValue: profile.Routing.PreprocessModel})
	}
	if profile.Session.RetentionCount != nil && (current.Session.RetentionCount == nil || *current.Session.RetentionCount != *profile.Session.RetentionCount) {
		old := ""
		if current.Session.RetentionCount != nil {
			old = fmt.Sprintf("%d", *current.Session.RetentionCount)
		}
		changes = append(changes, ConfigChange{Key: "session.retention_count", OldValue: old, NewValue: fmt.Sprintf("%d", *profile.Session.RetentionCount)})
	}
	if profile.Telemetry.Enabled != nil && (current.Telemetry.Enabled == nil || *current.Telemetry.Enabled != *profile.Telemetry.Enabled) {
		old := ""
		if current.Telemetry.Enabled != nil {
			old = fmt.Sprintf("%v", *current.Telemetry.Enabled)
		}
		changes = append(changes, ConfigChange{Key: "telemetry.enabled", OldValue: old, NewValue: fmt.Sprintf("%v", *profile.Telemetry.Enabled)})
	}
	if profile.TUI.Profile != "" && profile.TUI.Profile != current.TUI.Profile {
		changes = append(changes, ConfigChange{Key: "tui.profile", OldValue: current.TUI.Profile, NewValue: profile.TUI.Profile})
	}
	if profile.Agent.Config != "" && profile.Agent.Config != current.Agent.Config {
		changes = append(changes, ConfigChange{Key: "agent.config", OldValue: current.Agent.Config, NewValue: profile.Agent.Config})
	}

	return changes
}

// ApplyProfileConfig merges profileConfig onto the config at targetPath and writes the result.
// If targetPath does not exist, it is created. Existing keys not in the profile are preserved.
func ApplyProfileConfig(profileConfig *core.Config, targetPath string) error {
	if profileConfig == nil {
		return nil
	}

	var existing *core.Config
	data, err := os.ReadFile(targetPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("profiles: read config %s: %w", targetPath, err)
		}
		existing = &core.Config{}
	} else {
		var cfg core.Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("profiles: parse config %s: %w", targetPath, err)
		}
		existing = &cfg
	}

	merged := config.MergeConfig(existing, profileConfig)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("profiles: marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("profiles: create config dir: %w", err)
	}

	if err := fileutil.AtomicWriteFile(targetPath, out, 0o644); err != nil {
		return fmt.Errorf("profiles: write config %s: %w", targetPath, err)
	}

	return nil
}
