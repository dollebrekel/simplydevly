// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"

	"siply.dev/siply/internal/fileutil"
)

// registerFSAPI registers the siply.fs table on the siply global.
func registerFSAPI(L *lua.LState, siply *lua.LTable, plugin *Tier2Plugin) {
	fsTbl := L.NewTable()
	fsTbl.RawSetString("read", L.NewFunction(newSiplyFSRead(plugin)))
	fsTbl.RawSetString("write", L.NewFunction(newSiplyFSWrite(plugin)))
	fsTbl.RawSetString("list", L.NewFunction(newSiplyFSList(plugin)))
	fsTbl.RawSetString("exists", L.NewFunction(newSiplyFSExists(plugin)))
	siply.RawSetString("fs", fsTbl)
}

// resolveSafePath validates and resolves a relative path within the plugin's state directory.
// Returns the resolved absolute path or an error if the path escapes the state dir.
const maxFSFileSize = 10 << 20 // 10MB

func resolveSafePath(stateDir, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("%w: absolute paths not allowed", ErrLuaSandboxViolation)
	}

	resolved := filepath.Join(stateDir, filepath.Clean(relativePath))
	resolved = filepath.Clean(resolved)

	cleanBase := filepath.Clean(stateDir)
	if resolved != cleanBase && !strings.HasPrefix(resolved, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes state directory", ErrLuaSandboxViolation)
	}

	return resolved, nil
}

func newSiplyFSRead(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		relPath := L.CheckString(1)

		resolved, err := resolveSafePath(plugin.StateDir, relPath)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		info, err := os.Stat(resolved)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		if info.Size() > maxFSFileSize {
			L.Push(lua.LNil)
			L.Push(lua.LString(fmt.Sprintf("file too large: %d bytes exceeds %d byte limit", info.Size(), maxFSFileSize)))
			return 2
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LString(string(data)))
		return 1
	}
}

func newSiplyFSWrite(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		relPath := L.CheckString(1)
		content := L.CheckString(2)

		if len(content) > maxFSFileSize {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(fmt.Sprintf("content too large: %d bytes exceeds %d byte limit", len(content), maxFSFileSize)))
			return 2
		}

		resolved, err := resolveSafePath(plugin.StateDir, relPath)
		if err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		dir := filepath.Dir(resolved)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		if err := fileutil.AtomicWriteFile(resolved, []byte(content), 0o644); err != nil {
			L.Push(lua.LBool(false))
			L.Push(lua.LString(err.Error()))
			return 2
		}

		L.Push(lua.LBool(true))
		return 1
	}
}

func newSiplyFSList(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		relPath := L.OptString(1, ".")

		resolved, err := resolveSafePath(plugin.StateDir, relPath)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		entries, err := os.ReadDir(resolved)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		result := L.NewTable()
		for _, entry := range entries {
			result.Append(lua.LString(entry.Name()))
		}

		L.Push(result)
		return 1
	}
}

func newSiplyFSExists(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		relPath := L.CheckString(1)

		resolved, err := resolveSafePath(plugin.StateDir, relPath)
		if err != nil {
			L.Push(lua.LBool(false))
			return 1
		}

		_, err = os.Stat(resolved)
		L.Push(lua.LBool(err == nil))
		return 1
	}
}
