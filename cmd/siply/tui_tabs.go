// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	tea "charm.land/bubbletea/v2"
	"siply.dev/siply/internal/agent"
	appstartup "siply.dev/siply/internal/app"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tui"
	"siply.dev/siply/internal/tui/panels"
)

// readOnlyTools is the tool set granted to a "Read-only / Research" center tab:
// inspection only, no file mutation or shell execution.
var readOnlyTools = []string{"file_read", "search", "web"}

// tabAgent wraps a per-tab startup and its private event bus. Closing it
// releases the agent's provider resources and stops the bus so its bridge stops
// forwarding events.
type tabAgent struct {
	startup *appstartup.Startup
	bus     *events.Bus
}

func (a *tabAgent) Run(ctx context.Context, userMessage string) error {
	if a == nil || a.startup == nil || a.startup.Agent == nil {
		return errors.New("tui: tab agent unavailable")
	}
	return a.startup.Agent.Run(ctx, userMessage)
}

func (a *tabAgent) Stop(ctx context.Context) error {
	if a == nil || a.startup == nil || a.startup.Agent == nil {
		return nil
	}
	return a.startup.Agent.Stop(ctx)
}

func (a *tabAgent) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	var errs []error
	if a.startup != nil {
		errs = append(errs, a.startup.Stop(ctx))
	}
	if a.bus != nil {
		errs = append(errs, a.bus.Stop(ctx))
	}
	return errors.Join(errs...)
}

// centerTabFactory builds additional center tabs at runtime. It shares the
// plugin layer (hooks) and the slash dispatcher with the primary tab, so new
// tabs never reload side-panel plugins (E1). Each new tab gets its own event
// bus and a bridge that tags streamed output with the tab id.
type centerTabFactory struct {
	homeDir     string
	flags       tui.CLIFlags
	checkpoint  core.CheckpointManager
	theme       tui.Theme
	rc          tui.RenderConfig
	hooks       core.AgentHooks
	slash       *skills.SlashDispatcher
	skillLoader *skills.SkillLoader

	mu   sync.Mutex
	prog *tea.Program
}

// SetProgram records the Bubble Tea program used by per-tab event bridges. It is
// called once inside the RunApp setup callback, before the event loop starts.
func (f *centerTabFactory) SetProgram(prog *tea.Program) {
	f.mu.Lock()
	f.prog = prog
	f.mu.Unlock()
}

func (f *centerTabFactory) program() *tea.Program {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.prog
}

func allowedToolsForToolset(toolset tui.ToolsetChoice) []string {
	if toolset == tui.ToolsetReadOnly {
		return readOnlyTools
	}
	return nil
}

func flagsForModelOption(flags tui.CLIFlags, option tui.ModelOption) (tui.CLIFlags, string, bool) {
	providerName := option.Provider
	preferLocal := option.Kind == "local" || option.Provider == "ollama"
	if preferLocal {
		providerName = "ollama"
		flags.Local = true
	} else {
		flags.Local = false
	}
	return flags, providerName, preferLocal
}

// NewTab builds a REPL panel and an agent for a new center tab. The toolset
// selects the agent's tool registry. Implements tui.CenterTabFactory.
func (f *centerTabFactory) NewTab(tabID int, toolset tui.ToolsetChoice) (tui.SubPanel, tui.AgentRunner, error) {
	prog := f.program()
	if prog == nil {
		return nil, nil, errors.New("tui: tab factory not ready")
	}
	f.mu.Lock()
	flags := f.flags
	f.mu.Unlock()

	ctx := context.Background()
	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return nil, nil, err
	}
	if err := bus.Start(ctx); err != nil {
		_ = bus.Stop(ctx)
		return nil, nil, err
	}

	repl := panels.NewREPLPanel(f.theme, f.rc)
	if f.slash != nil {
		repl.SetSlashDispatcher(f.slash, f.skillLoader)
	}
	repl.SetBordered(false)

	startup, err := appstartup.Start(ctx, appstartup.Options{
		Surface:        appstartup.SurfaceTUI,
		HomeDir:        f.homeDir,
		ModelOverride:  flags.ModelOverride,
		PreferLocal:    flags.Local,
		EventBus:       bus,
		Hooks:          f.hooks,
		Checkpoint:     f.checkpoint,
		PermissionMode: permission.ModeAutoAccept,
		AllowedTools:   allowedToolsForToolset(toolset),
		OnPermissionEvaluator: func(perm core.PermissionEvaluator) {
			if ctrl, ok := perm.(interface {
				SetMode(permission.Mode) error
				Mode() permission.Mode
			}); ok {
				repl.SetPermissionController(ctrl)
			}
		},
	})
	if err != nil {
		_ = bus.Stop(ctx)
		return nil, nil, err
	}

	bridgeTabBus(bus, prog, tabID)
	return repl, &tabAgent{startup: startup, bus: bus}, nil
}

