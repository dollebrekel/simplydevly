// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCountConversationTurns(t *testing.T) {
	tests := []struct {
		name string
		msgs []Message
		want int
	}{
		{"empty", nil, 0},
		{"system only", []Message{{Role: "system", Content: "hello"}}, 0},
		{"one user", []Message{{Role: "user", Content: "hi"}}, 1},
		{"user+assistant", []Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		}, 2},
		{"mixed with system", []Message{
			{Role: "system", Content: "system msg"},
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
			{Role: "user", Content: "bye"},
		}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countConversationTurns(tt.msgs)
			if got != tt.want {
				t.Errorf("countConversationTurns() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSplitMessages_LessThanKeepTurns(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	older, recent := splitMessages(msgs, 3)
	if len(older) != 0 {
		t.Errorf("expected no older messages, got %d", len(older))
	}
	if len(recent) != 2 {
		t.Errorf("expected 2 recent messages, got %d", len(recent))
	}
}

func TestSplitMessages_ExactKeepTurns(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "1"},
		{Role: "assistant", Content: "2"},
		{Role: "user", Content: "3"},
	}
	older, recent := splitMessages(msgs, 3)
	if len(older) != 0 {
		t.Errorf("expected no older messages, got %d", len(older))
	}
	if len(recent) != 3 {
		t.Errorf("expected 3 recent messages, got %d", len(recent))
	}
}

func TestSplitMessages_MoreThanKeepTurns(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "old1"},
		{Role: "assistant", Content: "old2"},
		{Role: "user", Content: "recent1"},
		{Role: "assistant", Content: "recent2"},
		{Role: "user", Content: "recent3"},
	}
	older, recent := splitMessages(msgs, 3)
	if len(older) != 2 {
		t.Errorf("expected 2 older messages, got %d", len(older))
	}
	if len(recent) != 3 {
		t.Errorf("expected 3 recent messages, got %d", len(recent))
	}
	if older[0].Content != "old1" {
		t.Errorf("expected first older to be old1, got %s", older[0].Content)
	}
}

func TestSplitMessages_WithSystemMessages(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "old"},
		{Role: "assistant", Content: "old response"},
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "recent response"},
		{Role: "user", Content: "latest"},
	}
	older, recent := splitMessages(msgs, 3)
	if len(older) != 2 {
		t.Errorf("expected 2 older messages (conversation only, system preserved), got %d", len(older))
	}
	if len(recent) != 4 {
		t.Errorf("expected 4 recent messages (system + 3 conversation), got %d", len(recent))
	}
	if recent[0].Role != "system" {
		t.Errorf("expected first recent message to be preserved system prompt, got %s", recent[0].Role)
	}
}

func TestAssembleResult(t *testing.T) {
	recent := []Message{
		{Role: "user", Content: "latest"},
		{Role: "assistant", Content: "response"},
	}
	result := assembleResult("KEY_DECISIONS: use Go", recent)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Error("first message should be system")
	}
	if result[0].Content != "[Context Distillate]\nKEY_DECISIONS: use Go" {
		t.Errorf("unexpected distillate content: %s", result[0].Content)
	}
	if result[1].Content != "latest" {
		t.Error("recent messages should follow distillate")
	}
}

func TestEstimateTokens(t *testing.T) {
	got := estimateTokens("hello world 1234")
	if got != 4 {
		t.Errorf("estimateTokens() = %d, want 4", got)
	}
}

func TestPreQuery_Passthrough_FewTurns(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	payload, _ := json.Marshal(msgs)

	p := &distillationPlugin{
		config: Config{Enabled: true, KeepTurns: 3},
	}
	p.initialized = true

	resp, err := p.handlePreQuery(context.TODO(), payload)
	if err != nil {
		t.Fatalf("handlePreQuery error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", *resp.Error)
	}

	var result []Message
	json.Unmarshal(resp.Result, &result)
	if len(result) != 2 {
		t.Errorf("expected passthrough (2 messages), got %d", len(result))
	}
}

func TestPreQuery_Passthrough_Disabled(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello"},
	}
	payload, _ := json.Marshal(msgs)

	p := &distillationPlugin{
		config: Config{Enabled: false, KeepTurns: 3},
	}
	p.initialized = true

	resp, err := p.handlePreQuery(context.TODO(), payload)
	if err != nil {
		t.Fatalf("handlePreQuery error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{"valid defaults", DefaultConfig(), false},
		{"empty model", Config{Model: "", OllamaURL: "http://localhost:11434", KeepTurns: 3}, true},
		{"invalid url", Config{Model: "test", OllamaURL: "not-a-url", KeepTurns: 3}, true},
		{"zero keep turns", Config{Model: "test", OllamaURL: "http://localhost:11434", KeepTurns: 0}, true},
		{"custom valid", Config{Model: "phi3", OllamaURL: "http://192.168.1.100:11434", KeepTurns: 5}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
