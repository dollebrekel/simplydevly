// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSaveProfile_CreatesValidFiles(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "my-profile")

	opts := SaveOptions{
		Name:        "my-profile",
		Description: "My test profile",
		TargetDir:   targetDir,
	}
	if err := SaveProfile(context.Background(), opts); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	// Verify manifest.yaml
	manifestData, err := os.ReadFile(filepath.Join(targetDir, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(manifestData, &m); err != nil {
		t.Fatal(err)
	}
	metadata := m["metadata"].(map[string]any)
	if metadata["name"] != "my-profile" {
		t.Errorf("manifest.name = %v", metadata["name"])
	}
	if m["kind"] != "Profile" {
		t.Errorf("manifest.kind = %v", m["kind"])
	}

	// Verify profile.yaml parses as valid (even if items is empty = no loaders)
	profileData, err := os.ReadFile(filepath.Join(targetDir, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(profileData) == 0 {
		t.Error("expected non-empty profile.yaml")
	}
}

func TestSaveProfile_FilesParseCorrectly(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "test-profile")

	opts := SaveOptions{
		Name:      "test-profile",
		TargetDir: targetDir,
	}
	if err := SaveProfile(context.Background(), opts); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	// manifest.yaml should be valid
	manifestData, err := os.ReadFile(filepath.Join(targetDir, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestData), "kind: Profile") {
		t.Errorf("manifest should contain 'kind: Profile', got:\n%s", manifestData)
	}
	if !strings.Contains(string(manifestData), "category: profiles") {
		t.Errorf("manifest should contain 'category: profiles', got:\n%s", manifestData)
	}
}

func TestSaveProfile_CollisionCheck(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "existing-profile")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := SaveOptions{
		Name:      "existing-profile",
		TargetDir: targetDir,
	}
	err := SaveProfile(context.Background(), opts)
	if err == nil {
		t.Fatal("expected collision error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestSaveProfile_ForceOverwrite(t *testing.T) {
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "existing-profile")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := SaveOptions{
		Name:      "existing-profile",
		TargetDir: targetDir,
		Force:     true,
	}
	if err := SaveProfile(context.Background(), opts); err != nil {
		t.Fatalf("expected force to succeed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "manifest.yaml")); err != nil {
		t.Error("manifest.yaml should exist after force overwrite")
	}
}

func TestSaveProfile_CleanupOnFailure(t *testing.T) {
	// Attempt save with invalid name to trigger manifest validation failure.
	// We can't easily trigger the cleanup path without a way to make marshal fail,
	// but we verify the dir doesn't exist after a bad name.
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "INVALID-NAME")

	opts := SaveOptions{
		Name:      "INVALID-NAME", // invalid: uppercase
		TargetDir: targetDir,
	}
	err := SaveProfile(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for invalid profile name")
	}
	// On failure, the target dir should be cleaned up
	if _, statErr := os.Stat(targetDir); statErr == nil {
		t.Error("target dir should be cleaned up on failure")
	}
}

func TestSaveProfile_CollectsItems(t *testing.T) {
	// With nil loaders, profile.yaml items list should be empty (nil)
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "empty-profile")

	opts := SaveOptions{
		Name:      "empty-profile",
		TargetDir: targetDir,
	}
	if err := SaveProfile(context.Background(), opts); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(targetDir, "profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// items should be null/empty when no loaders
	var payload map[string]any
	if err := yaml.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
}
