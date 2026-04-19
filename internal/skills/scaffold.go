// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/fileutil"
	"siply.dev/siply/internal/plugins"
)

// promptsFile is the on-disk structure of prompts.yaml.
type promptsFile struct {
	Prompts map[string]promptEntry `yaml:"prompts"`
}

type promptEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Template    string `yaml:"template"`
}

// ScaffoldSkill writes manifest.yaml and prompts.yaml into an existing directory.
// name must equal filepath.Base(dir): the loader validates that dir name == manifest name.
func ScaffoldSkill(dir, name, description string) error {
	manifestData, err := buildManifestYAML(name, description)
	if err != nil {
		return fmt.Errorf("scaffold: build manifest: %w", err)
	}
	promptsData, err := buildPromptsYAML(name, description)
	if err != nil {
		return fmt.Errorf("scaffold: build prompts: %w", err)
	}

	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := fileutil.AtomicWriteFile(manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("scaffold: write manifest.yaml: %w", err)
	}
	if err := fileutil.AtomicWriteFile(filepath.Join(dir, "prompts.yaml"), promptsData, 0o644); err != nil {
		// Roll back manifest to avoid half-scaffolded skill.
		os.Remove(manifestPath)
		return fmt.Errorf("scaffold: write prompts.yaml: %w", err)
	}
	return nil
}

func buildManifestYAML(name, description string) ([]byte, error) {
	m := plugins.Manifest{
		APIVersion: "siply/v1",
		Kind:       "Plugin",
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
			Tier:         1,
			Capabilities: map[string]string{},
			Category:     "skills",
		},
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("scaffold: invalid manifest: %w", err)
	}
	return yaml.Marshal(m)
}

func buildPromptsYAML(name, description string) ([]byte, error) {
	pf := promptsFile{
		Prompts: map[string]promptEntry{
			name: {
				Name:        toTitleCase(name),
				Description: description,
				// Trailing newline causes yaml.v3 to emit a literal block scalar (|).
				Template: "{{.input}}\n",
			},
		},
	}
	return yaml.Marshal(pf)
}

// toTitleCase converts a hyphenated name to Title Case ("my-skill" → "My Skill").
func toTitleCase(name string) string {
	parts := strings.Split(name, "-")
	filtered := parts[:0]
	for _, p := range parts {
		if len(p) > 0 {
			filtered = append(filtered, strings.ToUpper(p[:1])+p[1:])
		}
	}
	return strings.Join(filtered, " ")
}
