# Panel System Reference — AI Agent & Developer Guide

> **Purpose:** Complete reference for building, debugging, and maintaining TUI panels in Siply.
> **Audience:** AI agents (autonomous panel builders), plugin developers, human contributors.
> **Source:** All patterns extracted from production code + bugs discovered during development.

---

## Architecture Overview

```
Terminal
  └── App (internal/tui/app.go)
        ├── PanelManager (internal/tui/panels/manager.go)
        │     ├── Left slot    → Tier 3 plugins (e.g., tree-local)
        │     ├── Right slot   → Tier 3 plugins (e.g., markdown-preview)
        │     ├── Bottom slot  → future use
        │     └── Overlays     → floating panels (lipgloss.Compositor)
        ├── REPL panel         → center content
        ├── Activity feed      → center content
        ├── Diff view          → center content
        └── Status bar         → bottom row
```

Panels are **plugin-driven**. The TUI knows nothing about tree views or markdown — plugins provide content via gRPC, the panel system renders it.

---

## 1. PanelConfig — Registration Contract

File: `internal/core/panel.go`

```go
type PanelConfig struct {
    Name        string           // REQUIRED — unique identifier
    Position    PanelPosition    // REQUIRED — PanelLeft, PanelRight, PanelBottom, PanelOverlay
    PluginName  string           // REQUIRED for plugin panels — matches manifest name
    MinWidth    int              // minimum width in cells (used as initial width)
    MaxWidth    int              // maximum width (0 = unlimited)
    Collapsible bool             // can be collapsed via keybind
    Keybind     string           // key to toggle collapse (e.g., "ctrl+t")
    Icon        string           // emoji shown in collapsed state and tab bar
    MenuLabel   string           // label in menu and tab bar
    ContentFunc func(w, h int) string  // renders panel content at given dimensions
    OnActivate  func() error     // callback on first activation (lazy init)
    LazyInit    bool             // if true, OnActivate runs only once
    // Overlay-only fields:
    OverlayX    int              // horizontal offset
    OverlayY    int              // vertical offset
    OverlayZ    int              // z-index (higher = on top)
}
```

### Critical Rules

- **Name must be unique** across all panels. Duplicate → registration error.
- **ContentFunc receives actual panel dimensions** (inner width minus borders, content height minus tab bar). ALWAYS use these, never hardcode sizes.
- **ContentFunc must not block.** It runs on the render thread. Do expensive work in background goroutines and cache the result.
- **PluginName links the panel to its plugin** for key/click event routing via ActionProvider.

---

## 2. Content Rendering Pipeline

### How content flows from plugin to screen:

```
Plugin binary (gRPC)
    ↓ tier3Loader.Execute(name, "render", [w,h,w,h])
ContentProvider closure (cmd/siply/tui.go)
    ↓ returns rendered string
ContentFunc(width, height) on PanelConfig
    ↓ called by renderSlot()
PanelManager.renderSlot()
    ↓ adds │ borders, tab bar, padding
PanelManager.View()
    ↓ JoinHorizontal(left, center, right)
    ↓ JoinVertical(mainRow, bottom)
App.renderStandard()
    ↓ adds status bar
Terminal
```

### The 4-byte dimension payload

```go
payload := []byte{byte(width >> 8), byte(width), byte(height >> 8), byte(height)}
```

Plugin decodes:
```go
w := int(payload[0])<<8 | int(payload[1])
h := int(payload[2])<<8 | int(payload[3])
```

### ContentFunc vs Viewport

| Feature | ContentFunc | Viewport |
|---------|------------|----------|
| Use case | Plugin-rendered content | gRPC streaming content |
| Scroll | Plugin manages internally | viewport.Model handles |
| Caching | lastRender map (dirty flag) | viewport dirty flag |
| Dimensions | Receives (width, height) | SetSize(width, height) |

**Default choice: ContentFunc.** Use viewport only for real-time streaming content.

### Dirty Flag Cache

