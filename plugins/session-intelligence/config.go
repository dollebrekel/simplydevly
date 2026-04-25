// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"net/url"
)

type Config struct {
	Enabled             bool
	Model               string
	OllamaURL           string
	MinTurns            int
	MaxDistillates      int
	ConsolidationTokens int
}

func DefaultConfig() Config {
	return Config{
		Enabled:             true,
		Model:               "qwen3:0.6b-q8_0",
		OllamaURL:           "http://localhost:11434",
		MinTurns:            5,
		MaxDistillates:      10,
		ConsolidationTokens: 800,
	}
}

func ValidateConfig(c Config) error {
	if c.Model == "" {
		return &ConfigError{Field: "model", Message: "model name must not be empty"}
	}
	parsed, err := url.ParseRequestURI(c.OllamaURL)
	if err != nil {
		return &ConfigError{Field: "ollama_url", Message: "invalid URL: " + err.Error()}
	}
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return &ConfigError{Field: "ollama_url", Message: "must be localhost (localhost, 127.0.0.1, or ::1)"}
	}
	if c.MinTurns < 1 {
		return &ConfigError{Field: "min_turns", Message: "must be at least 1"}
	}
	if c.MaxDistillates < 2 {
		return &ConfigError{Field: "max_distillates", Message: "must be at least 2"}
	}
	if c.ConsolidationTokens < 100 {
		return &ConfigError{Field: "consolidation_tokens", Message: "must be at least 100"}
	}
	return nil
}

type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config." + e.Field + ": " + e.Message
}
