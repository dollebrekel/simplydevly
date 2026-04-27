// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadKeybindingConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "ctrl+t"
    action: "toggle-tree-panel"
    force: true
  - key: "ctrl+shift+f"
    action: "search-files"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := LoadKeybindingConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Len(t, cfg.Keybindings, 2)

	assert.Equal(t, "ctrl+t", cfg.Keybindings[0].Key)
	assert.Equal(t, "toggle-tree-panel", cfg.Keybindings[0].Action)
	assert.True(t, cfg.Keybindings[0].Force)

	assert.Equal(t, "ctrl+shift+f", cfg.Keybindings[1].Key)
	assert.Equal(t, "search-files", cfg.Keybindings[1].Action)
	assert.False(t, cfg.Keybindings[1].Force)
}

func TestLoadKeybindingConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	require.NoError(t, os.WriteFile(path, []byte(""), 0644))

	cfg, err := LoadKeybindingConfig(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Empty(t, cfg.Keybindings)
}

func TestLoadKeybindingConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	require.NoError(t, os.WriteFile(path, []byte("keybindings:\n  - [broken"), 0644))

	_, err := LoadKeybindingConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "keybindings")
}

func TestLoadKeybindingConfig_DuplicateKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "ctrl+t"
    action: "action-one"
  - key: "ctrl+t"
    action: "action-two"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := LoadKeybindingConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
	assert.Contains(t, err.Error(), "ctrl+t")
}

func TestLoadKeybindingConfig_ForceFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "ctrl+t"
    action: "toggle-tree"
    force: true
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := LoadKeybindingConfig(path)
	require.NoError(t, err)
	assert.True(t, cfg.Keybindings[0].Force)
}

func TestLoadKeybindingConfig_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	data := []byte("keybindings:\n  - key: " + strings.Repeat("x", 1<<20) + "\n")
	require.NoError(t, os.WriteFile(path, data, 0644))

	_, err := LoadKeybindingConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 1MB")
}

func TestLoadKeybindingConfig_UnknownFieldsAllowed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "ctrl+t"
    action: "toggle-tree"
    future_field: "ignored"
unknown_top_level: true
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := LoadKeybindingConfig(path)
	require.NoError(t, err)
	require.Len(t, cfg.Keybindings, 1)
	assert.Equal(t, "ctrl+t", cfg.Keybindings[0].Key)
}

func TestLoadKeybindingConfig_MissingFile(t *testing.T) {
	_, err := LoadKeybindingConfig("/nonexistent/keybindings.yaml")
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestLoadKeybindingConfig_EmptyKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: ""
    action: "some-action"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := LoadKeybindingConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty key")
}

func TestLoadKeybindingConfig_EmptyAction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "ctrl+t"
    action: ""
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := LoadKeybindingConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty action")
}

func TestLoadKeybindingConfig_NormalizesKeysToLowercase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "Ctrl+T"
    action: "toggle-tree"
  - key: "CTRL+SHIFT+F"
    action: "search"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg, err := LoadKeybindingConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "ctrl+t", cfg.Keybindings[0].Key)
	assert.Equal(t, "ctrl+shift+f", cfg.Keybindings[1].Key)
}

func TestLoadKeybindingConfig_DuplicateKeysAfterNormalization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keybindings.yaml")
	content := `keybindings:
  - key: "Ctrl+T"
    action: "action-one"
  - key: "ctrl+t"
    action: "action-two"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	_, err := LoadKeybindingConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}