```go
// In renderSlot:
if vp, hasVP := m.viewports[name]; hasVP {
    if !vp.IsDirty() {
        if cached, ok := m.lastRender[name]; ok {
            return cached  // skip re-render
        }
    }
}
```

The cache key is the panel name. Cache is invalidated when:
- Viewport content changes (dirty flag set)
- Panel is resized (width/height change)
- Viewport is detached

---

## 3. Layout System

File: `internal/tui/layout.go` — `CalculateLayoutWithPanels()`

### Layout Modes

| Mode | Terminal Width | Behavior |
|------|--------------|----------|
| UltraCompact | < 60 | All panels collapsed, full width to center |
| Compact | 60-79 | All panels collapsed |
| Standard | 80-119 | ONE side panel max (left preferred over right) |
| SplitAvailable | ≥ 120 | Both side panels, each max 1/3 of width |

### Width Clamping

```go
// SplitAvailable mode:
lc.LeftPanelWidth = clampInt(leftWidth, 0, MaxContentWidth/3)
lc.RightPanelWidth = clampInt(rightWidth, 0, MaxContentWidth/3)
center := MaxContentWidth - left - right
if center < 40 { collapse all panels }
```

### CRITICAL: Center Width Padding

**Bug discovered:** If center content is narrower than `CenterWidth`, `lipgloss.JoinHorizontal` shifts the right panel left. The divider detection then looks at the wrong X position.

**Fix:** `padLinesToWidth()` — pads every line to exactly CenterWidth:

```go
func padLinesToWidth(content string, width int) string {
    lines := strings.Split(content, "\n")
    for i, line := range lines {
        lw := ansi.StringWidth(line)
        if lw < width {
            line += strings.Repeat(" ", width-lw)
        } else {
            line = ansi.Truncate(line, width, "")
        }
    }
}
```

**Why ansi.StringWidth:** ANSI escape codes (colors, bold) have zero display width. `len(line)` counts bytes. `ansi.StringWidth` counts display cells. Using `len()` here would break everything.

---

## 4. Mouse Interaction

### Event Flow

```
Terminal mouse event
    ↓
tea.MouseClickMsg / MouseMotionMsg / MouseReleaseMsg / MouseWheelMsg
    ↓
App.Update() — routes to PanelManager.Update()
    ↓
PanelManager handles: overlay hit → divider drag → focus change → plugin click
```

### CRITICAL: Mouse Must Be Enabled

```go
// In App.View():
if menuOpen || slashOpen || a.panelManager != nil {
    v.MouseMode = tea.MouseModeCellMotion
}
```

**Bug discovered:** Mouse was OFF by default. Only enabled when menu was open. Panels never received mouse events. Fix: enable when PanelManager exists.

### CRITICAL: All Mouse Types Must Be Routed

```go
// In App.Update() — ALL of these must exist:
case tea.MouseClickMsg:    → panelManager.Update(msg)
case tea.MouseMotionMsg:   → panelManager.Update(msg)
case tea.MouseReleaseMsg:  → panelManager.Update(msg)
case tea.MouseWheelMsg:    → panelManager.Update(msg)
```

**Bug discovered:** Only MouseClickMsg was handled. Motion/Release/Wheel were silently dropped. Drag resize was impossible.

### Divider Detection

```go
renderedLeftW := m.lastRenderedLeftW   // from CalculateLayoutWithPanels
renderedRightW := m.lastRenderedRightW

if abs(msg.X - renderedLeftW) <= 2 { start left drag }
if abs(msg.X - (totalW - renderedRightW)) <= 2 { start right drag }
```

**Bug discovered:** Used `m.left.width` (desired) instead of `lc.LeftPanelWidth` (rendered). After clamping, these differ. Fix: store `lastRenderedLeftW`/`lastRenderedRightW` from `View()`.

**Hitbox is ±2 pixels** to compensate for sub-pixel width mismatches.

### Drag Resize

