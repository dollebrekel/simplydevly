// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"fmt"
	"log/slog"
	"time"

	lua "github.com/yuin/gopher-lua"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/pkg/siplyui"
)

// registerPanelAPI registers siply.panel, siply.ui, siply.menu, and siply.keybind tables.
func registerPanelAPI(L *lua.LState, siply *lua.LTable, plugin *Tier2Plugin, extMgr core.ExtensionRegistration) {
	// siply.panel
	panelTbl := L.NewTable()
	panelTbl.RawSetString("create", L.NewFunction(newSiplyPanelCreate(L, plugin, extMgr)))
	panelTbl.RawSetString("update", L.NewFunction(newSiplyPanelUpdate(plugin)))
	siply.RawSetString("panel", panelTbl)

	// siply.ui
	uiTbl := L.NewTable()
	uiTbl.RawSetString("tree", L.NewFunction(newSiplyUITree()))
	uiTbl.RawSetString("markdown", L.NewFunction(newSiplyUIMarkdown()))
	uiTbl.RawSetString("card", L.NewFunction(newSiplyUICard()))
	uiTbl.RawSetString("toast", L.NewFunction(newSiplyUIToast()))
	siply.RawSetString("ui", uiTbl)

	// siply.menu
	menuTbl := L.NewTable()
	menuTbl.RawSetString("add", L.NewFunction(newSiplyMenuAdd(plugin, extMgr)))
	siply.RawSetString("menu", menuTbl)

	// siply.keybind
	keybindTbl := L.NewTable()
	keybindTbl.RawSetString("add", L.NewFunction(newSiplyKeybindAdd(L, plugin, extMgr)))
	siply.RawSetString("keybind", keybindTbl)
}

var positionMap = map[string]core.PanelPosition{
	"left":   core.PanelLeft,
	"right":  core.PanelRight,
	"bottom": core.PanelBottom,
}

func newSiplyPanelCreate(L *lua.LState, plugin *Tier2Plugin, extMgr core.ExtensionRegistration) lua.LGFunction {
	return func(L *lua.LState) int {
		configTbl := L.CheckTable(1)

		name := getStringField(configTbl, "name")
		if name == "" {
			L.ArgError(1, "panel config must have a 'name' field")
			return 0
		}

		posStr := getStringField(configTbl, "position")
		pos, ok := positionMap[posStr]
		if !ok {
			pos = core.PanelRight
		}

		collapsible := getBoolField(configTbl, "collapsible")
		keybind := getStringField(configTbl, "keybind")
		onRender := getFunctionField(configTbl, "on_render")
		onActivate := getFunctionField(configTbl, "on_activate")

		cfg := core.PanelConfig{
			Name:        name,
			Position:    pos,
			Collapsible: collapsible,
			Keybind:     keybind,
			PluginName:  plugin.Name,
			LazyInit:    true,
		}

		if onRender != nil {
			cfg.ContentFunc = func(_, _ int) string {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("lua panel render panic", "plugin", plugin.Name, "panel", name, "panic", r)
						publishCrashEvent(plugin, fmt.Sprintf("panic in panel render %s: %v", name, r))
					}
				}()

				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				if err := safeCallLua(L, 1, onRender, lua.LNumber(80), lua.LNumber(24)); err != nil {
					slog.Error("lua panel render error", "plugin", plugin.Name, "panel", name, "error", err)
					publishCrashEvent(plugin, err.Error())
					return ""
				}
				ret := L.Get(-1)
				L.Pop(1)
				if s, ok := ret.(lua.LString); ok {
					return string(s)
				}
				return ""
			}
		}

		if onActivate != nil {
			cfg.OnActivate = func() error {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("lua panel activate panic", "plugin", plugin.Name, "panel", name, "panic", r)
						publishCrashEvent(plugin, fmt.Sprintf("panic in panel activate %s: %v", name, r))
					}
				}()

				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				return safeCallLua(L, 0, onActivate)
			}
		}

		if extMgr != nil {
			if err := extMgr.RegisterPanel(cfg); err != nil {
				L.ArgError(1, "panel registration failed: "+err.Error())
				return 0
			}
		}

		return 0
	}
}

func newSiplyPanelUpdate(plugin *Tier2Plugin) lua.LGFunction {
	return func(L *lua.LState) int {
		_ = L.CheckString(1) // panel name — re-render is triggered by the TUI polling ContentFunc
		return 0
	}
}

// siply.ui component renderers — each returns a rendered string using pkg/siplyui.

func newSiplyUITree() lua.LGFunction {
	return func(L *lua.LState) int {
		nodesTbl := L.CheckTable(1)
		nodes := luaTableToTreeNodes(nodesTbl)

		theme := siplyui.DefaultTheme()
		cfg := siplyui.DefaultRenderConfig()
		tree := siplyui.NewTree(nodes, theme, cfg)
		result := tree.Render(80, 24)

		L.Push(lua.LString(result))
		return 1
	}
}

func newSiplyUIMarkdown() lua.LGFunction {
	return func(L *lua.LState) int {
		text := L.CheckString(1)

		theme := siplyui.DefaultTheme()
		cfg := siplyui.DefaultRenderConfig()
		mv := siplyui.NewMarkdownView(theme, cfg)
		result := mv.Render(text, 80)

		L.Push(lua.LString(result))
		return 1
	}
}

