// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testdataDir = "testdata"

func TestSkillLoader_LoadFromGlobal(t *testing.T) {
	globalDir := testdataDir
	loader := NewSkillLoader(globalDir, "")
	err := loader.LoadAll(context.Background())
	require.NoError(t, err)

	skill, err := loader.Get("valid-skill")
	require.NoError(t, err)
	assert.Equal(t, "valid-skill", skill.Name)
	assert.Equal(t, "1.0.0", skill.Version)
	assert.Equal(t, "global", skill.Source)
	assert.NotEmpty(t, skill.Prompts)
}

func TestSkillLoader_LoadFromProject(t *testing.T) {
	// Project dir has valid-skill; global dir is empty temp dir.
	globalDir := t.TempDir()
	projectDir := testdataDir

	loader := NewSkillLoader(globalDir, projectDir)
	err := loader.LoadAll(context.Background())
	require.NoError(t, err)

	skill, err := loader.Get("valid-skill")
	require.NoError(t, err)
	assert.Equal(t, "project", skill.Source)
}

func TestSkillLoader_ProjectOverridesGlobal(t *testing.T) {
	// Create a global dir with a copy of valid-skill.
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	copySkillDir(t, filepath.Join(testdataDir, "valid-skill"), filepath.Join(globalDir, "valid-skill"))
	copySkillDir(t, filepath.Join(testdataDir, "valid-skill"), filepath.Join(projectDir, "valid-skill"))

	loader := NewSkillLoader(globalDir, projectDir)
	err := loader.LoadAll(context.Background())
	require.NoError(t, err)

	skill, err := loader.Get("valid-skill")
	require.NoError(t, err)
	// Project-level must win (AC#4).
	assert.Equal(t, "project", skill.Source)
}

func TestSkillLoader_MissingManifest(t *testing.T) {
	// invalid-skill has no prompts file — loader skips it silently.
	globalDir := testdataDir
	loader := NewSkillLoader(globalDir, "")
	err := loader.LoadAll(context.Background())
	require.NoError(t, err)

	// invalid-skill has no prompts, so it is skipped.
	_, err = loader.Get("invalid-skill")
	assert.ErrorIs(t, err, ErrSkillNotFound)
}

func TestSkillLoader_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bad-yaml-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	// Write a valid manifest.
	require.NoError(t, copyFile(
		filepath.Join(testdataDir, "valid-skill", "manifest.yaml"),
		filepath.Join(skillDir, "manifest.yaml"),
	))

	// Write invalid YAML as prompts.yaml.
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "prompts.yaml"), []byte("prompts: !!python/object {}"), 0600))

	loader := NewSkillLoader(dir, "")
	err := loader.LoadAll(context.Background())
	require.NoError(t, err) // skipped, not fatal

	_, err = loader.Get("bad-yaml-skill")
	assert.ErrorIs(t, err, ErrSkillNotFound)
}

func TestSkillLoader_Get_NotFound(t *testing.T) {
	loader := NewSkillLoader(t.TempDir(), "")
	_ = loader.LoadAll(context.Background())
	_, err := loader.Get("nonexistent")
	assert.ErrorIs(t, err, ErrSkillNotFound)
}

func TestSkillLoader_List_Sorted(t *testing.T) {
	// Create two skills: "beta-skill" and "alpha-skill".
	dir := t.TempDir()
	for _, name := range []string{"alpha-skill", "beta-skill"} {
		skillDir := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		writeTestSkill(t, skillDir, name)
	}

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	skills := loader.List()
	require.Len(t, skills, 2)
	assert.Equal(t, "alpha-skill", skills[0].Name)
	assert.Equal(t, "beta-skill", skills[1].Name)
}

func TestSkillLoader_MissingDir_NotError(t *testing.T) {
	loader := NewSkillLoader("/nonexistent/path", "")
	err := loader.LoadAll(context.Background())
	assert.NoError(t, err) // missing dir is not an error
}

func TestGlobalDir_DefaultPath(t *testing.T) {
	t.Setenv("SIPLY_HOME", "")
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".siply", "skills")
	assert.Equal(t, expected, GlobalDir(home))
}

func TestGlobalDir_SiplyHomeEnv(t *testing.T) {
	t.Setenv("SIPLY_HOME", "/custom/siply")
	assert.Equal(t, "/custom/siply/skills", GlobalDir("/home/user"))
}

// --- helpers ---

func copySkillDir(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dst, 0755))
	for _, fname := range []string{"manifest.yaml", "prompts.yaml"} {
		srcPath := filepath.Join(src, fname)
		if _, err := os.Stat(srcPath); errors.Is(err, os.ErrNotExist) {
			continue
		}
		require.NoError(t, copyFile(srcPath, filepath.Join(dst, fname)))
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func TestSkillLoader_LoadsScaffoldedSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "loaded-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "loaded-skill", "Scaffold load test"))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	skill, err := loader.Get("loaded-skill")
	require.NoError(t, err)
	assert.Equal(t, "loaded-skill", skill.Name)
	assert.Equal(t, "global", skill.Source)
	assert.NotEmpty(t, skill.Prompts)
}

func TestSlashDispatcher_DispatchScaffoldedSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "piped-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, ScaffoldSkill(skillDir, "piped-skill", "Dispatch scaffold test"))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	d := NewSlashDispatcher(loader)
	assert.True(t, d.IsSlashCommand("/piped-skill some input"))

	result, err := d.Dispatch("/piped-skill test input")
	require.NoError(t, err)
	assert.Contains(t, result, "test input")
}

func writeTestSkill(t *testing.T, dir, name string) {
	t.Helper()
	manifest := "apiVersion: siply/v1\nkind: Plugin\nmetadata:\n  name: " + name +
		"\n  version: 1.0.0\n  siply_min: 0.1.0\n  description: Test skill " + name +
		"\n  author: test\n  license: Apache-2.0\n  updated: \"2026-01-01\"\nspec:\n  tier: 1\n  capabilities: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0600))

	prompts := "prompts:\n  " + name + ":\n    name: " + name + "\n    description: test\n    template: |\n      {{.input}}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prompts.yaml"), []byte(prompts), 0600))
}
