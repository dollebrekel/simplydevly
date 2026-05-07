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

var defaultTips = []string{
	"Type /help for available commands",
	"Use Ctrl+C to cancel the current agent",
	"Press Tab to autocomplete slash commands",
	"Use ↑/↓ to navigate command history",
}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

const evictionDelay = 2 * time.Second

type agentEntry struct {
	id          string
	name        string
	description string
	status      tui.AgentStatus
	toolUses    int
	tokenCount  int
	startTime   time.Time
	endTime     time.Time
	evictAfter  time.Time
}

func (e *agentEntry) isTerminal() bool {
	return e.status == tui.AgentDone || e.status == tui.AgentError
}

// AgentStatusPanel renders per-agent status when sub-agents are running.
type AgentStatusPanel struct {
	agents       []agentEntry
	width        int
	tipIndex     int
	frameIndex   int
	theme        tui.Theme
	renderConfig tui.RenderConfig
}

// NewAgentStatusPanel creates a new agent status display.
func NewAgentStatusPanel(theme tui.Theme, config tui.RenderConfig) *AgentStatusPanel {
	return &AgentStatusPanel{
		width:        80,
		theme:        theme,
		renderConfig: config,
	}
}

// HandleAgentStatus processes an agent status update message.
func (p *AgentStatusPanel) HandleAgentStatus(msg tui.AgentStatusUpdateMsg) {
	for i := range p.agents {
		if p.agents[i].id == msg.AgentID {
			p.agents[i].name = msg.Name
			p.agents[i].description = msg.Description
			p.agents[i].status = msg.Status
			p.agents[i].toolUses = msg.ToolUses
			p.agents[i].tokenCount = msg.TokenCount
			if p.agents[i].isTerminal() && p.agents[i].endTime.IsZero() {
				now := time.Now()
				p.agents[i].endTime = now
				p.agents[i].evictAfter = now.Add(evictionDelay)
			}
			return
		}
	}
	entry := agentEntry{
		id:          msg.AgentID,
		name:        msg.Name,
		description: msg.Description,
		status:      msg.Status,
		toolUses:    msg.ToolUses,
		tokenCount:  msg.TokenCount,
		startTime:   time.Now(),
	}
	if entry.isTerminal() {
		now := time.Now()
		entry.endTime = now
		entry.evictAfter = now.Add(evictionDelay)
	}
	p.agents = append(p.agents, entry)
}

// Tick advances the spinner frame, rotates tips, and evicts terminal agents.
func (p *AgentStatusPanel) Tick() {
	p.frameIndex = (p.frameIndex + 1) % len(spinnerFrames)

	if p.frameIndex%20 == 0 && len(defaultTips) > 0 {
		p.tipIndex = (p.tipIndex + 1) % len(defaultTips)
	}

	now := time.Now()
	alive := p.agents[:0]
	for _, a := range p.agents {
		if a.isTerminal() && !a.evictAfter.IsZero() && now.After(a.evictAfter) {
			continue
		}
		alive = append(alive, a)
	}
	p.agents = alive
}

// HasAgents returns true when there are agents being tracked.
func (p *AgentStatusPanel) HasAgents() bool {
	return len(p.agents) > 0
}

// SetSize updates the available width.
func (p *AgentStatusPanel) SetSize(width int) {
	if width < 1 {
		width = 1
	}
	p.width = width
}

// Render produces the agent status display string.
func (p *AgentStatusPanel) Render(width int) string {
	if width < 1 {
		return ""
	}
	if len(p.agents) == 0 {
		return ""
	}

	cs := p.renderConfig.Color
	accentStyle := p.theme.Accent.Resolve(cs)
	mutedStyle := p.theme.TextMuted.Resolve(cs)
	successStyle := p.theme.Success.Resolve(cs)
	errorStyle := p.theme.Error.Resolve(cs)

	var b strings.Builder

	runningCount := 0
	for _, a := range p.agents {
		if !a.isTerminal() {
			runningCount++
		}
	}

	var header string
	if runningCount > 0 {
		frame := spinnerFrames[p.frameIndex%len(spinnerFrames)]
		header = fmt.Sprintf("%c Running %d agent", frame, runningCount)
		if runningCount != 1 {
			header += "s"
		}
		header += "..."
		b.WriteString(accentStyle.Render(header))
	} else {
		b.WriteString(mutedStyle.Render("Agents completed"))
	}

	for i, a := range p.agents {
		b.WriteByte('\n')

		connector := "├─"
		if i == len(p.agents)-1 {
			connector = "└─"
		}

		suffix := p.renderSuffix(a)
		connectorPrefix := fmt.Sprintf("  %s ", connector)
		connectorWidth := ansi.StringWidth(connectorPrefix)
		suffixWidth := ansi.StringWidth(suffix)

		name := a.name
		nameWidth := ansi.StringWidth(name)
		maxNameWidth := width - connectorWidth - suffixWidth - 4
		if maxNameWidth > 0 && nameWidth > maxNameWidth {
			name = ansi.Truncate(name, maxNameWidth, "…")
			nameWidth = ansi.StringWidth(name)
		}

		descWidth := width - connectorWidth - nameWidth - 2 - suffixWidth - 1
		desc := a.description
		if descWidth > 0 && ansi.StringWidth(desc) > descWidth {
			desc = ansi.Truncate(desc, descWidth, "…")
		} else if descWidth <= 0 {
			desc = ""
		}

		b.WriteString(mutedStyle.Render(connectorPrefix))
		b.WriteString(accentStyle.Render(name))
		b.WriteString(mutedStyle.Render(": "))
		if desc != "" {
			b.WriteString(desc)
		}

		switch a.status {
		case tui.AgentDone:
			b.WriteString(successStyle.Render(suffix))
		case tui.AgentError:
			b.WriteString(errorStyle.Render(suffix))
		default:
			b.WriteString(mutedStyle.Render(suffix))
		}
	}

	if len(defaultTips) > 0 {
		b.WriteByte('\n')
		tip := fmt.Sprintf("💡 Tip: %s", defaultTips[p.tipIndex%len(defaultTips)])
		b.WriteString(mutedStyle.Render(tip))
	}

	return b.String()
}

func (p *AgentStatusPanel) renderSuffix(a agentEntry) string {
	switch a.status {
	case tui.AgentDone:
		elapsed := a.endTime.Sub(a.startTime).Truncate(time.Second)
		return fmt.Sprintf(" ✓ Done (%s)", elapsed)
	case tui.AgentError:
		elapsed := a.endTime.Sub(a.startTime).Truncate(time.Second)
		return fmt.Sprintf(" ✗ Error (%s)", elapsed)
	default:
		elapsed := time.Since(a.startTime).Truncate(time.Second)
		var parts []string
		parts = append(parts, fmt.Sprintf("▶ %s", elapsed))
		if a.toolUses > 0 {
			parts = append(parts, fmt.Sprintf("%d tool uses", a.toolUses))
		}
		if a.tokenCount > 0 {
			if a.tokenCount >= 1000 {
				parts = append(parts, fmt.Sprintf("↓ %.1fk tokens", float64(a.tokenCount)/1000))
			} else {
				parts = append(parts, fmt.Sprintf("↓ %d tokens", a.tokenCount))
			}
		}
		return " " + strings.Join(parts, " · ")
	}
}
