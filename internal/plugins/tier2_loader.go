// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	lua "github.com/yuin/gopher-lua"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
)

// Sentinel errors for Tier2Loader operations.
var (
	ErrNotTier2            = errors.New("plugins: plugin is not Tier 2")
	ErrLuaExecution        = errors.New("plugins: lua execution error")
	ErrLuaSandboxViolation = errors.New("plugins: lua sandbox violation")
)

// Tier2Plugin represents a loaded Tier 2 Lua plugin.
type Tier2Plugin struct {
	Name          string
	LState        *lua.LState
	StateDir      string
	HTTPAllowlist []string
	cancelCtx     context.CancelFunc
	subscriptions []func() // EventBus unsubscribe functions
	eventBus      core.EventBus
	mu            sync.Mutex     // protects all LState access (gopher-lua is not goroutine-safe)
	stateMu       sync.Mutex     // protects state file I/O for this plugin
	handlerWg     sync.WaitGroup // tracks in-flight event handlers for graceful shutdown
}

// Tier2Loader loads and manages Tier 2 Lua plugins running in-process via gopher-lua.
type Tier2Loader struct {
	registry *LocalRegistry
	eventBus core.EventBus
	extMgr   core.ExtensionRegistration
	loaded   map[string]*Tier2Plugin
	mu       sync.RWMutex
}

// NewTier2Loader creates a new Tier2Loader with the given dependencies.
func NewTier2Loader(registry *LocalRegistry, eventBus core.EventBus, extMgr core.ExtensionRegistration) *Tier2Loader {
	return &Tier2Loader{
		registry: registry,
		eventBus: eventBus,
		extMgr:   extMgr,
		loaded:   make(map[string]*Tier2Plugin),
	}
}

// Load loads a Tier 2 Lua plugin by name. It reads the manifest, creates a sandboxed
// Lua VM, registers the siply API, and executes main.lua.
func (l *Tier2Loader) Load(ctx context.Context, name string) error {
	if l.registry == nil {
		return fmt.Errorf("plugins: tier2: registry is nil")
	}

	pluginDir, err := l.pluginDir(name)
	if err != nil {
		return err
	}

	manifest, err := LoadManifestFromDir(pluginDir)
	if err != nil {
		return fmt.Errorf("plugins: tier2: load manifest %s: %w", name, err)
	}

	if manifest.Spec.Tier != 2 {
		return fmt.Errorf("%w: %s has tier %d", ErrNotTier2, name, manifest.Spec.Tier)
	}

	mainLua := filepath.Join(pluginDir, "main.lua")
	if _, err := os.Stat(mainLua); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("plugins: tier2: main.lua not found for %s", name)
		}
		return fmt.Errorf("plugins: tier2: stat main.lua %s: %w", name, err)
	}

	// Unload previous instance if reloading.
	l.mu.RLock()
	_, alreadyLoaded := l.loaded[name]
	l.mu.RUnlock()
	if alreadyLoaded {
		if err := l.Unload(name); err != nil {
			slog.Warn("tier2 plugin unload before reload failed", "name", name, "err", err)
		}
	}

	// Create sandboxed LState with cancellation context.
	luaCtx, luaCancel := context.WithCancel(ctx)
	L := NewSandboxedState(luaCtx)

	// Determine state directory.
	home, err := os.UserHomeDir()
	if err != nil {
		luaCancel()
		L.Close()
		return fmt.Errorf("plugins: tier2: user home: %w", err)
	}
	stateDir := filepath.Join(home, ".siply", "plugins", name, "state")

	plugin := &Tier2Plugin{
		Name:          name,
		LState:        L,
		StateDir:      stateDir,
		HTTPAllowlist: manifest.Spec.HTTPAllowlist,
		cancelCtx:     luaCancel,
		eventBus:      l.eventBus,
	}

	// Register siply API tables on the LState.
	registerSiplyAPI(L, plugin, l.eventBus, l.extMgr)

	// Execute main.lua in protected mode.
	if err := L.DoFile(mainLua); err != nil {
		luaCancel()
		L.Close()
		if l.eventBus != nil {
			_ = l.eventBus.Publish(ctx, events.NewPluginCrashedEvent(name, err.Error()))
		}
		return fmt.Errorf("%w: %s: %v", ErrLuaExecution, name, err)
	}

	l.mu.Lock()
	l.loaded[name] = plugin
	l.mu.Unlock()

	// Publish PluginLoadedEvent.
	if l.eventBus != nil {
		_ = l.eventBus.Publish(ctx, events.NewPluginLoadedEvent(name, manifest.Metadata.Version, 2))
	}

	slog.Info("tier2 plugin loaded", "name", name, "version", manifest.Metadata.Version)
	return nil
}

