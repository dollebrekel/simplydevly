// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAgentSettings_MarshalUnmarshal(t *testing.T) {
	cfg := Config{
		Agent: AgentSettings{
			Config: "code-reviewer",
		},
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var got Config
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if got.Agent.Config != "code-reviewer" {
		t.Errorf("Agent.Config = %q, want code-reviewer", got.Agent.Config)
	}
}

func TestAgentSettings_OmitEmpty(t *testing.T) {
	// When Agent.Config is empty, the field should still marshal/unmarshal correctly.
	cfg := Config{}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	var got Config
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}

	if got.Agent.Config != "" {
		t.Errorf("Agent.Config = %q, want empty", got.Agent.Config)
	}
}

func TestAgentSettings_FromYAML(t *testing.T) {
	yamlData := `
agent:
  config: my-agent
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if cfg.Agent.Config != "my-agent" {
		t.Errorf("Agent.Config = %q, want my-agent", cfg.Agent.Config)
	}
}
