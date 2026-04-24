// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"siply.dev/siply/internal/core"
)

// SubmitMsg is sent when the user submits input via Enter.
type SubmitMsg struct {
	Text string
}

// CancelMsg is sent when the user presses Ctrl+C while the agent is running.
type CancelMsg struct{}

// AgentOutputMsg is sent when the agent produces output text.
type AgentOutputMsg struct {
	Text string
}

// AgentDoneMsg is sent when the agent finishes processing.
type AgentDoneMsg struct{}

// SubPanel is the interface for panel sub-models managed by App.
// Panels mutate via pointer receiver and return only tea.Cmd from Update.
type SubPanel interface {
	Init() tea.Cmd
	Update(msg tea.Msg) tea.Cmd
	View() string
	SetSize(width, height int)
	SetBordered(bordered bool)
}

// StatusRenderer is the interface for the status bar component.
// Implemented by statusline.StatusBar to avoid import cycles.
type StatusRenderer interface {
	Render(width int) string
	SetSize(width int, compact bool)
	SetProfile(profile string)
}

// FeedState represents the current state of the activity feed.
type FeedState int

const (
	FeedIdle FeedState = iota
	FeedStreaming
	FeedComplete
	FeedCanceled
)

// ActivityFeedRenderer is the interface for the activity feed component.
// Implemented by components.ActivityFeed to avoid import cycles.
type ActivityFeedRenderer interface {
	Render(width, height int) string
	SetSize(width, height int)
	HandleFeedEntry(msg FeedEntryMsg)
	HandleFeedState(msg FeedStateMsg)
	HandleFeedback(msg FeedbackMsg)
}

// DiffViewState represents the current state of the diff viewer.
type DiffViewState int

const (
	DiffViewing DiffViewState = iota
	DiffAccepted
	DiffRejected
	DiffEditing
)

// DiffViewRenderer is the interface for the diff view component.
// Implemented by components.DiffView to avoid import cycles.
type DiffViewRenderer interface {
	Render(width, height int) string
	SetSize(width, height int)
	HandleKey(key string) tea.Msg
	LoadDiff(filePath, oldContent, newContent string)
	IsActive() bool
}

// DiffViewMsg is sent when a diff should be displayed.
type DiffViewMsg struct {
	FilePath   string
	OldContent string
	NewContent string
}

// DiffAcceptedMsg is sent when the user accepts a diff.
type DiffAcceptedMsg struct {
	FilePath   string
	NewContent string
}

// DiffRejectedMsg is sent when the user rejects a diff.
type DiffRejectedMsg struct {
	FilePath string
}

// MarkdownRenderer is the interface for the markdown rendering component.
// Implemented by components.MarkdownView to avoid import cycles.
type MarkdownRenderer interface {
	Render(input string, width int) string
}

// MenuOverlay is the interface for the menu overlay component.
// Implemented by menu.Overlay to avoid import cycles.
type MenuOverlay interface {
	Render(width, height int) string
	IsOpen() bool
	Open()
	Close()
	Toggle()
	HandleKey(key string) tea.Msg
	HandleMouse(msg tea.Msg) tea.Cmd
	SetSize(width, height int)
}

// MenuItemSelectedMsg is sent when the user selects a menu item.
type MenuItemSelectedMsg struct {
	Label string
}

// LearnCloseMsg is sent when the user presses Esc in the Learn view,
// signaling a return to the menu overlay.
type LearnCloseMsg struct{}

// FeedbackLevel identifies the severity of a feedback message.
type FeedbackLevel int

const (
	LevelSuccess FeedbackLevel = iota
	LevelError
	LevelWarning
	LevelInfo
)

// FeedbackMsg represents a feedback message to display in the activity feed.
type FeedbackMsg struct {
	Level   FeedbackLevel
	Summary string
	Detail  string // "why" for errors, explanation for warnings
	Action  string // "fix" for errors, suggestion for empty states
}

// ProgressStartMsg is sent when a long-running operation begins.
type ProgressStartMsg struct {
	Label string
}

// ProgressDoneMsg is sent when a long-running operation completes.
type ProgressDoneMsg struct {
	Label  string
	Result string
}

// EmptyStateMsg represents an empty state with explanation and next action.
type EmptyStateMsg struct {
	Reason     string
	Suggestion string
}

// FeedbackRenderer is the interface for rendering feedback messages.
type FeedbackRenderer interface {
	RenderFeedback(msg FeedbackMsg) string
	RenderEmptyState(msg EmptyStateMsg) string
}

// MarketplaceBrowser is the interface for the marketplace browser component.
// Implemented by components.MarketBrowser to avoid import cycles.
type MarketplaceBrowser interface {
	Init() tea.Cmd
	Update(msg tea.Msg) tea.Cmd
	View() string
	SetSize(width, height int)
	IsOpen() bool
	Open()
	Close()
}

// MarketplaceOpenMsg is sent when the marketplace browser should open.
type MarketplaceOpenMsg struct{}

// MarketplaceCloseMsg is sent when the marketplace browser should close.
type MarketplaceCloseMsg struct{}

// MarketplaceInstallResultMsg is sent when a marketplace install completes.
type MarketplaceInstallResultMsg struct {
	Name    string
	Version string
	Err     error
}

// MarketplaceRateResultMsg is sent when a marketplace rating submission completes.
type MarketplaceRateResultMsg struct {
	Name  string
	Score int
	PRURL string
	Err   error
}

// MarketplaceReviewsResultMsg is sent when reviews are fetched asynchronously.
type MarketplaceReviewsResultMsg struct {
	ItemName string
	Content  string
	Err      error
}

// PanelManager is the interface for the full panel system.
// Defined here (not in tui/panels) to prevent import cycles.
type PanelManager interface {
	Update(msg tea.Msg) tea.Cmd
	View(width, height int, centerContent string) string
	LeftPanelWidth() int
	RightPanelWidth() int
}

// ExtensionManager is the interface for the extension registration system.
// Defined here (not in internal/extensions) to prevent import cycles.
// Uses core types directly since tui already imports core.
type ExtensionManager interface {
	AllMenuItems() []core.MenuItem
	AllKeybindings() []core.Keybinding
}

// PluginLoadedMsg is sent when a Tier 3 plugin is loaded and initialized.
type PluginLoadedMsg struct {
	Name    string
	Version string
	Tier    int
}

// PanelActivatedMsg is sent when a panel becomes active.
type PanelActivatedMsg struct {
	Name string
}

// MenuChangedMsg is sent when extension menu items change.
type MenuChangedMsg struct{}

// KeybindChangedMsg is sent when extension keybindings change.
type KeybindChangedMsg struct{}

// FeedEntryMsg is sent when a new activity entry should be displayed.
type FeedEntryMsg struct {
	Type     string
	Label    string
	Detail   string
	Duration time.Duration
	IsError  bool
}

// FeedStateMsg is sent when the activity feed state changes.
type FeedStateMsg struct {
	State FeedState
}
