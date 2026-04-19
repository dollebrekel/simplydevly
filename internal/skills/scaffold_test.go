// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/plugins"
)

func TestScaffoldSkill_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))

	require.NoError(t, ScaffoldSkill(skillDir, "my-skill", "A custom skill"))

	assert.FileExists(t, filepath.Join(skillDir, "manifest.yaml"))
	assert.FileExists(t, filepath.Join(skillDir, "prompts.yaml"))
}

func TestScaffoldSkill_ManifestParsesCorrectly(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "my-skill", "A custom skill"))

	m, err := plugins.LoadManifestFromDir(skillDir)
	require.NoError(t, err)
	assert.Equal(t, "my-skill", m.Metadata.Name)
	assert.Equal(t, "0.1.0", m.Metadata.Version)
	assert.Equal(t, "A custom skill", m.Metadata.Description)
	assert.Equal(t, 1, m.Spec.Tier)
	assert.Equal(t, "skills", m.Spec.Category)
}

func TestScaffoldSkill_PromptsParsesCorrectly(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "test-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "test-skill", "Test description"))

	data, err := os.ReadFile(filepath.Join(skillDir, "prompts.yaml"))
	require.NoError(t, err)

	parsed, err := plugins.ParsePluginYAML(data)
	require.NoError(t, err)

	promptsRaw, ok := parsed["prompts"]
	require.True(t, ok, "prompts.yaml must have 'prompts' key")

	promptsMap, ok := promptsRaw.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, promptsMap, "test-skill")
}

func TestScaffoldSkill_TemplateContainsInputVariable(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "my-skill", "desc"))

	data, err := os.ReadFile(filepath.Join(skillDir, "prompts.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "{{.input}}")
}

func TestScaffoldSkill_LoadsViaSkillLoader(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "scaffold-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "scaffold-skill", "Scaffolded skill"))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	skill, err := loader.Get("scaffold-skill")
	require.NoError(t, err)
	assert.Equal(t, "scaffold-skill", skill.Name)
	assert.NotEmpty(t, skill.Prompts)
}

func TestScaffoldSkill_DispatchWorks(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "dispatch-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "dispatch-skill", "Dispatch test"))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	dispatcher := NewSlashDispatcher(loader)
	assert.True(t, dispatcher.IsSlashCommand("/dispatch-skill hello"))

	result, err := dispatcher.Dispatch("/dispatch-skill hello world")
	require.NoError(t, err)
	assert.Equal(t, "hello world", strings.TrimSpace(result))
}

func TestScaffoldSkill_PublishCompatibility(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "pub-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "pub-skill", "Publish test skill"))

	// LoadManifestFromDir applies the same validation as the publish flow.
	m, err := plugins.LoadManifestFromDir(skillDir)
	require.NoError(t, err, "scaffolded manifest must pass full publish-flow validation")

	assert.Equal(t, "siply/v1", m.APIVersion)
	assert.Equal(t, "Plugin", m.Kind)
	assert.NotEmpty(t, m.Metadata.Name)
	assert.NotEmpty(t, m.Metadata.Version)
	assert.NotEmpty(t, m.Metadata.SiplyMin)
	assert.NotEmpty(t, m.Metadata.Description)
	assert.NotEmpty(t, m.Metadata.Author)
	assert.NotEmpty(t, m.Metadata.License)
	assert.NotEmpty(t, m.Metadata.Updated)
	assert.Equal(t, 1, m.Spec.Tier)
}

func TestToTitleCase(t *testing.T) {
	cases := []struct {
		name, want string
	}{
		{"my-skill", "My Skill"},
		{"test", "Test"},
		{"a-b-c", "A B C"},
		{"code-review", "Code Review"},
		{"x", "X"},
		{"my-skill-", "My Skill"},
		{"my--skill", "My Skill"},
		{"a-", "A"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, toTitleCase(tc.name), "name=%s", tc.name)
	}
}
