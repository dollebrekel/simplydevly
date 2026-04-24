// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package completion

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

// mockRegistry implements core.PluginRegistry for testing.
type mockRegistry struct {
	plugins []core.PluginMeta
	listErr error
}

func (m *mockRegistry) Init(_ context.Context) error              { return nil }
func (m *mockRegistry) Start(_ context.Context) error             { return nil }
func (m *mockRegistry) Stop(_ context.Context) error              { return nil }
func (m *mockRegistry) Health() error                             { return nil }
func (m *mockRegistry) Install(_ context.Context, _ string) error { return nil }
func (m *mockRegistry) Load(_ context.Context, _ string) error    { return nil }
func (m *mockRegistry) Remove(_ context.Context, _ string) error  { return nil }
func (m *mockRegistry) DevMode(_ context.Context, _ string) error { return nil }
func (m *mockRegistry) List(_ context.Context) ([]core.PluginMeta, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.plugins, nil
}

func TestPluginNameCompletionFunc_ReturnsInstalledPluginNames(t *testing.T) {
	registry := &mockRegistry{
		plugins: []core.PluginMeta{
			{Name: "code-review", Version: "1.0.0", Tier: 1},
			{Name: "memory-store", Version: "2.1.0", Tier: 3},
			{Name: "test-runner", Version: "0.5.0", Tier: 1},
		},
	}

	fn := PluginNameCompletionFunc(registry)
	names, directive := fn(&cobra.Command{}, nil, "")

	require.Len(t, names, 3)
	assert.Contains(t, names, "code-review")
	assert.Contains(t, names, "memory-store")
	assert.Contains(t, names, "test-runner")
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestPluginNameCompletionFunc_EmptyRegistryReturnsEmptyList(t *testing.T) {
	registry := &mockRegistry{
		plugins: []core.PluginMeta{},
	}

	fn := PluginNameCompletionFunc(registry)
	names, directive := fn(&cobra.Command{}, nil, "")

	assert.Empty(t, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestPluginNameCompletionFunc_NilRegistryReturnsEmptyList(t *testing.T) {
	fn := PluginNameCompletionFunc(nil)
	names, directive := fn(&cobra.Command{}, nil, "")

	assert.Nil(t, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestPluginNameCompletionFunc_ListErrorReturnsEmptyList(t *testing.T) {
	registry := &mockRegistry{
		listErr: fmt.Errorf("registry unavailable"),
	}

	fn := PluginNameCompletionFunc(registry)
	names, directive := fn(&cobra.Command{}, nil, "")

	assert.Nil(t, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
}

func TestPluginNameCompletionFunc_ReturnsNoFileCompDirective(t *testing.T) {
	tests := []struct {
		name     string
		registry core.PluginRegistry
	}{
		{"with plugins", &mockRegistry{plugins: []core.PluginMeta{{Name: "a"}}}},
		{"empty registry", &mockRegistry{plugins: []core.PluginMeta{}}},
		{"nil registry", nil},
		{"error registry", &mockRegistry{listErr: fmt.Errorf("err")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := PluginNameCompletionFunc(tt.registry)
			_, directive := fn(&cobra.Command{}, nil, "")
			assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
		})
	}
}
