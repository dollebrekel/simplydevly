// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"testing"

	"siply.dev/siply/internal/core"
)

func ptr[T any](v T) *T { return &v }

func TestValidateConfig_RoutingDisabled_NoWarning(t *testing.T) {
	ValidateConfig(core.RoutingConfig{Enabled: ptr(false)}, 1, nil)
}

func TestValidateConfig_RoutingEnabledOneProvider(t *testing.T) {
	ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "ollama"},
		},
	}, 1, []string{"ollama"})
}

func TestValidateConfig_PreferCheapestNoPricing(t *testing.T) {
	ValidateConfig(core.RoutingConfig{
		Enabled:        ptr(true),
		PreferCheapest: ptr(true),
	}, 2, nil)
}

func TestValidateConfig_DuplicateRules(t *testing.T) {
	ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "ollama"},
			{Category: "preprocess", Provider: "openai"},
		},
	}, 2, []string{"ollama", "openai"})
}

func TestValidateConfig_ValidConfig(t *testing.T) {
	ValidateConfig(core.RoutingConfig{
		Enabled:        ptr(true),
		PreferCheapest: ptr(true),
		Pricing: map[string]core.ProviderPricing{
			"anthropic": {InputPer1M: 3.0, OutputPer1M: 15.0},
			"ollama":    {InputPer1M: 0.0, OutputPer1M: 0.0},
		},
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "ollama"},
			{Category: "primary", Provider: "anthropic"},
		},
	}, 2, []string{"anthropic", "ollama"})
}

func TestValidateConfig_UnknownCategory(t *testing.T) {
	ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "priamry", Provider: "anthropic"},
		},
	}, 2, []string{"anthropic", "ollama"})
}

func TestValidateConfig_UnknownProvider(t *testing.T) {
	ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "nonexistent"},
		},
	}, 2, []string{"anthropic", "ollama"})
}
