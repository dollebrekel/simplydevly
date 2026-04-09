// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/tui"
)

// feedbackEmoji returns the emoji for a feedback level when emoji is enabled.
func feedbackEmoji(level tui.FeedbackLevel) string {
	switch level {
	case tui.LevelSuccess:
		return "\u2705" // checkmark
	case tui.LevelError:
		return "\u274C" // cross
	case tui.LevelWarning:
		return "\u26A0\uFE0F" // warning
	case tui.LevelInfo:
		return "\u2139\uFE0F" // info circle
	default:
		return ""
	}
}

// feedbackTextPrefix returns the text prefix for a feedback level when emoji is off.
func feedbackTextPrefix(level tui.FeedbackLevel) string {
	switch level {
	case tui.LevelSuccess:
		return "OK:"
	case tui.LevelError:
		return "ERROR:"
	case tui.LevelWarning:
		return "!!"
	case tui.LevelInfo:
		return "i:"
	default:
		return ""
	}
}

// feedbackAccessibleTag returns the bracketed tag for accessible mode.
func feedbackAccessibleTag(level tui.FeedbackLevel) string {
	switch level {
	case tui.LevelSuccess:
		return "[OK]"
	case tui.LevelError:
		return "[ERROR]"
	case tui.LevelWarning:
		return "[WARN]"
	case tui.LevelInfo:
		return "[INFO]"
	default:
		return ""
	}
}

// RenderFeedback renders a feedback message using theme tokens and render config.
// It adapts to color depth, emoji toggle, and accessible mode.
func RenderFeedback(msg tui.FeedbackMsg, theme *tui.Theme, rc *tui.RenderConfig, width int) string {
	if width < 1 {
		width = 80
	}

	if rc.Verbosity == tui.VerbosityAccessible {
		return renderFeedbackAccessible(msg, width)
	}

	cs := rc.Color

	// Build prefix: emoji or text label.
	var prefix string
	if rc.Emoji {
		prefix = feedbackEmoji(msg.Level) + " "
	} else {
		prefix = feedbackTextPrefix(msg.Level) + " "
	}

	switch msg.Level {
	case tui.LevelSuccess:
		line := theme.Success.Resolve(cs).Render(prefix + msg.Summary)
		return truncateLine(line, width)

	case tui.LevelError:
		detail := msg.Detail
		if detail == "" {
			detail = "Unknown cause"
		}
		action := msg.Action
		if action == "" {
			action = "Check logs for details"
		}

		var b strings.Builder
		b.WriteString(truncateLine(theme.Error.Resolve(cs).Render(prefix+msg.Summary), width))
		b.WriteByte('\n')
		b.WriteString(truncateLine(theme.TextMuted.Resolve(cs).Render("  Why: "+detail), width))
		b.WriteByte('\n')
		b.WriteString(truncateLine(theme.Text.Resolve(cs).Render("  Fix: "+action), width))
		return b.String()

	case tui.LevelWarning:
		var b strings.Builder
		b.WriteString(truncateLine(theme.Warning.Resolve(cs).Render(prefix+msg.Summary), width))
		if msg.Detail != "" {
			b.WriteByte('\n')
			b.WriteString(truncateLine(theme.TextMuted.Resolve(cs).Render(" [?] "+msg.Detail), width))
		}
		return b.String()

	case tui.LevelInfo:
		line := theme.TextMuted.Resolve(cs).Render(prefix + msg.Summary)
		return truncateLine(line, width)

	default:
		return msg.Summary
	}
}

// renderFeedbackAccessible renders feedback in accessible mode with bracketed tags.
func renderFeedbackAccessible(msg tui.FeedbackMsg, width int) string {
	tag := feedbackAccessibleTag(msg.Level)

	switch msg.Level {
	case tui.LevelSuccess:
		return truncateLine(tag+" "+msg.Summary, width)

	case tui.LevelError:
		detail := msg.Detail
		if detail == "" {
			detail = "Unknown cause"
		}
		action := msg.Action
		if action == "" {
			action = "Check logs for details"
		}
		return truncateLine(tag+" "+msg.Summary+" | "+detail+" | "+action, width)

	case tui.LevelWarning:
		return truncateLine(tag+" "+msg.Summary, width)

	case tui.LevelInfo:
		return truncateLine(tag+" "+msg.Summary, width)

	default:
		return msg.Summary
	}
}

// truncateLine truncates a line to the given width using ANSI-safe truncation.
func truncateLine(line string, width int) string {
	if ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, "...")
	}
	return line
}
