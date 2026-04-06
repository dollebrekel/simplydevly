// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const maxSearchResults = 100

// SearchTool performs code search using ripgrep (with grep fallback).
type SearchTool struct{}

type searchInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Include string `json:"include,omitempty"`
}

func (t *SearchTool) Name() string        { return "search" }
func (t *SearchTool) Description() string { return "Search code using ripgrep" }
func (t *SearchTool) Destructive() bool   { return false }
func (t *SearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Search pattern (regex)"},"path":{"type":"string","description":"Directory or file to search in (default: cwd)"},"include":{"type":"string","description":"File glob pattern to include (e.g. *.go)"}},"required":["pattern"]}`)
}

func (t *SearchTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params searchInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("search: invalid input: %w", err)
	}
	if params.Pattern == "" {
		return "", fmt.Errorf("search: pattern is required")
	}

	output, err := t.tryRipgrep(ctx, params)
	if err != nil {
		// If rg not found, fall back to grep.
		if isNotFoundError(err) {
			return t.tryGrep(ctx, params)
		}
		return output, err
	}
	return output, nil
}

func (t *SearchTool) tryRipgrep(ctx context.Context, params searchInput) (string, error) {
	args := []string{"-n", "--max-count", fmt.Sprintf("%d", maxSearchResults)}

	if params.Include != "" {
		args = append(args, "--glob", params.Include)
	}

	args = append(args, params.Pattern)

	if params.Path != "" {
		args = append(args, params.Path)
	}

	cmd := exec.CommandContext(ctx, "rg", args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 means no matches — not an error.
			if exitErr.ExitCode() == 1 {
				return "No matches found", nil
			}
		}
		return output, fmt.Errorf("search: rg: %w", err)
	}

	return truncateSearchOutput(output), nil
}

func (t *SearchTool) tryGrep(ctx context.Context, params searchInput) (string, error) {
	args := []string{"-rn", "-m", fmt.Sprintf("%d", maxSearchResults)}

	if params.Include != "" {
		args = append(args, "--include", params.Include)
	}

	args = append(args, params.Pattern)

	searchPath := "."
	if params.Path != "" {
		searchPath = params.Path
	}
	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, "grep", args...)
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return "No matches found", nil
			}
		}
		return output, fmt.Errorf("search: grep: %w", err)
	}

	return truncateSearchOutput(output), nil
}

func truncateSearchOutput(output string) string {
	output = strings.TrimRight(output, "\n")
	lines := strings.Split(output, "\n")
	if len(lines) > maxSearchResults {
		truncated := strings.Join(lines[:maxSearchResults], "\n")
		return truncated + fmt.Sprintf("\n... truncated (%d total matches, showing %d)", len(lines), maxSearchResults)
	}
	return output
}

func isNotFoundError(err error) bool {
	if execErr, ok := err.(*exec.Error); ok {
		return execErr.Err == exec.ErrNotFound
	}
	return false
}
