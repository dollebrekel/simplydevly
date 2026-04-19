// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"errors"
	"testing"
)

func TestParseProfileConfig_Valid(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{
				"name":     "memory-default",
				"version":  "0.1.0",
				"category": "plugins",
				"pinned":   true,
			},
		},
	}
	p, err := parseProfileConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(p.Items))
	}
	if p.Items[0].Name != "memory-default" {
		t.Errorf("name = %q", p.Items[0].Name)
	}
	if p.Items[0].Category != "plugins" {
		t.Errorf("category = %q", p.Items[0].Category)
	}
	if !p.Items[0].Pinned {
		t.Errorf("expected pinned = true")
	}
}

func TestParseProfileConfig_AllFields(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{
				"name":     "skill-a",
				"version":  "1.0.0",
				"category": "skills",
				"pinned":   false,
			},
			map[string]any{
				"name":     "agent-b",
				"version":  "2.0.0",
				"category": "agents",
				"pinned":   true,
			},
		},
		"config": map[string]any{
			"provider": map[string]any{
				"default": "anthropic",
				"model":   "claude-sonnet-4-6",
			},
		},
	}
	p, err := parseProfileConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(p.Items))
	}
	if p.Config == nil {
		t.Fatal("expected config to be set")
	}
	if p.Config.Provider.Default != "anthropic" {
		t.Errorf("provider.default = %q", p.Config.Provider.Default)
	}
}

func TestParseProfileConfig_NilData(t *testing.T) {
	_, err := parseProfileConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}
	if !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("expected ErrInvalidProfile, got %v", err)
	}
}

func TestParseProfileConfig_EmptyItems(t *testing.T) {
	data := map[string]any{
		"items": []any{},
	}
	_, err := parseProfileConfig(data)
	if err == nil {
		t.Fatal("expected error for empty items")
	}
	if !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("expected ErrInvalidProfile, got %v", err)
	}
}

func TestParseProfileConfig_MissingItems(t *testing.T) {
	data := map[string]any{}
	_, err := parseProfileConfig(data)
	if err == nil {
		t.Fatal("expected error for missing items")
	}
}

func TestParseProfileConfig_InvalidCategory(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{
				"name":     "foo",
				"version":  "1.0.0",
				"category": "bundles", // invalid
				"pinned":   true,
			},
		},
	}
	_, err := parseProfileConfig(data)
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
	if !errors.Is(err, ErrInvalidProfile) {
		t.Errorf("expected ErrInvalidProfile, got %v", err)
	}
}

func TestParseProfileConfig_MissingName(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{
				"version":  "1.0.0",
				"category": "plugins",
			},
		},
	}
	_, err := parseProfileConfig(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseProfileConfig_MissingVersion(t *testing.T) {
	data := map[string]any{
		"items": []any{
			map[string]any{
				"name":     "foo",
				"category": "plugins",
			},
		},
	}
	_, err := parseProfileConfig(data)
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}