func newSiplyUICard() lua.LGFunction {
	return func(L *lua.LState) int {
		dataTbl := L.CheckTable(1)

		title := getStringField(dataTbl, "title")
		if title == "" {
			title = getStringField(dataTbl, "name")
		}
		description := getStringField(dataTbl, "description")
		version := getStringField(dataTbl, "version")
		author := getStringField(dataTbl, "author")

		theme := siplyui.DefaultTheme()
		cfg := siplyui.DefaultRenderConfig()
		card := siplyui.Card{
			Title:       title,
			Description: description,
			Version:     version,
			Author:      author,
		}
		result := siplyui.RenderCard(card, theme, cfg, 60)

		L.Push(lua.LString(result))
		return 1
	}
}

func newSiplyUIToast() lua.LGFunction {
	return func(L *lua.LState) int {
		message := L.CheckString(1)
		levelStr := L.OptString(2, "info")

		var level siplyui.FeedbackLevel
		switch levelStr {
		case "success":
			level = siplyui.LevelSuccess
		case "warning":
			level = siplyui.LevelWarning
		case "error":
			level = siplyui.LevelError
		default:
			level = siplyui.LevelInfo
		}

		theme := siplyui.DefaultTheme()
		cfg := siplyui.DefaultRenderConfig()
		tm := siplyui.NewToastManager(theme, cfg)
		tm.Push(message, level, 5*time.Second)
		result := tm.Render(60)

		L.Push(lua.LString(result))
		return 1
	}
}

func newSiplyMenuAdd(plugin *Tier2Plugin, extMgr core.ExtensionRegistration) lua.LGFunction {
	return func(L *lua.LState) int {
		configTbl := L.CheckTable(1)

		label := getStringField(configTbl, "label")
		icon := getStringField(configTbl, "icon")
		keybind := getStringField(configTbl, "keybind")
		category := getStringField(configTbl, "category")
		actionFn := getFunctionField(configTbl, "action")

		item := core.MenuItem{
			Label:      label,
			Icon:       icon,
			Keybind:    keybind,
			Category:   category,
			PluginName: plugin.Name,
		}

		if actionFn != nil {
			item.Action = func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("lua menu action panic", "plugin", plugin.Name, "label", label, "panic", r)
						publishCrashEvent(plugin, fmt.Sprintf("panic in menu action %s: %v", label, r))
					}
				}()

				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				if err := safeCallLua(L, 0, actionFn); err != nil {
					slog.Error("lua menu action error", "plugin", plugin.Name, "label", label, "error", err)
					publishCrashEvent(plugin, err.Error())
				}
			}
		}

		if extMgr != nil {
			if err := extMgr.RegisterMenuItem(item); err != nil {
				L.ArgError(1, "menu registration failed: "+err.Error())
				return 0
			}
		}
		return 0
	}
}

func newSiplyKeybindAdd(L *lua.LState, plugin *Tier2Plugin, extMgr core.ExtensionRegistration) lua.LGFunction {
	return func(L *lua.LState) int {
		configTbl := L.CheckTable(1)

		key := getStringField(configTbl, "key")
		description := getStringField(configTbl, "description")
		handlerFn := getFunctionField(configTbl, "handler")

		kb := core.Keybinding{
			Key:         key,
			Description: description,
			PluginName:  plugin.Name,
		}

		if handlerFn != nil {
			kb.Handler = func() error {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("lua keybind handler panic", "plugin", plugin.Name, "key", key, "panic", r)
						publishCrashEvent(plugin, fmt.Sprintf("panic in keybind handler %s: %v", key, r))
					}
				}()

				plugin.mu.Lock()
				defer plugin.mu.Unlock()

				return safeCallLua(L, 0, handlerFn)
			}
		}

		if extMgr != nil {
			if err := extMgr.RegisterKeybinding(kb); err != nil {
				L.ArgError(1, "keybind registration failed: "+err.Error())
				return 0
			}
		}
		return 0
	}
}

// Helper functions for reading Lua table fields.

func getStringField(tbl *lua.LTable, key string) string {
	v := tbl.RawGetString(key)
	if s, ok := v.(lua.LString); ok {
		return string(s)
	}
	return ""
}

func getBoolField(tbl *lua.LTable, key string) bool {
	v := tbl.RawGetString(key)
	if b, ok := v.(lua.LBool); ok {
		return bool(b)
	}
	return false
}

func getFunctionField(tbl *lua.LTable, key string) *lua.LFunction {
	v := tbl.RawGetString(key)
	if fn, ok := v.(*lua.LFunction); ok {
		return fn
	}
	return nil
}

func luaTableToTreeNodes(tbl *lua.LTable) []siplyui.TreeNode {
	return luaTableToTreeNodesDepth(tbl, 0)
}

func luaTableToTreeNodesDepth(tbl *lua.LTable, depth int) []siplyui.TreeNode {
	if depth >= maxLuaTableDepth {
		return nil
	}
	var nodes []siplyui.TreeNode
	tbl.ForEach(func(_, value lua.LValue) {
		if t, ok := value.(*lua.LTable); ok {
			node := siplyui.TreeNode{
				Label:    getStringField(t, "label"),
				Icon:     getStringField(t, "icon"),
				Expanded: getBoolField(t, "expanded"),
				Selected: getBoolField(t, "selected"),
			}
			childrenVal := t.RawGetString("children")
			if ct, ok := childrenVal.(*lua.LTable); ok {
				node.Children = luaTableToTreeNodesDepth(ct, depth+1)
			}
			nodes = append(nodes, node)
		}
	})
	return nodes
}
