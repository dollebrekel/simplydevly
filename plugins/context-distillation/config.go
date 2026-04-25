// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"net/url"
)

type Config struct {
	Enabled   bool
	Model     string
	OllamaURL string
	KeepTurns int
}

func DefaultConfig() Config {
	return Config{
		Enabled:   true,
		Model:     "qwen3:0.6b-q8_0",
		OllamaURL: "http://localhost:11434",
		KeepTurns: 3,
	}
}

func ValidateConfig(c Config) error {
	if c.Model == "" {
		return &ConfigError{Field: "model", Message: "model name must not be empty"}
	}
	if _, err := url.ParseRequestURI(c.OllamaURL); err != nil {
		return &ConfigError{Field: "ollama_url", Message: "invalid URL: " + err.Error()}
	}
	if c.KeepTurns < 1 {
		return &ConfigError{Field: "keep_turns", Message: "must be at least 1"}
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
