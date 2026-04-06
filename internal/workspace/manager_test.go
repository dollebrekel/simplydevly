// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	require.NotNil(t, m)
	assert.Equal(t, dir, m.globalDir)
	assert.False(t, m.initialized)
}

func TestInit_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", ".siply")
	m := NewManager(dir)

	require.NoError(t, m.Init(context.Background()))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(dirPermissions), info.Mode().Perm())
}

func TestHealth_BeforeInit(t *testing.T) {
	m := NewManager(t.TempDir())
	err := m.Health()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestHealth_AfterInit(t *testing.T) {
	m := NewManager(t.TempDir())
	require.NoError(t, m.Init(context.Background()))
	assert.NoError(t, m.Health())
}

func TestStartStop_NoOps(t *testing.T) {
	m := NewManager(t.TempDir())
	require.NoError(t, m.Init(context.Background()))
	assert.NoError(t, m.Start(context.Background()))
	assert.NoError(t, m.Stop(context.Background()))
}

func TestCreate_RegistersAndPersists(t *testing.T) {
	// AC#1: workspace created for a project
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Create a fake git repo.
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	ws, err := m.Create(context.Background(), "test-project", projectDir)
	require.NoError(t, err)
	assert.Equal(t, "test-project", ws.Name)
	assert.Equal(t, projectDir, ws.RootDir)
	assert.Equal(t, projectDir, ws.GitRoot)
	assert.Equal(t, filepath.Join(projectDir, ".siply"), ws.ConfigDir)

	// Verify persisted to file.
	path := filepath.Join(globalDir, "workspaces.yaml")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(filePermissions), info.Mode().Perm())

	// Verify content contains workspace.
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "test-project")
}

func TestCreate_DuplicateReturnsError(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	_, err := m.Create(context.Background(), "dup", projectDir)
	require.NoError(t, err)

	_, err = m.Create(context.Background(), "dup", projectDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `workspace "dup" already exists`)
}

