// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeTestAgentDir creates a minimal valid agent config directory for testing.
func writeTestAgentDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: ` + name + `
  version: 0.1.0
  description: "Test agent"
  siply_min: "0.1.0"
  author: test
  license: MIT
  updated: "2026-01-01"
spec:
  tier: 1
  category: agents
  capabilities: {}
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	config := `system_prompt: "You are a test agent."
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestLoadAll_GlobalOnly(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "agents")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestAgentDir(t, globalDir, "code-reviewer")

	loader := NewAgentConfigLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	list := loader.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(list))
	}
	if list[0].Name != "code-reviewer" {
		t.Errorf("name = %q", list[0].Name)
	}
	if list[0].Source != "global" {
		t.Errorf("source = %q", list[0].Source)
	}
}

func TestLoadAll_ProjectOverridesGlobal(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "global", "agents")
	projectDir := filepath.Join(tmp, "project", "agents")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestAgentDir(t, globalDir, "code-reviewer")
	writeTestAgentDir(t, projectDir, "code-reviewer")

	loader := NewAgentConfigLoader(globalDir, projectDir)
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	list := loader.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 agent (project overrides global), got %d", len(list))
	}
	if list[0].Source != "project" {
		t.Errorf("expected source=project after override, got %q", list[0].Source)
	}
}

func TestLoadAll_MissingDirIgnored(t *testing.T) {
	loader := NewAgentConfigLoader("/nonexistent/path", "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("missing dir should be silently ignored, got: %v", err)
	}
	if got := loader.List(); len(got) != 0 {
		t.Errorf("expected 0 agents, got %d", len(got))
	}
}

func TestLoadAll_InvalidManifestSkipped(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "agents")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Valid agent.
	writeTestAgentDir(t, globalDir, "valid-agent")

	// Invalid: manifest only, no config.yaml.
	badDir := filepath.Join(globalDir, "bad-agent")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "manifest.yaml"), []byte("apiVersion: siply/v1\nkind: Plugin\nmetadata:\n  name: bad-agent\n  version: 0.1.0\n  description: \"Bad agent\"\n  siply_min: \"0.1.0\"\n  author: test\n  license: MIT\n  updated: \"2026-01-01\"\nspec:\n  tier: 1\n  category: agents\n  capabilities: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Note: no config.yaml — should be skipped with a warning.

	loader := NewAgentConfigLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	list := loader.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 valid agent, got %d: %v", len(list), list)
	}
	if list[0].Name != "valid-agent" {
		t.Errorf("name = %q", list[0].Name)
	}
}

func TestLoadAll_DirNameMustMatchManifestName(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "agents")
	mismatchDir := filepath.Join(globalDir, "different-name")
	if err := os.MkdirAll(mismatchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: wrong-name
  version: 0.1.0
  siply_min: "0.1.0"
  author: test
  license: MIT
spec:
  tier: 1
  category: agents
  capabilities: {}
`
	if err := os.WriteFile(filepath.Join(mismatchDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mismatchDir, "config.yaml"), []byte(`system_prompt: "test"`), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewAgentConfigLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// Mismatch should be silently skipped.
	if list := loader.List(); len(list) != 0 {
		t.Errorf("expected 0 agents (dir/manifest name mismatch), got %d", len(list))
	}
}

func TestGet_NotFound(t *testing.T) {
	loader := NewAgentConfigLoader(t.TempDir(), "")
	_ = loader.LoadAll(context.Background())
	_, err := loader.Get("nonexistent")
	if !errors.Is(err, ErrAgentConfigNotFound) {
		t.Errorf("expected ErrAgentConfigNotFound, got %v", err)
	}
}

func TestGet_EmptyName(t *testing.T) {
	loader := NewAgentConfigLoader(t.TempDir(), "")
	_, err := loader.Get("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestList_SortedByName(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "agents")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestAgentDir(t, globalDir, "zebra-agent")
	writeTestAgentDir(t, globalDir, "alpha-agent")
	writeTestAgentDir(t, globalDir, "mid-agent")

	loader := NewAgentConfigLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	list := loader.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(list))
	}
	if list[0].Name != "alpha-agent" || list[1].Name != "mid-agent" || list[2].Name != "zebra-agent" {
		t.Errorf("unexpected order: %v", []string{list[0].Name, list[1].Name, list[2].Name})
	}
}

func TestGlobalDir_SIPLYHome(t *testing.T) {
	t.Setenv("SIPLY_HOME", "/custom/home")
	got := GlobalDir("/irrelevant")
	if got != "/custom/home/agents" {
		t.Errorf("GlobalDir = %q, want /custom/home/agents", got)
	}
}

func TestGlobalDir_Default(t *testing.T) {
	t.Setenv("SIPLY_HOME", "")
	got := GlobalDir("/home/user")
	if got != "/home/user/.siply/agents" {
		t.Errorf("GlobalDir = %q, want /home/user/.siply/agents", got)
	}
}
