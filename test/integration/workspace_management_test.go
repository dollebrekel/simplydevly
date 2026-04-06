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

	"siply.dev/siply/internal/workspace"
)

// TestWorkspaceManagement_FullLifecycle tests the complete workspace flow:
// detect from git dir → register → re-init → workspace still known.
func TestWorkspaceManagement_FullLifecycle(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	ctx := context.Background()

	// Create fake git repo.
	require.NoError(t, os.Mkdir(filepath.Join(projectDir, ".git"), 0755))

	// Phase 1: Detect workspace from git root.
	m1 := workspace.NewManager(globalDir)
	require.NoError(t, m1.Init(ctx))

	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(projectDir))
	defer func() { _ = os.Chdir(origDir) }()

	ws, err := m1.Detect(ctx)
	require.NoError(t, err)
	require.NotNil(t, ws)
	assert.Equal(t, filepath.Base(projectDir), ws.Name)

	// Verify file was created.
	wsPath := filepath.Join(globalDir, "workspaces.yaml")
	_, err = os.Stat(wsPath)
	require.NoError(t, err, "workspaces file should exist after detection")

	// Phase 2: Re-init from file — workspace still known.
	m2 := workspace.NewManager(globalDir)
	require.NoError(t, m2.Init(ctx))

	list := m2.List(ctx)
	require.Len(t, list, 1)
	assert.Equal(t, filepath.Base(projectDir), list[0].Name)

	// Can open it.
	ws2, err := m2.Open(ctx, filepath.Base(projectDir))
	require.NoError(t, err)
	assert.Equal(t, projectDir, ws2.RootDir)
}

// TestWorkspaceManagement_CreateSwitchPreserveState tests:
// create workspace → switch away → switch back → state preserved.
func TestWorkspaceManagement_CreateSwitchPreserveState(t *testing.T) {
	globalDir := t.TempDir()
	ctx := context.Background()

	dir1 := filepath.Join(t.TempDir(), "proj1")
	dir2 := filepath.Join(t.TempDir(), "proj2")
	require.NoError(t, os.MkdirAll(filepath.Join(dir1, ".git"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir2, ".git"), 0755))

	m := workspace.NewManager(globalDir)
	require.NoError(t, m.Init(ctx))
	require.NoError(t, m.Start(ctx))

	// Create two workspaces.
	ws1, err := m.Create(ctx, "proj1", dir1)
	require.NoError(t, err)
	assert.Equal(t, "proj1", ws1.Name)

	ws2, err := m.Create(ctx, "proj2", dir2)
	require.NoError(t, err)
	assert.Equal(t, "proj2", ws2.Name)

	// Active is proj2 (last created).
	assert.Equal(t, "proj2", m.Active().Name)

	// Switch to proj1.
	switched, err := m.Switch(ctx, "proj1")
	require.NoError(t, err)
	assert.Equal(t, "proj1", switched.Name)
	assert.Equal(t, "proj1", m.Active().Name)

	// Stop to persist state.
	require.NoError(t, m.Stop(ctx))

	// Re-init and verify both workspaces still exist.
	m2 := workspace.NewManager(globalDir)
	require.NoError(t, m2.Init(ctx))

	list := m2.List(ctx)
	require.Len(t, list, 2)
	assert.Equal(t, "proj1", list[0].Name)
	assert.Equal(t, "proj2", list[1].Name)
}
