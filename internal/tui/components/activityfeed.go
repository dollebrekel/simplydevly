// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/tui"
)

// EntryType identifies the kind of activity.
type EntryType int

const (
	EntryRead   EntryType = iota
	EntryEdit
	EntrySearch
	EntryBash
	EntryWeb
	EntryTool
)

// FeedState is an alias for tui.FeedState within the components package.
type FeedState = tui.FeedState

// Feed state constants re-exported from tui package for local use.
const (
	FeedIdle      = tui.FeedIdle
	FeedStreaming  = tui.FeedStreaming
	FeedComplete   = tui.FeedComplete
	FeedCancelled  = tui.FeedCancelled
)

// maxEntries is the maximum number of entries before oldest are dropped.
const maxEntries = 500

// FeedEntry represents a single activity entry.
type FeedEntry struct {
	Type      EntryType
	Label     string
	Detail    string
	Duration  time.Duration
	IsError   bool
	Timestamp time.Time
}

// ActivityFeed is a pure rendering component for the activity feed.
// It does NOT implement tea.Model — App.View() calls Render() directly.
type ActivityFeed struct {
	theme        tui.Theme
	renderConfig tui.RenderConfig
	entries      []FeedEntry
	state        FeedState
	scrollOffset int
	width        int
	height       int
}

// NewActivityFeed creates an ActivityFeed configured with the given theme and config.
func NewActivityFeed(theme tui.Theme, config tui.RenderConfig) *ActivityFeed {
	return &ActivityFeed{
		theme:        theme,
		renderConfig: config,
		width:        80,
		height:       10,
	}
}

// SetSize updates the feed dimensions. Width and height are clamped to minimum 1.
func (af *ActivityFeed) SetSize(width, height int) {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	af.width = width
	af.height = height
}

// AddEntry appends an entry and enforces the 500-entry cap.
func (af *ActivityFeed) AddEntry(entry FeedEntry) {
	if af.state == FeedIdle {
		af.state = FeedStreaming
	}
	// Check auto-scroll BEFORE appending (maxScrollOffset changes after append).
	wasAtBottom := af.isAtBottom()
	af.entries = append(af.entries, entry)
	if len(af.entries) > maxEntries {
		trimCount := len(af.entries) - maxEntries
		af.entries = af.entries[trimCount:]
		// Adjust scrollOffset so it still points to the same logical entry.
		af.scrollOffset -= trimCount
		if af.scrollOffset < 0 {
			af.scrollOffset = 0
		}
	}
	if wasAtBottom {
		af.scrollToBottom()
	}
}

// SetState transitions the feed state.
func (af *ActivityFeed) SetState(state FeedState) {
	af.state = state
}

// HandleScroll scrolls the feed. direction -1 = up, +1 = down.
func (af *ActivityFeed) HandleScroll(direction int) {
	af.scrollOffset += direction
	// Clamp.
	if af.scrollOffset < 0 {
		af.scrollOffset = 0
	}
	maxOffset := af.maxScrollOffset()
	if af.scrollOffset > maxOffset {
		af.scrollOffset = maxOffset
	}
}

