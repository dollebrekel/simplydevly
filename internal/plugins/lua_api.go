// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	lua "github.com/yuin/gopher-lua"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/fileutil"
)

const maxLuaTableDepth = 32

func publishCrashEvent(plugin *Tier2Plugin, errMsg string) {
	if plugin.eventBus != nil {
		_ = plugin.eventBus.Publish(context.Background(), events.NewPluginCrashedEvent(plugin.Name, errMsg))
	}
}

// registerSiplyAPI registers the complete `siply` global table on the LState.
func registerSiplyAPI(L *lua.LState, plugin *Tier2Plugin, eventBus core.EventBus, extMgr core.ExtensionRegistration) {
	siply := L.NewTable()

	// siply.log
	siply.RawSetString("log", L.NewFunction(newSiplyLog(plugin.Name)))

	// siply.on / siply.emit
	siply.RawSetString("on", L.NewFunction(newSiplyOn(L, plugin, eventBus)))
	siply.RawSetString("emit", L.NewFunction(newSiplyEmit(plugin.Name, eventBus)))

	// siply.config
	configTable := L.NewTable()
	configTable.RawSetString("get", L.NewFunction(newSiplyConfigGet(plugin.Name)))
	siply.RawSetString("config", configTable)

	// siply.state
	stateTable := L.NewTable()
	stateTable.RawSetString("get", L.NewFunction(newSiplyStateGet(plugin)))
	stateTable.RawSetString("set", L.NewFunction(newSiplyStateSet(plugin)))
	siply.RawSetString("state", stateTable)

	// siply.panel / siply.ui / siply.menu / siply.keybind
	registerPanelAPI(L, siply, plugin, extMgr)

	// siply.http
	registerHTTPAPI(L, siply, plugin)

	// siply.fs
	registerFSAPI(L, siply, plugin)

	L.SetGlobal("siply", siply)
}

// newSiplyLog returns a Lua function that logs via slog with plugin name prefix.
func newSiplyLog(pluginName string) lua.LGFunction {
	return func(L *lua.LState) int {
		level := L.CheckString(1)
		message := L.CheckString(2)

		switch level {
		case "info":
			slog.Info(message, "plugin", pluginName)
		case "warn":
			slog.Warn(message, "plugin", pluginName)
		case "error":
			slog.Error(message, "plugin", pluginName)
		default:
			slog.Info(message, "plugin", pluginName, "level", level)
		}
		return 0
	}
}

// newSiplyOn returns a Lua function that subscribes to EventBus events.
func newSiplyOn(L *lua.LState, plugin *Tier2Plugin, eventBus core.EventBus) lua.LGFunction {
	return func(L *lua.LState) int {
		eventType := L.CheckString(1)
		handler := L.CheckFunction(2)

		if eventBus == nil {
			L.ArgError(1, "event bus not available")
			return 0
		}

		unsub := eventBus.Subscribe(eventType, func(ctx context.Context, event core.Event) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("lua event handler panic", "plugin", plugin.Name, "event", eventType, "panic", r)
					publishCrashEvent(plugin, fmt.Sprintf("panic in event handler %s: %v", eventType, r))
				}
			}()

			plugin.mu.Lock()
			defer plugin.mu.Unlock()

			dataTable := eventToLuaTable(L, event)
			if err := safeCallLua(L, 0, handler, dataTable); err != nil {
				slog.Error("lua event handler error", "plugin", plugin.Name, "event", eventType, "error", err)
				publishCrashEvent(plugin, err.Error())
			}
		})

		plugin.subscriptions = append(plugin.subscriptions, unsub)
		return 0
	}
}

// newSiplyEmit returns a Lua function that publishes custom events on EventBus.
func newSiplyEmit(pluginName string, eventBus core.EventBus) lua.LGFunction {
	return func(L *lua.LState) int {
		eventType := L.CheckString(1)
		dataTable := L.OptTable(2, L.NewTable())

		if eventBus == nil {
			L.ArgError(1, "event bus not available")
			return 0
		}

		prefixed := "lua." + pluginName + "." + eventType
		data := luaTableToMap(dataTable)
		evt := &luaCustomEvent{
			eventType: prefixed,
			data:      data,
			ts:        time.Now(),
		}

		if err := eventBus.Publish(context.Background(), evt); err != nil {
			slog.Error("lua emit failed", "plugin", pluginName, "event", prefixed, "error", err)
		}
		return 0
	}
}

// newSiplyConfigGet returns a Lua function that reads plugin-scoped config values.
func newSiplyConfigGet(pluginName string) lua.LGFunction {
	return func(L *lua.LState) int {
		_ = L.CheckString(1) // key
		// Config integration is a stub — returns nil for now.
		// Full config integration requires ConfigMerger (Tier 1 territory).
		L.Push(lua.LNil)
		return 1
	}
}

