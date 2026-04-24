// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package extensions_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/extensions"
)

// stubPanelRegistry is a minimal PanelRegistry for testing.
type stubPanelRegistry struct {
	mu         sync.Mutex
	panels     map[string]core.PanelConfig
	failRegErr error
}

func newStubPanelRegistry() *stubPanelRegistry {
	return &stubPanelRegistry{panels: make(map[string]core.PanelConfig)}
}

func (s *stubPanelRegistry) Init(_ context.Context) error  { return nil }
func (s *stubPanelRegistry) Start(_ context.Context) error { return nil }
func (s *stubPanelRegistry) Stop(_ context.Context) error  { return nil }
func (s *stubPanelRegistry) Health() error                 { return nil }

func (s *stubPanelRegistry) Register(cfg core.PanelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failRegErr != nil {
		return s.failRegErr
	}
	if _, exists := s.panels[cfg.Name]; exists {
		return errors.New("panel already exists")
	}
	s.panels[cfg.Name] = cfg
	return nil
}

func (s *stubPanelRegistry) Unregister(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.panels, name)
	return nil
}

func (s *stubPanelRegistry) Panel(name string) (core.PanelInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, ok := s.panels[name]
	if !ok {
		return core.PanelInfo{}, false
	}
	return core.PanelInfo{Config: cfg}, true
}

func (s *stubPanelRegistry) Panels() []core.PanelInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []core.PanelInfo
	for _, cfg := range s.panels {
		result = append(result, core.PanelInfo{Config: cfg})
	}
	return result
}

func (s *stubPanelRegistry) Activate(_ string) error   { return nil }
func (s *stubPanelRegistry) Deactivate(_ string) error { return nil }

func TestManager_RegisterPanel(t *testing.T) {
	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, "")

	cfg := core.PanelConfig{
		Name:       "test-panel",
		Position:   core.PanelRight,
		PluginName: "my-plugin",
		MinWidth:   20,
	}

	if err := m.RegisterPanel(cfg); err != nil {
		t.Fatalf("RegisterPanel: unexpected error: %v", err)
	}

	regs := m.Registrations("my-plugin")
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].Kind != core.RegistrationPanel {
		t.Errorf("expected RegistrationPanel, got %d", regs[0].Kind)
	}

	if _, ok := pr.Panel("test-panel"); !ok {
		t.Error("panel not registered in PanelRegistry")
	}
}

func TestManager_RegisterPanel_EmptyName(t *testing.T) {
	pr := newStubPanelRegistry()
	m := extensions.NewManager(pr, nil, "")

	err := m.RegisterPanel(core.PanelConfig{PluginName: "p"})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestManager_RegisterPanel_EmptyPluginName(t *testing.T) {
	pr := newStubPanelRegistry()
	m := extensions.NewManager(pr, nil, "")

	err := m.RegisterPanel(core.PanelConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected error for empty plugin name")
	}
}

func TestManager_RegisterPanel_Duplicate(t *testing.T) {
	pr := newStubPanelRegistry()
	m := extensions.NewManager(pr, nil, "")

	cfg := core.PanelConfig{Name: "test", PluginName: "p", Position: core.PanelLeft}
	if err := m.RegisterPanel(cfg); err != nil {
		t.Fatalf("first register: %v", err)
	}

	err := m.RegisterPanel(cfg)
	if !errors.Is(err, core.ErrExtensionAlreadyRegistered) {
		t.Errorf("expected ErrExtensionAlreadyRegistered, got %v", err)
	}
}

func TestManager_RegisterMenuItem(t *testing.T) {
	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, "")
	startManager(t, m)

	item := core.MenuItem{Label: "Test Action", Category: "Extensions"}
	if err := m.RegisterMenuItemForPlugin("my-plugin", item); err != nil {
		t.Fatalf("RegisterMenuItemForPlugin: %v", err)
	}

	regs := m.Registrations("my-plugin")
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].Kind != core.RegistrationMenu {
		t.Errorf("expected RegistrationMenu, got %d", regs[0].Kind)
	}

	items := m.AllMenuItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 menu item, got %d", len(items))
	}
	if items[0].Label != "Test Action" {
		t.Errorf("expected label 'Test Action', got %q", items[0].Label)
	}
}

func TestManager_RegisterMenuItem_EmptyLabel(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	err := m.RegisterMenuItemForPlugin("p", core.MenuItem{})
	if err == nil {
		t.Fatal("expected error for empty label")
	}
}

