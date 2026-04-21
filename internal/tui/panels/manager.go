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

// PanelManager manages the three panel positions (left, right, bottom).
// It handles focus cycling, tab switching, collapse/expand, resize, and lazy loading.
type PanelManager struct {
	mu sync.Mutex

	left   slot
	right  slot
	bottom slot

	// name → slot ref.
	registry map[string]panelRef

	// Tracks which panels have been lazily initialized (by name).
	initialized map[string]bool

	// Focus: "repl", "left", "right", "bottom".
	focus string

	// Mouse drag state.
	dragging   bool
	dragTarget string
	dragStartX int

	theme        tui.Theme
	renderConfig tui.RenderConfig
}

// NewPanelManager creates a PanelManager with the given theme and render config.
func NewPanelManager(theme tui.Theme, rc tui.RenderConfig) *PanelManager {
	return &PanelManager{
		registry:     make(map[string]panelRef),
		initialized:  make(map[string]bool),
		focus:        focusRepl,
		theme:        theme,
		renderConfig: rc,
	}
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

	s := m.slotForPosition(ref.position)
	s.panels = append(s.panels[:ref.index], s.panels[ref.index+1:]...)

	// Rebuild registry indices for panels shifted by the removal.
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
	s := m.slotForPosition(ref.position)
	info := &s.panels[ref.index]

	if info.Active {
		m.mu.Unlock()
		return nil
	}

	info.Active = true
	s.collapsed = false
	s.activeTab = ref.index

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

	// Call callback outside lock to prevent deadlock if callback calls PanelManager methods.
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
	s := m.slotForPosition(ref.position)
	s.panels[ref.index].Active = false
	return nil
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
		default:
			m.handlePanelKeybind(msg.String())
		}

	case tea.MouseClickMsg:
		m.dragging = false
		// Header click detection would use a hit-map; deferred for now.

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
	}

	return nil
}

// View renders [left]|[center placeholder]|[right] + [bottom].
// The center content is filled in by App; this renders only the panel chrome.
func (m *PanelManager) View(width, height int) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	leftW := 0
	if !m.left.collapsed && len(m.left.panels) > 0 {
		leftW = m.left.width
	}
	rightW := 0
	if !m.right.collapsed && len(m.right.panels) > 0 {
		rightW = m.right.width
	}

	lc := tui.CalculateLayoutWithPanels(width, height, leftW, rightW, 0)

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

	leftStr := m.renderSlot(&m.left, lc.LeftPanelWidth, mainH)
	rightStr := m.renderSlot(&m.right, lc.RightPanelWidth, mainH)
	centerPlaceholder := strings.Repeat(" ", lc.CenterWidth)

	mainRow := joinH(leftStr, centerPlaceholder, rightStr, mainH)

	var b strings.Builder
	b.WriteString(mainRow)

	if bottomH > 0 {
		bottomStr := m.renderSlot(&m.bottom, width, bottomH)
		if bottomStr != "" {
			b.WriteByte('\n')
			b.WriteString(bottomStr)
		}
	}

	return b.String()
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
		_ = yaml.Unmarshal(data, &raw)
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

func (m *PanelManager) renderSlot(s *slot, width, height int) string {
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
	if info.Config.ContentFunc != nil {
		content = info.Config.ContentFunc()
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
		b.WriteString("│" + ansi.Truncate(line, innerW, "") + strings.Repeat(" ", pad) + "│")
		if i < contentHeight-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
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

// joinH places left, center line (repeated to height), and right side by side.
func joinH(left, centerLine, right string, height int) string {
	leftLines := linesOf(left, height)
	rightLines := linesOf(right, height)
	centerLines := make([]string, height)
	for i := range centerLines {
		centerLines[i] = centerLine
	}

	var b strings.Builder
	for i := 0; i < height; i++ {
		b.WriteString(leftLines[i])
		b.WriteString(centerLines[i])
		b.WriteString(rightLines[i])
		if i < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// linesOf splits s by newline and pads/truncates to exactly n lines.
func linesOf(s string, n int) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, n)
	for i := 0; i < n; i++ {
		if i < len(raw) {
			out[i] = raw[i]
		}
	}
	return out
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