```go
case tea.MouseMotionMsg:
    delta := msg.X - m.dragStartX
    m.dragStartX = msg.X
    case focusLeft:  m.left.width += delta    // drag right = wider
    case focusRight: m.right.width -= delta   // drag left = wider (inverted)
```

### Click → Plugin

```go
target := m.slotAtX(msg.X)
m.focus = target
m.sendClickToPlugin(target, msg.Y)  // goroutine: actionSender(plugin, "click", [row])
```

### Mouse Wheel → Scroll

```go
case tea.MouseWheelMsg:
    // Try viewport first
    if cmd := m.routeScrollToSlot(target, msg); cmd != nil { return cmd }
    // No viewport — forward as key event to plugin
    key := "down"
    if msg.Button == tea.MouseWheelUp { key = "up" }
    m.sendKeyToFocusedPlugin(key)
```

---

## 5. Key Routing

### Flow

```
User presses key
    ↓
App.Update(tea.KeyPressMsg)
    ↓
Built-in keys: Ctrl+C (quit), Ctrl+Space (menu), Ctrl+B (borders)
    ↓
PanelManager.Update(msg)
    ↓
Tab/Shift+Tab → cycleFocus()
Alt+Left/Right → resizeFocusedPanel()
Ctrl+]/[ → switchTab()
Up/Down/PgUp/PgDown → routeScrollKey() then sendKeyToFocusedPlugin()
Enter/Space → sendKeyToFocusedPlugin()
Other → handlePanelKeybind() (collapse/expand toggle)
```

### sendKeyToFocusedPlugin

```go
func (m *PanelManager) sendKeyToFocusedPlugin(key string) {
    s := m.focusedSlot()
    info, ok := s.activePanel()
    if !ok || info.Config.PluginName == "" { return }
    go m.actionSender(info.Config.PluginName, "key", []byte(key))
}
```

**CRITICAL:** Runs in a goroutine (`go sender(...)`) to avoid blocking the render thread. Plugin Execute calls are gRPC round-trips (~1ms).

---

## 6. Plugin Action Protocol

### Standard Actions

| Action | Payload | Purpose |
|--------|---------|---------|
| `render` | `[wH, wL, hH, hL]` (4 bytes) | Get panel content at dimensions |
| `key` | `[]byte(keyName)` | Forward keyboard input |
| `click` | `[]byte{row}` | Forward mouse click (panel-relative Y) |
| `select` | `[]byte(path)` | Select an item (tree-local specific) |
| `refresh` | nil | Clear cache, rebuild content |

### Wiring in tui.go

```go
// ContentProvider — creates ContentFunc closures
em.SetContentProvider(func(pluginName string) func(w, h int) string {
    return func(w, h int) string {
        payload := []byte{byte(w>>8), byte(w), byte(h>>8), byte(h)}
        result, _ := tier3Loader.Execute(ctx, pluginName, "render", payload)
        return string(result)
    }
})

// ActionProvider — forwards key/click/scroll to plugins
em.SetActionProvider(func(pluginName, action string, payload []byte) {
    tier3Loader.Execute(ctx, pluginName, action, payload)
})

// Wire to PanelManager
panelMgr.SetActionSender(em.SendAction)
```

---

## 7. Event System — Cross-Plugin Communication

### file.selected Flow

```
tree-local: handleSelect(path)
    ↓ hostClient.PublishEvent({type:"file.selected", payload:path})
HostServer.PublishEvent()
    ↓ eventBus.Publish(FileSelectedEvent)
EventBus async delivery
    ↓ callback for each subscriber
markdown-preview: HandleEvent({type:"file.selected", payload:path})
    ↓ if .md file: store path, reset scrollOffset
    ↓ next render call returns rendered markdown
```

### Plugin Event Subscription (in Initialize)

```go
p.hostClient.SubscribeEvent(ctx, &SubscribeEventRequest{
    EventType:  "file.selected",
    PluginName: p.name,
    PluginAddr: p.ownAddr,  // so host can call back HandleEvent
})
```

---

