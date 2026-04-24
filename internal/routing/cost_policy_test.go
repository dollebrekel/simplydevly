// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/core"
)

func TestCostPolicy_SelectCheapest(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00},
			"openai":    {InputPer1M: 2.50, OutputPer1M: 10.00},
			"ollama":    {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"anthropic": {SupportsToolCalls: true, SupportsVision: true, MaxContextTokens: 200000},
			"openai":    {SupportsToolCalls: true, SupportsVision: true, MaxContextTokens: 128000},
			"ollama":    {SupportsToolCalls: false, SupportsVision: false, MaxContextTokens: 8192},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{HintKeyCategory: "primary"})
	assert.Equal(t, "ollama", sel.Provider)
	assert.Contains(t, sel.Reason, "$0.00")
}

func TestCostPolicy_CapabilityFiltering_Tools(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00},
			"ollama":    {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"anthropic": {SupportsToolCalls: true, MaxContextTokens: 200000},
			"ollama":    {SupportsToolCalls: false, MaxContextTokens: 8192},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{
		HintKeyCategory:      "primary",
		HintKeyRequiresTools: "true",
	})
	assert.Equal(t, "anthropic", sel.Provider)
}

func TestCostPolicy_CapabilityFiltering_Vision(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00},
			"ollama":    {InputPer1M: 0.00, OutputPer1M: 0.00},
			"kimi":      {InputPer1M: 1.00, OutputPer1M: 2.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"anthropic": {SupportsVision: true, MaxContextTokens: 200000},
			"ollama":    {SupportsVision: false, MaxContextTokens: 8192},
			"kimi":      {SupportsVision: false, MaxContextTokens: 131072},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{
		HintKeyCategory:       "primary",
		HintKeyRequiresVision: "true",
	})
	assert.Equal(t, "anthropic", sel.Provider)
}

func TestCostPolicy_CapabilityFiltering_MinContext(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00},
			"ollama":    {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"anthropic": {MaxContextTokens: 200000},
			"ollama":    {MaxContextTokens: 8192},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{
		HintKeyCategory:   "primary",
		HintKeyMinContext: "100000",
	})
	assert.Equal(t, "anthropic", sel.Provider)
}

func TestCostPolicy_CategoryRuleOverridesCost(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00},
			"ollama":    {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"anthropic": {SupportsToolCalls: true, MaxContextTokens: 200000},
			"ollama":    {SupportsToolCalls: false, MaxContextTokens: 8192},
		},
		DefaultProvider: "anthropic",
		Rules: []RoutingRule{
			{Category: CategoryPreprocess, Provider: "ollama", Model: "qwen2.5-coder:7b"},
		},
	})

	sel := policy.Select(map[string]string{HintKeyCategory: "preprocess"})
	assert.Equal(t, "ollama", sel.Provider)
	assert.Equal(t, "qwen2.5-coder:7b", sel.Model)
	assert.Contains(t, sel.Reason, "matched rule")
}

func TestCostPolicy_NoCandidatesFallsBackToDefault(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"ollama": {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"ollama": {SupportsToolCalls: false, MaxContextTokens: 8192},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{
		HintKeyCategory:      "primary",
		HintKeyRequiresTools: "true",
	})
	assert.Equal(t, "anthropic", sel.Provider)
	assert.Contains(t, sel.Reason, "no capable provider")
}

func TestCostPolicy_CostComparison(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"openai":    {InputPer1M: 2.50, OutputPer1M: 10.00},
			"anthropic": {InputPer1M: 3.00, OutputPer1M: 15.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"openai":    {SupportsToolCalls: true, MaxContextTokens: 128000},
			"anthropic": {SupportsToolCalls: true, MaxContextTokens: 200000},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{HintKeyCategory: "primary"})
	assert.Equal(t, "openai", sel.Provider)
	assert.Contains(t, sel.Reason, "openai")
	assert.Contains(t, sel.Reason, "vs")
	assert.Contains(t, sel.Reason, "anthropic")
}

func TestCostPolicy_NoPricingDefaultsToExpensive(t *testing.T) {
	policy := NewCostPolicy(CostPolicyConfig{
		Pricing: map[string]core.ProviderPricing{
			"ollama": {InputPer1M: 0.00, OutputPer1M: 0.00},
		},
		Capabilities: map[string]core.ProviderCapabilities{
			"ollama":    {MaxContextTokens: 8192},
			"anthropic": {MaxContextTokens: 200000},
		},
		DefaultProvider: "anthropic",
	})

	sel := policy.Select(map[string]string{HintKeyCategory: "primary"})
	assert.Equal(t, "ollama", sel.Provider)
}

func TestConfigPolicy_SelectNewCategories(t *testing.T) {
	policy := NewConfigPolicy(RoutingConfig{
		Rules: []RoutingRule{
			{Category: CategoryReview, Provider: "openai", Model: "gpt-4o-mini"},
			{Category: CategoryChat, Provider: "ollama"},
			{Category: CategoryVision, Provider: "anthropic"},
		},
		DefaultProvider: "anthropic",
		Enabled:         true,
	})

	tests := []struct {
		category     string
		wantProvider string
		wantModel    string
	}{
		{"review", "openai", "gpt-4o-mini"},
		{"chat", "ollama", ""},
		{"vision", "anthropic", ""},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			sel := policy.Select(map[string]string{HintKeyCategory: tt.category})
			assert.Equal(t, tt.wantProvider, sel.Provider)
			assert.Equal(t, tt.wantModel, sel.Model)
		})
	}
}
