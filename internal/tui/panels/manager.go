// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"gopkg.in/yaml.v3"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
	"siply.dev/siply/internal/tui"
)

// compile-time checks.
var _ core.PanelRegistry = (*PanelManager)(nil)
var _ tui.PanelManager = (*PanelManager)(nil)

const (
	focusRepl   = "repl"
	focusLeft   = "left"
	focusRight  = "right"
	focusBottom = "bottom"
	keyTab      = "tab"
)

// slot holds all panels registered for one position, supporting tabs.
type slot struct {
	panels    []core.PanelInfo
	activeTab int
	width     int
	collapsed bool
}

// activePanel returns the currently visible PanelInfo for this slot, if any.
func (s *slot) activePanel() (core.PanelInfo, bool) {
	if len(s.panels) == 0 {
		return core.PanelInfo{}, false
	}
	idx := s.activeTab
	if idx < 0 || idx >= len(s.panels) {
		idx = 0
	}
	return s.panels[idx], true
}

type panelRef struct {
	position core.PanelPosition
	index    int
}

// PanelManager manages panel positions (left, right, bottom, overlay).
// It handles focus cycling, tab switching, collapse/expand, resize, lazy loading,
// and overlay compositing via lipgloss.Compositor.
type PanelManager struct {
	mu sync.Mutex

	left   slot
	right  slot
	bottom slot

	// Overlay panels (floating, composited via lipgloss.Compositor).
	overlays []overlayEntry

	// name → slot ref (dock panels) or overlay index.
	registry map[string]panelRef

	// Tracks which panels have been lazily initialized (by name).
	initialized map[string]bool

	// Focus: "repl", "left", "right", "bottom", "overlay".
	focus string

	// Mouse drag state.
	dragging   bool
	dragTarget string
	dragStartX int

	theme        tui.Theme
	renderConfig tui.RenderConfig

	// viewports holds an optional per-panel viewport keyed by panel name.
	// When set, the viewport provides scrollable content instead of ContentFunc.
	viewports map[string]*panelViewport

	// contentReceiver receives gRPC streams and routes to viewports.
	contentReceiver *ContentReceiver

	// lastRender caches the last rendered output per panel name (dirty-flag optimization).
	lastRender map[string]string

	// lastViewWidth caches the total width from the last View() call,
	// used for mouse coordinate → slot resolution.
	lastViewWidth int

	// Rendered panel widths from the last View() call (after layout clamping).
	// These may differ from slot.width due to CalculateLayoutWithPanels clamping.
	lastRenderedLeftW  int
	lastRenderedRightW int

	// actionSender forwards key/click events to plugins via the extension manager.
	actionSender func(pluginName, action string, payload []byte)
}

// overlayEntry holds an overlay panel and its positioning.
type overlayEntry struct {
	info core.PanelInfo
	x, y int
	z    int
}

// NewPanelManager creates a PanelManager with the given theme and render config.
func NewPanelManager(theme tui.Theme, rc tui.RenderConfig) *PanelManager {
	pm := &PanelManager{
		registry:     make(map[string]panelRef),
		initialized:  make(map[string]bool),
		focus:        focusRepl,
		theme:        theme,
		renderConfig: rc,
		viewports:    make(map[string]*panelViewport),
		lastRender:   make(map[string]string),
	}
	pm.contentReceiver = NewContentReceiver(pm)
	return pm
}

// ─── core.Lifecycle ──────────────────────────────────────────────────────────

func (m *PanelManager) Init(_ context.Context) error  { return nil }
func (m *PanelManager) Start(_ context.Context) error { return nil }
func (m *PanelManager) Stop(_ context.Context) error  { return nil }
func (m *PanelManager) Health() error                 { return nil }

// ─── core.PanelRegistry ──────────────────────────────────────────────────────