## 8. Manifest-Based Auto-Registration

File: `internal/extensions/manager.go` — `handlePluginLoaded()`

When a plugin is loaded, ExtensionManager reads its manifest and auto-registers:

```yaml
# manifest.yaml
spec:
  extensions:
    panels:
      - name: tree-local
        position: left       # left, right, bottom
        min_width: 20
        max_width: 40
        collapsible: true
        keybind: ctrl+t
        icon: "📁"
        menu_label: File Tree
    menu_items:
      - label: Toggle File Tree
        icon: "📁"
        keybind: ctrl+t
        category: Panels
    keybindings:
      - key: ctrl+t
        description: Toggle file tree panel
```

The ExtensionManager creates `PanelConfig` from manifest fields and sets `ContentFunc` via the ContentProvider.

---

## 9. Working Directory Propagation

**Bug discovered:** Plugins call `os.Getwd()` but the plugin binary may run from a different directory than where the user launched `siply tui`.

**Fix:** `tier3_loader.go` injects `SIPLY_WORKING_DIR`:

```go
hostCWD, _ := os.Getwd()
cmd.Env = append(os.Environ(),
    fmt.Sprintf("SIPLY_WORKING_DIR=%s", hostCWD),
)
```

Plugin reads it:
```go
cwd := os.Getenv("SIPLY_WORKING_DIR")
if cwd == "" { cwd, _ = os.Getwd() }
```

---

## 10. Building a New Panel Plugin

### Step 1: Create plugin directory

```
plugins/my-plugin/
  ├── main.go          # gRPC server
  ├── go.mod           # separate module
  ├── manifest.yaml    # registration config
  └── README.md
```

### Step 2: manifest.yaml

```yaml
apiVersion: siply/v1
kind: Plugin
metadata:
  name: my-plugin
  version: 1.0.0
  description: What this panel shows
spec:
  tier: 3
  extensions:
    panels:
      - name: my-plugin
        position: right
        min_width: 25
        max_width: 50
        collapsible: true
        keybind: ctrl+m
        icon: "🔧"
        menu_label: My Panel
```

### Step 3: main.go skeleton

```go
package main

import (
    "context"
    "fmt"
    "net"
    "os"
    "strings"
    "sync"

    "google.golang.org/grpc"
    siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

type myPlugin struct {
    siplyv1.UnimplementedSiplyPluginServiceServer
    mu           sync.RWMutex
    initialized  bool
    scrollOffset int
    lastHeight   int
}

func (p *myPlugin) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    p.initialized = true
    return &siplyv1.InitializeResponse{Success: true, Capabilities: []string{"panel"}}, nil
}

func (p *myPlugin) Execute(_ context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
    switch req.GetAction() {
    case "render":
        return p.handleRender(req.GetPayload())
    case "key":
        return p.handleKey(req.GetPayload())
    case "click":
        return p.handleClick(req.GetPayload())
    default:
        return &siplyv1.ExecuteResponse{Success: false, Error: new(string)}, nil
    }
}

func (p *myPlugin) handleRender(payload []byte) (*siplyv1.ExecuteResponse, error) {
    width, height := 40, 24
    if len(payload) >= 4 {
        width = int(payload[0])<<8 | int(payload[1])
        height = int(payload[2])<<8 | int(payload[3])
    }
    p.mu.Lock()
    p.lastHeight = height
    p.mu.Unlock()

    // Build your content here using width and height
    content := fmt.Sprintf("Panel: %dx%d", width, height)

    return &siplyv1.ExecuteResponse{Success: true, Result: []byte(content)}, nil
}

func (p *myPlugin) handleKey(payload []byte) (*siplyv1.ExecuteResponse, error) {
    key := strings.TrimSpace(string(payload))
    p.mu.Lock()
    switch key {
    case "up":   p.scrollOffset--
    case "down": p.scrollOffset++
    }
    if p.scrollOffset < 0 { p.scrollOffset = 0 }
    p.mu.Unlock()
    return &siplyv1.ExecuteResponse{Success: true}, nil
}

func (p *myPlugin) handleClick(payload []byte) (*siplyv1.ExecuteResponse, error) {
    if len(payload) < 1 { return &siplyv1.ExecuteResponse{Success: true}, nil }
    row := int(payload[0])
    // Handle click at row
    _ = row
    return &siplyv1.ExecuteResponse{Success: true}, nil
}

// main(), Shutdown(), HandleEvent() — same pattern as tree-local
```

