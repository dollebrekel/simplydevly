// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"strings"
	"testing"
)

func TestDistiller_BuildPrompt_BasicConversation(t *testing.T) {
	d := NewDistiller(nil, 3)
	msgs := []Message{
		{Role: "user", Content: "Fix the login bug"},
		{Role: "assistant", Content: "I'll look at auth.go"},
		{Role: "user", Content: "Also check the tests"},
	}

	prompt := d.BuildPrompt(msgs)

	if !strings.Contains(prompt, "Summarize this conversation") {
		t.Error("prompt missing summarization instruction")
	}
	if !strings.Contains(prompt, "Fix the login bug") {
		t.Error("prompt missing user message content")
	}
	if !strings.Contains(prompt, "KEY_DECISIONS") {
		t.Error("prompt missing structured output instruction")
	}
}

func TestDistiller_BuildPrompt_SkipsSystemMessages(t *testing.T) {
	d := NewDistiller(nil, 3)
	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	prompt := d.BuildPrompt(msgs)

	if strings.Contains(prompt, "You are a helpful assistant") {
		t.Error("prompt should not include non-code-context system messages")
	}
	if !strings.Contains(prompt, "Hello") {
		t.Error("prompt should include user messages")
	}
}

func TestDistiller_BuildPrompt_IncludesTreeSitterContext(t *testing.T) {
	d := NewDistiller(nil, 3)
	msgs := []Message{
		{Role: "system", Content: "[Code Context — tree-sitter]\nfunc Login(user string)"},
		{Role: "user", Content: "Fix the login"},
		{Role: "assistant", Content: "Looking at it"},
	}

	prompt := d.BuildPrompt(msgs)

	if !strings.Contains(prompt, "CODE CONTEXT") {
		t.Error("prompt should include code context header")
	}
	if !strings.Contains(prompt, "func Login") {
		t.Error("prompt should include tree-sitter symbols")
	}
}

func TestDistiller_BuildPrompt_SkipsToolResults(t *testing.T) {
	d := NewDistiller(nil, 3)
	msgs := []Message{
		{Role: "user", Content: "Read the file"},
		{Role: "user", Content: "file contents here", ToolID: "tool_123"},
		{Role: "assistant", Content: "I see the file"},
	}

	prompt := d.BuildPrompt(msgs)

	if strings.Contains(prompt, "file contents here") {
		t.Error("prompt should not include tool results")
	}
	if !strings.Contains(prompt, "Read the file") {
		t.Error("prompt should include regular user messages")
	}
}

func TestDistiller_BuildPrompt_EmptyMessages(t *testing.T) {
	d := NewDistiller(nil, 3)
	prompt := d.BuildPrompt(nil)

	if !strings.Contains(prompt, "Summarize this conversation") {
		t.Error("prompt should still contain template even with no messages")
	}
}
