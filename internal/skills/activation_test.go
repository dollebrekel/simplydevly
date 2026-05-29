// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestActivator_MatchesSkillNameWithoutSlash(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "bmad-help")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: bmad-help
description: BMad workflow help.
---

# BMad Help

Follow the BMad help workflow.
`), 0o600))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	activator := NewActivator(loader)

	msgs, err := activator.PreQueryHook(context.Background(), []core.Message{
		{Role: "user", Content: "bmad-help hoe los ik skills op?"},
	})

	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.True(t, msgs[0].Hidden)
	assert.True(t, msgs[0].Meta)
	assert.Equal(t, "skill:bmad-help", msgs[0].Source)
	assert.Contains(t, msgs[0].Content, "Follow the BMad help workflow.")
	assert.Contains(t, msgs[0].Content, "Do not simulate files")
	assert.Contains(t, msgs[0].Content, "use the available tools")
	assert.Contains(t, msgs[0].Content, "Treat XML-like workflow markup as private execution instructions")
	assert.Contains(t, msgs[0].Content, "Never include raw workflow tags")
	assert.Equal(t, "bmad-help hoe los ik skills op?", msgs[1].Content)
}

func TestActivator_DedupesPerSession(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, filepath.Join(dir, "bmad-help"), "bmad-help", "help body")

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	activator := NewActivator(loader)
	input := []core.Message{{Role: "user", Content: "bmad-help first"}}

	first, err := activator.PreQueryHook(context.Background(), input)
	require.NoError(t, err)
	second, err := activator.PreQueryHook(context.Background(), input)
	require.NoError(t, err)

	assert.Len(t, first, 2)
	assert.Len(t, second, 1)
}

func TestActivator_MatchesSkillNameWithoutBMadPrefix(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, filepath.Join(dir, "bmad-sprint-status"), "bmad-sprint-status", "status body")
	projectRoot := t.TempDir()
	t.Chdir(projectRoot)
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "_bmad", "bmm"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "_bmad-output", "implementation-artifacts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "_bmad", "bmm", "config.yaml"), []byte(`implementation_artifacts: "{project-root}/_bmad-output/implementation-artifacts"`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "_bmad-output", "implementation-artifacts", "sprint-status.yaml"), []byte(`project: reverseEngineer
development_status:
  fi-1-skill-trigger-metadata-schema: in-progress
`), 0o600))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	activator := NewActivator(loader)

	msgs, err := activator.PreQueryHook(context.Background(), []core.Message{
		{Role: "user", Content: "sprint status"},
	})

	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, "skill:bmad-sprint-status", msgs[0].Source)
	assert.Contains(t, msgs[0].Content, "<workspace_context>")
	assert.Contains(t, msgs[0].Content, filepath.Join(projectRoot, "_bmad-output", "implementation-artifacts", "sprint-status.yaml"))
	assert.Contains(t, msgs[0].Content, "fi-1-skill-trigger-metadata-schema: in-progress")
	assert.NotContains(t, msgs[0].Content, "/mnt/data/MasterMind/IMPLEMENTATION_ARTIFACTS")
}

func TestActivator_DoesNotMatchGDSSkillWithoutGDSPrefix(t *testing.T) {
	dir := t.TempDir()
	writeSkillMD(t, filepath.Join(dir, "gds-sprint-status"), "gds-sprint-status", "gds status body")

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	activator := NewActivator(loader)

	msgs, err := activator.PreQueryHook(context.Background(), []core.Message{
		{Role: "user", Content: "sprint status"},
	})

	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "sprint status", msgs[0].Content)
}
