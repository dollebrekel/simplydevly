// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"siply.dev/siply/pkg/siplyui"
)

func TestMarkdown_Headers(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	out := mv.Render("# H1\n## H2\n### H3", 80)
	if !strings.Contains(out, "H1") || !strings.Contains(out, "H2") || !strings.Contains(out, "H3") {
		t.Errorf("expected H1/H2/H3 in output, got: %q", out)
	}
}

func TestMarkdown_CodeBlock(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	out := mv.Render("```\nfoo bar\n```", 80)
	if !strings.Contains(out, "foo bar") {
		t.Errorf("expected code block content, got: %q", out)
	}
}

func TestMarkdown_Table(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	input := "| A | B |\n|---|---|\n| 1 | 2 |"
	out := mv.Render(input, 80)
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Errorf("expected table headers in output, got: %q", out)
	}
	if !strings.Contains(out, "1") || !strings.Contains(out, "2") {
		t.Errorf("expected table cells in output, got: %q", out)
	}
}

func TestMarkdown_NestedList(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	input := "- item1\n  - nested\n    - deep"
	out := mv.Render(input, 80)
	if !strings.Contains(out, "item1") || !strings.Contains(out, "nested") || !strings.Contains(out, "deep") {
		t.Errorf("expected nested list items in output, got: %q", out)
	}
}

func TestMarkdown_HorizontalRule(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	out := mv.Render("before\n---\nafter", 20)
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Errorf("expected content around HR, got: %q", out)
	}
	// Should contain a run of dashes or ─.
	if !strings.Contains(out, "---") && !strings.Contains(out, "───") {
		t.Errorf("expected HR line in output, got: %q", out)
	}
}

func TestMarkdown_Link(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	out := mv.Render("[click here](https://example.com)", 80)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "click here") {
		t.Errorf("expected link text in output, got: %q", out)
	}
	if !strings.Contains(plain, "example.com") {
		t.Errorf("expected URL in output, got: %q", out)
	}
}

func TestMarkdown_BoldItalic(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	out := mv.Render("**bold** and *italic*", 80)
	if !strings.Contains(out, "bold") || !strings.Contains(out, "italic") {
		t.Errorf("expected bold and italic text, got: %q", out)
	}
}

func TestMarkdown_InlineCode(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	out := mv.Render("Use `fmt.Println` for output", 80)
	if !strings.Contains(out, "fmt.Println") {
		t.Errorf("expected inline code in output, got: %q", out)
	}
}

func TestMarkdown_EmptyInput(t *testing.T) {
	theme, cfg := defaultTestSetup()
	mv := siplyui.NewMarkdownView(theme, cfg)
	if out := mv.Render("", 80); out != "" {
		t.Errorf("expected empty output for empty input, got %q", out)
	}
}
