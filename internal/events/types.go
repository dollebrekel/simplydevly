// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import "time"

// Event type constants for all 10 core event types.
// Stream and tool events have existing structs in internal/agent/ — these
// constants provide canonical type strings only.
const (
	EventFileSelected     = "file.selected"
	EventStreamText       = "stream.text"
	EventStreamToolCall   = "stream.tool_call"
	EventStreamDone       = "stream.done"
	EventToolExecuted     = "tool.executed"
	EventPermissionDenied = "permission.denied"
	EventPluginLoaded     = "plugin.loaded"
	EventPluginCrashed    = "plugin.crashed"
	EventPluginDisabled   = "plugin.disabled"
	EventConfigChanged    = "config.changed"
	EventSessionStarted   = "session.started"
	EventPanelActivated   = "panel.activated"
	EventMenuChanged      = "menu.changed"
	EventKeybindChanged   = "keybind.changed"
	EventPluginReloaded   = "plugin.reloaded"
	EventOfflineMode      = "offline.mode"
	EventSessionEnded     = "session.ended"
	EventCheckpoint       = "checkpoint"
)

// PluginLoadedEvent is published when a plugin loads successfully.
type PluginLoadedEvent struct {
	Name    string
	Version string
	Tier    int
	Ts      time.Time
}

// NewPluginLoadedEvent creates a PluginLoadedEvent with the current time.
func NewPluginLoadedEvent(name, version string, tier int) *PluginLoadedEvent {
	return &PluginLoadedEvent{Name: name, Version: version, Tier: tier, Ts: time.Now()}
}

func (e *PluginLoadedEvent) Type() string         { return EventPluginLoaded }
func (e *PluginLoadedEvent) Timestamp() time.Time { return e.Ts }

// PluginCrashedEvent is published when a plugin crashes.
type PluginCrashedEvent struct {
	Name string
	Err  string
	Ts   time.Time
}

// NewPluginCrashedEvent creates a PluginCrashedEvent with the current time.
func NewPluginCrashedEvent(name, err string) *PluginCrashedEvent {
	return &PluginCrashedEvent{Name: name, Err: err, Ts: time.Now()}
}

func (e *PluginCrashedEvent) Type() string         { return EventPluginCrashed }
func (e *PluginCrashedEvent) Timestamp() time.Time { return e.Ts }

// PluginDisabledEvent is published when a plugin is skipped at startup due to incompatibility.
type PluginDisabledEvent struct {
	Name    string
	Version string
	Reason  string
	Ts      time.Time
}

// NewPluginDisabledEvent creates a PluginDisabledEvent with the current time.
func NewPluginDisabledEvent(name, version, reason string) *PluginDisabledEvent {
	return &PluginDisabledEvent{Name: name, Version: version, Reason: reason, Ts: time.Now()}
}

func (e *PluginDisabledEvent) Type() string         { return EventPluginDisabled }
func (e *PluginDisabledEvent) Timestamp() time.Time { return e.Ts }

// ConfigChangedEvent is published when a configuration value changes.
// Delivery is synchronous — all consumers see new config before the next action.
type ConfigChangedEvent struct {
	Key      string
	OldValue string
	NewValue string
	Ts       time.Time
}

// NewConfigChangedEvent creates a ConfigChangedEvent with the current time.
func NewConfigChangedEvent(key, oldValue, newValue string) *ConfigChangedEvent {
	return &ConfigChangedEvent{Key: key, OldValue: oldValue, NewValue: newValue, Ts: time.Now()}
}

func (e *ConfigChangedEvent) Type() string         { return EventConfigChanged }
func (e *ConfigChangedEvent) Timestamp() time.Time { return e.Ts }

// PermissionDeniedEvent is published when a tool action is denied.
type PermissionDeniedEvent struct {
	ToolName string
	Reason   string
	Ts       time.Time
}

// NewPermissionDeniedEvent creates a PermissionDeniedEvent with the current time.
func NewPermissionDeniedEvent(toolName, reason string) *PermissionDeniedEvent {
	return &PermissionDeniedEvent{ToolName: toolName, Reason: reason, Ts: time.Now()}
}

func (e *PermissionDeniedEvent) Type() string         { return EventPermissionDenied }
func (e *PermissionDeniedEvent) Timestamp() time.Time { return e.Ts }

// SessionStartedEvent is published when a new session begins.
type SessionStartedEvent struct {
	SessionID string
	Ts        time.Time
}