// Register adds a panel to the manager.
func (m *PanelManager) Register(cfg core.PanelConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.Name == "" {
		return fmt.Errorf("panel name must not be empty")
	}
	if _, exists := m.registry[cfg.Name]; exists {
		return fmt.Errorf("panel %q already registered", cfg.Name)
	}

	info := core.PanelInfo{
		Config: cfg,
		Width:  cfg.MinWidth,
	}

	if cfg.Position == core.PanelOverlay {
		idx := len(m.overlays)
		m.overlays = append(m.overlays, overlayEntry{
			info: info,
			x:    cfg.OverlayX,
			y:    cfg.OverlayY,
			z:    cfg.OverlayZ,
		})
		m.registry[cfg.Name] = panelRef{position: cfg.Position, index: idx}
		return nil
	}

	s := m.slotForPosition(cfg.Position)
	if s == nil {
		return fmt.Errorf("unknown panel position %d", cfg.Position)
	}

	idx := len(s.panels)
	s.panels = append(s.panels, info)
	if s.width == 0 && cfg.MinWidth > 0 {
		s.width = cfg.MinWidth
	}
	m.registry[cfg.Name] = panelRef{position: cfg.Position, index: idx}
	return nil
}

// Unregister removes a panel from the manager.
func (m *PanelManager) Unregister(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ref, ok := m.registry[name]
	if !ok {
		return fmt.Errorf("panel %q not registered", name)
	}

	if ref.position == core.PanelOverlay {
		m.overlays = append(m.overlays[:ref.index], m.overlays[ref.index+1:]...)
		for n, r := range m.registry {
			if r.position == core.PanelOverlay && r.index > ref.index {
				m.registry[n] = panelRef{position: r.position, index: r.index - 1}
			}
		}
		delete(m.registry, name)
		delete(m.initialized, name)
		return nil
	}

	s := m.slotForPosition(ref.position)
	s.panels = append(s.panels[:ref.index], s.panels[ref.index+1:]...)

	for n, r := range m.registry {
		if r.position == ref.position && r.index > ref.index {
			m.registry[n] = panelRef{position: r.position, index: r.index - 1}
		}
	}
	delete(m.registry, name)
	delete(m.initialized, name)

	if s.activeTab >= len(s.panels) && len(s.panels) > 0 {
		s.activeTab = len(s.panels) - 1
	}
	return nil
}

// Panel returns info for a named panel.
func (m *PanelManager) Panel(name string) (core.PanelInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.registry[name]
	if !ok {
		return core.PanelInfo{}, false
	}
	if ref.position == core.PanelOverlay {
		if ref.index < len(m.overlays) {
			return m.overlays[ref.index].info, true
		}
		return core.PanelInfo{}, false
	}
	s := m.slotForPosition(ref.position)
	return s.panels[ref.index], true
}

// Panels returns all registered panels across all positions.
func (m *PanelManager) Panels() []core.PanelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []core.PanelInfo
	for _, s := range []*slot{&m.left, &m.right, &m.bottom} {
		out = append(out, s.panels...)
	}
	for _, oe := range m.overlays {
		out = append(out, oe.info)
	}
	return out
}

// Activate marks a panel active and calls OnActivate / lazy-init on first open.
func (m *PanelManager) Activate(name string) error {
	m.mu.Lock()

	ref, ok := m.registry[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("panel %q not registered", name)
	}

	var info *core.PanelInfo
	if ref.position == core.PanelOverlay {
		if ref.index >= len(m.overlays) {
			m.mu.Unlock()
			return fmt.Errorf("panel %q overlay index out of range", name)
		}
		info = &m.overlays[ref.index].info
	} else {
		s := m.slotForPosition(ref.position)
		info = &s.panels[ref.index]
	}

	if info.Active {
		m.mu.Unlock()
		return nil
	}

	info.Active = true
	if ref.position != core.PanelOverlay {
		s := m.slotForPosition(ref.position)
		s.collapsed = false
		s.activeTab = ref.index
	}

	var callback func() error
	shouldCall := false

	if info.Config.OnActivate != nil {
		if info.Config.LazyInit {
			if !m.initialized[name] {
				callback = info.Config.OnActivate
				shouldCall = true
				m.initialized[name] = true
			}
		} else {
			callback = info.Config.OnActivate
			shouldCall = true
		}
	}

	m.mu.Unlock()

	if shouldCall && callback != nil {
		start := time.Now()
		if err := callback(); err != nil {
			slog.Warn("panel OnActivate error", "panel", name, "error", err)
		}
		if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
			slog.Warn("panel lazy init exceeded 100ms", "panel", name, "elapsed", elapsed)
		}
	}
	return nil
}

