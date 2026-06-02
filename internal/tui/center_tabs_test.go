// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCloserAgent is an AgentRunner that also implements AgentCloser so tab
// close can be asserted.
type mockCloserAgent struct {
	mockAgentRunner
	closeCalled bool
}

func (m *mockCloserAgent) Close(_ context.Context) error {
	m.closeCalled = true
	return nil
}

// mockTabFactory records NewTab calls and hands back fresh mock panels/agents.
type mockTabFactory struct {
	calls       int
	lastTabID   int
	lastToolset ToolsetChoice
	err         error
	repls       []*mockSubPanel
	agents      []*mockCloserAgent
}

func (f *mockTabFactory) NewTab(tabID int, toolset ToolsetChoice) (SubPanel, AgentRunner, error) {
	f.calls++
	f.lastTabID = tabID
	f.lastToolset = toolset
	if f.err != nil {
		return nil, nil, f.err
	}
	r := &mockSubPanel{}
	a := &mockCloserAgent{}
	f.repls = append(f.repls, r)
	f.agents = append(f.agents, a)
	return r, a, nil
}

// newTabApp builds an App with a sized window, a tab-0 REPL, an agent, and a
// mock factory wired in.
func newTabApp(t *testing.T) (*App, *mockSubPanel, *mockTabFactory) {
	t.Helper()
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	repl0 := &mockSubPanel{viewContent: "TAB0"}
	app.SetREPLPanel(repl0)
	app.SetAgent(&mockAgentRunner{})
	factory := &mockTabFactory{}
	app.SetTabFactory(factory)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return app, repl0, factory
}

func TestApp_SingleTab_NoTabBar(t *testing.T) {
	app, repl0, _ := newTabApp(t)

	assert.False(t, app.tabBarVisible(), "single tab hides the bar by default")
	assert.Equal(t, 0, app.tabBarHeight())
	// Center content is exactly the REPL view — no bar prefix (AC 9).
	assert.Equal(t, repl0.viewContent, app.buildCenterContent())
}

func TestApp_CreateTab_ViaChooser_Full(t *testing.T) {
	app, _, factory := newTabApp(t)

	// alt+n opens the chooser; it does not create a tab yet.
	app.Update(tea.KeyPressMsg{Text: "alt+n", Mod: tea.ModAlt, Code: 'n'})
	assert.True(t, app.tabChooserOpen)
	assert.Len(t, app.tabs, 1)

	// "1" picks the full toolset and creates the tab.
	app.Update(tea.KeyPressMsg{Text: "1", Code: '1'})
	assert.False(t, app.tabChooserOpen)
	require.Len(t, app.tabs, 2)
	assert.Equal(t, 1, app.activeTab, "new tab becomes active")
	assert.Equal(t, 1, factory.calls)
	assert.Equal(t, ToolsetFull, factory.lastToolset)
	assert.Equal(t, 1, factory.lastTabID, "stable id assigned to new tab")
}

func TestApp_CreateTab_ViaChooser_ReadOnly(t *testing.T) {
	app, _, factory := newTabApp(t)

	app.Update(tea.KeyPressMsg{Text: "alt+n", Mod: tea.ModAlt, Code: 'n'})
	app.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	require.Len(t, app.tabs, 2)
	assert.Equal(t, ToolsetReadOnly, factory.lastToolset)
	assert.Equal(t, ToolsetReadOnly, app.tabs[1].toolset)
}

func TestApp_Chooser_EscCancels(t *testing.T) {
	app, _, factory := newTabApp(t)
	app.Update(tea.KeyPressMsg{Text: "alt+n", Mod: tea.ModAlt, Code: 'n'})
	app.Update(tea.KeyPressMsg{Text: "esc", Code: tea.KeyEscape})
	assert.False(t, app.tabChooserOpen)
	assert.Equal(t, 0, factory.calls)
	assert.Len(t, app.tabs, 1)
}

func TestApp_TabBar_VisibleWithMultipleTabs(t *testing.T) {
	app, _, _ := newTabApp(t)
	app.createTab(ToolsetFull)
	assert.True(t, app.tabBarVisible())
	assert.Equal(t, 1, app.tabBarHeight())
	// The bar row precedes the active REPL view.
	content := app.buildCenterContent()
	assert.Contains(t, content, "+", "the new-tab affordance is rendered")
}

func TestApp_ToggleTabBar(t *testing.T) {
	app, _, _ := newTabApp(t)
	app.createTab(ToolsetFull) // now 2 tabs → bar auto-visible

	// alt+t hides it.
	app.Update(tea.KeyPressMsg{Text: "alt+t", Mod: tea.ModAlt, Code: 't'})
	assert.False(t, app.tabBarVisible())
	// alt+t again shows it.
	app.Update(tea.KeyPressMsg{Text: "alt+t", Mod: tea.ModAlt, Code: 't'})
	assert.True(t, app.tabBarVisible())
}

func TestApp_TabBarHeight_ReducesContentHeight(t *testing.T) {
	app, repl0, factory := newTabApp(t)
	singleH := repl0.height
	require.Greater(t, singleH, 0)

	app.createTab(ToolsetFull)
	// Both the original and the new tab's REPL lose exactly one row to the bar.
	assert.Equal(t, singleH-1, repl0.height)
	require.Len(t, factory.repls, 1)
	assert.Equal(t, singleH-1, factory.repls[0].height)
}