// Render produces the activity feed string for the given dimensions.
// It updates the stored dimensions so scroll calculations stay consistent.
func (af *ActivityFeed) Render(width, height int) string {
	if width < 1 || height < 1 || len(af.entries) == 0 {
		return ""
	}

	// Sync stored dimensions so maxScrollOffset/isAtBottom stay consistent.
	af.width = width
	af.height = height

	cs := af.renderConfig.Color

	// Determine visible range.
	total := len(af.entries)
	start := af.scrollOffset
	end := start + height
	if end > total {
		end = total
	}
	if start >= total {
		start = max(total-height, 0)
		end = total
	}

	var b strings.Builder
	for i := start; i < end; i++ {
		line := af.renderEntry(af.entries[i], width, cs)
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// renderEntry formats a single entry line.
func (af *ActivityFeed) renderEntry(entry FeedEntry, width int, cs tui.ColorSetting) string {
	if af.renderConfig.Verbosity == tui.VerbosityAccessible {
		return af.renderEntryAccessible(entry, width)
	}

	typeLabel := af.entryTypeLabel(entry.Type)
	duration := formatDuration(entry.Duration)

	// typeLabel padded to 9 chars ("Searching" is longest).
	paddedLabel := fmt.Sprintf("%-9s", typeLabel)

	var detail string
	if entry.Detail != "" {
		detail = " " + entry.Detail
	}

	// Color the prefix+label based on entry type.
	style := af.entryToken(entry).Resolve(cs)

	// In emoji mode: "{emoji} {typeLabel}  {label}". In no-emoji: "{typeLabel}  {label}".
	var leftPart string
	if af.renderConfig.Emoji {
		leftPart = af.entryPrefix(entry.Type) + " " + paddedLabel + " "
	} else {
		leftPart = paddedLabel + " "
	}
	leftWidth := ansi.StringWidth(leftPart)
	durationWidth := ansi.StringWidth(duration)

	// Available space for file path + detail.
	pathSpace := width - leftWidth - durationWidth - 2 // 2 for spacing around duration
	if pathSpace < 1 {
		pathSpace = 1
	}

	labelWithDetail := entry.Label + detail
	if ansi.StringWidth(labelWithDetail) > pathSpace {
		labelWithDetail = ansi.Truncate(labelWithDetail, pathSpace, "...")
	}

	// Pad to right-align duration.
	labelWidth := ansi.StringWidth(labelWithDetail)
	padding := max(pathSpace-labelWidth, 0)

	line := style.Render(leftPart+labelWithDetail) + strings.Repeat(" ", padding) + "  " + duration

	// Final truncation to ensure we never exceed width.
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}

	return line
}

// renderEntryAccessible renders an entry in accessible mode: [TYPE] label detail  duration
func (af *ActivityFeed) renderEntryAccessible(entry FeedEntry, width int) string {
	tag := af.entryAccessibleTag(entry.Type)
	duration := formatDuration(entry.Duration)

	var detail string
	if entry.Detail != "" {
		detail = " " + entry.Detail
	}

	line := tag + " " + entry.Label + detail + "  " + duration
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return line
}

// entryPrefix returns the emoji or text prefix for an entry type.
func (af *ActivityFeed) entryPrefix(t EntryType) string {
	if af.renderConfig.Emoji {
		switch t {
		case EntryRead:
			return "\U0001F4D6" // open book emoji
		case EntryEdit:
			return "\u270F\uFE0F" // pencil emoji
		case EntrySearch:
			return "\U0001F50D" // magnifying glass
		case EntryBash:
			return "\u26A1" // lightning
		case EntryWeb:
			return "\U0001F310" // globe
		default:
			return "\U0001F527" // wrench
		}
	}
	return af.entryTypeLabel(t)
}

// entryTypeLabel returns the text label for an entry type.
func (af *ActivityFeed) entryTypeLabel(t EntryType) string {
	switch t {
	case EntryRead:
		return "Reading"
	case EntryEdit:
		return "Editing"
	case EntrySearch:
		return "Searching"
	case EntryBash:
		return "Bash"
	case EntryWeb:
		return "Web"
	default:
		return "Tool"
	}
}

// entryAccessibleTag returns the bracketed tag for accessible mode.
func (af *ActivityFeed) entryAccessibleTag(t EntryType) string {
	switch t {
	case EntryRead:
		return "[READ]"
	case EntryEdit:
		return "[EDIT]"
	case EntrySearch:
		return "[SEARCH]"
	case EntryBash:
		return "[BASH]"
	case EntryWeb:
		return "[WEB]"
	default:
		return "[TOOL]"
	}
}

// entryToken returns the style token for the entry type.
func (af *ActivityFeed) entryToken(entry FeedEntry) tui.Token {
	if entry.IsError {
		return af.theme.Error
	}
	switch entry.Type {
	case EntryRead:
		return af.theme.TextMuted
	case EntryEdit:
		return af.theme.Secondary
	case EntrySearch:
		return af.theme.Primary
	case EntryBash:
		return af.theme.Warning
	case EntryWeb:
		return af.theme.Link
	default:
		return af.theme.Text
	}
}

// isAtBottom returns true if the scroll position is at or near the bottom.
func (af *ActivityFeed) isAtBottom() bool {
	return af.scrollOffset >= af.maxScrollOffset()
}

// scrollToBottom sets the scroll offset to show the latest entries.
func (af *ActivityFeed) scrollToBottom() {
	af.scrollOffset = af.maxScrollOffset()
}

// maxScrollOffset returns the maximum valid scroll offset.
func (af *ActivityFeed) maxScrollOffset() int {
	maxOff := len(af.entries) - af.height
	if maxOff < 0 {
		return 0
	}
	return maxOff
}

// formatDuration formats a duration as "Xms" or "X.Xs".
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// HandleFeedEntry converts a FeedEntryMsg to a FeedEntry and adds it to the feed.
func (af *ActivityFeed) HandleFeedEntry(msg tui.FeedEntryMsg) {
	af.AddEntry(FeedEntry{
		Type:      ParseEntryType(msg.Type),
		Label:     msg.Label,
		Detail:    msg.Detail,
		Duration:  msg.Duration,
		IsError:   msg.IsError,
		Timestamp: time.Now(),
	})
}

// HandleFeedState transitions the feed state from a FeedStateMsg.
func (af *ActivityFeed) HandleFeedState(msg tui.FeedStateMsg) {
	af.SetState(msg.State)
}

// ParseEntryType converts a tool name string to an EntryType.
func ParseEntryType(toolName string) EntryType {
	switch toolName {
	case "file_read":
		return EntryRead
	case "file_edit", "file_write":
		return EntryEdit
	case "grep", "glob", "search":
		return EntrySearch
	case "bash":
		return EntryBash
	case "web_fetch", "web_search":
		return EntryWeb
	default:
		return EntryTool
	}
}