// Deactivate marks a panel inactive.
func (m *PanelManager) Deactivate(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ref, ok := m.registry[name]
	if !ok {
		return fmt.Errorf("panel %q not registered", name)
	}
	if ref.position == core.PanelOverlay {
		if ref.index < len(m.overlays) {
			m.overlays[ref.index].info.Active = false
		}
		return nil
	}
	s := m.slotForPosition(ref.position)
	s.panels[ref.index].Active = false
	return nil
}

// ─── Viewport management ─────────────────────────────────────────────────────

// PanelViewport returns the viewport for a named panel, or nil if not found.
// Implements ViewportRegistry. Thread-safe.
func (m *PanelManager) PanelViewport(name string) *panelViewport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.viewports[name]
}

// AttachViewport creates and attaches a per-panel viewport for the named panel.
// If the panel already has a viewport, it is replaced.
func (m *PanelManager) AttachViewport(name string, width, height int, pluginName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.viewports[name] = newPanelViewport(width, height, pluginName)
}

// DetachViewport removes the viewport for the named panel.
func (m *PanelManager) DetachViewport(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.viewports, name)
	delete(m.lastRender, name)
}

// ContentRecv returns the content receiver for gRPC stream subscriptions.
func (m *PanelManager) ContentRecv() *ContentReceiver {
	return m.contentReceiver
}

// SetActionSender sets the function used to forward actions to plugins.
func (m *PanelManager) SetActionSender(fn func(pluginName, action string, payload []byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionSender = fn
}

// ─── tui.PanelManager ────────────────────────────────────────────────────────

// Update handles tea messages for the panel system.
func (m *PanelManager) Update(msg tea.Msg) tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applyAutoCollapse(msg.Width)

	case tea.KeyPressMsg:
		switch msg.String() {
		case keyTab:
			m.cycleFocus(1)
		case "shift+" + keyTab:
			m.cycleFocus(-1)
		case "alt+left":
			m.resizeFocusedPanel(-2)
		case "alt+right":
			m.resizeFocusedPanel(2)
		case "ctrl+]":
			m.switchTab(1)
		case "ctrl+[":
			m.switchTab(-1)
		case "up", "down", "pgup", "pgdown", "home", "end", "k", "j":
			if cmd := m.routeScrollKey(msg); cmd != nil {
				return cmd
			}
			m.sendKeyToFocusedPlugin(msg.String())
		case "enter", " ":
			m.sendKeyToFocusedPlugin(msg.String())
		default:
			m.handlePanelKeybind(msg.String())
		}

	case tea.MouseClickMsg:
		m.dragging = false
		// Route clicks to overlay panels first (z-index priority).
		if m.hasActiveOverlays() {
			if _, hit := m.hitOverlayUnlocked(msg.X, msg.Y); hit {
				m.focus = "overlay"
				return nil
			}
		}

		// Check if click is on a divider between panels → start dragging.
		// Use lastRendered widths (post-clamping) for accurate hit detection.
		renderedLeftW := m.lastRenderedLeftW
		renderedRightW := m.lastRenderedRightW
		totalW := m.lastViewWidth

		if renderedLeftW > 0 && abs(msg.X-renderedLeftW) <= 1 {
			m.dragging = true
			m.dragTarget = focusLeft
			m.dragStartX = msg.X
			return nil
		}
		if renderedRightW > 0 && totalW > 0 && abs(msg.X-(totalW-renderedRightW)) <= 1 {
			m.dragging = true
			m.dragTarget = focusRight
			m.dragStartX = msg.X
			return nil
		}

		// Click within a panel region → change focus + forward click to plugin.
		target := m.slotAtX(msg.X)
		m.focus = target
		m.sendClickToPlugin(target, msg.Y)

	case tea.MouseMotionMsg:
		if m.dragging {
			delta := msg.X - m.dragStartX
			m.dragStartX = msg.X
			switch m.dragTarget {
			case focusLeft:
				m.left.width = clampSlotWidth(&m.left, m.left.width+delta)
			case focusRight:
				m.right.width = clampSlotWidth(&m.right, m.right.width-delta)
			}
		}

	case tea.MouseReleaseMsg:
		m.dragging = false

	case tea.MouseWheelMsg:
		// Route scroll events to the panel under the cursor.
		target := m.slotAtX(msg.X)
		if cmd := m.routeScrollToSlot(target, msg); cmd != nil {
			return cmd
		}
	}

	return nil
}

