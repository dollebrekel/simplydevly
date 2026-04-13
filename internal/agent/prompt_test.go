// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptAssembler_BaseOnly(t *testing.T) {
	pa := NewPromptAssembler("base prompt", "", "")
	result := pa.Assemble()
	if result != "base prompt" {
		t.Fatalf("expected 'base prompt', got %q", result)
	}
}

func TestPromptAssembler_WithProjectCLAUDEmd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Project instructions here"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := NewPromptAssembler("base prompt", dir, "")
	result := pa.Assemble()

	if !strings.Contains(result, "base prompt") {
		t.Error("expected base prompt in result")
	}
	if !strings.Contains(result, "Project instructions here") {
		t.Error("expected project instructions in result")
	}
	if !strings.Contains(result, "# Project Instructions") {
		t.Error("expected '# Project Instructions' header")
	}
}

func TestPromptAssembler_WithDotClaudeCLAUDEmd(t *testing.T) {
	dir := t.TempDir()
	dotClaude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(dotClaude, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotClaude, "CLAUDE.md"), []byte("Nested instructions"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := NewPromptAssembler("base prompt", dir, "")
	result := pa.Assemble()

	if !strings.Contains(result, "Nested instructions") {
		t.Error("expected nested CLAUDE.md instructions in result")
	}
}

func TestPromptAssembler_RootCLAUDEmdTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	// Create both CLAUDE.md and .claude/CLAUDE.md
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Root level"), 0644); err != nil {
		t.Fatal(err)
	}
	dotClaude := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(dotClaude, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotClaude, "CLAUDE.md"), []byte("Nested level"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := NewPromptAssembler("base prompt", dir, "")
	result := pa.Assemble()

	if !strings.Contains(result, "Root level") {
		t.Error("expected root CLAUDE.md to be loaded")
	}
	if strings.Contains(result, "Nested level") {
		t.Error("should not load nested CLAUDE.md when root exists")
	}
}

func TestPromptAssembler_WithUserGlobal(t *testing.T) {
	homeDir := t.TempDir()
	dotClaude := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(dotClaude, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotClaude, "CLAUDE.md"), []byte("Global user prefs"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := NewPromptAssembler("base prompt", "", homeDir)
	result := pa.Assemble()

	if !strings.Contains(result, "Global user prefs") {
		t.Error("expected global user instructions in result")
	}
	if !strings.Contains(result, "# User Instructions") {
		t.Error("expected '# User Instructions' header")
	}
}

func TestPromptAssembler_FullStack(t *testing.T) {
	// Set up home dir with global instructions
	homeDir := t.TempDir()
	homeClaude := filepath.Join(homeDir, ".claude")
	if err := os.MkdirAll(homeClaude, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeClaude, "CLAUDE.md"), []byte("Global rules"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up project dir with project instructions
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "CLAUDE.md"), []byte("Project rules"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := NewPromptAssembler("base prompt", projectDir, homeDir)
	result := pa.Assemble()

	// Verify ordering: base, user global, project
	baseIdx := strings.Index(result, "base prompt")
	globalIdx := strings.Index(result, "Global rules")
	projectIdx := strings.Index(result, "Project rules")

	if baseIdx < 0 || globalIdx < 0 || projectIdx < 0 {
		t.Fatalf("missing sections in result: %q", result)
	}
	if baseIdx >= globalIdx {
		t.Error("base prompt should come before global instructions")
	}
	if globalIdx >= projectIdx {
		t.Error("global instructions should come before project instructions")
	}
}

func TestPromptAssembler_NoCLAUDEmd(t *testing.T) {
	emptyDir := t.TempDir()
	pa := NewPromptAssembler("base prompt", emptyDir, emptyDir)
	result := pa.Assemble()

	// Should just be the base prompt when no instruction files exist
	if result != "base prompt" {
		t.Fatalf("expected just base prompt, got %q", result)
	}
}

func TestPromptAssembler_Stability(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("Stable content"), 0644); err != nil {
		t.Fatal(err)
	}

	pa := NewPromptAssembler("base", dir, "")
	result1 := pa.Assemble()
	result2 := pa.Assemble()

	if result1 != result2 {
		t.Error("prompt assembler should produce identical results across calls (cache-friendly)")
	}
}
