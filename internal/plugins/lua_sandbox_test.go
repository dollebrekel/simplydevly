// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSandboxedState_BlockedGlobals(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	for _, name := range []string{"os", "io", "loadfile", "dofile", "debug", "rawget", "rawset", "rawequal", "rawlen"} {
		val := L.GetGlobal(name)
		assert.Equal(t, lua.LNil, val, "global %q should be nil in sandbox", name)
	}
}

func TestNewSandboxedState_AllowedGlobals(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	for _, name := range []string{"string", "table", "math", "coroutine"} {
		val := L.GetGlobal(name)
		assert.NotEqual(t, lua.LNil, val, "global %q should be available in sandbox", name)
	}
}

func TestNewSandboxedState_PackageLoadlibRemoved(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	// package table itself may be nil since we skip most libs — that's also safe.
	pkg := L.GetGlobal("package")
	if pkg == lua.LNil {
		return // package not loaded at all — loadlib is unreachable, which is safe
	}
	tbl, ok := pkg.(*lua.LTable)
	require.True(t, ok)
	val := tbl.RawGetString("loadlib")
	assert.Equal(t, lua.LNil, val, "package.loadlib should be nil")
}

func TestNewSandboxedState_BasicLuaWorks(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	err := L.DoString(`
		local x = math.floor(3.7)
		local s = string.upper("hello")
		local t = {1, 2, 3}
		table.insert(t, 4)
	`)
	require.NoError(t, err)
}

func TestNewSandboxedState_ContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	L := NewSandboxedState(ctx)
	defer L.Close()

	err := L.DoString(`
		local i = 0
		while true do
			i = i + 1
		end
	`)
	assert.Error(t, err, "infinite loop should be interrupted by context cancellation")
}
