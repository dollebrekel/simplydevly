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

	"siply.dev/siply/internal/agents"
	"siply.dev/siply/internal/skills"
)

// executeAgentsListWithDirs runs agents list against specific global/project dirs.
func executeAgentsListWithDirs(t *testing.T, globalDir, projectDir string) (string, error) {
	t.Helper()

	loader := agents.NewAgentConfigLoader(globalDir, projectDir)
	require.NoError(t, loader.LoadAll(context.Background()))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	all := loader.List()
	if len(all) == 0 {
		buf.WriteString("No agent configurations installed.\n")
		return buf.String(), nil
	}

	buf.WriteString("NAME\tVERSION\tSOURCE\tDESCRIPTION\n")
	for _, a := range all {
		buf.WriteString(a.Name + "\t" + a.Version + "\t" + a.Source + "\t" + a.Description + "\n")
	}
	return buf.String(), nil
}

func TestAgentsList_NoAgents(t *testing.T) {
	out, err := executeAgentsListWithDirs(t, t.TempDir(), "")
	require.NoError(t, err)
	assert.Contains(t, out, "No agent configurations installed.")
}

func TestAgentsList_WithAgents(t *testing.T) {
	globalDir := t.TempDir()
	writeAgentFixture(t, globalDir, "code-reviewer")

	out, err := executeAgentsListWithDirs(t, globalDir, "")
	require.NoError(t, err)
	assert.Contains(t, out, "code-reviewer")
	assert.Contains(t, out, "global")
}

func TestAgentsList_ShowsProjectOverride(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	writeAgentFixture(t, globalDir, "my-agent")
	writeAgentFixture(t, projectDir, "my-agent")

	loader := agents.NewAgentConfigLoader(globalDir, projectDir)
	require.NoError(t, loader.LoadAll(context.Background()))

	all := loader.List()
	require.Len(t, all, 1)
	assert.Equal(t, "project", all[0].Source, "project-level agent config must override global")
}

// --- create command tests ---

func TestAgentsCreate_Success(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "my-agent")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeAgentsCreate(cmd, "my-agent", "A custom agent", targetDir)
	require.NoError(t, err)

	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, "manifest.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "config.yaml"))
	assert.Contains(t, buf.String(), "my-agent")
	assert.Contains(t, buf.String(), "agent.config: my-agent")
}

func TestAgentsCreate_WithDescription(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "desc-agent")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeAgentsCreate(cmd, "desc-agent", "My custom agent description", targetDir)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(targetDir, "manifest.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "My custom agent description")
}

func TestAgentsCreate_Global(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	targetDir := filepath.Join(tmpHome, "agents", "global-agent")

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)

	err := executeAgentsCreate(cmd, "global-agent", "Global scope agent", targetDir)
	require.NoError(t, err)
	assert.DirExists(t, targetDir)
}

func TestAgentsCreate_Collision(t *testing.T) {
	base := t.TempDir()
	targetDir := filepath.Join(base, "existing-agent")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := executeAgentsCreate(cmd, "existing-agent", "desc", targetDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestAgentsCreate_InvalidName_Uppercase(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := executeAgentsCreate(cmd, "MyAgent", "desc", t.TempDir()+"/MyAgent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase")
}

func TestAgentsCreate_InvalidName_StartsWithDigit(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := executeAgentsCreate(cmd, "1agent", "desc", t.TempDir()+"/1agent")
	require.Error(t, err)
}

func TestAgentsCreate_ReservedName(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	for _, reserved := range []string{"help", "yolo", "auto-accept", "default", "code", "chat", "plan", "research", "marketplace"} {
		err := executeAgentsCreate(cmd, reserved, "desc", t.TempDir()+"/"+reserved)
		require.Error(t, err, "name=%s", reserved)
		assert.ErrorIs(t, err, skills.ErrReservedCommand, "name=%s", reserved)
	}
}

func TestResolveAgentCreateDir_Global(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("SIPLY_HOME", tmpHome)

	dir, err := resolveAgentCreateDir("my-agent", true)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpHome, "agents", "my-agent"), dir)
}

func TestValidateAgentName(t *testing.T) {
	valid := []string{"my-agent", "code-review", "a", "abc123", "a-b-c"}
	for _, name := range valid {
		assert.NoError(t, validateAgentName(name), "name=%s", name)
	}

	invalid := []string{"MyAgent", "1agent", "", "-agent", "agent!", strings.Repeat("a", 64)}
	for _, name := range invalid {
		assert.Error(t, validateAgentName(name), "name=%s", name)
	}
}

// writeAgentFixture creates a minimal valid agent config in dir/name.
func writeAgentFixture(t *testing.T, dir, name string) {
	t.Helper()
	agentDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(agentDir, 0o755))

	manifest := "apiVersion: siply/v1\nkind: Plugin\nmetadata:\n  name: " + name +
		"\n  version: 1.0.0\n  siply_min: 0.1.0\n  description: Test agent\n  author: test\n  license: Apache-2.0\n  updated: \"2026-01-01\"\nspec:\n  tier: 1\n  category: agents\n  capabilities: {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "manifest.yaml"), []byte(manifest), 0o600))

	config := "system_prompt: \"You are a test agent.\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(agentDir, "config.yaml"), []byte(config), 0o600))
}
