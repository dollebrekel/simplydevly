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
	lipgloss "charm.land/lipgloss/v2"
)

// centerTab is one conversation session shown in the center window: its own
// REPL panel, its own agent, and an independent toolset. Side-panel plugins are
// shared across all tabs and never live here. The id is stable for the tab's
// lifetime and is used to route tab-tagged messages (AgentOutputMsg.TabID etc.)
// back to the correct tab even after other tabs are closed and indices shift.
type centerTab struct {
	id        int
	repl      SubPanel
	agent     AgentRunner
	agentBusy bool
	toolset   ToolsetChoice
	title     string
}

// App is the root Bubble Tea Model for the siply TUI.
// It implements the Model-View-Update pattern.
type App struct {
	caps             Capabilities
	renderConfig     RenderConfig
	theme            Theme
	layout           LayoutConstraints
	panelManager     PanelManager
	activityFeed     ActivityFeedRenderer
	diffView         DiffViewRenderer
	markdownView     MarkdownRenderer
	menuOverlay      MenuOverlay
	marketBrowser    MarketplaceBrowser
	modelPicker      ModelPicker
	modelController  ModelController
	extensionManager ExtensionManager
	kbRefresher      KeybindingRefresher
	statusBar        StatusRenderer

	// Center tabs (Story 11.14). tabs[0] is the original single-tab session;
	// when only one tab exists the behavior is identical to before this story.
	tabs       []*centerTab
	activeTab  int
	nextTabID  int
	tabFactory CenterTabFactory
	// tabBarOverride is nil for auto behavior (hidden at one tab, shown at
	// more than one). A non-nil value is an explicit user toggle (alt+t).
	tabBarOverride *bool
	// tabChooserOpen is true while the toolset chooser overlay is shown after
	// the user requests a new tab (alt+n / "+").
	tabChooserOpen bool
	// tabBarSegments records the clickable x-ranges (relative to the center
	// origin) of the most recently rendered tab bar, for mouse hit-testing.
	tabBarSegments []tabBarSegment

	modelSwitching bool
	width          int
	height         int
	ready          bool
}

// ensureTab0 lazily creates the first center tab so that SetREPLPanel and
// SetAgent (called in either order during wiring) populate the same tab.
func (a *App) ensureTab0() *centerTab {
	if len(a.tabs) == 0 {
		a.tabs = append(a.tabs, &centerTab{id: 0, toolset: ToolsetFull, title: "1"})
		a.nextTabID = 1
	}
	return a.tabs[0]
}

// active returns the currently focused center tab, or nil if none exists.
func (a *App) active() *centerTab {
	if a.activeTab < 0 || a.activeTab >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.activeTab]
}

// activeREPL returns the focused tab's REPL panel, or nil.
func (a *App) activeREPL() SubPanel {
	if t := a.active(); t != nil {
		return t.repl
	}
	return nil
}

// activeAgent returns the focused tab's agent, or nil.
func (a *App) activeAgent() AgentRunner {
	if t := a.active(); t != nil {
		return t.agent
	}
	return nil
}

// tabByID returns the tab with the given stable id, or nil. Used to route
// tab-tagged messages regardless of the tab's current slice index.
func (a *App) tabByID(id int) *centerTab {
	for _, t := range a.tabs {
		if t.id == id {
			return t
		}
	}
	return nil
}

// tabBarVisible reports whether the center tab bar should be rendered.
func (a *App) tabBarVisible() bool {
	if a.tabBarOverride != nil {
		return *a.tabBarOverride
	}
	return len(a.tabs) > 1
}

// tabBarHeight returns the number of terminal rows the tab bar occupies.
func (a *App) tabBarHeight() int {
	if a.tabBarVisible() {
		return 1
	}
	return 0
}

func (a *App) centerTabActionForKey(key string) (string, bool) {
	if a.kbRefresher != nil {
		if action, ok := a.kbRefresher.ActionForKey(key); ok {
			return strings.ToLower(strings.TrimSpace(action)), true
		}
	}
	switch key {
	case "alt+n":
		return "new tab (choose toolset)", true
	case "alt+w":
		return "close tab (last tab stays)", true
	case "ctrl+pgdown":
		return "next center tab", true
	case "ctrl+pgup":
		return "previous center tab", true
	case "alt+t":
		return "show/hide tab bar", true
	}
	return "", false
}

