// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/agents"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/skills"
)

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// SaveOptions configures the SaveProfile operation.
type SaveOptions struct {
	Name           string
	Description    string
	TargetDir      string
	Force          bool
	ConfigResolver core.ConfigResolver
	PluginRegistry core.PluginRegistry
	SkillLoader    *skills.SkillLoader
	AgentLoader    *agents.AgentConfigLoader
}

// SaveProfile captures the current workspace and writes it as a profile directory.
func SaveProfile(ctx context.Context, opts SaveOptions) error {
	if opts.Name == "" || !namePattern.MatchString(opts.Name) {
		return fmt.Errorf("%w: name %q must match ^[a-z][a-z0-9-]{0,62}$", ErrInvalidProfile, opts.Name)
	}

	// Collision check
	if _, err := os.Stat(opts.TargetDir); err == nil {
		if !opts.Force {
			return fmt.Errorf("profile %q already exists at %s (use --force to overwrite)", opts.Name, opts.TargetDir)
		}
		if rmErr := os.RemoveAll(opts.TargetDir); rmErr != nil {
			return fmt.Errorf("profiles: remove existing dir %s: %w", opts.TargetDir, rmErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("profiles: check target dir: %w", err)
	}

	if err := os.MkdirAll(opts.TargetDir, 0o755); err != nil {
		return fmt.Errorf("profiles: create directory: %w", err)
	}

	// Collect items from all registries.
	var items []ProfileItem

	if opts.PluginRegistry != nil {
		pluginMetas, err := opts.PluginRegistry.List(ctx)
		if err != nil {
			cleanupOnFailure(opts.TargetDir)
			return fmt.Errorf("profiles: list plugins: %w", err)
		}
		for _, pm := range pluginMetas {
			items = append(items, ProfileItem{
				Name:     pm.Name,
				Version:  pm.Version,
				Category: "plugins",
				Pinned:   true,
			})
		}
	}

	if opts.SkillLoader != nil {
		for _, s := range opts.SkillLoader.List() {
			items = append(items, ProfileItem{
				Name:     s.Name,
				Version:  s.Version,
				Category: "skills",
				Pinned:   true,
			})
		}
	}

	if opts.AgentLoader != nil {
		for _, a := range opts.AgentLoader.List() {
			items = append(items, ProfileItem{
				Name:     a.Name,
				Version:  a.Version,
				Category: "agents",
				Pinned:   true,
			})
		}
	}

	var cfg *core.Config
	if opts.ConfigResolver != nil {
		cfg = opts.ConfigResolver.Config()
	}

	// Build and write manifest.yaml.
	manifestData, err := buildProfileManifestYAML(opts.Name, opts.Description)
	if err != nil {
		cleanupOnFailure(opts.TargetDir)
		return fmt.Errorf("profiles: build manifest: %w", err)
	}

	// Build and write profile.yaml.
	profileData, err := buildProfileYAML(items, cfg)
	if err != nil {
		cleanupOnFailure(opts.TargetDir)
		return fmt.Errorf("profiles: build profile.yaml: %w", err)
	}

	if err := fileutil.AtomicWriteFile(filepath.Join(opts.TargetDir, "manifest.yaml"), manifestData, 0o644); err != nil {
		cleanupOnFailure(opts.TargetDir)
		return fmt.Errorf("profiles: write manifest.yaml: %w", err)
	}

	if err := fileutil.AtomicWriteFile(filepath.Join(opts.TargetDir, "profile.yaml"), profileData, 0o644); err != nil {
		cleanupOnFailure(opts.TargetDir)
		return fmt.Errorf("profiles: write profile.yaml: %w", err)
	}

	return nil
}

func buildProfileManifestYAML(name, description string) ([]byte, error) {
	if description == "" {
		description = fmt.Sprintf("Profile: %s", name)
	}
	m := plugins.Manifest{
		APIVersion: "siply/v1",
		Kind:       "Profile",
		Metadata: plugins.Metadata{
			Name:        name,
			Version:     "0.1.0",
			SiplyMin:    "0.1.0",
			Description: description,
			Author:      "developer",
			License:     "MIT",
			Updated:     time.Now().UTC().Format("2006-01-02"),
		},
		Spec: plugins.Spec{
			Tier:     1,
			Category: "profiles",
		},
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	return yaml.Marshal(m)
}

// profilePayload is the YAML representation of profile.yaml.
type profilePayload struct {
	Items  []profileItemPayload `yaml:"items"`
	Config *core.Config         `yaml:"config,omitempty"`
}

type profileItemPayload struct {
	Name     string `yaml:"name"`
	Version  string `yaml:"version"`
	Category string `yaml:"category"`
	Pinned   bool   `yaml:"pinned"`
}

func buildProfileYAML(items []ProfileItem, cfg *core.Config) ([]byte, error) {
	payload := profilePayload{Config: cfg}
	for _, item := range items {
		payload.Items = append(payload.Items, profileItemPayload(item))
	}
	return yaml.Marshal(payload)
}

func cleanupOnFailure(dir string) {
	_ = os.RemoveAll(dir)
}