### Step 4: Build and install

```bash
cd plugins/my-plugin
go build -o my-plugin .
cp my-plugin ~/.siply/plugins/my-plugin/my-plugin
cp manifest.yaml ~/.siply/plugins/my-plugin/manifest.yaml
```

### Step 5: Test

```bash
siply tui
# Panel should appear automatically via manifest auto-registration
```

---

## 11. Common Bugs and How to Avoid Them

| Bug | Root Cause | Prevention |
|-----|-----------|------------|
| Panel shows icon+name only | ContentFunc is nil — ContentProvider not wired | Ensure `em.SetContentProvider()` before plugin loading |
| Tree shows wrong directory | Plugin uses `os.Getwd()` without checking `SIPLY_WORKING_DIR` | Always read env var first |
| Mouse doesn't work | MouseMode not enabled in App.View() | Check `panelManager != nil` condition |
| Drag doesn't work | Mouse events not routed to PanelManager | All 4 mouse message types must be handled in App.Update() |
| Divider detection wrong | Using slot.width instead of rendered width | Store `lastRenderedLeftW`/`lastRenderedRightW` from CalculateLayoutWithPanels |
| Right panel at wrong position | Center content not padded to exact CenterWidth | Use `padLinesToWidth()` |
| Content cut off at 50% | ContentFunc ignores height, renders fixed 24 rows | Always use the `height` parameter |
| No scroll in panels | Mouse wheel not forwarded to plugin as key event | Fallback from viewport scroll to sendKeyToFocusedPlugin |
| ANSI corruption | Using strings.Replace on styled content | Use lipgloss.JoinHorizontal — never manipulate ANSI strings directly |
| Panel borders invisible | Using len() instead of ansi.StringWidth() for width | Always use ansi.StringWidth for display width |

---

## 12. Performance Budgets

From benchmarks (`bench_test.go`):

| Scenario | Measured | Budget | Margin |
|----------|----------|--------|--------|
| Dock-only (2 panels) | ~41μs | 16ms | 390x |
| With overlay | ~256μs | 16ms | 62x |
| Unicode content (CJK/emoji) | ~35μs | 16ms | 457x |

**Rules:**
- ContentFunc must return in < 1ms
- ActionSender runs in goroutine — no render blocking
- Dirty flag prevents unnecessary re-renders
- Compositor only instantiated when overlays are active

---

## 13. Key Files Quick Reference

| File | Purpose |
|------|---------|
| `internal/core/panel.go` | PanelConfig, PanelInfo, PanelRegistry interface |
| `internal/tui/panels/manager.go` | PanelManager — the core panel engine |
| `internal/tui/panels/viewport_wrapper.go` | Per-panel viewport with dirty flags |
| `internal/tui/panels/content_receiver.go` | gRPC stream → viewport routing |
| `internal/tui/app.go` | Mouse/key event routing from App to PanelManager |
| `internal/tui/layout.go` | Layout calculation, width clamping |
| `internal/tui/tokens.go` | Theme tokens (Primary, OverlayBg) |
| `internal/extensions/manager.go` | Auto-registration from manifests, ContentProvider, ActionProvider |
| `internal/plugins/tier3_loader.go` | Plugin process management, SIPLY_WORKING_DIR |
| `cmd/siply/tui.go` | Wiring everything together |
| `plugins/tree-local/main.go` | Reference: interactive panel with tree, key nav, click, events |
| `plugins/markdown-preview/main.go` | Reference: content viewer with scroll, event subscription |
