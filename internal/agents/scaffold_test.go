// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"siply.dev/siply/internal/plugins"
)

func TestScaffoldAgentConfig_CreatesValidFiles(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "code-reviewer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ScaffoldAgentConfig(dir, "code-reviewer", "A strict code reviewer"); err != nil {
		t.Fatalf("ScaffoldAgentConfig: %v", err)
	}

	// manifest.yaml must exist and be valid.
	m, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		t.Fatalf("LoadManifestFromDir: %v", err)
	}
	if m.Metadata.Name != "code-reviewer" {
		t.Errorf("manifest.metadata.name = %q", m.Metadata.Name)
	}
	if m.Spec.Category != "agents" {
		t.Errorf("manifest.spec.category = %q", m.Spec.Category)
	}
	if m.Spec.Tier != 1 {
		t.Errorf("manifest.spec.tier = %d", m.Spec.Tier)
	}

	// config.yaml must exist and be parseable.
	configData, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	parsed, err := plugins.ParsePluginYAML(configData)
	if err != nil {
		t.Fatalf("ParsePluginYAML config.yaml: %v", err)
	}
	profile, err := parseAgentConfig(parsed)
	if err != nil {
		t.Fatalf("parseAgentConfig: %v", err)
	}
	if profile.SystemPrompt == "" {
		t.Error("expected non-empty system_prompt in scaffolded config")
	}
}

func TestScaffoldAgentConfig_TitleCase(t *testing.T) {
	if got := toAgentTitleCase("code-reviewer"); got != "Code Reviewer" {
		t.Errorf("toAgentTitleCase = %q, want 'Code Reviewer'", got)
	}
	if got := toAgentTitleCase("my-agent"); got != "My Agent" {
		t.Errorf("toAgentTitleCase = %q, want 'My Agent'", got)
	}
	if got := toAgentTitleCase("simple"); got != "Simple" {
		t.Errorf("toAgentTitleCase = %q, want 'Simple'", got)
	}
}

func TestScaffoldAgentConfig_TitleCaseTrailingHyphen(t *testing.T) {
	// Trailing/consecutive hyphens should not produce empty segments (Story 10.2 F1).
	got := toAgentTitleCase("my--agent")
	if got == "My  Agent" {
		t.Errorf("empty segment not filtered: got %q", got)
	}
}

func TestScaffoldAgentConfig_PublishCompatibility(t *testing.T) {
	// The scaffolded manifest must pass LoadManifestFromDir (publish validation).
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "my-agent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ScaffoldAgentConfig(dir, "my-agent", "An agent"); err != nil {
		t.Fatalf("ScaffoldAgentConfig: %v", err)
	}

	m, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		t.Fatalf("publish compatibility check failed: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("manifest.Validate(): %v", err)
	}
}

func TestScaffoldAgentConfig_LoadableByLoader(t *testing.T) {
	// End-to-end: scaffold then load via AgentConfigLoader.
	tmp := t.TempDir()
	agentsDir := filepath.Join(tmp, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(agentsDir, "test-agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ScaffoldAgentConfig(agentDir, "test-agent", "Test agent"); err != nil {
		t.Fatalf("ScaffoldAgentConfig: %v", err)
	}

	loader := NewAgentConfigLoader(agentsDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	profile, err := loader.Get("test-agent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if profile.Name != "test-agent" {
		t.Errorf("name = %q", profile.Name)
	}
}
