// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"strings"
	"testing"

	"siply.dev/siply/internal/core"
)

func ptr[T any](v T) *T { return &v }

func TestValidateConfig_RoutingDisabled_NoWarning(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{Enabled: ptr(false)}, 1, nil)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateConfig_RoutingEnabledOneProvider(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "ollama"},
		},
	}, 1, []string{"ollama"})
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "only 1 provider") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestValidateConfig_PreferCheapestNoPricing(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{
		Enabled:        ptr(true),
		PreferCheapest: ptr(true),
	}, 2, nil)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "no pricing") {
		t.Errorf("unexpected warning: %s", warnings[0])
	}
}

func TestValidateConfig_DuplicateRules(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "ollama"},
			{Category: "preprocess", Provider: "openai"},
		},
	}, 2, []string{"ollama", "openai"})
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate warning, got: %v", warnings)
	}
}

func TestValidateConfig_ValidConfig(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{
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
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for valid config, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateConfig_UnknownCategory(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "priamry", Provider: "anthropic"},
		},
	}, 2, []string{"anthropic", "ollama"})
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "unknown category") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown category warning, got: %v", warnings)
	}
}

func TestValidateConfig_UnknownProvider(t *testing.T) {
	warnings := ValidateConfig(core.RoutingConfig{
		Enabled: ptr(true),
		Rules: []core.RoutingRule{
			{Category: "preprocess", Provider: "nonexistent"},
		},
	}, 2, []string{"anthropic", "ollama"})
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "unknown provider") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected unknown provider warning, got: %v", warnings)
	}
}
