// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"

	lua "github.com/yuin/gopher-lua"
)

// dangerousGlobals are Lua globals removed from the sandboxed environment.
var dangerousGlobals = []string{
	"os", "io", "loadfile", "dofile", "debug",
	"rawget", "rawset", "rawequal", "rawlen",
	"require",
}

// NewSandboxedState creates a gopher-lua LState with dangerous libraries removed.
// Only safe libs are loaded: base (with removals), table, string, math, coroutine.
// The returned LState respects the given context for cancellation.
func NewSandboxedState(ctx context.Context) *lua.LState {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})

	// Set context for cancellation / deadline support.
	L.SetContext(ctx)

	// Selectively open only safe standard libraries.
	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
		{lua.CoroutineLibName, lua.OpenCoroutine},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	// Remove dangerous globals from the base library.
	for _, name := range dangerousGlobals {
		L.SetGlobal(name, lua.LNil)
	}

	// Remove package.loadlib to prevent C library loading.
	if pkg := L.GetGlobal("package"); pkg != lua.LNil {
		if tbl, ok := pkg.(*lua.LTable); ok {
			tbl.RawSetString("loadlib", lua.LNil)
		}
	}

	return L
}
