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

func TestLoadIndex_ProjectIndexHasPriority(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	writeIndexFile(t, projectRoot, "project-skill")
	writeIndexFile(t, homeDir, "global-skill")

	idx, warnings, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     homeDir,
	})

	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, idx.Skills, 1)
	assert.Equal(t, "project-skill", idx.Skills[0].Name)
	assert.Equal(t, filepath.Join(projectRoot, ".siply", "skills.index.json"), idx.LoadedFrom)
}

func TestLoadIndex_GlobalFallback(t *testing.T) {
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	writeIndexFile(t, homeDir, "global-skill")

	idx, warnings, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     homeDir,
	})

	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, idx.Skills, 1)
	assert.Equal(t, "global-skill", idx.Skills[0].Name)
	assert.Equal(t, filepath.Join(homeDir, ".siply", "skills.index.json"), idx.LoadedFrom)
}

func TestLoadIndex_MissingIndexSynthesizesLegacySkills(t *testing.T) {
	globalDir := t.TempDir()
	skillDir := filepath.Join(globalDir, "legacy-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	writeTestSkill(t, skillDir, "legacy-skill")

	loader := NewSkillLoader(globalDir, "")
	require.NoError(t, loader.LoadAll(context.Background()))

	idx, warnings, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: t.TempDir(),
		HomeDir:     t.TempDir(),
		Loader:      loader,
	})

	require.NoError(t, err)
	require.Empty(t, warnings)
	require.True(t, idx.Synthesized)
	require.Len(t, idx.Skills, 1)
	assert.Equal(t, "legacy-skill", idx.Skills[0].Name)
	assert.Equal(t, SkillKindLegacyPrompt, idx.Skills[0].Kind)
	assert.Contains(t, idx.Skills[0].Activation.Commands, "/legacy-skill")
	assert.Contains(t, idx.Skills[0].Activation.Commands, "$legacy-skill")
}

func TestLoadIndex_UnsupportedVersion(t *testing.T) {
	projectRoot := t.TempDir()
	writeRawIndex(t, projectRoot, `{"version":2}`)

	_, _, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedIndexVersion)
}

func TestLoadIndex_InvalidJSON(t *testing.T) {
	projectRoot := t.TempDir()
	writeRawIndex(t, projectRoot, `{"version":1`)

	_, _, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse index")
}

func TestLoadIndex_InvalidPathEscapingProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	writeRawIndex(t, projectRoot, `{
		"version": 1,
		"sources": [{"type":"skill-md-directory","path":"../outside","format":"SKILL.md"}]
	}`)

	_, _, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidIndexPath)
}

func TestLoadIndex_DuplicateSkillNames(t *testing.T) {
	projectRoot := t.TempDir()
	writeRawIndex(t, projectRoot, `{
		"version": 1,
		"skills": [
			{"name":"dup","source":".siply/skills/dup/SKILL.md"},
			{"name":"dup","source":".agents/skills/dup/SKILL.md"}
		]
	}`)

	_, _, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateSkillName)
}

func TestLoadIndex_ClaudeFastRulesValidReference(t *testing.T) {
	projectRoot := t.TempDir()
	writeRawIndex(t, projectRoot, `{
		"version": 1,
		"sources": [{"type":"claudefast-rules","path":".claude/skills/skill-rules.json"}]
	}`)
	writeFile(t, filepath.Join(projectRoot, ".claude", "skills", "skill-rules.json"), `{"rules":[]}`)

	idx, warnings, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Len(t, idx.ClaudeFastRules, 1)
	assert.Equal(t, ".claude/skills/skill-rules.json", idx.ClaudeFastRules[0].Path)
}

func TestLoadIndex_InvalidClaudeFastRulesWarnsOnly(t *testing.T) {
	projectRoot := t.TempDir()
	writeRawIndex(t, projectRoot, `{
		"version": 1,
		"sources": [{"type":"claudefast-rules","path":".claude/skills/skill-rules.json"}]
	}`)
	writeFile(t, filepath.Join(projectRoot, ".claude", "skills", "skill-rules.json"), `{broken`)

	idx, warnings, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.NoError(t, err)
	require.Len(t, warnings, 1)
	require.Empty(t, idx.ClaudeFastRules)
	assert.Contains(t, warnings[0].Error(), "parse ClaudeFast rules")
}

func TestLoadIndex_IndexSizeLimit(t *testing.T) {
	projectRoot := t.TempDir()
	path := filepath.Join(projectRoot, ".siply", "skills.index.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, make([]byte, MaxIndexFileSize+1), 0o600))

	_, _, err := LoadIndex(context.Background(), LoadIndexOptions{
		ProjectRoot: projectRoot,
		HomeDir:     t.TempDir(),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestLoadIndex_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := LoadIndex(ctx, LoadIndexOptions{
		ProjectRoot: t.TempDir(),
		HomeDir:     t.TempDir(),
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func writeIndexFile(t *testing.T, root, skillName string) {
	t.Helper()
	writeRawIndex(t, root, `{
		"version": 1,
		"sources": [
			{"type":"skill-md-directory","path":".siply/skills","format":"SKILL.md"},
			{"type":"claudefast-rules","path":".claude/skills/skill-rules.json"}
		],
		"skills": [
			{
				"name":"`+skillName+`",
				"source":".siply/skills/`+skillName+`/SKILL.md",
				"description":"test skill",
				"activation":{"keywords":["`+skillName+`"]},
				"loading":{"mode":"on_demand","dedupePerSession":true,"visibleInChat":false}
			}
		],
		"smartPacks": [
			{"name":"go-dev-pack","skills":["`+skillName+`"],"compactDescription":"Go development pack."}
		]
	}`)
}

func writeRawIndex(t *testing.T, root, body string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".siply", "skills.index.json"), body)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}
