// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// luaTestEventBus is a minimal EventBus for testing.
type luaTestEventBus struct {
	published []core.Event
	handlers  map[string][]core.EventHandler
	mu        sync.Mutex
}

func newMockEventBus() *luaTestEventBus {
	return &luaTestEventBus{handlers: make(map[string][]core.EventHandler)}
}

func (b *luaTestEventBus) Init(_ context.Context) error  { return nil }
func (b *luaTestEventBus) Start(_ context.Context) error { return nil }
func (b *luaTestEventBus) Stop(_ context.Context) error  { return nil }
func (b *luaTestEventBus) Health() error                 { return nil }

func (b *luaTestEventBus) Publish(_ context.Context, event core.Event) error {
	b.mu.Lock()
	b.published = append(b.published, event)
	handlers := b.handlers[event.Type()]
	b.mu.Unlock()

	for _, h := range handlers {
		h(context.Background(), event)
	}
	return nil
}

func (b *luaTestEventBus) Subscribe(eventType string, handler core.EventHandler) func() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		handlers := b.handlers[eventType]
		for i, h := range handlers {
			// Compare function pointers by removing it.
			_ = h
			b.handlers[eventType] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}
}

func (b *luaTestEventBus) SubscribeChan(eventType string) (<-chan core.Event, func()) {
	ch := make(chan core.Event, 10)
	unsub := b.Subscribe(eventType, func(_ context.Context, e core.Event) {
		ch <- e
	})
	return ch, unsub
}

func TestSiplyLog(t *testing.T) {
	t.Parallel()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "test-log"}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`
		siply.log("info", "hello world")
		siply.log("warn", "be careful")
		siply.log("error", "oh no")
	`)
	require.NoError(t, err)
}

func TestSiplyOnReceivesEvents(t *testing.T) {
	t.Parallel()
	bus := newMockEventBus()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "test-on"}
	registerSiplyAPI(L, plugin, bus, nil)

	err := L.DoString(`
		received_type = ""
		siply.on("config.changed", function(data)
			received_type = data.type
		end)
	`)
	require.NoError(t, err)

	// Publish event.
	evt := &luaCustomEvent{eventType: "config.changed", data: map[string]any{"key": "val"}}
	require.NoError(t, bus.Publish(context.Background(), evt))

	// Check received.
	val := L.GetGlobal("received_type")
	assert.Equal(t, lua.LString("config.changed"), val)
}

func TestSiplyEmitPublishesEvents(t *testing.T) {
	t.Parallel()
	bus := newMockEventBus()
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "test-emit"}
	registerSiplyAPI(L, plugin, bus, nil)

	err := L.DoString(`
		siply.emit("custom.event", {key = "value"})
	`)
	require.NoError(t, err)

	bus.mu.Lock()
	defer bus.mu.Unlock()
	require.Len(t, bus.published, 1)
	assert.Equal(t, "lua.test-emit.custom.event", bus.published[0].Type())
}

func TestSiplyStatePersists(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "test-state", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	// Set a value.
	err := L.DoString(`siply.state.set("counter", 42)`)
	require.NoError(t, err)

	// Get the value.
	err = L.DoString(`result = siply.state.get("counter")`)
	require.NoError(t, err)

	val := L.GetGlobal("result")
	assert.Equal(t, lua.LNumber(42), val)

	// Verify file exists on disk.
	_, err = os.Stat(filepath.Join(stateDir, "state.json"))
	require.NoError(t, err)
}

func TestSiplyStateGetMissing(t *testing.T) {
	t.Parallel()
	stateDir := filepath.Join(t.TempDir(), "state")
	L := NewSandboxedState(context.Background())
	defer L.Close()

	plugin := &Tier2Plugin{Name: "test-state-missing", StateDir: stateDir}
	registerSiplyAPI(L, plugin, nil, nil)

	err := L.DoString(`result = siply.state.get("nonexistent")`)
	require.NoError(t, err)

	val := L.GetGlobal("result")
	assert.Equal(t, lua.LNil, val)
}

func TestLuaCustomEvent(t *testing.T) {
	t.Parallel()
	evt := &luaCustomEvent{eventType: "test.event", data: map[string]any{"key": "val"}, ts: time.Now()}
	assert.Equal(t, "test.event", evt.Type())
	assert.WithinDuration(t, time.Now(), evt.Timestamp(), time.Second)
}

func TestLuaToGoConversions(t *testing.T) {
	t.Parallel()
	L := lua.NewState()
	defer L.Close()

	assert.Nil(t, luaToGo(lua.LNil))
	assert.Equal(t, true, luaToGo(lua.LBool(true)))
	assert.Equal(t, float64(3.14), luaToGo(lua.LNumber(3.14)))
	assert.Equal(t, "hello", luaToGo(lua.LString("hello")))
}

func TestGoToLuaConversions(t *testing.T) {
	t.Parallel()
	L := lua.NewState()
	defer L.Close()

	assert.Equal(t, lua.LNil, goToLua(L, nil))
	assert.Equal(t, lua.LBool(true), goToLua(L, true))
	assert.Equal(t, lua.LNumber(42), goToLua(L, float64(42)))
	assert.Equal(t, lua.LString("test"), goToLua(L, "test"))
}
