// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Consolidator struct {
	client             *OllamaClient
	maxDistillates     int
	consolidationTokens int
}

func NewConsolidator(client *OllamaClient, maxDistillates, consolidationTokens int) *Consolidator {
	return &Consolidator{
		client:             client,
		maxDistillates:     maxDistillates,
		consolidationTokens: consolidationTokens,
	}
}

func (c *Consolidator) ShouldConsolidate(workspacePath string, store *DistillateStore) bool {
	return store.Count(workspacePath) > c.maxDistillates
}

func (c *Consolidator) Consolidate(ctx context.Context, workspacePath string, store *DistillateStore) (*Distillate, error) {
	oldest, err := store.LoadOldest(workspacePath, c.maxDistillates)
	if err != nil {
		return nil, fmt.Errorf("load oldest distillates: %w", err)
	}
	if len(oldest) < 2 {
		return nil, fmt.Errorf("not enough distillates to consolidate: %d", len(oldest))
	}

	c.removeStaleFiles(oldest, workspacePath)

	prompt := c.buildConsolidationPrompt(oldest)
	response, err := c.client.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("ollama consolidation: %w", err)
	}

	content, parseErr := parseDistillateContent(response)
	if parseErr != nil {
		slog.Warn("session-intelligence: consolidation parse failed", "err", parseErr)
		return nil, fmt.Errorf("parse consolidation result: %w", parseErr)
	}

	c.strengthenPatterns(content, oldest)

	tokenCount := estimateTokens(response)
	if tokenCount > c.consolidationTokens {
		tokenCount = c.consolidationTokens
	}

	consolidated := &Distillate{
		SessionID:  "consolidated",
		Workspace:  workspacePath,
		Model:      c.client.model,
		TokenCount: tokenCount,
		Content:    *content,
	}

	var replacedIDs []string
	for _, d := range oldest {
		if d.SessionID != "" && d.SessionID != "consolidated" {
			replacedIDs = append(replacedIDs, d.SessionID)
		}
	}

	if saveErr := store.SaveConsolidated(workspacePath, consolidated, replacedIDs); saveErr != nil {
		return nil, fmt.Errorf("save consolidated: %w", saveErr)
	}

	return consolidated, nil
}

const consolidationPromptTemplate = `Merge these session distillates into a single consolidated summary.

Rules:
- Remove outdated info (files that no longer exist are marked [REMOVED])
- Strengthen patterns mentioned in 3+ sessions — set confidence to "high"
- For conflicting decisions, keep ONLY the most recent
- Keep total output under %d tokens
- Output same JSON format as individual distillates

DISTILLATES (oldest first):
%s`

func (c *Consolidator) buildConsolidationPrompt(distillates []*Distillate) string {
	var parts []string
	for _, d := range distillates {
		data, _ := json.Marshal(d.Content)
		parts = append(parts, string(data))
	}
	return fmt.Sprintf(consolidationPromptTemplate, c.consolidationTokens, strings.Join(parts, "\n---\n"))
}

func (c *Consolidator) removeStaleFiles(distillates []*Distillate, workspacePath string) {
	for _, d := range distillates {
		var validFiles []string
		for _, f := range d.Content.ActiveFiles {
			path := f
			if !strings.HasPrefix(path, "/") {
				path = filepath.Join(workspacePath, f)
			}
			if _, err := os.Stat(path); err == nil {
				validFiles = append(validFiles, f)
			}
		}
		d.Content.ActiveFiles = validFiles
	}
}

func (c *Consolidator) strengthenPatterns(content *DistillateContent, sources []*Distillate) {
	patternCounts := make(map[string]int)
	for _, d := range sources {
		for _, p := range d.Content.Patterns {
			patternCounts[p.Pattern]++
		}
	}

	for i, p := range content.Patterns {
		if patternCounts[p.Pattern] >= 3 {
			content.Patterns[i].Confidence = "high"
		}
	}
}
