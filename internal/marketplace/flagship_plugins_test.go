// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repoRoot returns the simplydevly module root (two dirs up from this test file).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestValidateForPublish_TreeLocal verifies tree-local plugin passes pre-publish checks.
func TestValidateForPublish_TreeLocal(t *testing.T) {
	t.Parallel()
	pluginDir := filepath.Join(repoRoot(t), "plugins", "tree-local")

	result, err := ValidateForPublish(pluginDir)
	require.NoError(t, err, "tree-local must pass ValidateForPublish")
	require.NotNil(t, result.Manifest)

	assert.Equal(t, "tree-local", result.Manifest.Metadata.Name)
	assert.Equal(t, "Simply Devly", result.Manifest.Metadata.Author)
	assert.Equal(t, "Apache-2.0", result.Manifest.Metadata.License)
	assert.Equal(t, 3, result.Manifest.Spec.Tier)
	assert.NotEmpty(t, result.Readme)
	// Warnings are allowed (e.g., CHANGELOG.md present means fewer warnings)
}

// TestValidateForPublish_MarkdownPreview verifies markdown-preview plugin passes pre-publish checks.
func TestValidateForPublish_MarkdownPreview(t *testing.T) {
	t.Parallel()
	pluginDir := filepath.Join(repoRoot(t), "plugins", "markdown-preview")

	result, err := ValidateForPublish(pluginDir)
	require.NoError(t, err, "markdown-preview must pass ValidateForPublish")
	require.NotNil(t, result.Manifest)

	assert.Equal(t, "markdown-preview", result.Manifest.Metadata.Name)
	assert.Equal(t, "Simply Devly", result.Manifest.Metadata.Author)
	assert.Equal(t, "Apache-2.0", result.Manifest.Metadata.License)
	assert.Equal(t, 3, result.Manifest.Spec.Tier)
	assert.NotEmpty(t, result.Readme)
}