// View composes [left panel] | [center content] | [right panel] + [bottom panel]
// using lipgloss.JoinHorizontal/JoinVertical for ANSI-safe layout composition.
func (m *PanelManager) View(width, height int, centerContent string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastViewWidth = width

	leftW := 0
	if !m.left.collapsed && len(m.left.panels) > 0 {
		leftW = m.left.width
	}
	rightW := 0
	if !m.right.collapsed && len(m.right.panels) > 0 {
		rightW = m.right.width
	}

	lc := tui.CalculateLayoutWithPanels(width, height, leftW, rightW, 0)
	m.lastRenderedLeftW = lc.LeftPanelWidth
	m.lastRenderedRightW = lc.RightPanelWidth

	// Sync collapsed state when layout mode forces zero panel widths.
	if lc.LeftPanelWidth == 0 && leftW > 0 {
		m.left.collapsed = true
	}
	if lc.RightPanelWidth == 0 && rightW > 0 {
		m.right.collapsed = true
	}

	// Reserve space for bottom panel before rendering main row.
	bottomH := 0
	if m.hasActiveBottom() {
		bottomH = max(height/4, 3)
	}
	mainH := height - bottomH

	leftStr := m.renderSlot(&m.left, lc.LeftPanelWidth, mainH, m.focus == focusLeft)
	rightStr := m.renderSlot(&m.right, lc.RightPanelWidth, mainH, m.focus == focusRight)

	// Pad each line of centerContent to exactly CenterWidth so the right panel
	// starts at totalW - rightW (matching divider hit detection).
	if lc.CenterWidth > 0 {
		centerContent = padLinesToWidth(centerContent, lc.CenterWidth)
	}

	var sections []string
	if leftStr != "" {
		sections = append(sections, leftStr)
	}
	sections = append(sections, centerContent)
	if rightStr != "" {
		sections = append(sections, rightStr)
	}

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, sections...)

	var dockResult string
	if bottomH > 0 {
		bottomStr := m.renderSlot(&m.bottom, width, bottomH, m.focus == focusBottom)
		if bottomStr != "" {
			dockResult = lipgloss.JoinVertical(lipgloss.Left, mainRow, bottomStr)
		} else {
			dockResult = mainRow
		}
	} else {
		dockResult = mainRow
	}

	// Overlay compositing: only use Compositor when overlays are active.
	if m.hasActiveOverlays() {
		return m.composeOverlays(dockResult, width, height)
	}

	return dockResult
}

// hasActiveOverlays returns true if any overlay panel is active.
func (m *PanelManager) hasActiveOverlays() bool {
	for _, oe := range m.overlays {
		if oe.info.Active {
			return true
		}
	}
	return false
}

// composeOverlays renders active overlay panels on top of dock content
// using lipgloss.Compositor for cell-based compositing with z-index.
func (m *PanelManager) composeOverlays(dockContent string, width, height int) string {
	base := lipgloss.NewLayer(dockContent).Z(0)

	var overlayLayers []*lipgloss.Layer
	for i := range m.overlays {
		oe := &m.overlays[i]
		if !oe.info.Active {
			continue
		}

		panelW := oe.info.Width
		if panelW < oe.info.Config.MinWidth {
			panelW = oe.info.Config.MinWidth
		}
		if panelW == 0 {
			panelW = width / 3
		}

		panelH := height / 2
		if panelH < 5 {
			panelH = 5
		}

		content := m.renderOverlayContent(oe, panelW, panelH)

		// Solid background via lipgloss.Style to prevent dock content bleeding through.
		bgStyle := lipgloss.NewStyle().
			Width(panelW).
			Height(panelH).
			Background(m.theme.OverlayBg)
		styled := bgStyle.Render(content)

		layer := lipgloss.NewLayer(styled).
			X(oe.x).
			Y(oe.y).
			Z(oe.z).
			ID(oe.info.Config.Name)

		overlayLayers = append(overlayLayers, layer)
	}

	allLayers := append([]*lipgloss.Layer{base}, overlayLayers...)
	comp := lipgloss.NewCompositor(allLayers...)
	return comp.Render()
}

