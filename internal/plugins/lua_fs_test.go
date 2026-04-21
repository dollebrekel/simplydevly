// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	lua "github.com/yuin/gopher-lua"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSafePath_ValidRelative(t *testing.T) {
	t.Parallel()
	resolved, err := resolveSafePath("/base/dir", "data.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/base/dir", "data.json"), resolved)
}

func TestResolveSafePath_RejectAbsolute(t *testing.T) {
	t.Parallel()
	_, err := resolveSafePath("/base/dir", "/etc/passwd")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLuaSandboxViolation))
}

func TestResolveSafePath_RejectTraversal(t *testing.T) {
	t.Parallel()
	_, err := resolveSafePath("/base/dir", "../../../etc/passwd")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrLuaSandboxViolation))
}

func TestResolveSafePath_Subdirectory(t *testing.T) {
	t.Parallel()
	resolved, err := resolveSafePath("/base/dir", "sub/file.txt")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/base/dir", "sub", "file.txt"), resolved)
}

func TestSiplyFSReadWrite(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "fs-test", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	// Write a file.
	err := L.DoString(`result = siply.fs.write("test.txt", "hello world")`)
	require.NoError(t, err)
	val := L.GetGlobal("result")
	assert.Equal(t, lua.LBool(true), val)

	// Read it back.
	err = L.DoString(`content = siply.fs.read("test.txt")`)
	require.NoError(t, err)
	content := L.GetGlobal("content")
	assert.Equal(t, lua.LString("hello world"), content)
}

func TestSiplyFSPathEscapeBlocked(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "fs-escape", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`content, err_msg = siply.fs.read("../../etc/passwd")`)
	require.NoError(t, err)

	content := L.GetGlobal("content")
	assert.Equal(t, lua.LNil, content)

	errMsg := L.GetGlobal("err_msg")
	assert.Contains(t, errMsg.String(), "sandbox violation")
}

func TestSiplyFSAbsolutePathRejected(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "fs-abs", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`content, err_msg = siply.fs.read("/etc/passwd")`)
	require.NoError(t, err)

	content := L.GetGlobal("content")
	assert.Equal(t, lua.LNil, content)

	errMsg := L.GetGlobal("err_msg")
	assert.Contains(t, errMsg.String(), "absolute")
}

func TestSiplyFSList(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "b.txt"), []byte("b"), 0o644))

	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "fs-list", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`files = siply.fs.list(".")`)
	require.NoError(t, err)

	val := L.GetGlobal("files")
	tbl, ok := val.(*lua.LTable)
	require.True(t, ok)
	assert.Equal(t, 2, tbl.Len())
}

func TestSiplyFSExists(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "exists.txt"), []byte("x"), 0o644))

	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "fs-exists", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`
		found = siply.fs.exists("exists.txt")
		not_found = siply.fs.exists("nope.txt")
	`)
	require.NoError(t, err)

	assert.Equal(t, lua.LBool(true), L.GetGlobal("found"))
	assert.Equal(t, lua.LBool(false), L.GetGlobal("not_found"))
}
