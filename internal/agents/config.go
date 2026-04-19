// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agents

import (
	"errors"
	"fmt"
)

// Sentinel errors for agent config operations.
var (
	ErrAgentConfigNotFound = errors.New("agents: agent config not found")
	ErrNoAgentConfig       = errors.New("agents: agent config has no config.yaml")
	ErrInvalidAgentConfig  = errors.New("agents: invalid agent config")
)

// parseAgentConfig extracts an AgentProfile from a parsed YAML document.
// All fields are optional — a minimal config may contain only system_prompt.
func parseAgentConfig(data map[string]any) (*AgentProfile, error) {
	if data == nil {
		return nil, fmt.Errorf("%w: empty YAML document", ErrInvalidAgentConfig)
	}

	p := &AgentProfile{}

	if v, ok := data["system_prompt"].(string); ok {
		p.SystemPrompt = v
	}

	if modelRaw, ok := data["model"]; ok {
		m, ok := modelRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'model' must be a mapping, got %T", ErrInvalidAgentConfig, modelRaw)
		}
		if err := parseModelPrefs(m, &p.ModelPreferences); err != nil {
			return nil, err
		}
	}

	if behavRaw, ok := data["behavior"]; ok {
		b, ok := behavRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'behavior' must be a mapping, got %T", ErrInvalidAgentConfig, behavRaw)
		}
		if err := parseBehaviorPrefs(b, &p.BehaviorPresets); err != nil {
			return nil, err
		}
	}

	if toolsRaw, ok := data["tools"]; ok {
		t, ok := toolsRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'tools' must be a mapping, got %T", ErrInvalidAgentConfig, toolsRaw)
		}
		if err := parseToolRules(t, &p.ToolRestrictions); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func parseModelPrefs(m map[string]any, out *ModelPrefs) error {
	if v, ok := m["provider"].(string); ok {
		out.Provider = v
	}
	if v, ok := m["model"].(string); ok {
		out.Model = v
	}
	if v, ok := m["temperature"]; ok {
		f, err := toFloat64(v)
		if err != nil {
			return fmt.Errorf("%w: 'model.temperature' must be a number: %s", ErrInvalidAgentConfig, err)
		}
		out.Temperature = &f
	}
	if v, ok := m["max_tokens"]; ok {
		n, err := toInt(v)
		if err != nil {
			return fmt.Errorf("%w: 'model.max_tokens' must be an integer: %s", ErrInvalidAgentConfig, err)
		}
		out.MaxTokens = &n
	}
	return nil
}

func parseBehaviorPrefs(b map[string]any, out *BehaviorPrefs) error {
	if v, ok := b["parallel_tools"]; ok {
		bv, err := toBool(v)
		if err != nil {
			return fmt.Errorf("%w: 'behavior.parallel_tools' must be a boolean: %s", ErrInvalidAgentConfig, err)
		}
		out.ParallelTools = &bv
	}
	if v, ok := b["max_iterations"]; ok {
		n, err := toInt(v)
		if err != nil {
			return fmt.Errorf("%w: 'behavior.max_iterations' must be an integer: %s", ErrInvalidAgentConfig, err)
		}
		out.MaxIterations = &n
	}
	if v, ok := b["auto_approve"]; ok {
		bv, err := toBool(v)
		if err != nil {
			return fmt.Errorf("%w: 'behavior.auto_approve' must be a boolean: %s", ErrInvalidAgentConfig, err)
		}
		out.AutoApprove = &bv
	}
	return nil
}

func parseToolRules(t map[string]any, out *ToolRules) error {
	if v, ok := t["allowed"]; ok {
		list, err := toStringSlice(v, "tools.allowed")
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidAgentConfig, err)
		}
		out.Allowed = list
	}
	if v, ok := t["denied"]; ok {
		list, err := toStringSlice(v, "tools.denied")
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidAgentConfig, err)
		}
		out.Denied = list
	}
	return nil
}

// toFloat64 converts an any value to float64.
// YAML numbers may be int or float depending on whether they have a decimal point.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("got %T", v)
	}
}

// toInt converts an any value to int.
func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		i := int(n)
		if float64(i) != n {
			return 0, fmt.Errorf("fractional value %v cannot be converted to int", n)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("got %T", v)
	}
}

// toBool converts an any value to bool.
func toBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("got %T", v)
	}
	return b, nil
}

// toStringSlice converts an any value to []string.
func toStringSlice(v any, field string) ([]string, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("'%s' must be a list, got %T", field, v)
	}
	result := make([]string, 0, len(raw))
	for i, elem := range raw {
		s, ok := elem.(string)
		if !ok {
			return nil, fmt.Errorf("'%s[%d]' must be a string, got %T", field, i, elem)
		}
		result = append(result, s)
	}
	return result, nil
}
