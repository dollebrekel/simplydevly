// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestResolveModelSelectionPrecedence(t *testing.T) {
	tests := []struct {
		name string
		in   ModelSelectionInput
		want ModelSelection
	}{
		{
			name: "session selection wins",
			in: ModelSelectionInput{
				SessionModel: "session-model",
				CLIModel:     "cli-model",
				WorkspaceConfig: core.ProviderConfig{
					Model: "workspace-model",
				},
				ProjectConfig: core.ProviderConfig{Model: "project-model"},
				GlobalConfig:  core.ProviderConfig{Model: "global-model"},
				EnvModel:      "env-model",
				PreferLocal:   true,
			},
			want: ModelSelection{Model: "session-model", Source: ModelSourceSession},
		},
		{
			name: "cli override wins over config",
			in: ModelSelectionInput{
				CLIModel:        "cli-model",
				WorkspaceConfig: core.ProviderConfig{Model: "workspace-model"},
				ProjectConfig:   core.ProviderConfig{Model: "project-model"},
				GlobalConfig:    core.ProviderConfig{Model: "global-model"},
				EnvModel:        "env-model",
				PreferLocal:     true,
			},
			want: ModelSelection{Model: "cli-model", Source: ModelSourceCLI},
		},
		{
			name: "workspace config wins over lower layers",
			in: ModelSelectionInput{
				WorkspaceConfig: core.ProviderConfig{Model: "workspace-model"},
				ProjectConfig:   core.ProviderConfig{Model: "project-model"},
				GlobalConfig:    core.ProviderConfig{Model: "global-model"},
				EnvModel:        "env-model",
				PreferLocal:     true,
			},
			want: ModelSelection{Model: "workspace-model", Source: ModelSourceWorkspace},
		},
		{
			name: "project config wins over global and env",
			in: ModelSelectionInput{
				ProjectConfig: core.ProviderConfig{Model: "project-model"},
				GlobalConfig:  core.ProviderConfig{Model: "global-model"},
				EnvModel:      "env-model",
				PreferLocal:   true,
			},
			want: ModelSelection{Model: "project-model", Source: ModelSourceProject},
		},
		{
			name: "global config wins over env",
			in: ModelSelectionInput{
				GlobalConfig: core.ProviderConfig{Model: "global-model"},
				EnvModel:     "env-model",
				PreferLocal:  true,
			},
			want: ModelSelection{Model: "global-model", Source: ModelSourceGlobal},
		},
		{
			name: "env wins over local preference",
			in: ModelSelectionInput{
				EnvModel:     "env-model",
				PreferLocal:  true,
				MergedConfig: core.ProviderConfig{LocalModel: "local-model"},
			},
			want: ModelSelection{Model: "env-model", Source: ModelSourceEnv},
		},
		{
			name: "local preference uses local model when no stronger value exists",
			in: ModelSelectionInput{
				PreferLocal:  true,
				MergedConfig: core.ProviderConfig{LocalModel: "local-model"},
			},
			want: ModelSelection{Model: "local-model", Provider: "ollama", Source: ModelSourceLocalPreference},
		},
		{
			name: "provider default when no model is selected",
			in:   ModelSelectionInput{},
			want: ModelSelection{Source: ModelSourceProviderDefault},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveModelSelection(tt.in)
			assert.Equal(t, tt.want.Model, got.Model)
			assert.Equal(t, tt.want.Provider, got.Provider)
			assert.Equal(t, tt.want.Source, got.Source)
		})
	}
}

func TestStartupStopIsIdempotent(t *testing.T) {
	ctx := context.Background()
	lc := &recordingLifecycle{name: "owned"}
	st := &Startup{}
	st.addOwned("owned", lc)

	require.NoError(t, st.startOwned(ctx))
	require.NoError(t, st.Stop(ctx))
	require.NoError(t, st.Stop(ctx))

	assert.Equal(t, 1, lc.initCalls)
	assert.Equal(t, 1, lc.startCalls)
	assert.Equal(t, 1, lc.stopCalls)
}

func TestStartupStartRollbackStopsStartedComponentsInReverseOrder(t *testing.T) {
	ctx := context.Background()
	var order []string
	first := &recordingLifecycle{name: "first", order: &order}
	second := &recordingLifecycle{name: "second", order: &order}
	third := &recordingLifecycle{name: "third", order: &order, startErr: errors.New("boom")}

	st := &Startup{}
	st.addOwned("first", first)
	st.addOwned("second", second)
	st.addOwned("third", third)

	err := st.startOwned(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start third")
	assert.Equal(t, []string{
		"init:first", "init:second", "init:third",
		"start:first", "start:second", "start:third",
		"stop:second", "stop:first",
	}, order)
	assert.Equal(t, 0, third.stopCalls)
}

func TestResolveProviderNamePreferLocalDoesNotOverrideConfiguredProvider(t *testing.T) {
	tests := []struct {
		name string
		in   ProviderSelectionInput
		want string
	}{
		{
			name: "configured provider wins over local preference",
			in: ProviderSelectionInput{
				ConfigProvider: "anthropic",
				PreferLocal:    true,
			},
			want: "anthropic",
		},
		{
			name: "environment provider wins over local preference",
			in: ProviderSelectionInput{
				EnvProvider: "openai",
				PreferLocal: true,
			},
			want: "openai",
		},
		{
			name: "local preference wins over provider default",
			in: ProviderSelectionInput{
				PreferLocal: true,
			},
			want: "ollama",
		},
		{
			name: "default provider without stronger selection",
			in:   ProviderSelectionInput{},
			want: defaultProviderName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveProviderName(tt.in))
		})
	}
}

type recordingLifecycle struct {
	name       string
	order      *[]string
	initErr    error
	startErr   error
	stopErr    error
	initCalls  int
	startCalls int
	stopCalls  int
}

func (r *recordingLifecycle) Init(context.Context) error {
	r.initCalls++
	if r.order != nil {
		*r.order = append(*r.order, "init:"+r.name)
	}
	return r.initErr
}

func (r *recordingLifecycle) Start(context.Context) error {
	r.startCalls++
	if r.order != nil {
		*r.order = append(*r.order, "start:"+r.name)
	}
	return r.startErr
}

func (r *recordingLifecycle) Stop(context.Context) error {
	r.stopCalls++
	if r.order != nil {
		*r.order = append(*r.order, "stop:"+r.name)
	}
	return r.stopErr
}

func (r *recordingLifecycle) Health() error { return nil }