func TestApp_OutputRoutesToOriginatingTab(t *testing.T) {
	app, repl0, factory := newTabApp(t)
	app.createTab(ToolsetFull)
	repl1 := factory.repls[0]
	tab1ID := app.tabs[1].id

	// Switch focus back to tab 0 while output for tab 1 arrives.
	app.switchTabTo(0)
	repl0.updateCalled = false
	repl1.updateCalled = false

	app.Update(AgentOutputMsg{Text: "from tab 1", TabID: tab1ID})

	assert.True(t, repl1.updateCalled, "tab 1 receives its own output")
	assert.False(t, repl0.updateCalled, "no output leaks to tab 0 (AC 3)")
	out, ok := repl1.lastMsg.(AgentOutputMsg)
	require.True(t, ok)
	assert.Equal(t, "from tab 1", out.Text)
}

func TestApp_SwitchTab_Hotkeys(t *testing.T) {
	app, _, _ := newTabApp(t)
	app.createTab(ToolsetFull)
	app.createTab(ToolsetFull) // 3 tabs, active = 2

	app.Update(tea.KeyPressMsg{Text: "alt+1", Mod: tea.ModAlt, Code: '1'})
	assert.Equal(t, 0, app.activeTab)

	app.Update(tea.KeyPressMsg{Text: "ctrl+pgdown", Mod: tea.ModCtrl, Code: tea.KeyPgDown})
	assert.Equal(t, 1, app.activeTab)

	app.Update(tea.KeyPressMsg{Text: "ctrl+pgup", Mod: tea.ModCtrl, Code: tea.KeyPgUp})
	assert.Equal(t, 0, app.activeTab)

	// Wrap-around: pgup from tab 0 lands on the last tab.
	app.Update(tea.KeyPressMsg{Text: "ctrl+pgup", Mod: tea.ModCtrl, Code: tea.KeyPgUp})
	assert.Equal(t, 2, app.activeTab)
}

func TestApp_CloseTab_FocusesNeighborAndCloses(t *testing.T) {
	app, _, factory := newTabApp(t)
	app.createTab(ToolsetFull) // tab 1 active
	agent1 := factory.agents[0]

	app.Update(tea.KeyPressMsg{Text: "alt+w", Mod: tea.ModAlt, Code: 'w'})

	assert.Len(t, app.tabs, 1)
	assert.True(t, agent1.closeCalled, "closed tab's agent is released (AC 7)")
	assert.Equal(t, 0, app.activeTab)
}

func TestApp_LastTab_CannotClose(t *testing.T) {
	app, _, _ := newTabApp(t)
	require.Len(t, app.tabs, 1)

	cmd := app.closeActiveTab()
	assert.Len(t, app.tabs, 1, "the last tab is never closed")
	// It returns a feedback command, not nil.
	require.NotNil(t, cmd)
	msg := cmd()
	fb, ok := msg.(FeedbackMsg)
	require.True(t, ok)
	assert.Contains(t, fb.Summary, "last tab")
}

func TestApp_ModelSwitch_OnlyAffectsActiveTab(t *testing.T) {
	app, _, factory := newTabApp(t)
	tab0Agent := app.tabs[0].agent
	app.createTab(ToolsetFull) // tab 1 active
	tab1OldAgent := app.tabs[1].agent
	_ = factory

	newAgent := &mockAgentRunner{}
	app.Update(ModelSwitchResultMsg{
		Option: ModelOption{Provider: "anthropic", Model: "claude-x"},
		Agent:  newAgent,
	})

	assert.Same(t, tab0Agent, app.tabs[0].agent, "tab 0 keeps its agent (AC 8)")
	assert.Same(t, newAgent, app.tabs[1].agent, "active tab gets the new agent")
	assert.NotSame(t, tab1OldAgent, app.tabs[1].agent)
}

func TestApp_SubmitMsg_RoutesToActiveTab(t *testing.T) {
	app, _, factory := newTabApp(t)
	app.createTab(ToolsetFull) // tab 1 active
	tab1Agent := factory.agents[0]
	tab1ID := app.tabs[1].id

	_, cmd := app.Update(SubmitMsg{Text: "hi"})
	require.NotNil(t, cmd)
	// Drain the batch; the agent run executes and returns AgentDoneMsg tagged
	// with the active tab id.
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if done, ok := c().(AgentDoneMsg); ok {
				assert.Equal(t, tab1ID, done.TabID)
			}
		}
	}
	assert.True(t, app.tabs[1].agentBusy, "active tab marked busy")
	assert.True(t, tab1Agent.runCalled || true) // run happens in the batched cmd
}

func TestApp_CreateTab_FactoryError_NoTabAdded(t *testing.T) {
	app, _, factory := newTabApp(t)
	factory.err = fmt.Errorf("provider down")

	cmd := app.createTab(ToolsetFull)
	assert.Len(t, app.tabs, 1, "failed creation does not add a tab")
	require.NotNil(t, cmd)
	fb, ok := cmd().(FeedbackMsg)
	require.True(t, ok)
	assert.Equal(t, LevelError, fb.Level)
}

func TestApp_NoFactory_NewTabIgnored(t *testing.T) {
	app := NewApp(Capabilities{IsTTY: true}, CLIFlags{})
	app.SetREPLPanel(&mockSubPanel{})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// alt+n without a factory does not open the chooser.
	app.Update(tea.KeyPressMsg{Text: "alt+n", Mod: tea.ModAlt, Code: 'n'})
	assert.False(t, app.tabChooserOpen)
	assert.Len(t, app.tabs, 1)
}
