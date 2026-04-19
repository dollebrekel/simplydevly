// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agents

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

// ScaffoldAgentConfig writes manifest.yaml and config.yaml into an existing directory.
// name must equal filepath.Base(dir): the loader validates that dir name == manifest name.
func ScaffoldAgentConfig(dir, name, description string) error {
	manifestData, err := buildAgentManifestYAML(name, description)
	if err != nil {
		return fmt.Errorf("scaffold: build manifest: %w", err)
	}
	configData, err := buildAgentConfigYAML(name)
	if err != nil {
		return fmt.Errorf("scaffold: build config: %w", err)
	}

	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := fileutil.AtomicWriteFile(manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("scaffold: write manifest.yaml: %w", err)
	}
	if err := fileutil.AtomicWriteFile(filepath.Join(dir, "config.yaml"), configData, 0o644); err != nil {
		os.Remove(manifestPath)
		return fmt.Errorf("scaffold: write config.yaml: %w", err)
	}
	return nil
}

func buildAgentManifestYAML(name, description string) ([]byte, error) {
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
			Category:     "agents",
		},
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("scaffold: invalid manifest: %w", err)
	}
	return yaml.Marshal(m)
}

// agentConfigTemplate is the starter config.yaml for scaffolded agent configs.
type agentConfigTemplate struct {
	SystemPrompt string             `yaml:"system_prompt"`
	Model        agentModelTemplate `yaml:"model"`
	Behavior     agentBehavTemplate `yaml:"behavior"`
	Tools        agentToolsTemplate `yaml:"tools"`
}

type agentModelTemplate struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

type agentBehavTemplate struct {
	ParallelTools bool `yaml:"parallel_tools"`
	MaxIterations int  `yaml:"max_iterations"`
	AutoApprove   bool `yaml:"auto_approve"`
}

type agentToolsTemplate struct {
	Allowed []string `yaml:"allowed"`
	Denied  []string `yaml:"denied"`
}

func buildAgentConfigYAML(name string) ([]byte, error) {
	title := toAgentTitleCase(name)
	cfg := agentConfigTemplate{
		SystemPrompt: fmt.Sprintf("You are %s. Describe your role and behavior here.\n", title),
		Model: agentModelTemplate{
			Provider:    "anthropic",
			Model:       "claude-sonnet-4-6",
			Temperature: 0.3,
			MaxTokens:   8192,
		},
		Behavior: agentBehavTemplate{
			ParallelTools: true,
			MaxIterations: 20,
			AutoApprove:   false,
		},
		Tools: agentToolsTemplate{
			Allowed: []string{},
			Denied:  []string{},
		},
	}
	return yaml.Marshal(cfg)
}

// toAgentTitleCase converts a hyphenated name to Title Case ("code-reviewer" → "Code Reviewer").
func toAgentTitleCase(name string) string {
	parts := strings.Split(name, "-")
	filtered := parts[:0]
	for _, p := range parts {
		if len(p) > 0 {
			filtered = append(filtered, strings.ToUpper(p[:1])+p[1:])
		}
	}
	return strings.Join(filtered, " ")
}
