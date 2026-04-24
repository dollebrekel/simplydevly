// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

func TestResolveOfflineModel_Priority(t *testing.T) {
	tests := []struct {
		name     string
		override string
		cfg      core.ProviderConfig
		want     string
	}{
		{
			name:     "explicit override wins",
			override: "codellama:13b",
			cfg:      core.ProviderConfig{OfflineModel: "deepseek-coder:6.7b"},
			want:     "codellama:13b",
		},
		{
			name:     "config offline_model used when no override",
			override: "",
			cfg:      core.ProviderConfig{OfflineModel: "deepseek-coder:6.7b"},
			want:     "deepseek-coder:6.7b",
		},
		{
			name:     "default when nothing configured",
			override: "",
			cfg:      core.ProviderConfig{},
			want:     providers.DefaultOfflineModel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SIPLY_MODEL", "")
			got := providers.ResolveOfflineModel(tt.override, tt.cfg)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveOfflineModel_EnvVar(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "mistral:7b")
	got := providers.ResolveOfflineModel("", core.ProviderConfig{OfflineModel: "deepseek-coder:6.7b"})
	assert.Equal(t, "mistral:7b", got)
}

func TestResolveOfflineModel_FlagOverridesEnvVar(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "mistral:7b")
	got := providers.ResolveOfflineModel("codellama:13b", core.ProviderConfig{})
	assert.Equal(t, "codellama:13b", got)
}

func TestIsOfflineEnv(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "1")
	assert.True(t, providers.IsOfflineEnv())

	t.Setenv("SIPLY_OFFLINE", "true")
	assert.True(t, providers.IsOfflineEnv())

	t.Setenv("SIPLY_OFFLINE", "0")
	assert.False(t, providers.IsOfflineEnv())

	t.Setenv("SIPLY_OFFLINE", "")
	assert.False(t, providers.IsOfflineEnv())
}
