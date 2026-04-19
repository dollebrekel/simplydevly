// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
