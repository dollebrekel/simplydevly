// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import "time"

// Event type constants for all 10 core event types.
// Stream and tool events have existing structs in internal/agent/ — these
// constants provide canonical type strings only.
const (
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
