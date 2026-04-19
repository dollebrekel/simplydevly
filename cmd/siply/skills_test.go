// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/skills"
)

// executeSkillsListWithDirs runs skills list against specific global/project dirs.
func executeSkillsListWithDirs(t *testing.T, globalDir, projectDir string) (string, error) {
	t.Helper()

	loader := skills.NewSkillLoader(globalDir, projectDir)
	require.NoError(t, loader.LoadAll(context.Background()))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	all := loader.List()
	if len(all) == 0 {
		buf.WriteString("No skills installed.\n")
		return buf.String(), nil
	}

	buf.WriteString("NAME\tVERSION\tSOURCE\tDESCRIPTION\n")
	for _, s := range all {
		buf.WriteString(s.Name + "\t" + s.Version + "\t" + s.Source + "\t" + s.Description + "\n")
	}
	return buf.String(), nil
}

func TestSkillsList_NoSkills(t *testing.T) {
	out, err := executeSkillsListWithDirs(t, t.TempDir(), "")
	require.NoError(t, err)
	assert.Contains(t, out, "No skills installed.")
}

func TestSkillsList_WithSkills(t *testing.T) {
	// Use the testdata dir from internal/skills as our global dir.
	globalDir := filepath.Join("..", "..", "internal", "skills", "testdata")
	out, err := executeSkillsListWithDirs(t, globalDir, "")
	require.NoError(t, err)
	assert.Contains(t, out, "valid-skill")
	assert.Contains(t, out, "global")
}

func TestSkillsList_ShowsProjectOverride(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Write same skill to both dirs.
	writeSkillFixture(t, globalDir, "my-skill")
	writeSkillFixture(t, projectDir, "my-skill")

	loader := skills.NewSkillLoader(globalDir, projectDir)
	require.NoError(t, loader.LoadAll(context.Background()))

	all := loader.List()
	require.Len(t, all, 1)
	assert.Equal(t, "project", all[0].Source, "project-level skill must override global")
}

// --- create command tests ---

func TestSkillsCreate_Success(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "my-skill")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeSkillsCreate(cmd, "my-skill", "A custom skill", targetDir)
	require.NoError(t, err)

	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, "manifest.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "prompts.yaml"))
	assert.Contains(t, buf.String(), "my-skill")
	assert.Contains(t, buf.String(), "/my-skill")
}

func TestSkillsCreate_WithDescription(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "desc-skill")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeSkillsCreate(cmd, "desc-skill", "My custom description", targetDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetDir, "manifest.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "My custom description")
}

func TestSkillsCreate_Global(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	targetDir := filepath.Join(tmpHome, "skills", "global-skill")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeSkillsCreate(cmd, "global-skill", "Global scope skill", targetDir)
	require.NoError(t, err)
	assert.DirExists(t, targetDir)
}

func TestSkillsCreate_Collision(t *testing.T) {
	base := t.TempDir()
	targetDir := filepath.Join(base, "existing-skill")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := executeSkillsCreate(cmd, "existing-skill", "desc", targetDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestSkillsCreate_InvalidName_Uppercase(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := executeSkillsCreate(cmd, "MySkill", "desc", t.TempDir()+"/MySkill")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase")
}

func TestSkillsCreate_InvalidName_StartsWithDigit(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := executeSkillsCreate(cmd, "1skill", "desc", t.TempDir()+"/1skill")
	require.Error(t, err)
}

func TestSkillsCreate_ReservedName(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	for _, reserved := range []string{"help", "yolo", "code", "chat", "plan", "research", "marketplace"} {
		err := executeSkillsCreate(cmd, reserved, "desc", t.TempDir()+"/"+reserved)
		require.Error(t, err, "name=%s", reserved)
		assert.ErrorIs(t, err, skills.ErrReservedCommand, "name=%s", reserved)
	}
}

func TestResolveSkillCreateDir_Global(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	dir, err := resolveSkillCreateDir("my-skill", true)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpHome, "skills", "my-skill"), dir)
}

func TestValidateSkillName(t *testing.T) {
	valid := []string{"my-skill", "code-review", "a", "abc123", "a-b-c"}
	for _, name := range valid {
		assert.NoError(t, validateSkillName(name), "name=%s", name)
	}

	invalid := []string{"MySkill", "1skill", "", "-skill", "skill!", strings.Repeat("a", 64)}
	for _, name := range invalid {
		assert.Error(t, validateSkillName(name), "name=%s", name)
	}
}

// writeSkillFixture creates a minimal valid skill in dir/name.
func writeSkillFixture(t *testing.T, dir, name string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	manifest := "apiVersion: siply/v1\nkind: Plugin\nmetadata:\n  name: " + name +
		"\n  version: 1.0.0\n  siply_min: 0.1.0\n  description: Test skill\n  author: test\n  license: Apache-2.0\n  updated: \"2026-01-01\"\nspec:\n  tier: 1\n  capabilities: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "manifest.yaml"), []byte(manifest), 0600))

	prompts := "prompts:\n  " + name + ":\n    name: " + name + "\n    description: test\n    template: |\n      {{.input}}\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "prompts.yaml"), []byte(prompts), 0600))
}