// renderOverlayContent produces the visual content for an overlay panel.
func (m *PanelManager) renderOverlayContent(oe *overlayEntry, width, _ int) string {
	content := ""
	if oe.info.Config.ContentFunc != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					content = fmt.Sprintf("[panel error: %v]", r)
					slog.Error("overlay ContentFunc panic", "panel", oe.info.Config.Name, "error", r)
				}
			}()
			content = oe.info.Config.ContentFunc()
		}()
	} else {
		icon := oe.info.Config.Icon
		if icon == "" {
			icon = "◇"
		}
		content = icon + " " + oe.info.Config.Name
	}

	// Wrap in border using the panel's keybind as title hint.
	title := oe.info.Config.MenuLabel
	if title == "" {
		title = oe.info.Config.Name
	}
	return tui.RenderBorder(title, content, m.renderConfig, m.theme, width)
}

// HitOverlay tests whether a screen coordinate hits an active overlay.
// Returns the overlay panel name and true if hit, empty string and false otherwise.
func (m *PanelManager) HitOverlay(x, y int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hitOverlayUnlocked(x, y)
}

// hitOverlayUnlocked is the lock-free implementation of HitOverlay.
// Caller must hold m.mu.
func (m *PanelManager) hitOverlayUnlocked(x, y int) (string, bool) {
	if !m.hasActiveOverlays() {
		return "", false
	}

	base := lipgloss.NewLayer("").Z(0)
	var layers []*lipgloss.Layer
	layers = append(layers, base)

	for i := range m.overlays {
		oe := &m.overlays[i]
		if !oe.info.Active {
			continue
		}
		panelW := oe.info.Width
		if panelW == 0 {
			panelW = 30
		}
		panelH := 10
		placeholder := lipgloss.NewStyle().Width(panelW).Height(panelH).Render("")
		layer := lipgloss.NewLayer(placeholder).
			X(oe.x).
			Y(oe.y).
			Z(oe.z).
			ID(oe.info.Config.Name)
		layers = append(layers, layer)
	}

	comp := lipgloss.NewCompositor(layers...)
	hit := comp.Hit(x, y)
	if hit.Empty() {
		return "", false
	}
	return hit.ID(), true
}

// LeftPanelWidth returns the effective width of the left panel slot.
func (m *PanelManager) LeftPanelWidth() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.left.collapsed || len(m.left.panels) == 0 {
		return 0
	}
	return m.left.width
}

// RightPanelWidth returns the effective width of the right panel slot.
func (m *PanelManager) RightPanelWidth() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.right.collapsed || len(m.right.panels) == 0 {
		return 0
	}
	return m.right.width
}

// ─── Layout persistence ───────────────────────────────────────────────────────

// PanelState holds the persistent state for one panel.
type PanelState struct {
	Active    bool `yaml:"active"`
	Width     int  `yaml:"width"`
	Collapsed bool `yaml:"collapsed"`
	ActiveTab int  `yaml:"active_tab"`
}

// PanelLayoutConfig is the top-level config for tui.panels in config.yaml.
type PanelLayoutConfig struct {
	Focus  string                `yaml:"focus"`
	Panels map[string]PanelState `yaml:"panels"`
}

// RestoreLayout applies saved panel state.
func (m *PanelManager) RestoreLayout(saved PanelLayoutConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if saved.Focus != "" {
		m.focus = saved.Focus
	}
	// Track which slots have already had their slot-level fields set
	// to avoid nondeterministic last-writer-wins from map iteration.
	slotRestored := make(map[core.PanelPosition]bool)
	for name, ps := range saved.Panels {
		ref, ok := m.registry[name]
		if !ok {
			continue
		}
		s := m.slotForPosition(ref.position)
		s.panels[ref.index].Active = ps.Active
		if !slotRestored[ref.position] {
			if ps.Width > 0 {
				s.width = ps.Width
			}
			s.activeTab = ps.ActiveTab
			s.collapsed = ps.Collapsed
			slotRestored[ref.position] = true
		}
	}
}

// SaveLayout returns the current layout state for persistence.
func (m *PanelManager) SaveLayout() PanelLayoutConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := PanelLayoutConfig{
		Focus:  m.focus,
		Panels: make(map[string]PanelState),
	}
	for _, s := range []*slot{&m.left, &m.right, &m.bottom} {
		for _, info := range s.panels {
			out.Panels[info.Config.Name] = PanelState{
				Active:    info.Active,
				Width:     s.width,
				Collapsed: s.collapsed,
				ActiveTab: s.activeTab,
			}
		}
	}
	return out
}

