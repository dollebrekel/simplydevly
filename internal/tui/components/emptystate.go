// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"siply.dev/siply/internal/tui"
)

// emptyStateEmoji is the emoji for empty states (empty mailbox).
const emptyStateEmoji = "\U0001F4ED"

// RenderEmptyState renders an empty state message with explanation and next action.
func RenderEmptyState(msg tui.EmptyStateMsg, theme *tui.Theme, rc *tui.RenderConfig, width int) string {
	if width < 1 {
		width = 80
	}

	if rc.Verbosity == tui.VerbosityAccessible {
		line := "[EMPTY] " + msg.Reason + " \u2014 " + msg.Suggestion
		return truncateLine(line, width)
	}

	cs := rc.Color

	var prefix string
	if rc.Emoji {
		prefix = emptyStateEmoji + " "
	} else {
		prefix = "-- "
	}

	line1 := theme.TextMuted.Resolve(cs).Render(prefix + msg.Reason)
	line2 := theme.Primary.Resolve(cs).Render("Try: " + msg.Suggestion)

	return truncateLine(line1, width) + "\n" + truncateLine(line2, width)
}
