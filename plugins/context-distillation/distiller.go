// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"strings"
)

type Distiller struct {
	client    *OllamaClient
	keepTurns int
}

func NewDistiller(client *OllamaClient, keepTurns int) *Distiller {
	return &Distiller{client: client, keepTurns: keepTurns}
}

const distillationTemplate = `Summarize this conversation for an AI coding agent. Output ONLY:
- KEY_DECISIONS: (bullet list of decisions made)
- ACTIVE_FILES: (files being worked on)
- CURRENT_TASK: (one-line description of current work)
- CONSTRAINTS: (any constraints or requirements mentioned)

Keep output under 100 tokens. Be terse.

CONVERSATION:
%s`

func (d *Distiller) BuildPrompt(msgs []Message) string {
	var parts []string

	var treeSitterCtx string
	var conversationParts []string

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
			conversationParts = append(conversationParts, fmt.Sprintf("[%s]: %s", m.Role, m.Content))
		}
	}

	if treeSitterCtx != "" {
		parts = append(parts, "CODE CONTEXT:\n"+treeSitterCtx+"\n")
	}
	parts = append(parts, strings.Join(conversationParts, "\n"))

	return fmt.Sprintf(distillationTemplate, strings.Join(parts, "\n"))
}
