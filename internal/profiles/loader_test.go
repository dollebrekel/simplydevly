// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeTestProfileDir creates a minimal valid profile directory for testing.
func writeTestProfileDir(t *testing.T, parent, name string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `apiVersion: siply/v1
kind: Profile
metadata:
  name: ` + name + `
  version: 0.1.0
  description: "Test profile"
  siply_min: "0.1.0"
  author: test
  license: MIT
  updated: "2026-01-01"
spec:
  tier: 1
  category: profiles
`
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	profileYAML := `items:
  - name: memory-default
    version: 0.1.0
    category: plugins
    pinned: true
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestProfileLoader_LoadAll_GlobalOnly(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "profiles")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProfileDir(t, globalDir, "ml-workflow")

	loader := NewProfileLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	list := loader.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(list))
	}
	if list[0].Name != "ml-workflow" {
		t.Errorf("name = %q", list[0].Name)
	}
	if list[0].Source != "global" {
		t.Errorf("source = %q", list[0].Source)
	}
}

func TestProfileLoader_ProjectOverridesGlobal(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "global", "profiles")
	projectDir := filepath.Join(tmp, "project", "profiles")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProfileDir(t, globalDir, "my-profile")
	writeTestProfileDir(t, projectDir, "my-profile")

	loader := NewProfileLoader(globalDir, projectDir)
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	list := loader.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 profile (deduped), got %d", len(list))
	}
	if list[0].Source != "project" {
		t.Errorf("expected project source, got %q", list[0].Source)
	}
}

func TestProfileLoader_MissingDirIgnored(t *testing.T) {
	loader := NewProfileLoader("/nonexistent/global", "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatalf("expected missing dir to be silently ignored: %v", err)
	}
	if len(loader.List()) != 0 {
		t.Errorf("expected empty list for missing dir")
	}
}

func TestProfileLoader_InvalidManifestSkipped(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "profiles")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a directory with invalid manifest
	badDir := filepath.Join(globalDir, "bad-profile")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "manifest.yaml"), []byte("apiVersion: invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also write a valid profile
	writeTestProfileDir(t, globalDir, "good-profile")

	loader := NewProfileLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	list := loader.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 profile (bad one skipped), got %d", len(list))
	}
	if list[0].Name != "good-profile" {
		t.Errorf("expected good-profile, got %q", list[0].Name)
	}
}

func TestProfileLoader_DirNameMustMatchManifestName(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "profiles")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a dir named "wrong-name" but with manifest name "correct-name"
	wrongDir := filepath.Join(globalDir, "wrong-name")
	if err := os.MkdirAll(wrongDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `apiVersion: siply/v1
kind: Profile
metadata:
  name: correct-name
  version: 0.1.0
  description: "Test"
  siply_min: "0.1.0"
  author: test
  license: MIT
  updated: "2026-01-01"
spec:
  tier: 1
  category: profiles
`
	if err := os.WriteFile(filepath.Join(wrongDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	profileYAML := "items:\n  - name: x\n    version: 0.1.0\n    category: plugins\n    pinned: true\n"
	if err := os.WriteFile(filepath.Join(wrongDir, "profile.yaml"), []byte(profileYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewProfileLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Should be skipped (dir name != manifest name)
	if len(loader.List()) != 0 {
		t.Errorf("expected profile to be skipped due to name mismatch")
	}
}

func TestProfileLoader_Get_NotFound(t *testing.T) {
	loader := NewProfileLoader(t.TempDir(), "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	_, err := loader.Get("nonexistent")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestProfileLoader_Get_Found(t *testing.T) {
	tmp := t.TempDir()
	globalDir := filepath.Join(tmp, "profiles")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProfileDir(t, globalDir, "my-profile")

	loader := NewProfileLoader(globalDir, "")
	if err := loader.LoadAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	p, err := loader.Get("my-profile")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Name != "my-profile" {
		t.Errorf("name = %q", p.Name)
	}
}

func TestGlobalDir_DefaultPath(t *testing.T) {
	t.Setenv("SIPLY_HOME", "")
	dir := GlobalDir("/home/user")
	expected := "/home/user/.siply/profiles"
	if dir != expected {
		t.Errorf("GlobalDir = %q, want %q", dir, expected)
	}
}

func TestGlobalDir_SIPLY_HOME(t *testing.T) {
	t.Setenv("SIPLY_HOME", "/custom/home")
	dir := GlobalDir("/home/user")
	expected := "/custom/home/profiles"
	if dir != expected {
		t.Errorf("GlobalDir = %q, want %q", dir, expected)
	}
}