// SaveLayoutToConfig persists panel layout to ~/.siply/config.yaml atomically.
func (m *PanelManager) SaveLayoutToConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("panel layout save: %w", err)
	}
	dir := filepath.Join(home, ".siply")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("panel layout save: mkdir: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")

	var raw map[string]any
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &raw); err != nil {
			slog.Warn("panel layout load: corrupt config, starting fresh", "path", path, "error", err)
		}
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	tuiSection, _ := raw["tui"].(map[string]any)
	if tuiSection == nil {
		tuiSection = make(map[string]any)
	}

	layout := m.SaveLayout()
	layoutBytes, err := yaml.Marshal(layout)
	if err != nil {
		return fmt.Errorf("panel layout save: marshal layout: %w", err)
	}
	var layoutMap map[string]any
	if err := yaml.Unmarshal(layoutBytes, &layoutMap); err != nil {
		return fmt.Errorf("panel layout save: re-parse layout: %w", err)
	}
	tuiSection["panels"] = layoutMap
	raw["tui"] = tuiSection

	out, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("panel layout save: marshal config: %w", err)
	}
	return fileutil.AtomicWriteFile(path, out, 0o600)
}

// LoadLayoutFromConfig reads saved panel layout from ~/.siply/config.yaml.
func LoadLayoutFromConfig() (PanelLayoutConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return PanelLayoutConfig{}, err
	}
	path := filepath.Join(home, ".siply", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return PanelLayoutConfig{}, err
	}

	var raw struct {
		TUI struct {
			Panels PanelLayoutConfig `yaml:"panels"`
		} `yaml:"tui"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return PanelLayoutConfig{}, err
	}
	return raw.TUI.Panels, nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func (m *PanelManager) slotForPosition(pos core.PanelPosition) *slot {
	switch pos {
	case core.PanelLeft:
		return &m.left
	case core.PanelRight:
		return &m.right
	default:
		return &m.bottom
	}
}

func (m *PanelManager) cycleFocus(dir int) {
	order := []string{focusRepl, focusLeft, focusRight, focusBottom}
	idx := 0
	for i, f := range order {
		if f == m.focus {
			idx = i
			break
		}
	}
	n := len(order)
	// Skip positions with no registered panels.
	for attempt := 0; attempt < n; attempt++ {
		idx = ((idx+dir)%n + n) % n
		candidate := order[idx]
		if m.slotHasPanels(candidate) {
			m.focus = candidate
			return
		}
	}
	m.focus = focusRepl
}

func (m *PanelManager) slotHasPanels(name string) bool {
	switch name {
	case focusRepl:
		return true
	case focusLeft:
		return len(m.left.panels) > 0
	case focusRight:
		return len(m.right.panels) > 0
	case focusBottom:
		return len(m.bottom.panels) > 0
	default:
		return false
	}
}

func (m *PanelManager) switchTab(dir int) {
	var s *slot
	switch m.focus {
	case focusLeft:
		s = &m.left
	case focusRight:
		s = &m.right
	case focusBottom:
		s = &m.bottom
	default:
		return
	}
	if len(s.panels) == 0 {
		return
	}
	n := len(s.panels)
	s.activeTab = ((s.activeTab+dir)%n + n) % n
}

func (m *PanelManager) resizeFocusedPanel(delta int) {
	var s *slot
	switch m.focus {
	case focusLeft:
		s = &m.left
	case focusRight:
		s = &m.right
	default:
		return
	}
	s.width = clampSlotWidth(s, s.width+delta)
}

func (m *PanelManager) handlePanelKeybind(key string) {
	// Check overlay panels first — keybind toggles active state.
	for i := range m.overlays {
		oe := &m.overlays[i]
		if oe.info.Config.Keybind == key {
			oe.info.Active = !oe.info.Active
			return
		}
	}

	// Prefer matching the focused slot first, then fall back to all slots.
	focused := m.focusedSlot()
	if focused != nil {
		for _, info := range focused.panels {
			if info.Config.Keybind == key && info.Config.Collapsible {
				focused.collapsed = !focused.collapsed
				return
			}
		}
	}
	for _, s := range []*slot{&m.left, &m.right, &m.bottom} {
		if s == focused {
			continue
		}
		for _, info := range s.panels {
			if info.Config.Keybind == key && info.Config.Collapsible {
				s.collapsed = !s.collapsed
				return
			}
		}
	}
}

// routeScrollKey forwards scroll-related key messages to the focused panel's
// viewport, if one is attached. Returns nil if no viewport is handling the key.
// Caller must hold m.mu.
func (m *PanelManager) routeScrollKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.focusedSlot()
	if s == nil {
		return nil
	}
	info, ok := s.activePanel()
	if !ok {
		return nil
	}
	vp, hasVP := m.viewports[info.Config.Name]
	if !hasVP {
		return nil
	}
	return vp.Update(msg)
}

func (m *PanelManager) focusedSlot() *slot {
	switch m.focus {
	case focusLeft:
		return &m.left
	case focusRight:
		return &m.right
	case focusBottom:
		return &m.bottom
	default:
		return nil
	}
}

func (m *PanelManager) applyAutoCollapse(termWidth int) {
	leftW := 0
	if !m.left.collapsed {
		leftW = m.left.width
	}
	rightW := 0
	if !m.right.collapsed {
		rightW = m.right.width
	}
	available := termWidth - leftW - rightW
	if available < 40 {
		if !m.right.collapsed && len(m.right.panels) > 0 {
			m.right.collapsed = true
			return
		}
		if !m.left.collapsed && len(m.left.panels) > 0 {
			m.left.collapsed = true
			return
		}
		if !m.bottom.collapsed && len(m.bottom.panels) > 0 {
			m.bottom.collapsed = true
		}
	}
}

func (m *PanelManager) hasActiveBottom() bool {
	for _, p := range m.bottom.panels {
		if p.Active {
			return true
		}
	}
	return false
}

func (m *PanelManager) renderSlot(s *slot, width, height int, focused bool) string {
	if width <= 0 || len(s.panels) == 0 {
		return ""
	}
	if s.collapsed {
		return renderCollapsed(s, height)
	}

	var b strings.Builder

	tabBarHeight := 0
	if len(s.panels) > 1 {
		b.WriteString(renderTabBar(s, width))
		b.WriteByte('\n')
		tabBarHeight = 1
	}

	info, ok := s.activePanel()
	if !ok {
		return b.String()
	}

	contentHeight := height - tabBarHeight
	if contentHeight < 1 {
		contentHeight = 1
	}

	content := ""
	// Gebruik viewport als deze beschikbaar is voor dit panel.
	if vp, hasVP := m.viewports[info.Config.Name]; hasVP {
		if !vp.IsDirty() {
			if cached, ok := m.lastRender[info.Config.Name]; ok {
				return cached
			}
		}
		content = vp.View()
		vp.MarkClean()
	} else if info.Config.ContentFunc != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					content = fmt.Sprintf("[panel error: %v]", r)
					slog.Error("panel ContentFunc panic", "panel", info.Config.Name, "error", r)
				}
			}()
			content = info.Config.ContentFunc()
		}()
	} else {
		icon := info.Config.Icon
		if icon == "" {
			icon = "◇"
		}
		content = icon + " " + info.Config.Name
	}

	// Select border character based on focused state.
	borderChar := "│"
	if focused {
		borderStyle := m.theme.Primary.Resolve(m.renderConfig.Color)
		borderChar = borderStyle.Render("│")
	}

	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}

	lines := strings.Split(content, "\n")
	for i := 0; i < contentHeight; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		lw := ansi.StringWidth(line)
		pad := max(innerW-lw, 0)
		b.WriteString(borderChar + ansi.Truncate(line, innerW, "") + strings.Repeat(" ", pad) + borderChar)
		if i < contentHeight-1 {
			b.WriteByte('\n')
		}
	}

	result := b.String()
	// Cache het resultaat voor dirty-flag optimalisatie.
	m.lastRender[info.Config.Name] = result
	return result
}

func renderCollapsed(s *slot, height int) string {
	info, ok := s.activePanel()
	if !ok {
		return ""
	}
	icon := info.Config.Icon
	if icon == "" {
		icon = "▶"
	}
	var b strings.Builder
	for i := 0; i < height; i++ {
		b.WriteString("│" + icon + "│")
		if i < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func renderTabBar(s *slot, width int) string {
	var b strings.Builder
	for i, info := range s.panels {
		label := info.Config.MenuLabel
		if label == "" {
			label = info.Config.Name
		}
		if i == s.activeTab {
			b.WriteString("[" + label + "]")
		} else {
			b.WriteString(" " + label + " ")
		}
	}
	result := b.String()
	if ansi.StringWidth(result) > width {
		result = ansi.Truncate(result, width, "")
	}
	return result
}

// slotAtX determines which panel slot a screen X coordinate falls into.
// Returns focusLeft, focusRight, or focusRepl. Caller must hold m.mu.
func (m *PanelManager) slotAtX(x int) string {
	totalW := m.lastViewWidth
	if totalW == 0 {
		return focusRepl
	}

	if m.lastRenderedLeftW > 0 && x < m.lastRenderedLeftW {
		return focusLeft
	}
	if m.lastRenderedRightW > 0 && x >= totalW-m.lastRenderedRightW {
		return focusRight
	}
	return focusRepl
}

// routeScrollToSlot routes a mouse wheel message to the viewport of the
// panel at the given slot name. Caller must hold m.mu.
func (m *PanelManager) routeScrollToSlot(target string, msg tea.MouseWheelMsg) tea.Cmd {
	var s *slot
	switch target {
	case focusLeft:
		s = &m.left
	case focusRight:
		s = &m.right
	case focusBottom:
		s = &m.bottom
	default:
		return nil
	}
	info, ok := s.activePanel()
	if !ok {
		return nil
	}
	vp, hasVP := m.viewports[info.Config.Name]
	if !hasVP {
		return nil
	}
	return vp.Update(msg)
}

// sendKeyToFocusedPlugin forwards a key event to the focused panel's plugin.
// Caller must hold m.mu.
func (m *PanelManager) sendKeyToFocusedPlugin(key string) {
	if m.actionSender == nil {
		return
	}
	s := m.focusedSlot()
	if s == nil {
		return
	}
	info, ok := s.activePanel()
	if !ok || info.Config.PluginName == "" {
		return
	}
	pluginName := info.Config.PluginName
	sender := m.actionSender
	go sender(pluginName, "key", []byte(key))
}

// sendClickToPlugin forwards a click event to the panel's plugin with the row index.
// Caller must hold m.mu.
func (m *PanelManager) sendClickToPlugin(target string, absY int) {
	if m.actionSender == nil {
		return
	}
	var s *slot
	switch target {
	case focusLeft:
		s = &m.left
	case focusRight:
		s = &m.right
	case focusBottom:
		s = &m.bottom
	default:
		return
	}
	info, ok := s.activePanel()
	if !ok || info.Config.PluginName == "" {
		return
	}
	tabBarHeight := 0
	if len(s.panels) > 1 {
		tabBarHeight = 1
	}
	row := absY - tabBarHeight
	if row < 0 {
		row = 0
	}
	pluginName := info.Config.PluginName
	sender := m.actionSender
	go sender(pluginName, "click", []byte{byte(row)})
}

// padLinesToWidth pads each line of content to exactly width cells.
// Uses ansi.StringWidth for ANSI-safe width measurement.
func padLinesToWidth(content string, width int) string {
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		lw := ansi.StringWidth(line)
		if lw < width {
			b.WriteString(line)
			b.WriteString(strings.Repeat(" ", width-lw))
		} else {
			b.WriteString(ansi.Truncate(line, width, ""))
		}
	}
	return b.String()
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func clampSlotWidth(s *slot, w int) int {
	if len(s.panels) == 0 {
		return w
	}
	// Use the tightest constraints across all panels in the slot.
	minW := 0
	maxW := 0
	for _, info := range s.panels {
		if info.Config.MinWidth > minW {
			minW = info.Config.MinWidth
		}
		if info.Config.MaxWidth > 0 && (maxW == 0 || info.Config.MaxWidth < maxW) {
			maxW = info.Config.MaxWidth
		}
	}
	if w < minW {
		return minW
	}
	if maxW > 0 && w > maxW {
		return maxW
	}
	return w
}