// Unload closes a Tier 2 plugin's Lua VM and cleans up all registrations.
func (l *Tier2Loader) Unload(name string) error {
	l.mu.Lock()
	plugin, ok := l.loaded[name]
	if !ok {
		l.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, name)
	}
	delete(l.loaded, name)
	l.mu.Unlock()

	// Unsubscribe all EventBus subscriptions (prevents new handler invocations).
	for _, unsub := range plugin.subscriptions {
		unsub()
	}

	// Cancel context to signal in-flight handlers.
	if plugin.cancelCtx != nil {
		plugin.cancelCtx()
	}

	// Wait for all in-flight event handlers to complete before closing LState.
	plugin.handlerWg.Wait()

	// Close LState under lock to prevent concurrent access.
	plugin.mu.Lock()
	plugin.LState.Close()
	plugin.mu.Unlock()

	// Unregister all extensions.
	if l.extMgr != nil {
		if um, ok := l.extMgr.(interface{ UnregisterAll(string) }); ok {
			um.UnregisterAll(name)
		}
	}

	slog.Info("tier2 plugin unloaded", "name", name)
	return nil
}

// IsLoaded returns true if the named plugin is currently loaded (thread-safe).
func (l *Tier2Loader) IsLoaded(name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.loaded[name]
	return ok
}

// Plugin returns the Tier2Plugin for the given name (thread-safe).
func (l *Tier2Loader) Plugin(name string) (*Tier2Plugin, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	p, ok := l.loaded[name]
	return p, ok
}

// pluginDir returns the effective directory for a plugin, respecting dev mode paths.
func (l *Tier2Loader) pluginDir(name string) (string, error) {
	if l.registry.registryDir == "" {
		return "", fmt.Errorf("plugins: tier2: registry not initialized (empty registryDir)")
	}

	if strings.ContainsAny(name, "/\\") || name == ".." || strings.Contains(name, "..") {
		return "", fmt.Errorf("plugins: tier2: invalid plugin name %q: path traversal not allowed", name)
	}

	l.registry.mu.RLock()
	devPath, isDev := l.registry.devPaths[name]
	l.registry.mu.RUnlock()

	if isDev {
		return devPath, nil
	}

	dir := filepath.Join(l.registry.registryDir, name)

	cleanDir := filepath.Clean(dir)
	cleanBase := filepath.Clean(l.registry.registryDir)
	if !strings.HasPrefix(cleanDir, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("plugins: tier2: invalid plugin name %q: path escapes registry", name)
	}

	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return "", fmt.Errorf("plugins: tier2: stat plugin dir %s: %w", name, err)
	}
	return dir, nil
}

// safeCallLua wraps a Lua function call with panic recovery and error handling.
// nret controls how many return values are pushed onto the stack (caller must pop).
func safeCallLua(L *lua.LState, nret int, fn *lua.LFunction, args ...lua.LValue) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: recovered panic: %v", ErrLuaExecution, r)
		}
	}()

	if err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    nret,
		Protect: true,
	}, args...); err != nil {
		return fmt.Errorf("%w: %v", ErrLuaExecution, err)
	}
	return nil
}
