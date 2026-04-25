// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("default should be enabled")
	}
	if cfg.Model != "qwen3:0.6b-q8_0" {
		t.Errorf("unexpected default model: %s", cfg.Model)
	}
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("unexpected default URL: %s", cfg.OllamaURL)
	}
	if cfg.MinTurns != 5 {
		t.Errorf("unexpected default min turns: %d", cfg.MinTurns)
	}
	if cfg.MaxDistillates != 10 {
		t.Errorf("unexpected default max distillates: %d", cfg.MaxDistillates)
	}
	if cfg.ConsolidationTokens != 800 {
		t.Errorf("unexpected default consolidation tokens: %d", cfg.ConsolidationTokens)
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	if err := ValidateConfig(DefaultConfig()); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidateConfig_EmptyModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = ""
	if err := ValidateConfig(cfg); err == nil {
		t.Error("empty model should be invalid")
	}
}

func TestValidateConfig_InvalidURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OllamaURL = "not a url"
	if err := ValidateConfig(cfg); err == nil {
		t.Error("invalid URL should be rejected")
	}
}

func TestValidateConfig_NonLocalhostURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OllamaURL = "http://192.168.1.100:11434"
	if err := ValidateConfig(cfg); err == nil {
		t.Error("non-localhost URL should be rejected")
	}
}

func TestValidateConfig_LocahostVariants(t *testing.T) {
	for _, url := range []string{
		"http://localhost:11434",
		"http://127.0.0.1:11434",
		"http://[::1]:11434",
	} {
		cfg := DefaultConfig()
		cfg.OllamaURL = url
		if err := ValidateConfig(cfg); err != nil {
			t.Errorf("localhost variant %q should be valid: %v", url, err)
		}
	}
}

func TestValidateConfig_MinTurnsTooLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinTurns = 0
	if err := ValidateConfig(cfg); err == nil {
		t.Error("min_turns=0 should be invalid")
	}
}

func TestValidateConfig_MaxDistillatesTooLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxDistillates = 1
	if err := ValidateConfig(cfg); err == nil {
		t.Error("max_distillates=1 should be invalid")
	}
}

func TestValidateConfig_ConsolidationTokensTooLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConsolidationTokens = 50
	if err := ValidateConfig(cfg); err == nil {
		t.Error("consolidation_tokens=50 should be invalid")
	}
}

func TestConfigError(t *testing.T) {
	err := &ConfigError{Field: "model", Message: "must not be empty"}
	if err.Error() != "config.model: must not be empty" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}
