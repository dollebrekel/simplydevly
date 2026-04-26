// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// App is the root Bubble Tea Model for the siply TUI.
// It implements the Model-View-Update pattern.
type App struct {
	caps             Capabilities
	renderConfig     RenderConfig
	theme            Theme
	layout           LayoutConstraints
	replPanel        SubPanel
	panelManager     PanelManager
	activityFeed     ActivityFeedRenderer
	diffView         DiffViewRenderer
	markdownView     MarkdownRenderer
	menuOverlay      MenuOverlay
	marketBrowser    MarketplaceBrowser
	extensionManager ExtensionManager
	statusBar        StatusRenderer
	agent            AgentRunner
	width            int
	height           int
	ready            bool
}

// NewApp creates a new App with the given capabilities and CLI flags.
func NewApp(caps Capabilities, flags CLIFlags) *App {
	return &App{
		caps:         caps,
		renderConfig: NewRenderConfig(caps, flags),
		theme:        DefaultTheme(),
	}
}

// NewAppWithTheme creates a new App with an explicit theme.
func NewAppWithTheme(caps Capabilities, flags CLIFlags, theme Theme) *App {
	return &App{
		caps:         caps,
		renderConfig: NewRenderConfig(caps, flags),
		theme:        theme,
	}
}

// SetREPLPanel sets the REPL panel sub-model.
func (a *App) SetREPLPanel(p SubPanel) {
	a.replPanel = p
}

// SetActivityFeed sets the activity feed renderer.
func (a *App) SetActivityFeed(af ActivityFeedRenderer) {
	a.activityFeed = af
}

// SetDiffView sets the diff view renderer.
func (a *App) SetDiffView(dv DiffViewRenderer) {
	a.diffView = dv
}

// SetMarkdownView sets the markdown renderer.
func (a *App) SetMarkdownView(mv MarkdownRenderer) {
	a.markdownView = mv
}

// SetMenuOverlay sets the menu overlay component.
func (a *App) SetMenuOverlay(mo MenuOverlay) {
	a.menuOverlay = mo
}

// SetMarketplaceBrowser sets the marketplace browser component.
func (a *App) SetMarketplaceBrowser(mb MarketplaceBrowser) {
	a.marketBrowser = mb
}

// SetStatusBar sets the status bar renderer.
func (a *App) SetStatusBar(sb StatusRenderer) {
	a.statusBar = sb
}

// SetPanelManager wires the full panel manager.
// When set, App delegates layout rendering to the PanelManager.
func (a *App) SetPanelManager(pm PanelManager) {
	a.panelManager = pm
}

// SetExtensionManager wires the extension registration manager.
func (a *App) SetExtensionManager(em ExtensionManager) {
	a.extensionManager = em
}

// SetAgent wires the AI agent for handling user queries.
func (a *App) SetAgent(ar AgentRunner) {
	a.agent = ar
}

// Init returns initial commands. Window size is automatically provided by
// Bubble Tea v2 at program start via WindowSizeMsg.
func (a *App) Init() tea.Cmd {
	if a.replPanel != nil {
		return a.replPanel.Init()
	}
	return nil
}

