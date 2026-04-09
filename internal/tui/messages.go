// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
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
}

// StatusRenderer is the interface for the status bar component.
// Implemented by statusline.StatusBar to avoid import cycles.
type StatusRenderer interface {
	Render(width int) string
	SetSize(width int, compact bool)
}

// FeedState represents the current state of the activity feed.
type FeedState int

const (
	FeedIdle      FeedState = iota
	FeedStreaming
	FeedComplete
	FeedCancelled
)

// ActivityFeedRenderer is the interface for the activity feed component.
// Implemented by components.ActivityFeed to avoid import cycles.
type ActivityFeedRenderer interface {
	Render(width, height int) string
	SetSize(width, height int)
	HandleFeedEntry(msg FeedEntryMsg)
	HandleFeedState(msg FeedStateMsg)
}

// DiffViewState represents the current state of the diff viewer.
type DiffViewState int

const (
	DiffViewing  DiffViewState = iota
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