func TestOpen_ErrorForUnknown(t *testing.T) {
	m := NewManager(t.TempDir())
	require.NoError(t, m.Init(context.Background()))

	_, err := m.Open(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown workspace "nonexistent"`)
}

func TestOpen_ReturnsKnownWorkspace(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))
	_, err := m.Create(context.Background(), "my-ws", projectDir)
	require.NoError(t, err)

	ws, err := m.Open(context.Background(), "my-ws")
	require.NoError(t, err)
	assert.Equal(t, "my-ws", ws.Name)
	assert.Equal(t, projectDir, ws.RootDir)
}

func TestList_ReturnsSorted(t *testing.T) {
	// AC#4: list shows all known workspaces
	globalDir := t.TempDir()
	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	for _, name := range []string{"charlie", "alpha", "bravo"} {
		dir := filepath.Join(t.TempDir(), name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		_, err := m.Create(context.Background(), name, dir)
		require.NoError(t, err)
	}

	list := m.List(context.Background())
	require.Len(t, list, 3)
	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "bravo", list[1].Name)
	assert.Equal(t, "charlie", list[2].Name)
}

func TestSwitch_ChangesActive(t *testing.T) {
	// AC#5: switch to a different workspace
	globalDir := t.TempDir()
	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	dir1 := filepath.Join(t.TempDir(), "proj1")
	dir2 := filepath.Join(t.TempDir(), "proj2")
	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.MkdirAll(dir2, 0755))

	_, err := m.Create(context.Background(), "proj1", dir1)
	require.NoError(t, err)
	_, err = m.Create(context.Background(), "proj2", dir2)
	require.NoError(t, err)

	assert.Equal(t, "proj2", m.Active().Name) // last created is active

	ws, err := m.Switch(context.Background(), "proj1")
	require.NoError(t, err)
	assert.Equal(t, "proj1", ws.Name)
	assert.Equal(t, "proj1", m.Active().Name)
}

func TestDetectGitRoot(t *testing.T) {
	// AC#7: auto-detect from git root
	projectDir := t.TempDir()
	gitDir := filepath.Join(projectDir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0755))

	// From project root.
	root, err := detectGitRoot(projectDir)
	require.NoError(t, err)
	assert.Equal(t, projectDir, root)

	// From subdirectory.
	subDir := filepath.Join(projectDir, "src", "pkg")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	root, err = detectGitRoot(subDir)
	require.NoError(t, err)
	assert.Equal(t, projectDir, root)
}

func TestDetectGitRoot_NoGitReturnsEmpty(t *testing.T) {
	// No git root → empty string, not error
	dir := t.TempDir()
	root, err := detectGitRoot(dir)
	require.NoError(t, err)
	assert.Equal(t, "", root)
}

func TestDetect_NoGitReturnsNil(t *testing.T) {
	// AC#7: no git root returns nil workspace (not error)
	globalDir := t.TempDir()
	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	// Change to a dir without .git (TempDir has no .git ancestors).
	origDir, _ := os.Getwd()
	noGitDir := t.TempDir()
	require.NoError(t, os.Chdir(noGitDir))
	defer func() { _ = os.Chdir(origDir) }()

	ws, err := m.Detect(context.Background())
	require.NoError(t, err)
	assert.Nil(t, ws)
}

func TestDetect_RegistersNewWorkspace(t *testing.T) {
	// AC#7: auto-detect from current directory's git root
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(projectDir))
	defer func() { _ = os.Chdir(origDir) }()

	ws, err := m.Detect(context.Background())
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, filepath.Base(projectDir), ws.Name)
	assert.Equal(t, projectDir, ws.GitRoot)

	// Verify registered.
	list := m.List(context.Background())
	assert.Len(t, list, 1)
}

func TestPersistAcrossReInit(t *testing.T) {
	// AC#3: workspace state persists between sessions
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m1 := NewManager(globalDir)
	require.NoError(t, m1.Init(context.Background()))
	_, err := m1.Create(context.Background(), "persisted", projectDir)
	require.NoError(t, err)

	// Re-init and verify workspace still known.
	m2 := NewManager(globalDir)
	require.NoError(t, m2.Init(context.Background()))

	ws, err := m2.Open(context.Background(), "persisted")
	require.NoError(t, err)
	assert.Equal(t, "persisted", ws.Name)
	assert.Equal(t, projectDir, ws.RootDir)
}

func TestConfigDir_ReturnsActiveWorkspacePath(t *testing.T) {
	// AC#6: workspace-scoped config loaded when active
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	assert.Equal(t, "", m.ConfigDir()) // no active workspace

	_, err := m.Create(context.Background(), "my-proj", projectDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(projectDir, ".siply"), m.ConfigDir())
}

func TestWorkspaceState_PersistsOnStop(t *testing.T) {
	// AC#3: state persists between sessions
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))
	_, err := m.Create(context.Background(), "stateful", projectDir)
	require.NoError(t, err)

	require.NoError(t, m.Stop(context.Background()))

	// Verify state file created.
	statePath := filepath.Join(projectDir, ".siply", "state.yaml")
	_, err = os.Stat(statePath)
	require.NoError(t, err, "state file should exist after Stop")

	info, err := os.Stat(statePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(filePermissions), info.Mode().Perm())
}

func TestCorruptYAML_ProducesActionableError(t *testing.T) {
	globalDir := t.TempDir()
	wsFile := filepath.Join(globalDir, "workspaces.yaml")
	require.NoError(t, os.WriteFile(wsFile, []byte("{{invalid yaml"), filePermissions))

	m := NewManager(globalDir)
	err := m.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace: failed to parse workspaces file")
}

func TestPermissionsSelfHeal(t *testing.T) {
	globalDir := t.TempDir()
	wsFile := filepath.Join(globalDir, "workspaces.yaml")
	content := "workspaces:\n  test:\n    root_dir: /tmp/test\n    git_root: /tmp/test\n"
	require.NoError(t, os.WriteFile(wsFile, []byte(content), 0644))

	m := NewManager(globalDir)
	require.NoError(t, m.Init(context.Background()))

	info, err := os.Stat(wsFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(filePermissions), info.Mode().Perm())
}

func TestWorkspaceName(t *testing.T) {
	tests := []struct {
		gitRoot  string
		expected string
	}{
		{"/home/user/projects/myapp", "myapp"},
		{"/tmp/test-project", "test-project"},
		{"/", "/"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, workspaceName(tc.gitRoot))
		})
	}
}

func TestActive_NilWhenNoWorkspace(t *testing.T) {
	m := NewManager(t.TempDir())
	require.NoError(t, m.Init(context.Background()))
	assert.Nil(t, m.Active())
}