// NewSessionStartedEvent creates a SessionStartedEvent with the current time.
func NewSessionStartedEvent(sessionID string) *SessionStartedEvent {
	return &SessionStartedEvent{SessionID: sessionID, Ts: time.Now()}
}

func (e *SessionStartedEvent) Type() string         { return EventSessionStarted }
func (e *SessionStartedEvent) Timestamp() time.Time { return e.Ts }

// PanelActivatedEvent is published when a TUI panel becomes active.
type PanelActivatedEvent struct {
	PanelName string
	Ts        time.Time
}

// NewPanelActivatedEvent creates a PanelActivatedEvent with the current time.
func NewPanelActivatedEvent(panelName string) *PanelActivatedEvent {
	return &PanelActivatedEvent{PanelName: panelName, Ts: time.Now()}
}

func (e *PanelActivatedEvent) Type() string         { return EventPanelActivated }
func (e *PanelActivatedEvent) Timestamp() time.Time { return e.Ts }

// MenuChangedEvent is published when extension menu items change.
type MenuChangedEvent struct {
	Ts time.Time
}

// NewMenuChangedEvent creates a MenuChangedEvent with the current time.
func NewMenuChangedEvent() *MenuChangedEvent {
	return &MenuChangedEvent{Ts: time.Now()}
}

func (e *MenuChangedEvent) Type() string         { return EventMenuChanged }
func (e *MenuChangedEvent) Timestamp() time.Time { return e.Ts }

// KeybindChangedEvent is published when extension keybindings change.
type KeybindChangedEvent struct {
	Ts time.Time
}

// NewKeybindChangedEvent creates a KeybindChangedEvent with the current time.
func NewKeybindChangedEvent() *KeybindChangedEvent {
	return &KeybindChangedEvent{Ts: time.Now()}
}

func (e *KeybindChangedEvent) Type() string         { return EventKeybindChanged }
func (e *KeybindChangedEvent) Timestamp() time.Time { return e.Ts }

// PluginReloadedEvent is published when a dev-mode plugin is reloaded.
type PluginReloadedEvent struct {
	Name string
	Ts   time.Time
}

// NewPluginReloadedEvent creates a PluginReloadedEvent with the current time.
func NewPluginReloadedEvent(name string) *PluginReloadedEvent {
	return &PluginReloadedEvent{Name: name, Ts: time.Now()}
}

func (e *PluginReloadedEvent) Type() string         { return EventPluginReloaded }
func (e *PluginReloadedEvent) Timestamp() time.Time { return e.Ts }

// FileSelectedEvent is published when a user selects a file (e.g. from tree-local panel).
// The agent subscribes to this event to add the file path to conversation context.
type FileSelectedEvent struct {
	Path string
	Ts   time.Time
}

// NewFileSelectedEvent creates a FileSelectedEvent with the current time.
func NewFileSelectedEvent(path string) *FileSelectedEvent {
	return &FileSelectedEvent{Path: path, Ts: time.Now()}
}

func (e *FileSelectedEvent) Type() string         { return EventFileSelected }
func (e *FileSelectedEvent) Timestamp() time.Time { return e.Ts }

// OfflineModeEvent is published when offline mode is activated at startup.
type OfflineModeEvent struct {
	Provider string
	Model    string
	Ts       time.Time
}

// NewOfflineModeEvent creates an OfflineModeEvent with the current time.
func NewOfflineModeEvent(provider, model string) *OfflineModeEvent {
	return &OfflineModeEvent{Provider: provider, Model: model, Ts: time.Now()}
}

func (e *OfflineModeEvent) Type() string         { return EventOfflineMode }
func (e *OfflineModeEvent) Timestamp() time.Time { return e.Ts }

// SessionEndedEvent is published when a session ends (clean exit or signal).
type SessionEndedEvent struct {
	SessionID    string
	MessageCount int
	TurnCount    int
	Ts           time.Time
}

// NewSessionEndedEvent creates a SessionEndedEvent with the current time.
func NewSessionEndedEvent(sessionID string, messageCount, turnCount int) *SessionEndedEvent {
	return &SessionEndedEvent{SessionID: sessionID, MessageCount: messageCount, TurnCount: turnCount, Ts: time.Now()}
}

func (e *SessionEndedEvent) Type() string         { return EventSessionEnded }
func (e *SessionEndedEvent) Timestamp() time.Time { return e.Ts }
