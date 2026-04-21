// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const toastBorderChar = "▌"

// FeedbackLevel identifies the severity of a feedback/toast message.
type FeedbackLevel int

const (
	LevelInfo FeedbackLevel = iota
	LevelSuccess
	LevelWarning
	LevelError
)

// Toast is a single temporary notification.
type Toast struct {
	Message   string
	Level     FeedbackLevel
	Duration  time.Duration
	CreatedAt time.Time
}

// isExpired returns true if the toast has outlived its duration.
func (t Toast) isExpired() bool {
	return time.Since(t.CreatedAt) >= t.Duration
}

// ToastManager manages a stack of toasts and handles rendering.
type ToastManager struct {
	toasts       []Toast
	maxVisible   int
	theme        Theme
	renderConfig RenderConfig
}

// NewToastManager creates a ToastManager with the given theme and render config.
func NewToastManager(theme Theme, config RenderConfig) *ToastManager {
	return &ToastManager{
		maxVisible:   3,
		theme:        theme,
		renderConfig: config,
	}
}

// Push adds a new toast.
func (tm *ToastManager) Push(msg string, level FeedbackLevel, duration time.Duration) {
	tm.toasts = append(tm.toasts, Toast{
		Message:   msg,
		Level:     level,
		Duration:  duration,
		CreatedAt: time.Now(),
	})
}

// Tick removes expired toasts. Returns true if any were removed (caller should re-render).
func (tm *ToastManager) Tick() bool {
	before := len(tm.toasts)
	kept := tm.toasts[:0]
	for _, t := range tm.toasts {
		if !t.isExpired() {
			kept = append(kept, t)
		}
	}
	tm.toasts = kept
	return len(tm.toasts) != before
}

// Render renders the visible toasts stacked bottom-to-top, right-aligned within width.
func (tm *ToastManager) Render(width int) string {
	if width < 1 {
		width = 1
	}
	if len(tm.toasts) == 0 {
		return ""
	}

	cs := tm.renderConfig.Color

	// Show only the last maxVisible toasts.
	start := len(tm.toasts) - tm.maxVisible
	if start < 0 {
		start = 0
	}
	visible := tm.toasts[start:]

	toastWidth := width / 2
	if toastWidth < 20 {
		toastWidth = 20
	}
	if toastWidth > width {
		toastWidth = width
	}
	padLeft := width - toastWidth
	if padLeft < 0 {
		padLeft = 0
	}
	prefix := strings.Repeat(" ", padLeft)

	var lines []string
	for _, t := range visible {
		line := tm.renderToast(t, toastWidth, cs)
		lines = append(lines, prefix+line)
	}

	return strings.Join(lines, "\n")
}

func (tm *ToastManager) renderToast(t Toast, width int, cs ColorSetting) string {
	noColor := cs == ColorNone

	var border, label string
	switch t.Level {
	case LevelSuccess:
		border = toastBorderChar
		label = "[OK] "
		if !noColor {
			border = tm.theme.Success.Resolve(cs).Render(border)
		}
	case LevelError:
		border = toastBorderChar
		label = "[ERR] "
		if !noColor {
			border = tm.theme.Error.Resolve(cs).Render(border)
		}
	case LevelWarning:
		border = toastBorderChar
		label = "[WARN] "
		if !noColor {
			border = tm.theme.Warning.Resolve(cs).Render(border)
		}
	default: // Info
		border = toastBorderChar
		label = "[INFO] "
		if !noColor {
			border = tm.theme.Primary.Resolve(cs).Render(border)
		}
	}

	innerWidth := width - 2 // 1 border + 1 space
	if innerWidth < 1 {
		innerWidth = 1
	}
	msg := t.Message
	if noColor {
		msg = label + msg
	}
	msg = ansi.Truncate(msg, innerWidth, "…")

	return border + " " + msg
}

// Count returns the number of active toasts.
func (tm *ToastManager) Count() int { return len(tm.toasts) }