func TestManager_RegisterMenuItem_Duplicate(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	item := core.MenuItem{Label: "Test", Category: "Tools"}

	if err := m.RegisterMenuItemForPlugin("p1", item); err != nil {
		t.Fatal(err)
	}

	err := m.RegisterMenuItemForPlugin("p2", item)
	if !errors.Is(err, core.ErrMenuItemDuplicate) {
		t.Errorf("expected ErrMenuItemDuplicate, got %v", err)
	}
}

func TestManager_RegisterKeybinding(t *testing.T) {
	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, "")
	startManager(t, m)

	kb := core.Keybinding{Key: "ctrl+e", Description: "Execute", PluginName: "my-plugin"}
	if err := m.RegisterKeybinding(kb); err != nil {
		t.Fatalf("RegisterKeybinding: %v", err)
	}

	regs := m.Registrations("my-plugin")
	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}
	if regs[0].Kind != core.RegistrationKeybind {
		t.Errorf("expected RegistrationKeybind, got %d", regs[0].Kind)
	}

	bindings := m.AllKeybindings()
	if len(bindings) != 1 {
		t.Fatalf("expected 1 keybinding, got %d", len(bindings))
	}
}

func TestManager_RegisterKeybinding_EmptyKey(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	err := m.RegisterKeybinding(core.Keybinding{PluginName: "p"})
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestManager_RegisterKeybinding_BuiltinConflict(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	err := m.RegisterKeybinding(core.Keybinding{Key: "ctrl+c", PluginName: "p"})
	if !errors.Is(err, core.ErrKeybindConflict) {
		t.Errorf("expected ErrKeybindConflict, got %v", err)
	}
}

func TestManager_RegisterKeybinding_PluginConflict(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	kb := core.Keybinding{Key: "ctrl+e", Description: "A", PluginName: "p1"}
	if err := m.RegisterKeybinding(kb); err != nil {
		t.Fatal(err)
	}

	kb2 := core.Keybinding{Key: "ctrl+e", Description: "B", PluginName: "p2"}
	err := m.RegisterKeybinding(kb2)
	if !errors.Is(err, core.ErrKeybindConflict) {
		t.Errorf("expected ErrKeybindConflict, got %v", err)
	}
}

func TestManager_UnregisterAll(t *testing.T) {
	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, "")
	startManager(t, m)

	cfg := core.PanelConfig{Name: "panel1", Position: core.PanelRight, PluginName: "test-plugin"}
	if err := m.RegisterPanel(cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterMenuItemForPlugin("test-plugin", core.MenuItem{Label: "Test", Category: "Ext"}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterKeybinding(core.Keybinding{Key: "ctrl+e", PluginName: "test-plugin"}); err != nil {
		t.Fatal(err)
	}

	if len(m.Registrations("test-plugin")) != 3 {
		t.Fatalf("expected 3 registrations before unregister")
	}

	m.UnregisterAll("test-plugin")

	if regs := m.Registrations("test-plugin"); len(regs) != 0 {
		t.Errorf("expected 0 registrations after unregister, got %d", len(regs))
	}

	if _, ok := pr.Panel("panel1"); ok {
		t.Error("panel should have been unregistered from PanelRegistry")
	}
}

func TestManager_UnregisterAll_NonexistentPlugin(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	m.UnregisterAll("nonexistent")
}

func TestManager_Registrations_NoPlugin(t *testing.T) {
	m := extensions.NewManager(newStubPanelRegistry(), nil, "")
	regs := m.Registrations("nonexistent")
	if regs != nil {
		t.Errorf("expected nil, got %v", regs)
	}
}

func TestManager_Lifecycle(t *testing.T) {
	bus := newTestBus(t)
	m := extensions.NewManager(newStubPanelRegistry(), bus, "")

	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Health(); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestManager_PluginCrashed_Cleanup(t *testing.T) {
	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, "")

	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cfg := core.PanelConfig{Name: "crash-panel", Position: core.PanelLeft, PluginName: "crashy"}
	if err := m.RegisterPanel(cfg); err != nil {
		t.Fatal(err)
	}

	crashEvent := events.NewPluginCrashedEvent("crashy", "segfault")
	if err := bus.Publish(ctx, crashEvent); err != nil {
		t.Fatal(err)
	}

	// Bus delivery is async — poll until cleanup completes.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if regs := m.Registrations("crashy"); len(regs) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if _, ok := pr.Panel("crash-panel"); ok {
		t.Error("panel should have been unregistered after crash")
	}
	if regs := m.Registrations("crashy"); len(regs) != 0 {
		t.Errorf("expected 0 registrations after crash, got %d", len(regs))
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	pr := newStubPanelRegistry()
	m := extensions.NewManager(pr, nil, "")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(3)
		idx := i
		go func() {
			defer wg.Done()
			_ = m.RegisterPanel(core.PanelConfig{
				Name:       "panel-" + itoa(idx),
				Position:   core.PanelRight,
				PluginName: "plugin-" + itoa(idx),
			})
		}()
		go func() {
			defer wg.Done()
			_ = m.RegisterMenuItemForPlugin("plugin-"+itoa(idx), core.MenuItem{
				Label:    "item-" + itoa(idx),
				Category: "Extensions",
			})
		}()
		go func() {
			defer wg.Done()
			_ = m.Registrations("plugin-" + itoa(idx))
		}()
	}
	wg.Wait()
}

func itoa(i int) string {
	return string(rune('0' + i))
}

// newTestBus creates and initializes an EventBus for testing.
func newTestBus(t *testing.T) *testBus {
	t.Helper()
	b := &testBus{bus: events.NewBus()}
	ctx := context.Background()
	if err := b.bus.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.bus.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.bus.Stop(ctx) })
	return b
}

