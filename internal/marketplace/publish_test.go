// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validManifestYAML = `apiVersion: siply/v1
kind: Plugin
metadata:
  name: test-plugin
  version: "1.0.0"
  siply_min: "0.1.0"
  description: A test plugin
  author: test-author
  license: MIT
  updated: "2025-01-01"
spec:
  tier: 1
  capabilities:
    filesystem: read
`

func createValidPluginDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(validManifestYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Plugin\nA test."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte("MIT License"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateForPublish_Valid(t *testing.T) {
	t.Parallel()
	dir := createValidPluginDir(t)

	result, err := ValidateForPublish(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Manifest == nil {
		t.Fatal("expected manifest")
	}
	if result.Readme == "" {
		t.Error("expected readme content")
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", result.Warnings)
	}
}

func TestValidateForPublish_MissingReadme(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(validManifestYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateForPublish(dir)
	if err == nil {
		t.Fatal("expected error for missing README.md")
	}
	if !strings.Contains(err.Error(), "README.md is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateForPublish_MissingManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateForPublish(dir)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestValidateForPublish_InvalidManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("invalid: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateForPublish(dir)
	if err == nil {
		t.Fatal("expected error for invalid manifest")
	}
}

func TestValidateForPublish_MissingOptionalFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(validManifestYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateForPublish(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}
}

func TestValidateForPublish_ReadmeExactlyAtLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(validManifestYAML), 0644); err != nil {
		t.Fatal(err)
	}
	readme := make([]byte, 1<<20)
	for i := range readme {
		readme[i] = 'A'
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), readme, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateForPublish(dir)
	if err != nil {
		t.Fatalf("expected 1MB README to pass, got error: %v", err)
	}
	if len(result.Readme) != 1<<20 {
		t.Errorf("expected readme length %d, got %d", 1<<20, len(result.Readme))
	}
}

func TestValidateForPublish_ReadmeExceedsLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(validManifestYAML), 0644); err != nil {
		t.Fatal(err)
	}
	readme := make([]byte, 1<<20+1)
	for i := range readme {
		readme[i] = 'A'
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), readme, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidateForPublish(dir)
	if err == nil {
		t.Fatal("expected error for README exceeding 1MB")
	}
	if !strings.Contains(err.Error(), "exceeds 1MB") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateForPublish_UpdatesStaleDate(t *testing.T) {
	t.Parallel()
	dir := createValidPluginDir(t)

	result, err := ValidateForPublish(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if result.Manifest.Metadata.Updated != today {
		t.Errorf("expected updated to be %s, got %s", today, result.Manifest.Metadata.Updated)
	}
}

// --- Bundle publish tests ---

func TestValidateForPublish_Bundle(t *testing.T) {
	dir := t.TempDir()

	manifest := `apiVersion: siply/v1
kind: Bundle
metadata:
  name: test-bundle
  version: 1.0.0
  siply_min: 0.1.0
  description: "A test bundle"
  author: test-author
  license: MIT
  updated: "2026-04-17"
spec:
  components:
    - name: memory-default
      version: "1.0.0"
    - name: prompt-basic
      version: "1.0.0"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Bundle"), 0644))

	result, err := ValidateForPublish(dir)
	require.NoError(t, err)
	assert.Equal(t, "Bundle", result.Manifest.Kind)
	assert.Equal(t, "bundles", result.Manifest.Spec.Category)
	assert.Equal(t, "test-bundle", result.Manifest.Metadata.Name)
	assert.Len(t, result.Manifest.Spec.Components, 2)
}

func TestValidateForPublish_BundleRequiresReadme(t *testing.T) {
	dir := t.TempDir()

	manifest := `apiVersion: siply/v1
kind: Bundle
metadata:
  name: test-bundle
  version: 1.0.0
  siply_min: 0.1.0
  description: "A test bundle"
  author: test-author
  license: MIT
  updated: "2026-04-17"
spec:
  components:
    - name: memory-default
      version: "1.0.0"
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644))

	_, err := ValidateForPublish(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "README.md is required")
}