// Update handles incoming messages and updates the model state.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		var panelCmd tea.Cmd
		if a.panelManager != nil {
			panelCmd = a.panelManager.Update(msg)
			leftW := a.panelManager.LeftPanelWidth()
			rightW := a.panelManager.RightPanelWidth()
			a.layout = CalculateLayoutWithPanels(a.width, a.height, leftW, rightW, 0)
		} else {
			a.layout = CalculateLayout(a.width, a.height)
		}
		a.ready = true
		// When PanelManager is active, propagate center width to sub-panels.
		// Without PanelManager, use the full terminal width (original behavior).
		replW := a.width
		if a.panelManager != nil {
			replW = a.layout.CenterWidth
			if replW == 0 {
				replW = a.width
			}
		}
		if a.replPanel != nil {
			a.replPanel.SetSize(replW, a.layout.MaxContentHeight)
		}
		if a.activityFeed != nil {
			a.activityFeed.SetSize(replW, a.feedHeight())
		}
		if a.diffView != nil {
			a.diffView.SetSize(replW, a.diffHeight())
		}
		if a.menuOverlay != nil {
			a.menuOverlay.SetSize(a.width, a.layout.MaxContentHeight)
		}
		if a.marketBrowser != nil {
			a.marketBrowser.SetSize(a.width, a.layout.MaxContentHeight)
		}
		if a.statusBar != nil {
			a.statusBar.SetSize(a.width, a.layout.CompactStatusBar)
		}
		return a, panelCmd

	case SubmitMsg:
		if a.replPanel != nil {
			a.replPanel.Update(AgentOutputMsg{Text: "> " + msg.Text + "\n"})
		}
		if a.agent == nil {
			if a.replPanel != nil {
				cmd := a.replPanel.Update(AgentOutputMsg{Text: "Error: no AI provider configured. Set SIPLY_PROVIDER or use --local flag.\n"})
				cmd2 := a.replPanel.Update(AgentDoneMsg{})
				return a, tea.Batch(cmd, cmd2)
			}
			return a, nil
		}
		text := msg.Text
		return a, func() tea.Msg {
			err := a.agent.Run(context.Background(), text)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return AgentDoneMsg{}
				}
				return AgentErrorMsg{Err: err}
			}
			return AgentDoneMsg{}
		}

	case CancelMsg:
		if a.agent != nil {
			_ = a.agent.Stop(context.Background())
		}
		if a.replPanel != nil {
			cmd := a.replPanel.Update(AgentDoneMsg{})
			return a, cmd
		}
		return a, nil

	case AgentErrorMsg:
		if a.replPanel != nil {
			cmd := a.replPanel.Update(AgentOutputMsg{Text: "\nError: " + msg.Err.Error() + "\n"})
			cmd2 := a.replPanel.Update(AgentDoneMsg{})
			return a, tea.Batch(cmd, cmd2)
		}
		return a, nil

	case AgentOutputMsg, AgentDoneMsg:
		if a.replPanel != nil {
			cmd := a.replPanel.Update(msg)
			return a, cmd
		}
		return a, nil

	case MarketplaceOpenMsg:
		if a.marketBrowser != nil {
			a.marketBrowser.Open()
			a.marketBrowser.SetSize(a.width, a.layout.MaxContentHeight)
			return a, a.marketBrowser.Init()
		}
		return a, nil

	case MarketplaceCloseMsg:
		if a.marketBrowser != nil {
			a.marketBrowser.Close()
		}
		return a, nil

	case MarketplaceInstallResultMsg:
		if a.marketBrowser != nil {
			cmd := a.marketBrowser.Update(msg)
			return a, cmd
		}
		return a, nil

	case MenuItemSelectedMsg:
		if a.menuOverlay != nil {
			a.menuOverlay.Close()
		}
		if msg.Label == "Marketplace" {
			return a, func() tea.Msg { return MarketplaceOpenMsg{} }
		}
		return a, nil

	case PluginLoadedMsg:
		slog.Info("tui: plugin loaded, refreshing panels", "plugin", msg.Name)
		if a.panelManager != nil {
			cmd := a.panelManager.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
			return a, cmd
		}
		return a, nil

	case PanelActivatedMsg:
		return a, nil

	case MenuChangedMsg:
		return a, nil

	case KeybindChangedMsg:
		return a, nil

	case DiffViewMsg:
		if a.diffView != nil {
			a.diffView.LoadDiff(msg.FilePath, msg.OldContent, msg.NewContent)
		}
		return a, nil

	case DiffAcceptedMsg:
		// Stub: log action. Future stories will apply the edit.
		return a, nil

	case DiffRejectedMsg:
		// Stub: log action. Future stories will discard the edit.
		return a, nil

	case FeedEntryMsg:
		if a.activityFeed != nil {
			a.activityFeed.HandleFeedEntry(msg)
		}
		return a, nil

	case FeedStateMsg:
		if a.activityFeed != nil {
			a.activityFeed.HandleFeedState(msg)
		}
		return a, nil

	case FeedbackMsg:
		if a.activityFeed != nil {
			a.activityFeed.HandleFeedback(msg)
		}
		return a, nil

	case ProgressStartMsg:
		// Progress indicator lifecycle: start. Managed by activity feed
		// via feedback messages. Full spinner integration deferred to
		// when ActivityFeed becomes a tea.Model.
		if a.activityFeed != nil {
			a.activityFeed.HandleFeedback(FeedbackMsg{
				Level:   LevelInfo,
				Summary: msg.Label,
			})
		}
		return a, nil

	case ProgressDoneMsg:
		// Progress indicator lifecycle: complete.
		if a.activityFeed != nil {
			summary := msg.Label
			if msg.Result != "" {
				summary += ": " + msg.Result
			}
			a.activityFeed.HandleFeedback(FeedbackMsg{
				Level:   LevelSuccess,
				Summary: summary,
			})
		}
		return a, nil

	case tea.MouseClickMsg:
		// Route click events to menu when open.
		if a.menuOverlay != nil && a.menuOverlay.IsOpen() {
			cmd := a.menuOverlay.HandleMouse(msg)
			return a, cmd
		}
		// Route click events to PanelManager for focus and divider drag.
		if a.panelManager != nil {
			cmd := a.panelManager.Update(msg)
			if cmd != nil {
				return a, cmd
			}
		}
		// Route click events to REPL panel (for slash overlay clicks).
		if a.replPanel != nil {
			cmd := a.replPanel.Update(msg)
			return a, cmd
		}

	case tea.MouseMotionMsg:
		if a.panelManager != nil {
			return a, a.panelManager.Update(msg)
		}

	case tea.MouseReleaseMsg:
		if a.panelManager != nil {
			return a, a.panelManager.Update(msg)
		}

	case tea.MouseWheelMsg:
		if a.panelManager != nil {
			return a, a.panelManager.Update(msg)
		}

	case tea.MouseMsg:
		// Route non-click mouse events to menu when open.
		if a.menuOverlay != nil && a.menuOverlay.IsOpen() {
			cmd := a.menuOverlay.HandleMouse(msg)
			return a, cmd
		}

	case tea.KeyPressMsg:
		key := msg.String()

		// Ctrl+C always quits, even when menu is open.
		if key == "ctrl+c" {
			return a, tea.Quit
		}

		// Ctrl+Space toggles menu (always, even when menu is open).
		if key == "ctrl+@" || key == "ctrl+space" {
			if a.menuOverlay != nil {
				a.menuOverlay.Toggle()
			}
			return a, nil
		}

		// When marketplace browser is open, route ALL keys to it.
		if a.marketBrowser != nil && a.marketBrowser.IsOpen() {
			cmd := a.marketBrowser.Update(msg)
			return a, cmd
		}

		// Ctrl+B toggles borders.
		if key == "ctrl+b" {
			if a.renderConfig.Borders == BorderNone {
				if a.caps.Unicode {
					a.renderConfig.Borders = BorderUnicode
				} else {
					a.renderConfig.Borders = BorderASCII
				}
			} else {
				a.renderConfig.Borders = BorderNone
			}
			a.layout.ShowBorders = a.renderConfig.Borders != BorderNone
			if a.replPanel != nil {
				a.replPanel.SetBordered(a.layout.ShowBorders)
			}
			return a, nil
		}

		// When menu is open, route ALL keys to menu.
		if a.menuOverlay != nil && a.menuOverlay.IsOpen() {
			result := a.menuOverlay.HandleKey(key)
			if result != nil {
				return a, func() tea.Msg { return result }
			}
			return a, nil
		}

		// Route diff-related keys to DiffView only when a diff is loaded.
		if a.diffView != nil && a.diffView.IsActive() {
			switch key {
			case "tab", "esc", "e", "up", "down", "k", "j":
				result := a.diffView.HandleKey(key)
				if result != nil {
					// Route via tea.Cmd to avoid recursive Update calls.
					return a, func() tea.Msg { return result }
				}
				return a, nil
			}
		}

		// Route panel navigation keys to PanelManager.
		if a.panelManager != nil {
			switch key {
			case "tab", "shift+tab", "alt+left", "alt+right", "ctrl+]", "ctrl+[":
				cmd := a.panelManager.Update(msg)
				return a, cmd
			default:
				cmd := a.panelManager.Update(msg)
				if cmd != nil {
					return a, cmd
				}
			}
		}

		// Route extension keybindings (lower priority than built-in).
		if a.extensionManager != nil {
			for _, kb := range a.extensionManager.AllKeybindings() {
				if kb.Key == key && kb.Handler != nil {
					handler := kb.Handler
					kbKey := kb.Key
					kbPlugin := kb.PluginName
					cmd := func() tea.Msg {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						_ = ctx
						defer func() {
							if r := recover(); r != nil {
								slog.Error("extension keybind handler panicked", "key", kbKey, "plugin", kbPlugin, "panic", r)
							}
						}()
						if err := handler(); err != nil {
							slog.Warn("extension keybind handler error", "key", kbKey, "plugin", kbPlugin, "error", err)
						}
						return nil
					}
					return a, cmd
				}
			}
		}

		// Route to REPL panel for key handling.
		if a.replPanel != nil {
			cmd := a.replPanel.Update(msg)
			return a, cmd
		}
		// No REPL panel: legacy key handling.
		switch key {
		case "ctrl+c", "q":
			return a, tea.Quit
		}
	}

	// Save panel layout on clean quit.
	if _, ok := msg.(tea.QuitMsg); ok {
		if a.panelManager != nil {
			if pm, ok := a.panelManager.(interface{ SaveLayoutToConfig() error }); ok {
				if err := pm.SaveLayoutToConfig(); err != nil {
					_ = err // best-effort persistence
				}
			}
		}
	}

	return a, nil
}

