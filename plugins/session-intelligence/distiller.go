// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type SessionDistiller struct {
	client   *OllamaClient
	minTurns int
}

func NewSessionDistiller(client *OllamaClient, minTurns int) *SessionDistiller {
	return &SessionDistiller{client: client, minTurns: minTurns}
}

const sessionDistillationPrompt = `You are summarizing a coding session for future context recovery.
Output ONLY valid JSON with these fields:

{
  "key_decisions": ["decision 1", "decision 2"],
  "active_files": ["path/to/file1.go", "path/to/file2.go"],
  "current_task": "one-line description of current work",
  "constraints": ["constraint 1", "constraint 2"],
  "patterns": [{"pattern": "description", "confidence": "low|medium|high"}]
}

Rules:
- key_decisions: EXACT decisions made, not summaries
- active_files: files read, created, or modified
- current_task: what the user was working on at session end
- constraints: any requirements, limits, or rules mentioned
- patterns: user's coding preferences observed (naming, style, approach)
- Keep total output under 500 tokens
- Be terse and factual

SESSION CONVERSATION:
%s`

func (d *SessionDistiller) DistillSession(ctx context.Context, msgs []Message) (*Distillate, error) {
	turns := countConversationTurns(msgs)
	if turns <= d.minTurns {
		return nil, fmt.Errorf("session too short: %d turns (min %d)", turns, d.minTurns)
	}

	prompt := d.buildPrompt(msgs)

	response, err := d.client.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("ollama distillation: %w", err)
	}

	if strings.TrimSpace(response) == "" {
		return nil, fmt.Errorf("ollama returned empty response")
	}

	content, parseErr := parseDistillateContent(response)
	if parseErr != nil {
		slog.Warn("session-intelligence: parse distillate failed, using raw", "err", parseErr)
		content = &DistillateContent{
			CurrentTask: strings.TrimSpace(response),
		}
	}

	tokenCount := estimateTokens(response)
	const maxTokens = 500
	if tokenCount > maxTokens {
		tokenCount = maxTokens
	}

	return &Distillate{
		Timestamp:  time.Now().UTC(),
		Model:      d.client.model,
		TokenCount: tokenCount,
		Content:    *content,
	}, nil
}

func (d *SessionDistiller) buildPrompt(msgs []Message) string {
	var parts []string
	var treeSitterCtx string

	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "[Code Context") {
			treeSitterCtx = m.Content
			continue
		}
		if m.Role == "system" {
			continue
		}
		if m.ToolID != "" {
			continue
		}
		if m.Role == "user" || m.Role == "assistant" {
			parts = append(parts, fmt.Sprintf("[%s]: %s", m.Role, m.Content))
		}
	}

	conversation := strings.Join(parts, "\n")
	if treeSitterCtx != "" {
		conversation = "CODE CONTEXT:\n" + treeSitterCtx + "\n\n" + conversation
	}

	const maxPromptChars = 32000
	if len(conversation) > maxPromptChars {
		conversation = conversation[len(conversation)-maxPromptChars:]
	}

	return fmt.Sprintf(sessionDistillationPrompt, conversation)
}

func parseDistillateContent(response string) (*DistillateContent, error) {
	response = strings.TrimSpace(response)

	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start >= 0 && end > start {
		response = response[start : end+1]
	}

	var content DistillateContent
	if err := json.Unmarshal([]byte(response), &content); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	return &content, nil
}

func countConversationTurns(msgs []Message) int {
	count := 0
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			count++
		}
	}
	return count
}

func estimateTokens(s string) int {
	return len(s) / 4
}
