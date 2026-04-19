// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agents

import (
	"errors"
	"testing"
)

func TestParseAgentConfig_ValidFull(t *testing.T) {
	data := map[string]any{
		"system_prompt": "You are a code reviewer.",
		"model": map[string]any{
			"provider":    "anthropic",
			"model":       "claude-sonnet-4-6",
			"temperature": 0.3,
			"max_tokens":  8192,
		},
		"behavior": map[string]any{
			"parallel_tools": true,
			"max_iterations": 20,
			"auto_approve":   false,
		},
		"tools": map[string]any{
			"allowed": []any{},
			"denied":  []any{"bash_dangerous"},
		},
	}

	p, err := parseAgentConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.SystemPrompt != "You are a code reviewer." {
		t.Errorf("system_prompt = %q", p.SystemPrompt)
	}
	if p.ModelPreferences.Provider != "anthropic" {
		t.Errorf("model.provider = %q", p.ModelPreferences.Provider)
	}
	if p.ModelPreferences.Model != "claude-sonnet-4-6" {
		t.Errorf("model.model = %q", p.ModelPreferences.Model)
	}
	if p.ModelPreferences.Temperature == nil || *p.ModelPreferences.Temperature != 0.3 {
		t.Errorf("model.temperature = %v", p.ModelPreferences.Temperature)
	}
	if p.ModelPreferences.MaxTokens == nil || *p.ModelPreferences.MaxTokens != 8192 {
		t.Errorf("model.max_tokens = %v", p.ModelPreferences.MaxTokens)
	}
	if p.BehaviorPresets.ParallelTools == nil || *p.BehaviorPresets.ParallelTools != true {
		t.Errorf("behavior.parallel_tools = %v", p.BehaviorPresets.ParallelTools)
	}
	if p.BehaviorPresets.MaxIterations == nil || *p.BehaviorPresets.MaxIterations != 20 {
		t.Errorf("behavior.max_iterations = %v", p.BehaviorPresets.MaxIterations)
	}
	if p.BehaviorPresets.AutoApprove == nil || *p.BehaviorPresets.AutoApprove != false {
		t.Errorf("behavior.auto_approve = %v", p.BehaviorPresets.AutoApprove)
	}
	if len(p.ToolRestrictions.Allowed) != 0 {
		t.Errorf("tools.allowed = %v", p.ToolRestrictions.Allowed)
	}
	if len(p.ToolRestrictions.Denied) != 1 || p.ToolRestrictions.Denied[0] != "bash_dangerous" {
		t.Errorf("tools.denied = %v", p.ToolRestrictions.Denied)
	}
}

func TestParseAgentConfig_MinimalSystemPromptOnly(t *testing.T) {
	data := map[string]any{
		"system_prompt": "You are helpful.",
	}
	p, err := parseAgentConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.SystemPrompt != "You are helpful." {
		t.Errorf("system_prompt = %q", p.SystemPrompt)
	}
	if p.ModelPreferences.Provider != "" {
		t.Errorf("expected empty model prefs, got %+v", p.ModelPreferences)
	}
}

func TestParseAgentConfig_EmptyMapOK(t *testing.T) {
	data := map[string]any{}
	p, err := parseAgentConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil profile")
	}
}

func TestParseAgentConfig_NilDataError(t *testing.T) {
	_, err := parseAgentConfig(nil)
	if !errors.Is(err, ErrInvalidAgentConfig) {
		t.Errorf("expected ErrInvalidAgentConfig, got %v", err)
	}
}

func TestParseAgentConfig_InvalidModelType(t *testing.T) {
	data := map[string]any{
		"model": "not-a-map",
	}
	_, err := parseAgentConfig(data)
	if !errors.Is(err, ErrInvalidAgentConfig) {
		t.Errorf("expected ErrInvalidAgentConfig, got %v", err)
	}
}

func TestParseAgentConfig_InvalidBehaviorType(t *testing.T) {
	data := map[string]any{
		"behavior": 42,
	}
	_, err := parseAgentConfig(data)
	if !errors.Is(err, ErrInvalidAgentConfig) {
		t.Errorf("expected ErrInvalidAgentConfig, got %v", err)
	}
}

func TestParseAgentConfig_InvalidToolsType(t *testing.T) {
	data := map[string]any{
		"tools": "not-a-map",
	}
	_, err := parseAgentConfig(data)
	if !errors.Is(err, ErrInvalidAgentConfig) {
		t.Errorf("expected ErrInvalidAgentConfig, got %v", err)
	}
}

func TestParseAgentConfig_InvalidTemperatureType(t *testing.T) {
	data := map[string]any{
		"model": map[string]any{
			"temperature": "not-a-number",
		},
	}
	_, err := parseAgentConfig(data)
	if !errors.Is(err, ErrInvalidAgentConfig) {
		t.Errorf("expected ErrInvalidAgentConfig, got %v", err)
	}
}

func TestParseAgentConfig_InvalidMaxTokensType(t *testing.T) {
	data := map[string]any{
		"model": map[string]any{
			"max_tokens": "not-an-int",
		},
	}
	_, err := parseAgentConfig(data)
	if !errors.Is(err, ErrInvalidAgentConfig) {
		t.Errorf("expected ErrInvalidAgentConfig, got %v", err)
	}
}

func TestParseAgentConfig_IntAsFloat64Temperature(t *testing.T) {
	data := map[string]any{
		"model": map[string]any{
			"temperature": 1,
		},
	}
	p, err := parseAgentConfig(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ModelPreferences.Temperature == nil || *p.ModelPreferences.Temperature != 1.0 {
		t.Errorf("temperature = %v", p.ModelPreferences.Temperature)
	}
}
