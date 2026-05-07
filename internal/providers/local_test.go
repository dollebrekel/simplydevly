// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

func TestResolveLocalModel_Priority(t *testing.T) {
	tests := []struct {
		name     string
		override string
		cfg      core.ProviderConfig
		want     string
	}{
		{
			name:     "explicit override wins",
			override: "codellama:13b",
			cfg:      core.ProviderConfig{LocalModel: "deepseek-coder:6.7b"},
			want:     "codellama:13b",
		},
		{
			name:     "config local_model used when no override",
			override: "",
			cfg:      core.ProviderConfig{LocalModel: "deepseek-coder:6.7b"},
			want:     "deepseek-coder:6.7b",
		},
		{
			name:     "default when nothing configured",
			override: "",
			cfg:      core.ProviderConfig{},
			want:     providers.DefaultLocalModel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SIPLY_MODEL", "")
			got := providers.ResolveLocalModel(tt.override, tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveLocalModel_EnvVar(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "mistral:7b")
	got := providers.ResolveLocalModel("", core.ProviderConfig{LocalModel: "deepseek-coder:6.7b"})
	assert.Equal(t, "mistral:7b", got)
}

func TestResolveLocalModel_FlagOverridesEnvVar(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "mistral:7b")
	got := providers.ResolveLocalModel("codellama:13b", core.ProviderConfig{})
	assert.Equal(t, "codellama:13b", got)
}

func TestIsLocalEnv(t *testing.T) {
	t.Setenv("SIPLY_LOCAL", "1")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "true")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "TRUE")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "True")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "yes")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "YES")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", " true ")
	assert.True(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "0")
	assert.False(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "")
	assert.False(t, providers.IsLocalEnv())

	t.Setenv("SIPLY_LOCAL", "false")
	assert.False(t, providers.IsLocalEnv())
}
