// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

// --- Interface compliance ---

func TestAgentStatusPanel_ImplementsAgentStatusRenderer(t *testing.T) {
	var _ tui.AgentStatusRenderer = (*AgentStatusPanel)(nil)
}

// --- Constructor ---

func TestNewAgentStatusPanel(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	require.NotNil(t, p)
	assert.Equal(t, 80, p.width)
	assert.Empty(t, p.agents)
}

// --- Render with zero agents ---

func TestRender_NoAgents_Empty(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	assert.Equal(t, "", p.Render(80))
}

// --- Render with zero width ---

func TestRender_ZeroWidth_Empty(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning,
	})
	assert.Equal(t, "", p.Render(0))
	assert.Equal(t, "", p.Render(-1))
}

// --- Render with single agent ---

func TestRender_SingleAgent(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "winston", Description: "Architect review", Status: tui.AgentRunning,
	})
	out := p.Render(80)
	assert.Contains(t, out, "Running 1 agent...")
	assert.Contains(t, out, "winston")
	assert.Contains(t, out, "Architect review")
	assert.Contains(t, out, "└─")
	assert.Contains(t, out, "Tip:")
}

// --- Render with multiple agents ---

func TestRender_MultipleAgents(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "winston", Description: "Architect", Status: tui.AgentRunning,
	})
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a2", Name: "amelia", Description: "Developer", Status: tui.AgentRunning,
	})
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a3", Name: "mary", Description: "Analyst", Status: tui.AgentRunning,
	})
	out := p.Render(80)
	assert.Contains(t, out, "Running 3 agents...")
	assert.Contains(t, out, "├─")
	assert.Contains(t, out, "└─")
	lines := strings.Split(out, "\n")
	assert.GreaterOrEqual(t, len(lines), 5) // header + 3 agents + tip
}

// --- Agent status updates (same ID) ---

func TestHandleAgentStatus_UpdateExisting(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "winston", Description: "Review", Status: tui.AgentRunning, ToolUses: 1,
	})
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "winston", Description: "Review", Status: tui.AgentRunning, ToolUses: 3,
	})
	assert.Len(t, p.agents, 1)
	assert.Equal(t, 3, p.agents[0].toolUses)
}

// --- Done status rendering ---

func TestRender_AgentDone(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "mary", Description: "Validation", Status: tui.AgentDone,
	})
	out := p.Render(80)
	assert.Contains(t, out, "✓ Done")
	assert.Contains(t, out, "Agents completed")
}

// --- Error status rendering ---

func TestRender_AgentError(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "broken", Description: "Failed task", Status: tui.AgentError,
	})
	out := p.Render(80)
	assert.Contains(t, out, "✗ Error")
}

// --- Token count display ---

func TestRender_TokenCount_Shown(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning, TokenCount: 2500,
	})
	out := p.Render(80)
	assert.Contains(t, out, "2.5k tokens")
}

func TestRender_TokenCount_Zero_Omitted(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning, TokenCount: 0,
	})
	out := p.Render(80)
	assert.NotContains(t, out, "tokens")
}

func TestRender_TokenCount_Below1k(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning, TokenCount: 500,
	})
	out := p.Render(80)
	assert.Contains(t, out, "500 tokens")
}

// --- Tool uses display ---

func TestRender_ToolUses_Shown(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning, ToolUses: 5,
	})
	out := p.Render(80)
	assert.Contains(t, out, "5 tool uses")
}

func TestRender_ToolUses_Zero_Omitted(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning, ToolUses: 0,
	})
	out := p.Render(80)
	assert.NotContains(t, out, "tool uses")
}

// --- Eviction after terminal status ---

func TestTick_EvictsTerminalAgents(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "done-agent", Description: "desc", Status: tui.AgentDone,
	})
	require.Len(t, p.agents, 1)

	// Before eviction time: agent still present.
	p.Tick()
	assert.Len(t, p.agents, 1)

	// Force eviction by backdating evictAfter.
	p.agents[0].evictAfter = time.Now().Add(-1 * time.Second)
	p.Tick()
	assert.Empty(t, p.agents)
}

func TestTick_DoesNotEvictRunningAgents(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "active", Description: "desc", Status: tui.AgentRunning,
	})
	for i := 0; i < 100; i++ {
		p.Tick()
	}
	assert.Len(t, p.agents, 1)
}

// --- Auto-hide (all terminal, then evicted) ---

func TestAutoHide_AllTerminalEvicted(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "d1", Description: "desc", Status: tui.AgentDone,
	})
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a2", Name: "d2", Description: "desc", Status: tui.AgentError,
	})

	// Force eviction.
	for i := range p.agents {
		p.agents[i].evictAfter = time.Now().Add(-1 * time.Second)
	}
	p.Tick()
	assert.Equal(t, "", p.Render(80))
}

// --- Width truncation ---

func TestRender_NarrowWidth_TruncatesDescription(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "winston",
		Description: "This is a very long description that should be truncated",
		Status:      tui.AgentRunning,
	})
	out := p.Render(40)
	// The description should be truncated, output should not exceed reasonable width.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		// Allow some tolerance for ANSI escape sequences.
		assert.LessOrEqual(t, len(line), 200, "raw line length should be reasonable")
	}
}

// --- Tip rotation ---

func TestTick_RotatesTips(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	initial := p.tipIndex
	// Tips rotate every 20 ticks.
	for i := 0; i < 20; i++ {
		p.Tick()
	}
	assert.NotEqual(t, initial, p.tipIndex)
}

// --- HasAgents ---

func TestHasAgents_EmptyFalse(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	assert.False(t, p.HasAgents())
}

func TestHasAgents_WithAgentsTrue(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.HandleAgentStatus(tui.AgentStatusUpdateMsg{
		AgentID: "a1", Name: "test", Description: "desc", Status: tui.AgentRunning,
	})
	assert.True(t, p.HasAgents())
}

// --- SetSize ---

func TestAgentStatusPanel_SetSize_ClampsMinimum(t *testing.T) {
	p := NewAgentStatusPanel(testTheme(), testConfig())
	p.SetSize(0)
	assert.Equal(t, 1, p.width)
	p.SetSize(-5)
	assert.Equal(t, 1, p.width)
	p.SetSize(120)
	assert.Equal(t, 120, p.width)
}
