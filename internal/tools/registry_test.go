// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// mockPermission is a test helper that returns a configurable verdict.
type mockPermission struct {
	verdict core.ActionVerdict
	err     error
}

func (m *mockPermission) Init(_ context.Context) error  { return nil }
func (m *mockPermission) Start(_ context.Context) error { return nil }
func (m *mockPermission) Stop(_ context.Context) error  { return nil }
func (m *mockPermission) Health() error                 { return nil }
func (m *mockPermission) EvaluateCapabilities(_ context.Context, _ core.PluginMeta) (core.CapabilityVerdict, error) {
	return core.CapabilityVerdict{}, nil
}
func (m *mockPermission) EvaluateAction(_ context.Context, _ core.Action) (core.ActionVerdict, error) {
	return m.verdict, m.err
}

// mockTool is a simple test tool.
type mockTool struct {
	name        string
	destructive bool
	output      string
	err         error
}

func (m *mockTool) Name() string                 { return m.name }
func (m *mockTool) Description() string          { return "mock tool" }
func (m *mockTool) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (m *mockTool) Destructive() bool            { return m.destructive }
func (m *mockTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return m.output, m.err
}

func TestRegistry_RegisterAndExecute(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	tool := &mockTool{name: "test_tool", output: "hello"}
	require.NoError(t, reg.Register(tool))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:   "test_tool",
		Input:  json.RawMessage(`{}`),
		Source: "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", resp.Output)
	assert.False(t, resp.IsError)
	assert.Greater(t, resp.Duration.Nanoseconds(), int64(0))
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	tool := &mockTool{name: "dup"}
	require.NoError(t, reg.Register(tool))
	err := reg.Register(tool)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_ToolNotFound(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	_, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:  "nonexistent",
		Input: json.RawMessage(`{}`),
	})
	assert.ErrorIs(t, err, core.ErrToolNotFound)
}

func TestRegistry_PermissionDenied(t *testing.T) {
	perm := &mockPermission{verdict: core.Deny}
	reg := NewRegistry(perm)
	require.NoError(t, reg.Register(&mockTool{name: "test"}))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:  "test",
		Input: json.RawMessage(`{}`),
	})
	assert.ErrorIs(t, err, core.ErrPermissionDenied)
	assert.True(t, resp.IsError)
	assert.Equal(t, "permission denied", resp.Output)
}

func TestRegistry_PermissionAsk(t *testing.T) {
	perm := &mockPermission{verdict: core.Ask}
	reg := NewRegistry(perm)
	require.NoError(t, reg.Register(&mockTool{name: "test"}))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:  "test",
		Input: json.RawMessage(`{}`),
	})
	assert.NoError(t, err) // Ask returns nil error
	assert.True(t, resp.IsError)
	assert.Equal(t, "confirmation required", resp.Output)
}

func TestRegistry_ListTools(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	require.NoError(t, reg.Register(&mockTool{name: "a"}))
	require.NoError(t, reg.Register(&mockTool{name: "b"}))

	tools := reg.ListTools()
	assert.Len(t, tools, 2)

	names := make(map[string]bool)
	for _, td := range tools {
		names[td.Name] = true
	}
	assert.True(t, names["a"])
	assert.True(t, names["b"])
}

func TestListTools_DeterministicOrder(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	// Register tools in non-alphabetical order
	require.NoError(t, reg.Register(&mockTool{name: "zebra"}))
	require.NoError(t, reg.Register(&mockTool{name: "alpha"}))
	require.NoError(t, reg.Register(&mockTool{name: "middle"}))

	// Call ListTools twice and verify identical, sorted order
	tools1 := reg.ListTools()
	tools2 := reg.ListTools()

	require.Len(t, tools1, 3)
	require.Len(t, tools2, 3)

	expected := []string{"alpha", "middle", "zebra"}
	for i := range expected {
		assert.Equal(t, expected[i], tools1[i].Name, "first call: tool[%d]", i)
		assert.Equal(t, expected[i], tools2[i].Name, "second call: tool[%d]", i)
	}
}

func TestRegistry_Init_DeterministicOrder(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)
	require.NoError(t, reg.Init(context.Background()))

	tools := reg.ListTools()
	require.Len(t, tools, 6)

	// Built-in tools should be in alphabetical order
	expected := []string{"bash", "file_edit", "file_read", "file_write", "search", "web"}
	for i, name := range expected {
		assert.Equal(t, name, tools[i].Name, "tool[%d]", i)
	}
}

func TestRegistry_GetTool(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	require.NoError(t, reg.Register(&mockTool{name: "my_tool"}))

	def, err := reg.GetTool("my_tool")
	require.NoError(t, err)
	assert.Equal(t, "my_tool", def.Name)

	_, err = reg.GetTool("missing")
	assert.ErrorIs(t, err, core.ErrToolNotFound)
}

func TestRegistry_Init(t *testing.T) {
	perm := &mockPermission{verdict: core.Allow}
	reg := NewRegistry(perm)

	require.NoError(t, reg.Init(context.Background()))

	tools := reg.ListTools()
	assert.Len(t, tools, 6)

	expectedNames := map[string]bool{
		"file_read":  true,
		"file_write": true,
		"file_edit":  true,
		"bash":       true,
		"search":     true,
		"web":        true,
	}
	for _, td := range tools {
		assert.True(t, expectedNames[td.Name], "unexpected tool: %s", td.Name)
	}
}