// newSiplyStateGet returns a Lua function that reads persistent state.
func newSiplyStateGet(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)

		plugin.stateMu.Lock()
		defer plugin.stateMu.Unlock()

		data, err := readStateFile(plugin.StateDir)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}

		val, ok := data[key]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}

		L.Push(goToLua(L, val))
		return 1
	}
}

// newSiplyStateSet returns a Lua function that writes persistent state.
func newSiplyStateSet(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		value := L.Get(2)

		plugin.stateMu.Lock()
		defer plugin.stateMu.Unlock()

		data, _ := readStateFile(plugin.StateDir)
		if data == nil {
			data = make(map[string]any)
		}

		data[key] = luaToGo(value)

		if err := writeStateFile(plugin.StateDir, data); err != nil {
			slog.Error("lua state.set failed", "plugin", plugin.Name, "key", key, "error", err)
		}
		return 0
	}
}

func readStateFile(stateDir string) (map[string]any, error) {
	path := filepath.Join(stateDir, "state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func writeStateFile(stateDir string, data map[string]any) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	path := filepath.Join(stateDir, "state.json")
	return fileutil.AtomicWriteFile(path, raw, 0o644)
}

// eventToLuaTable converts a core.Event to a Lua table with type, timestamp, and payload data.
func eventToLuaTable(L *lua.LState, event core.Event) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("type", lua.LString(event.Type()))
	t.RawSetString("timestamp", lua.LString(event.Timestamp().Format("2006-01-02T15:04:05Z07:00")))

	switch e := event.(type) {
	case *events.PluginLoadedEvent:
		t.RawSetString("name", lua.LString(e.Name))
		t.RawSetString("version", lua.LString(e.Version))
		t.RawSetString("tier", lua.LNumber(e.Tier))
	case *events.PluginCrashedEvent:
		t.RawSetString("name", lua.LString(e.Name))
		t.RawSetString("error", lua.LString(e.Err))
	case *events.ConfigChangedEvent:
		t.RawSetString("key", lua.LString(e.Key))
		t.RawSetString("old_value", lua.LString(e.OldValue))
		t.RawSetString("new_value", lua.LString(e.NewValue))
	case *events.PluginDisabledEvent:
		t.RawSetString("name", lua.LString(e.Name))
		t.RawSetString("version", lua.LString(e.Version))
		t.RawSetString("reason", lua.LString(e.Reason))
	case *events.SessionStartedEvent:
		t.RawSetString("session_id", lua.LString(e.SessionID))
	case *events.PanelActivatedEvent:
		t.RawSetString("panel_name", lua.LString(e.PanelName))
	case *events.PluginReloadedEvent:
		t.RawSetString("name", lua.LString(e.Name))
	case *luaCustomEvent:
		for k, v := range e.data {
			t.RawSetString(k, goToLua(L, v))
		}
	}

	return t
}

// luaTableToMap converts a Lua table to a Go map[string]any with depth protection.
func luaTableToMap(tbl *lua.LTable) map[string]any {
	return luaTableToMapDepth(tbl, 0)
}

func luaTableToMapDepth(tbl *lua.LTable, depth int) map[string]any {
	if depth >= maxLuaTableDepth {
		return nil
	}
	result := make(map[string]any)
	tbl.ForEach(func(key, value lua.LValue) {
		if k, ok := key.(lua.LString); ok {
			result[string(k)] = luaToGoDepth(value, depth+1)
		}
	})
	return result
}

// luaToGo converts a Lua value to a Go value.
func luaToGo(value lua.LValue) any {
	return luaToGoDepth(value, 0)
}

func luaToGoDepth(value lua.LValue, depth int) any {
	switch v := value.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	case lua.LString:
		return string(v)
	case *lua.LTable:
		if depth >= maxLuaTableDepth {
			return nil
		}
		return luaTableToMapDepth(v, depth)
	default:
		return v.String()
	}
}

// goToLua converts a Go value to a Lua value.
func goToLua(L *lua.LState, value any) lua.LValue {
	switch v := value.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(v)
	case float64:
		return lua.LNumber(v)
	case string:
		return lua.LString(v)
	case map[string]any:
		t := L.NewTable()
		for k, val := range v {
			t.RawSetString(k, goToLua(L, val))
		}
		return t
	case []any:
		t := L.NewTable()
		for _, val := range v {
			t.Append(goToLua(L, val))
		}
		return t
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// luaCustomEvent is a simple Event implementation for Lua-emitted events.
type luaCustomEvent struct {
	eventType string
	data      map[string]any
	ts        time.Time
}

func (e *luaCustomEvent) Type() string         { return e.eventType }
func (e *luaCustomEvent) Timestamp() time.Time { return e.ts }
