// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package agents provides agent configuration loading, registry, and scaffolding.
// Agent configs are Tier 1 YAML plugins stored in a dedicated agents/ directory.
// They define system prompt overrides, model preferences, and behavior presets.
package agents

// AgentProfile represents a loaded agent configuration with its metadata and settings.
type AgentProfile struct {
	Name             string
	Version          string
	Description      string
	SystemPrompt     string
	ModelPreferences ModelPrefs
	BehaviorPresets  BehaviorPrefs
	ToolRestrictions ToolRules
	Source           string // "global" or "project"
	Dir              string
}

// ModelPrefs holds model provider and parameter overrides.
// Pointer fields are optional; nil means "use default".
type ModelPrefs struct {
	Provider    string   `yaml:"provider"`
	Model       string   `yaml:"model"`
	Temperature *float64 `yaml:"temperature"`
	MaxTokens   *int     `yaml:"max_tokens"`
}

// BehaviorPrefs holds agent behavior setting overrides.
// Pointer fields are optional; nil means "use default".
type BehaviorPrefs struct {
	ParallelTools *bool `yaml:"parallel_tools"`
	MaxIterations *int  `yaml:"max_iterations"`
	AutoApprove   *bool `yaml:"auto_approve"`
}

// ToolRules restricts the set of tools available to the agent.
// Empty slices mean no restriction (all tools allowed / none denied).
type ToolRules struct {
	Allowed []string `yaml:"allowed"`
	Denied  []string `yaml:"denied"`
}
