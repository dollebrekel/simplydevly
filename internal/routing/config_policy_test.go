// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigPolicy_Select(t *testing.T) {
	tests := []struct {
		name         string
		config       RoutingConfig
		hints        map[string]string
		wantProvider string
		wantModel    string
		wantDefault  bool
	}{
		{
			name: "match preprocess category",
			config: RoutingConfig{
				Rules: []RoutingRule{
					{Category: CategoryPreprocess, Provider: "ollama", Model: "llama3.2"},
					{Category: CategoryPrimary, Provider: "anthropic"},
				},
				DefaultProvider: "anthropic",
				Enabled:         true,
			},
			hints:        map[string]string{HintKeyCategory: "preprocess"},
			wantProvider: "ollama",
			wantModel:    "llama3.2",
		},
		{
			name: "match primary category",
			config: RoutingConfig{
				Rules: []RoutingRule{
					{Category: CategoryPreprocess, Provider: "ollama", Model: "llama3.2"},
					{Category: CategoryPrimary, Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
				},
				DefaultProvider: "anthropic",
				Enabled:         true,
			},
			hints:        map[string]string{HintKeyCategory: "primary"},
			wantProvider: "anthropic",
			wantModel:    "claude-sonnet-4-20250514",
		},
		{
			name: "fallback to default on unknown category",
			config: RoutingConfig{
				Rules: []RoutingRule{
					{Category: CategoryPreprocess, Provider: "ollama"},
				},
				DefaultProvider: "anthropic",
				Enabled:         true,
			},
			hints:        map[string]string{HintKeyCategory: "unknown"},
			wantProvider: "anthropic",
			wantDefault:  true,
		},
		{
			name: "fallback to default on empty rules",
			config: RoutingConfig{
				Rules:           nil,
				DefaultProvider: "anthropic",
				Enabled:         true,
			},
			hints:        map[string]string{HintKeyCategory: "preprocess"},
			wantProvider: "anthropic",
			wantDefault:  true,
		},
		{
			name: "nil hints fallback to default",
			config: RoutingConfig{
				Rules: []RoutingRule{
					{Category: CategoryPreprocess, Provider: "ollama"},
				},
				DefaultProvider: "anthropic",
				Enabled:         true,
			},
			hints:        nil,
			wantProvider: "anthropic",
			wantDefault:  true,
		},
		{
			name: "empty hints fallback to default",
			config: RoutingConfig{
				Rules: []RoutingRule{
					{Category: CategoryPreprocess, Provider: "ollama"},
				},
				DefaultProvider: "anthropic",
				Enabled:         true,
			},
			hints:        map[string]string{},
			wantProvider: "anthropic",
			wantDefault:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewConfigPolicy(tt.config)
			sel := policy.Select(tt.hints)

			assert.Equal(t, tt.wantProvider, sel.Provider)
			assert.Equal(t, tt.wantModel, sel.Model)
			assert.NotEmpty(t, sel.Reason)

			if tt.wantDefault {
				assert.Contains(t, sel.Reason, "default")
			}
		})
	}
}
