// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"siply.dev/siply/internal/core"
)

func TestPluginLoadedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewPluginLoadedEvent("test-plugin", "1.0.0", 1)
	assert.Equal(t, EventPluginLoaded, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
}

func TestPluginCrashedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewPluginCrashedEvent("test-plugin", "segfault")
	assert.Equal(t, EventPluginCrashed, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
}

func TestConfigChangedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewConfigChangedEvent("theme", "dark", "light")
	assert.Equal(t, EventConfigChanged, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
}

func TestPermissionDeniedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewPermissionDeniedEvent("bash", "unsafe command")
	assert.Equal(t, EventPermissionDenied, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
}

func TestSessionStartedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewSessionStartedEvent("abc-123")
	assert.Equal(t, EventSessionStarted, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
}

func TestPanelActivatedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewPanelActivatedEvent("repl")
	assert.Equal(t, EventPanelActivated, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
}

func TestEventTypeConstants(t *testing.T) {
	constants := map[string]string{
		"stream.text":       EventStreamText,
		"stream.tool_call":  EventStreamToolCall,
		"stream.done":       EventStreamDone,
		"tool.executed":     EventToolExecuted,
		"permission.denied": EventPermissionDenied,
		"plugin.loaded":     EventPluginLoaded,
		"plugin.crashed":    EventPluginCrashed,
		"config.changed":    EventConfigChanged,
		"session.started":   EventSessionStarted,
		"panel.activated":   EventPanelActivated,
	}
	for expected, got := range constants {
		assert.Equal(t, expected, got, "event type constant mismatch")
	}
}

func TestEventFields(t *testing.T) {
	t.Run("PluginLoadedEvent fields", func(t *testing.T) {
		ev := NewPluginLoadedEvent("myplugin", "2.1.0", 3)
		assert.Equal(t, "myplugin", ev.Name)
		assert.Equal(t, "2.1.0", ev.Version)
		assert.Equal(t, 3, ev.Tier)
	})

	t.Run("PluginCrashedEvent fields", func(t *testing.T) {
		ev := NewPluginCrashedEvent("myplugin", "nil pointer")
		assert.Equal(t, "myplugin", ev.Name)
		assert.Equal(t, "nil pointer", ev.Err)
	})

	t.Run("ConfigChangedEvent fields", func(t *testing.T) {
		ev := NewConfigChangedEvent("key", "old", "new")
		assert.Equal(t, "key", ev.Key)
		assert.Equal(t, "old", ev.OldValue)
		assert.Equal(t, "new", ev.NewValue)
	})

	t.Run("PermissionDeniedEvent fields", func(t *testing.T) {
		ev := NewPermissionDeniedEvent("bash", "blocked")
		assert.Equal(t, "bash", ev.ToolName)
		assert.Equal(t, "blocked", ev.Reason)
	})

	t.Run("SessionStartedEvent fields", func(t *testing.T) {
		ev := NewSessionStartedEvent("sess-42")
		assert.Equal(t, "sess-42", ev.SessionID)
	})

	t.Run("PanelActivatedEvent fields", func(t *testing.T) {
		ev := NewPanelActivatedEvent("diff")
		assert.Equal(t, "diff", ev.PanelName)
	})
}

func TestEventTimestampsAreRecent(t *testing.T) {
	before := time.Now()
	ev := NewPluginLoadedEvent("p", "1.0", 1)
	after := time.Now()

	assert.True(t, !ev.Timestamp().Before(before), "timestamp should be >= before")
	assert.True(t, !ev.Timestamp().After(after), "timestamp should be <= after")
}

func TestFileSelectedEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewFileSelectedEvent("/home/user/doc.md")
	assert.Equal(t, EventFileSelected, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
	fse, ok := ev.(*FileSelectedEvent)
	assert.True(t, ok)
	assert.Equal(t, "/home/user/doc.md", fse.Path)
}

func TestFileSelectedEvent_TypeString(t *testing.T) {
	assert.Equal(t, "file.selected", EventFileSelected)
}

func TestOfflineModeEvent_ImplementsEvent(t *testing.T) {
	var ev core.Event = NewOfflineModeEvent("ollama", "qwen2.5-coder:7b")
	assert.Equal(t, EventOfflineMode, ev.Type())
	assert.False(t, ev.Timestamp().IsZero())
	ome, ok := ev.(*OfflineModeEvent)
	assert.True(t, ok)
	assert.Equal(t, "ollama", ome.Provider)
	assert.Equal(t, "qwen2.5-coder:7b", ome.Model)
}

func TestOfflineModeEvent_TypeString(t *testing.T) {
	assert.Equal(t, "offline.mode", EventOfflineMode)
}