// View renders the TUI, adapting to the current layout mode.
func (a *App) View() tea.View {
	if !a.ready {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	var content string
	switch a.renderConfig.Verbosity {
	case VerbosityAccessible:
		content = a.renderAccessible()
	default:
		content = a.renderStandard()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	// Enable mouse when an interactive overlay is active (slash commands or menu).
	// Keeps text selection working on the main screen.
	menuOpen := a.menuOverlay != nil && a.menuOverlay.IsOpen()
	slashOpen := false
	if oc, ok := a.replPanel.(interface{ IsOverlayActive() bool }); ok {
		slashOpen = oc.IsOverlayActive()
	}
	if menuOpen || slashOpen || a.panelManager != nil {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// renderStandard renders the standard TUI view.
func (a *App) renderStandard() string {
	// Menu overlay renders on top of everything (including marketplace).
	if a.menuOverlay != nil && a.menuOverlay.IsOpen() {
		var b strings.Builder
		contentHeight := a.layout.MaxContentHeight
		b.WriteString(a.menuOverlay.Render(a.width, contentHeight))
		if a.layout.ShowStatusBar && a.statusBar != nil {
			b.WriteByte('\n')
			b.WriteString(a.statusBar.Render(a.width))
			b.WriteByte('\n')
		}
		return b.String()
	}

	// Marketplace browser replaces main content area when open.
	if a.marketBrowser != nil && a.marketBrowser.IsOpen() {
		var b strings.Builder
		b.WriteString(a.marketBrowser.View())
		if a.layout.ShowStatusBar && a.statusBar != nil {
			b.WriteByte('\n')
			b.WriteString(a.statusBar.Render(a.width))
			b.WriteByte('\n')
		}
		return b.String()
	}

	if a.panelManager != nil && a.replPanel != nil {
		var b strings.Builder
		centerW := a.layout.CenterWidth
		if centerW == 0 {
			centerW = a.width
		}
		centerContent := a.buildCenterContent(centerW)
		composed := a.panelManager.View(a.width, a.layout.MaxContentHeight, centerContent)
		b.WriteString(composed)
		if a.layout.ShowStatusBar {
			if a.statusBar != nil {
				b.WriteByte('\n')
				b.WriteString(a.statusBar.Render(a.width))
			}
			b.WriteByte('\n')
		}
		return b.String()
	}

	if a.replPanel != nil {
		var b strings.Builder
		b.WriteString(a.replPanel.View())

		if a.activityFeed != nil {
			feedHeight := a.feedHeight()
			if feedHeight > 0 {
				rendered := a.activityFeed.Render(a.width, feedHeight)
				if rendered != "" {
					b.WriteByte('\n')
					b.WriteString(rendered)
				}
			}
		}

		if a.diffView != nil {
			diffH := a.diffHeight()
			if diffH > 0 {
				rendered := a.diffView.Render(a.width, diffH)
				if rendered != "" {
					b.WriteByte('\n')
					b.WriteString(rendered)
				}
			}
		}

		if a.layout.ShowStatusBar {
			if a.statusBar != nil {
				b.WriteString(a.statusBar.Render(a.width))
			} else {
				// Fallback placeholder when no StatusBar is wired.
				mutedStyle := a.theme.Muted.Resolve(a.renderConfig.Color)
				statusText := "Ctrl+C to quit"
				if a.layout.CompactStatusBar {
					b.WriteString(mutedStyle.Render(statusText))
				} else {
					b.WriteString(mutedStyle.Render(statusText + " | siply " + a.layout.Mode.String()))
				}
			}
			b.WriteByte('\n')
		}

		return b.String()
	}

	// Legacy rendering (no REPL panel).
	var b strings.Builder

	cs := a.renderConfig.Color
	headingStyle := a.theme.Heading.Resolve(cs)
	mutedStyle := a.theme.Muted.Resolve(cs)

	body := "Ready."
	if a.renderConfig.Emoji {
		body = "✨ Ready."
	}

	info := fmt.Sprintf("%s | %dx%d", a.layout.Mode, a.width, a.height)
	body += "\n" + mutedStyle.Render(info)

	if a.layout.ShowBorders && a.renderConfig.Borders != BorderNone {
		title := headingStyle.Render("siply")
		b.WriteString(RenderBorder(title, body, a.renderConfig, a.theme, a.width))
	} else {
		b.WriteString(headingStyle.Render("siply"))
		b.WriteByte('\n')
		b.WriteString(body)
		b.WriteByte('\n')
	}

	if a.layout.ShowStatusBar {
		if a.statusBar != nil {
			b.WriteString(a.statusBar.Render(a.width))
		} else {
			statusText := "Press q to quit"
			if a.layout.CompactStatusBar {
				b.WriteString(mutedStyle.Render(statusText))
			} else {
				b.WriteString(mutedStyle.Render(statusText + " | siply " + a.layout.Mode.String()))
			}
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// diffHeight returns the number of lines allocated to the diff view.
// Returns 0 when no diff is actively loaded, to avoid reserving layout space.
// Coordinates with feedHeight to avoid exceeding MaxContentHeight.
func (a *App) diffHeight() int {
	if a.diffView == nil || !a.diffView.IsActive() {
		return 0
	}
	// Reserve space for feed first. MaxContentHeight already excludes status bar.
	available := a.layout.MaxContentHeight - a.feedHeight()
	if available <= 0 {
		return 0
	}
	h := a.layout.MaxContentHeight / 3
	if h < 5 {
		h = 5
	}
	if h > 25 {
		h = 25
	}
	if h > available {
		h = available
	}
	return h
}

// buildCenterContent assembles the REPL + feed + diff content for the center panel.
func (a *App) buildCenterContent(width int) string {
	var b strings.Builder
	if a.replPanel != nil {
		b.WriteString(a.replPanel.View())
	}
	if a.activityFeed != nil {
		fh := a.feedHeight()
		if fh > 0 {
			rendered := a.activityFeed.Render(width, fh)
			if rendered != "" {
				b.WriteByte('\n')
				b.WriteString(rendered)
			}
		}
	}
	if a.diffView != nil {
		dh := a.diffHeight()
		if dh > 0 {
			rendered := a.diffView.Render(width, dh)
			if rendered != "" {
				b.WriteByte('\n')
				b.WriteString(rendered)
			}
		}
	}
	return b.String()
}

// feedHeight returns the number of lines allocated to the activity feed.
func (a *App) feedHeight() int {
	h := a.layout.MaxContentHeight / 3
	if h < 1 {
		h = 1
	}
	if h > 15 {
		h = 15
	}
	return h
}

// renderAccessible renders the accessible mode view.
// Box-drawing chars are replaced by text headers.
// Spinners are replaced by static messages.
func (a *App) renderAccessible() string {
	// Menu overlay renders on top of everything (including marketplace).
	if a.menuOverlay != nil && a.menuOverlay.IsOpen() {
		var b strings.Builder
		contentHeight := a.layout.MaxContentHeight
		b.WriteString(a.menuOverlay.Render(a.width, contentHeight))
		if a.layout.ShowStatusBar && a.statusBar != nil {
			b.WriteByte('\n')
			b.WriteString(a.statusBar.Render(a.width))
			b.WriteByte('\n')
		}
		return b.String()
	}

	// Marketplace browser replaces main content area when open.
	if a.marketBrowser != nil && a.marketBrowser.IsOpen() {
		var b strings.Builder
		b.WriteString(a.marketBrowser.View())
		if a.layout.ShowStatusBar && a.statusBar != nil {
			b.WriteByte('\n')
			b.WriteString(a.statusBar.Render(a.width))
			b.WriteByte('\n')
		}
		return b.String()
	}

	if a.replPanel != nil {
		var b strings.Builder
		b.WriteString(a.replPanel.View())

		if a.activityFeed != nil {
			feedHeight := a.feedHeight()
			if feedHeight > 0 {
				rendered := a.activityFeed.Render(a.width, feedHeight)
				if rendered != "" {
					b.WriteByte('\n')
					b.WriteString(rendered)
				}
			}
		}

		if a.diffView != nil {
			diffH := a.diffHeight()
			if diffH > 0 {
				rendered := a.diffView.Render(a.width, diffH)
				if rendered != "" {
					b.WriteByte('\n')
					b.WriteString(rendered)
				}
			}
		}

		if a.layout.ShowStatusBar {
			if a.statusBar != nil {
				b.WriteString(a.statusBar.Render(a.width))
			} else {
				b.WriteString("Ctrl+C to quit")
			}
			b.WriteByte('\n')
		}

		return b.String()
	}

	// Legacy rendering (no REPL panel).
	var b strings.Builder

	body := "Ready."
	info := fmt.Sprintf("%s | %dx%d", a.layout.Mode, a.width, a.height)
	body += "\n" + info

	b.WriteString(RenderBorder("siply", body, a.renderConfig, a.theme, a.width))

	if a.layout.ShowStatusBar {
		if a.statusBar != nil {
			b.WriteString(a.statusBar.Render(a.width))
		} else {
			b.WriteString("Ctrl+C to quit")
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// Run starts the Bubble Tea program. This blocks until the program exits.
// Callers should wire components via Set* methods on the returned App before
// calling RunApp, or use this convenience entry point which creates a bare App.
func Run(caps Capabilities, flags CLIFlags) error {
	app := NewApp(caps, flags)
	return RunApp(app, caps)
}

// RunApp starts the Bubble Tea program with a pre-configured App.
// Use this when components have been wired via Set* methods.
// Optional setup callbacks run after tea.Program creation but before Run(),
// allowing callers to wire EventBus-to-BubbleTea bridges via prog.Send().
func RunApp(app *App, caps Capabilities, setup ...func(prog *tea.Program)) error {
	var opts []tea.ProgramOption

	// SSH sessions use reduced FPS for lower bandwidth (v2 equivalent of
	// WithBatchedRenderer which was removed in Bubble Tea v2).
	if caps.SSHSession {
		opts = append(opts, tea.WithFPS(10))
	}

	p := tea.NewProgram(app, opts...)
	for _, fn := range setup {
		fn(p)
	}
	_, err := p.Run()
	return err
}
