// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// InstructionFile names that are discovered and loaded into the system prompt.
// These follow the industry-standard convention used by Claude Code, Cursor, etc.
var instructionFileNames = []string{
	"CLAUDE.md",
	".claude/CLAUDE.md",
}

// PromptAssembler builds the system prompt by combining the base prompt with
// project instruction files (CLAUDE.md) discovered in the workspace.
type PromptAssembler struct {
	basePrompt string
	projectDir string // workspace root directory (git root)
	homeDir    string // user home directory for ~/.claude/CLAUDE.md
}

// NewPromptAssembler creates a new assembler. projectDir may be empty if no
// workspace is detected. homeDir may be empty to skip user-global instructions.
func NewPromptAssembler(basePrompt, projectDir, homeDir string) *PromptAssembler {
	return &PromptAssembler{
		basePrompt: basePrompt,
		projectDir: projectDir,
		homeDir:    homeDir,
	}
}

// Assemble builds the full system prompt by discovering and appending instruction
// files. The resulting prompt is deterministic and stable (no timestamps or
// dynamic content) so it can be effectively cached by the Anthropic API.
//
// Prompt structure:
//  1. Base prompt (hardcoded default)
//  2. User-global instructions (~/.claude/CLAUDE.md) if present
//  3. Project instructions (CLAUDE.md, .claude/CLAUDE.md) if present
func (pa *PromptAssembler) Assemble() string {
	var sections []string
	sections = append(sections, pa.basePrompt)

	// User-global instructions: ~/.claude/CLAUDE.md
	if pa.homeDir != "" {
		globalPath := filepath.Join(pa.homeDir, ".claude", "CLAUDE.md")
		if content, err := readInstructionFile(globalPath); err == nil && content != "" {
			sections = append(sections, fmt.Sprintf("# User Instructions\n\n%s", content))
			slog.Info("prompt: loaded user instructions", "path", globalPath, "bytes", len(content))
		}
	}

	// Project instructions: search from workspace root
	if pa.projectDir != "" {
		for _, name := range instructionFileNames {
			path := filepath.Join(pa.projectDir, name)
			if content, err := readInstructionFile(path); err == nil && content != "" {
				sections = append(sections, fmt.Sprintf("# Project Instructions\n\n%s", content))
				slog.Info("prompt: loaded project instructions", "path", path, "bytes", len(content))
				break // only load the first match
			}
		}
	}

	return strings.Join(sections, "\n\n---\n\n")
}

// readInstructionFile reads a file if it exists, returning empty string if not found.
func readInstructionFile(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("prompt: failed to read %s: %w", path, err)
	}

	content := strings.TrimSpace(string(data))
	return content, nil
}