// SwitchTabModel replaces a secondary tab's agent while preserving that tab's
// private event bus and tagged bridge. This keeps /model scoped to the active
// tab instead of falling back to the shared tab-0 bridge.
func (f *centerTabFactory) SwitchTabModel(ctx context.Context, tabID int, toolset tui.ToolsetChoice, option tui.ModelOption, repl tui.SubPanel) tui.ModelSwitchResultMsg {
	if option.Disabled {
		return tui.ModelSwitchResultMsg{Option: option, Err: errors.New("model is not selectable")}
	}
	prog := f.program()
	if prog == nil {
		return tui.ModelSwitchResultMsg{Option: option, Err: errors.New("tui: tab factory not ready")}
	}
	f.mu.Lock()
	baseFlags := f.flags
	f.mu.Unlock()
	flags, providerName, preferLocal := flagsForModelOption(baseFlags, option)
	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return tui.ModelSwitchResultMsg{Option: option, Err: err}
	}
	if err := bus.Start(ctx); err != nil {
		_ = bus.Stop(context.Background())
		return tui.ModelSwitchResultMsg{Option: option, Err: err}
	}
	startup, err := appstartup.Start(ctx, appstartup.Options{
		Surface:         appstartup.SurfaceTUI,
		HomeDir:         f.homeDir,
		SessionProvider: providerName,
		SessionModel:    option.Model,
		PreferLocal:     preferLocal,
		EventBus:        bus,
		Hooks:           f.hooks,
		Checkpoint:      f.checkpoint,
		PermissionMode:  permission.ModeAutoAccept,
		AllowedTools:    allowedToolsForToolset(toolset),
		OnPermissionEvaluator: func(perm core.PermissionEvaluator) {
			if panel, ok := repl.(*panels.REPLPanel); ok {
				if ctrl, ok := perm.(interface {
					SetMode(permission.Mode) error
					Mode() permission.Mode
				}); ok {
					panel.SetPermissionController(ctrl)
				}
			}
		},
	})
	if err != nil {
		_ = bus.Stop(context.Background())
		return tui.ModelSwitchResultMsg{Option: option, Err: err}
	}
	f.mu.Lock()
	f.flags = flags
	f.mu.Unlock()
	bridgeTabBus(bus, prog, tabID)
	return tui.ModelSwitchResultMsg{Option: option, Agent: &tabAgent{startup: startup, bus: bus}}
}

// bridgeTabBus forwards a single tab's agent stream and tool events to the
// Bubble Tea program, tagged with tabID so App routes them to the right tab.
// Plugin/menu/panel events stay on the shared app bus and are NOT bridged here.
func bridgeTabBus(bus *events.Bus, prog *tea.Program, tabID int) {
	bus.Subscribe(events.EventStreamText, func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			prog.Send(tui.AgentOutputMsg{Text: te.Text(), TabID: tabID})
		}
	})

	bus.Subscribe(events.EventStreamToolCall, func(_ context.Context, ev core.Event) {
		if tc, ok := ev.(interface{ ToolName() string }); ok {
			msg := tui.FeedEntryMsg{Type: "tool", Label: tc.ToolName(), TabID: tabID}
			if id, ok := ev.(interface{ ToolID() string }); ok {
				msg.ToolID = id.ToolID()
			}
			if input, ok := ev.(interface{ Input() json.RawMessage }); ok {
				msg.Input = []byte(input.Input())
			}
			prog.Send(msg)
		}
	})

	bus.Subscribe(events.EventToolExecuted, func(_ context.Context, ev core.Event) {
		if te, ok := ev.(*agent.ToolExecutedEvent); ok {
			prog.Send(tui.FeedEntryMsg{
				Type:     "tool-done",
				Label:    te.ToolName,
				Duration: te.Duration,
				IsError:  te.IsError,
				ToolID:   te.ToolID,
				Input:    []byte(te.Input),
				Output:   te.Output,
				TabID:    tabID,
			})
		}
	})
}