type testBus struct {
	bus *events.Bus
}

func (tb *testBus) Init(ctx context.Context) error  { return tb.bus.Init(ctx) }
func (tb *testBus) Start(ctx context.Context) error { return tb.bus.Start(ctx) }
func (tb *testBus) Stop(ctx context.Context) error  { return tb.bus.Stop(ctx) }
func (tb *testBus) Health() error                   { return tb.bus.Health() }

func (tb *testBus) Publish(ctx context.Context, event core.Event) error {
	return tb.bus.Publish(ctx, event)
}

func (tb *testBus) Subscribe(eventType string, handler core.EventHandler) (unsubscribe func()) {
	return tb.bus.Subscribe(eventType, handler)
}

func (tb *testBus) SubscribeChan(eventType string) (<-chan core.Event, func()) {
	return tb.bus.SubscribeChan(eventType)
}

func TestManager_HandlePluginLoaded_AutoRegisters(t *testing.T) {
	// Create a temporary plugin directory with a manifest that declares extensions.
	pluginsDir := t.TempDir()
	pluginDir := pluginsDir + "/test-plugin"
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `apiVersion: siply/v1
kind: Plugin
metadata:
  name: test-plugin
  version: 1.0.0
  siply_min: "0.1.0"
  description: Test plugin for auto-registration
  author: test
  license: MIT
  updated: "2026-04-01"
spec:
  tier: 3
  capabilities: {}
  extensions:
    panels:
      - name: test-tree
        position: left
        min_width: 20
        max_width: 40
        collapsible: true
        keybind: ctrl+t
        icon: "🌲"
    menu_items:
      - label: Test Action
        category: Extensions
    keybindings:
      - key: ctrl+e
        description: Execute test
`
	if err := os.WriteFile(pluginDir+"/manifest.yaml", []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, pluginsDir)
	startManager(t, m)

	// Publish a PluginLoadedEvent — this triggers handlePluginLoaded.
	ctx := context.Background()
	ev := events.NewPluginLoadedEvent("test-plugin", "1.0.0", 3)
	if err := bus.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}

	// Wait for async delivery.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if regs := m.Registrations("test-plugin"); len(regs) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	regs := m.Registrations("test-plugin")
	if len(regs) != 3 {
		t.Fatalf("expected 3 registrations (panel+menu+keybind), got %d", len(regs))
	}

	// Verify panel was registered in PanelRegistry.
	if _, ok := pr.Panel("test-tree"); !ok {
		t.Error("panel 'test-tree' not registered in PanelRegistry")
	}

	// Verify menu items and keybindings.
	items := m.AllMenuItems()
	if len(items) != 1 || items[0].Label != "Test Action" {
		t.Errorf("unexpected menu items: %v", items)
	}
	bindings := m.AllKeybindings()
	if len(bindings) != 1 || bindings[0].Key != "ctrl+e" {
		t.Errorf("unexpected keybindings: %v", bindings)
	}
}

func TestManager_HandlePluginLoaded_NoManifest(t *testing.T) {
	pluginsDir := t.TempDir()
	pr := newStubPanelRegistry()
	bus := newTestBus(t)
	m := extensions.NewManager(pr, bus, pluginsDir)
	startManager(t, m)

	ctx := context.Background()
	ev := events.NewPluginLoadedEvent("nonexistent", "1.0.0", 3)
	if err := bus.Publish(ctx, ev); err != nil {
		t.Fatal(err)
	}

	// Wait briefly — should not crash.
	time.Sleep(100 * time.Millisecond)

	regs := m.Registrations("nonexistent")
	if len(regs) != 0 {
		t.Errorf("expected 0 registrations for missing manifest, got %d", len(regs))
	}
}

func startManager(t *testing.T, m *extensions.Manager) {
	t.Helper()
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
}