func (a *App) handleCenterTabAction(key string) (tea.Cmd, bool) {
	if strings.HasPrefix(key, "alt+") && len(key) == len("alt+1") && key[len(key)-1] >= '1' && key[len(key)-1] <= '9' {
		a.switchTabTo(int(key[len(key)-1] - '1'))
		return nil, true
	}
	action, ok := a.centerTabActionForKey(key)
	if !ok {
		return nil, false
	}
	switch action {
	case "center-tab.new", "new tab (choose toolset)", "open new center tab":
		if a.tabFactory != nil {
			a.tabChooserOpen = true
		}
		return nil, true
	case "center-tab.close", "close tab (last tab stays)", "close center tab":
		return a.closeActiveTab(), true
	case "center-tab.next", "next center tab", "previous / next tab", "switch center tab":
		a.switchTabBy(1)
		return nil, true
	case "center-tab.previous", "previous center tab":
		a.switchTabBy(-1)
		return nil, true
	case "center-tab.toggle-bar", "show/hide tab bar", "toggle center tab bar":
		a.toggleTabBar()
		return nil, true
	default:
		return nil, false
	}
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

// SetREPLPanel sets the REPL panel sub-model for the first center tab.
func (a *App) SetREPLPanel(p SubPanel) {
	a.ensureTab0().repl = p
}

// SetTabFactory wires the factory used to build additional center tabs.
// Without a factory, new-tab requests are ignored (single-tab mode).
func (a *App) SetTabFactory(f CenterTabFactory) {
	a.tabFactory = f
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

// SetModelPicker wires the /model picker component.
func (a *App) SetModelPicker(mp ModelPicker) {
	a.modelPicker = mp
}

// SetModelController wires provider/model discovery and switching.
func (a *App) SetModelController(mc ModelController) {
	a.modelController = mc
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

// SetKeybindingResolver wires the keybinding resolver for refresh on plugin changes.
func (a *App) SetKeybindingResolver(kr KeybindingRefresher) {
	a.kbRefresher = kr
}

// SetAgent wires the AI agent for the first center tab.
func (a *App) SetAgent(ar AgentRunner) {
	a.ensureTab0().agent = ar
}

// Init returns initial commands. Window size is automatically provided by
// Bubble Tea v2 at program start via WindowSizeMsg.
func (a *App) Init() tea.Cmd {
	if repl := a.activeREPL(); repl != nil {
		return repl.Init()
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
		a.resizeTabs()
		if a.menuOverlay != nil {
			a.menuOverlay.SetSize(a.width, a.layout.MaxContentHeight)
		}
		if a.marketBrowser != nil {
			a.marketBrowser.SetSize(a.width, a.layout.MaxContentHeight)
		}
		if a.modelPicker != nil {
			a.modelPicker.SetSize(a.width, a.layout.MaxContentHeight)
		}
		if a.statusBar != nil {
			a.statusBar.SetSize(a.width, a.layout.CompactStatusBar)
		}
		return a, panelCmd

	case SubmitMsg:
		tab := a.active()
		if tab == nil {
			return a, nil
		}
		var echoCmd tea.Cmd
		if tab.repl != nil {
			echoCmd = tab.repl.Update(UserEchoMsg(msg))
		}
		if tab.agent == nil {
			if tab.repl != nil {
				cmd := tab.repl.Update(AgentOutputMsg{Text: "Error: no AI provider configured. Set SIPLY_PROVIDER or use --local flag.\n", TabID: tab.id})
				cmd2 := tab.repl.Update(AgentDoneMsg{TabID: tab.id})
				return a, tea.Batch(echoCmd, cmd, cmd2)
			}
			return a, nil
		}
		text := msg.Text
		tab.agentBusy = true
		// Capture stable id and agent so the closure routes its result to the
		// originating tab even if the active tab changes while it runs.
		tabID := tab.id
		ag := tab.agent
		runCmd := func() tea.Msg {
			err := ag.Run(context.Background(), text)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return AgentDoneMsg{TabID: tabID}
				}
				return AgentErrorMsg{Err: err, TabID: tabID}
			}
			return AgentDoneMsg{TabID: tabID}
		}
		return a, tea.Batch(echoCmd, runCmd)

	case CancelMsg:
		ag := a.activeAgent()
		return a, func() tea.Msg {
			if ag != nil {
				_ = ag.Stop(context.Background())
			}
			// Don't synthesize AgentDoneMsg — the in-flight Run goroutine
			// will return AgentDoneMsg when it detects cancellation.
			return nil
		}

	case AgentErrorMsg:
		tab := a.tabByID(msg.TabID)
		if tab == nil {
			return a, nil
		}
		tab.agentBusy = false
		if tab.repl != nil {
			cmd := tab.repl.Update(AgentOutputMsg{Text: "\nError: " + msg.Err.Error() + "\n", TabID: tab.id})
			cmd2 := tab.repl.Update(AgentDoneMsg{TabID: tab.id})
			return a, tea.Batch(cmd, cmd2)
		}
		return a, nil

	case AgentOutputMsg:
		tab := a.tabByID(msg.TabID)
		if tab == nil || tab.repl == nil {
			return a, nil
		}
		return a, tab.repl.Update(msg)

	case AgentDoneMsg:
		tab := a.tabByID(msg.TabID)
		if tab == nil {
			return a, nil
		}
		tab.agentBusy = false
		if tab.repl != nil {
			return a, tab.repl.Update(msg)
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

	case ModelOpenMsg:
		if a.modelPicker == nil || a.modelController == nil {
			return a, nil
		}
		a.modelPicker.OpenLoading()
		a.modelPicker.SetSize(a.width, a.layout.MaxContentHeight)
		controller := a.modelController
		return a, func() tea.Msg {
			return controller.ListModels(context.Background())
		}

	case ModelListResultMsg:
		if a.modelPicker != nil {
			a.modelPicker.SetOptions(msg.Options, msg.Err)
		}
		return a, nil

	case ModelSelectedMsg:
		if a.modelPicker == nil || a.modelController == nil || a.modelSwitching {
			return a, nil
		}
		if t := a.active(); t != nil && t.agentBusy {
			return a, func() tea.Msg {
				return FeedbackMsg{
					Level:   LevelWarning,
					Summary: "Model switch waits for the current response to finish",
				}
			}
		}
		a.modelSwitching = true
		controller := a.modelController
		option := msg.Option
		if t := a.active(); t != nil && t.id != 0 {
			if switcher, ok := a.tabFactory.(CenterTabModelSwitcher); ok {
				tabID := t.id
				toolset := t.toolset
				repl := t.repl
				return a, func() tea.Msg {
					return switcher.SwitchTabModel(context.Background(), tabID, toolset, option, repl)
				}
			}
		}
		return a, func() tea.Msg {
			return controller.SwitchModel(context.Background(), option)
		}

	case ModelSwitchResultMsg:
		a.modelSwitching = false
		if msg.Err != nil {
			return a, func() tea.Msg {
				return FeedbackMsg{
					Level:   LevelError,
					Summary: "Model switch failed",
					Detail:  msg.Err.Error(),
				}
			}
		}
		if msg.Agent != nil {
			// A model switch replaces ONLY the active tab's agent; other tabs
			// keep their own agent/model (AC 8).
			if t := a.active(); t != nil {
				if closer, ok := t.agent.(AgentCloser); ok {
					_ = closer.Close(context.Background())
				}
				t.agent = msg.Agent
			}
		}
		if a.statusBar != nil {
			if msg.Option.Kind == "local" || msg.Option.Provider == "ollama" {
				a.statusBar.SetLocal(msg.Option.Model)
			} else {
				a.statusBar.SetCloudModel(msg.Option.Provider, msg.Option.Model)
			}
		}
		if a.modelPicker != nil {
			a.modelPicker.Close()
		}
		return a, func() tea.Msg {
			return FeedbackMsg{
				Level:   LevelSuccess,
				Summary: "Model switched to " + msg.Option.Model,
			}
		}

	case MenuItemSelectedMsg:
		if a.menuOverlay != nil {
			a.menuOverlay.Close()
		}
		if msg.Label == "Marketplace" {
			return a, func() tea.Msg { return MarketplaceOpenMsg{} }
		}
		if msg.Label == "Settings" {
			return a, func() tea.Msg { return ModelOpenMsg{} }
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
		if a.kbRefresher != nil && a.extensionManager != nil {
			a.kbRefresher.SetPlugins(a.extensionManager.AllKeybindings())
			a.kbRefresher.LogForceWarnings()
		}
		if mo, ok := a.menuOverlay.(interface{ RefreshLearnBindings() }); ok {
			mo.RefreshLearnBindings()
		}
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

	case LayoutLockMsg:
		if a.panelManager != nil {
			a.panelManager.SetLayoutLocked(msg.Locked)
		}
		if a.statusBar != nil {
			a.statusBar.SetLayoutLocked(msg.Locked)
		}
		return a, nil

	case FeedEntryMsg:
		// The shared activity feed logs every tab's tool activity; the inline
		// REPL tool display is routed only to the originating tab (AC 3).
		if a.activityFeed != nil {
			a.activityFeed.HandleFeedEntry(msg)
		}
		var replCmd tea.Cmd
		if tab := a.tabByID(msg.TabID); tab != nil && tab.repl != nil {
			replCmd = tab.repl.Update(msg)
		}
		return a, replCmd

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
		// The center is not a managed panel, so center clicks fall through.
		if a.panelManager != nil {
			cmd := a.panelManager.Update(msg)
			if cmd != nil {
				return a, cmd
			}
		}
		// Clicks on the center tab bar switch tabs or open the chooser (AC 4).
		if cmd, handled := a.handleTabBarClick(msg); handled {
			return a, cmd
		}
		// Route click events to REPL panel (for slash overlay clicks).
		if repl := a.activeREPL(); repl != nil {
			cmd := repl.Update(msg)
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
			if cmd := a.panelManager.Update(msg); cmd != nil {
				return a, cmd
			}
		}
		if repl := a.activeREPL(); repl != nil {
			return a, repl.Update(msg)
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

		// The toolset chooser (after alt+n / "+") captures all keys until a
		// choice is made or it is dismissed.
		if a.tabChooserOpen {
			switch key {
			case "1", "f":
				a.tabChooserOpen = false
				return a, a.createTab(ToolsetFull)
			case "2", "r":
				a.tabChooserOpen = false
				return a, a.createTab(ToolsetReadOnly)
			case "esc":
				a.tabChooserOpen = false
			}
			return a, nil
		}

		if a.modelPicker != nil && a.modelPicker.IsOpen() {
			result := a.modelPicker.HandleKey(key)
			if result != nil {
				return a, func() tea.Msg { return result }
			}
			return a, nil
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
			for _, t := range a.tabs {
				if t.repl != nil {
					t.repl.SetBordered(a.layout.ShowBorders)
				}
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

		// NOTE: The standalone DiffView is no longer rendered — diffs are shown
		// inline in the REPL chat (see renderUnifiedDiffBlock). It must therefore
		// NOT intercept keys: when a diff was "loaded" the invisible view used to
		// swallow tab/esc/arrows/j/k, freezing navigation. Those keys now flow to
		// the panel manager below.

		// Center-tab management keys (Story 11.14). Handled BEFORE panel
		// navigation so they never collide with the slot-tab / focus hotkeys.
		// The alt family is used to avoid clashing with the tree-panel plugin
		// (ctrl+t) and the input's delete-word (ctrl+w).
		if cmd, handled := a.handleCenterTabAction(key); handled {
			return a, cmd
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

		// Route to the active tab's REPL panel for key handling.
		if repl := a.activeREPL(); repl != nil {
			cmd := repl.Update(msg)
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
	modelOpen := a.modelPicker != nil && a.modelPicker.IsOpen()
	slashOpen := false
	if oc, ok := a.activeREPL().(interface{ IsOverlayActive() bool }); ok {
		slashOpen = oc.IsOverlayActive()
	}
	if menuOpen || modelOpen || slashOpen || a.panelManager != nil {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// renderStandard renders the standard TUI view.
func (a *App) renderStandard() string {
	if a.modelPicker != nil && a.modelPicker.IsOpen() {
		var b strings.Builder
		contentHeight := a.layout.MaxContentHeight
		b.WriteString(a.modelPicker.Render(a.width, contentHeight))
		if a.layout.ShowStatusBar && a.statusBar != nil {
			b.WriteByte('\n')
			b.WriteString(a.statusBar.Render(a.width))
			b.WriteByte('\n')
		}
		return b.String()
	}

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

	if a.panelManager != nil && a.activeREPL() != nil {
		var b strings.Builder
		centerContent := a.buildCenterContent()
		composed := a.panelManager.View(a.width, a.layout.MaxContentHeight, centerContent)
		b.WriteString(composed)
		if a.layout.ShowStatusBar {
			b.WriteByte('\n')
			if a.statusBar != nil {
				b.WriteString(a.statusBar.Render(a.width))
			} else {
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

	if a.activeREPL() != nil {
		var b strings.Builder
		b.WriteString(a.buildCenterContent())

		if a.layout.ShowStatusBar {
			if a.statusBar != nil {
				b.WriteString(a.statusBar.Render(a.width))
			} else {
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

// buildCenterContent returns the center panel content: an optional tab bar on
// top of the active tab's REPL view. With a single tab and no explicit toggle
// the tab bar is hidden, so the output is byte-for-byte identical to before
// Story 11.14 (AC 9).
func (a *App) buildCenterContent() string {
	repl := a.activeREPL()
	if repl == nil {
		return ""
	}
	body := repl.View()
	if a.tabChooserOpen {
		body = a.renderTabChooser()
	}
	if !a.tabBarVisible() {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, a.renderCenterTabBar(), body)
}

// renderAccessible renders the accessible mode view.
// Box-drawing chars are replaced by text headers.
// Spinners are replaced by static messages.
func (a *App) renderAccessible() string {
	if a.modelPicker != nil && a.modelPicker.IsOpen() {
		var b strings.Builder
		contentHeight := a.layout.MaxContentHeight
		b.WriteString(a.modelPicker.Render(a.width, contentHeight))
		if a.layout.ShowStatusBar && a.statusBar != nil {
			b.WriteByte('\n')
			b.WriteString(a.statusBar.Render(a.width))
			b.WriteByte('\n')
		}
		return b.String()
	}

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

	if a.activeREPL() != nil {
		var b strings.Builder
		b.WriteString(a.buildCenterContent())

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
