// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/skills"
)

// skillsTestDir creates a temporary skills dir with a single valid skill.
func skillsTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "code-review")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: code-review
  version: 1.0.0
  siply_min: 0.1.0
  description: Automated code review skill
  author: test
  license: Apache-2.0
  updated: "2026-01-01"
spec:
  tier: 1
  capabilities: {}
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "manifest.yaml"), []byte(manifest), 0600))

	prompts := `prompts:
  code-review:
    name: Code Review
    description: Review code for issues
    template: |
      Review the following code:
      {{.input}}

      Identify bugs, security issues, and style violations.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "prompts.yaml"), []byte(prompts), 0600))
	return dir
}

// TestSkillDispatch_LoadAndInvoke verifies the full path: load skill from fixture dir
// → dispatch slash command → verify rendered prompt (AC#2, AC#3).
func TestSkillDispatch_LoadAndInvoke(t *testing.T) {
	dir := skillsTestDir(t)

	loader := skills.NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	dispatcher := skills.NewSlashDispatcher(loader)
	require.NotNil(t, dispatcher)

	// Dispatch a slash command with input (AC#3).
	result, err := dispatcher.Dispatch("/code-review func main() {}")
	require.NoError(t, err)

	assert.Contains(t, result, "func main() {}", "template {{.input}} should be substituted")
	assert.Contains(t, result, "Review the following code", "template text should be present")
}

// TestSkillDispatch_EmptyInput verifies empty input renders gracefully (AC#3).
func TestSkillDispatch_EmptyInput(t *testing.T) {
	dir := skillsTestDir(t)

	loader := skills.NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	dispatcher := skills.NewSlashDispatcher(loader)
	result, err := dispatcher.Dispatch("/code-review")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

// TestSkillDispatch_PromptFileVsConfigFile verifies that both prompts.yaml and
// config.yaml with a prompts: key are loaded correctly.
func TestSkillDispatch_ConfigYAMLFallback(t *testing.T) {
	// prompt-basic uses config.yaml with prompts: key.
	promptBasicRel := filepath.Join("..", "..", "plugins", "prompt-basic")
	promptBasicDir, err := filepath.Abs(promptBasicRel)
	if err != nil {
		t.Skipf("cannot resolve prompt-basic path: %v", err)
	}
	if _, err := os.Stat(promptBasicDir); err != nil {
		t.Skipf("prompt-basic not found: %v", err)
	}

	// Place it in a temp dir named by its manifest name.
	dir := t.TempDir()
	destDir := filepath.Join(dir, "prompt-basic")
	require.NoError(t, os.MkdirAll(destDir, 0755))

	for _, fname := range []string{"manifest.yaml", "config.yaml"} {
		data, err := os.ReadFile(filepath.Join(promptBasicDir, fname))
		if err != nil {
			continue // manifest might not have config.yaml
		}
		require.NoError(t, os.WriteFile(filepath.Join(destDir, fname), data, 0600))
	}

	loader := skills.NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	skill, err := loader.Get("prompt-basic")
	require.NoError(t, err)
	assert.NotEmpty(t, skill.Prompts, "prompt-basic should have prompts from config.yaml")
}
