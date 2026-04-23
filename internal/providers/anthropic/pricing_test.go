// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package anthropic

import (
	"math"
	"testing"

	"siply.dev/siply/internal/core"
)

func TestPricingTokenCounter_NoCacheTokens(t *testing.T) {
	p := &PricingTokenCounter{}
	usage := core.TokenUsage{InputTokens: 1000, OutputTokens: 500}
	cost, err := p.EstimateCost(usage, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1000 input * $3/M + 500 output * $15/M = $0.003 + $0.0075 = $0.0105
	expected := 0.0105
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("expected cost %f, got %f", expected, cost)
	}
}

func TestPricingTokenCounter_BackwardCompatible(t *testing.T) {
	p := &PricingTokenCounter{}
	// No cache tokens, both 0 — should behave identically to simple input+output pricing.
	usage := core.TokenUsage{InputTokens: 1_000_000, OutputTokens: 0}
	cost, err := p.EstimateCost(usage, "claude-sonnet-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := 3.00 // 1M input tokens * $3/M
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("expected cost %f, got %f", expected, cost)
	}
}

func TestPricingTokenCounter_CacheReadTokens(t *testing.T) {
	p := &PricingTokenCounter{}
	usage := core.TokenUsage{
		InputTokens:          1000,
		OutputTokens:         500,
		CacheReadInputTokens: 5000,
	}
	cost, err := p.EstimateCost(usage, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1000 input * $3/M + 500 output * $15/M + 5000 cache_read * $3/M * 0.10
	// = $0.003 + $0.0075 + $0.0015 = $0.012
	expected := 0.012
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("expected cost %f, got %f", expected, cost)
	}
}

func TestPricingTokenCounter_CacheCreationTokens(t *testing.T) {
	p := &PricingTokenCounter{}
	usage := core.TokenUsage{
		InputTokens:              1000,
		OutputTokens:             500,
		CacheCreationInputTokens: 2000,
	}
	cost, err := p.EstimateCost(usage, "claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1000 input * $3/M + 500 output * $15/M + 2000 cache_write * $3/M * 1.25
	// = $0.003 + $0.0075 + $0.0075 = $0.018
	expected := 0.018
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("expected cost %f, got %f", expected, cost)
	}
}

func TestPricingTokenCounter_OpusModel(t *testing.T) {
	p := &PricingTokenCounter{}
	usage := core.TokenUsage{
		InputTokens:          1000,
		OutputTokens:         500,
		CacheReadInputTokens: 3000,
	}
	cost, err := p.EstimateCost(usage, "claude-opus-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1000 * $15/M + 500 * $75/M + 3000 * $15/M * 0.10
	// = $0.015 + $0.0375 + $0.0045 = $0.057
	expected := 0.057
	if math.Abs(cost-expected) > 1e-9 {
		t.Fatalf("expected cost %f, got %f", expected, cost)
	}
}

func TestPricingTokenCounter_UnknownModel(t *testing.T) {
	p := &PricingTokenCounter{}
	usage := core.TokenUsage{InputTokens: 1000, OutputTokens: 500}
	cost, err := p.EstimateCost(usage, "unknown-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cost != 0.0 {
		t.Fatalf("expected 0.0 for unknown model, got %f", cost)
	}
}

func TestPricingTokenCounter_Count(t *testing.T) {
	p := &PricingTokenCounter{}
	count, err := p.Count("hello world", "claude-sonnet-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestStripDateSuffix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"claude-sonnet-4-20250514", "claude-sonnet-4"},
		{"claude-opus-4-6-20250514", "claude-opus-4-6"},
		{"claude-sonnet-4", "claude-sonnet-4"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"}, // "6" is < 6 chars, not a date
	}
	for _, tt := range tests {
		got := stripDateSuffix(tt.input)
		if got != tt.want {
			t.Errorf("stripDateSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripGenerationSuffix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"claude-sonnet-4-6", "claude-sonnet-4"},
		{"claude-opus-4-6", "claude-opus-4"},
		{"claude-sonnet-4", "claude-sonnet"},                     // "4" is short numeric, gets stripped
		{"claude-sonnet-4-20250514", "claude-sonnet-4-20250514"}, // date suffix (6+ digits), not gen
	}
	for _, tt := range tests {
		got := stripGenerationSuffix(tt.input)
		if got != tt.want {
			t.Errorf("stripGenerationSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
