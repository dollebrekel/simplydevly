// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/tui"
)

// helper to create a MarkdownView for standard TrueColor rendering.
func newTestMV() *MarkdownView {
	return NewMarkdownView(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     true,
		Verbosity: tui.VerbosityFull,
	})
}

// helper for no-color mode.
func newNoColorMV() *MarkdownView {
	return NewMarkdownView(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorNone,
		Emoji:     false,
		Verbosity: tui.VerbosityFull,
	})
}

// helper for accessible mode.
func newAccessibleMV() *MarkdownView {
	return NewMarkdownView(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Verbosity: tui.VerbosityAccessible,
	})
}

// helper for no-emoji mode (color on, emoji off).
func newNoEmojiMV() *MarkdownView {
	return NewMarkdownView(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Emoji:     false,
		Verbosity: tui.VerbosityFull,
	})
}

// --- Task 6.14: Interface compliance ---

func TestMarkdownView_ImplementsInterface(t *testing.T) {
	var _ tui.MarkdownRenderer = (*MarkdownView)(nil)
}

// --- Task 6.12: Empty input ---

func TestRender_EmptyInput(t *testing.T) {
	mv := newTestMV()
	assert.Equal(t, "", mv.Render("", 80))
}

// --- Task 6.1: Heading rendering ---

func TestRender_Headings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		text  string
	}{
		{"H1", "# Hello", "Hello"},
		{"H2", "## World", "World"},
		{"H3", "### Details", "Details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := newTestMV()
			result := mv.Render(tt.input, 80)
			stripped := ansi.Strip(result)
			assert.Contains(t, stripped, tt.text)
			// In TrueColor mode, headings should have ANSI escapes (bold + color).
			assert.NotEqual(t, stripped, result, "heading should have ANSI styling")
		})
	}
}

func TestRender_Headings_Accessible(t *testing.T) {
	mv := newAccessibleMV()

	tests := []struct {
		name   string
		input  string
		prefix string
		text   string
	}{
		{"H1", "# Hello", "[H1]", "Hello"},
		{"H2", "## World", "[H2]", "World"},
		{"H3", "### Details", "[H3]", "Details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.Render(tt.input, 80)
			assert.Contains(t, result, tt.prefix)
			assert.Contains(t, result, tt.text)
		})
	}
}

// --- Task 6.2: Code block rendering ---

func TestRender_CodeBlock(t *testing.T) {
	mv := newTestMV()
	input := "```\nfmt.Println(\"hello\")\n```"
	result := mv.Render(input, 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "fmt.Println")
}

func TestRender_CodeBlock_Accessible(t *testing.T) {
	mv := newAccessibleMV()
	input := "```\nfmt.Println(\"hello\")\n```"
	result := mv.Render(input, 80)
	assert.Contains(t, result, "[CODE]")
	assert.Contains(t, result, "[/CODE]")
	assert.Contains(t, result, "fmt.Println")
}

// --- Task 6.3: Inline code rendering ---

func TestRender_InlineCode(t *testing.T) {
	mv := newTestMV()
	result := mv.Render("Use `fmt.Println` here", 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "fmt.Println")
	// Should have styling applied.
	assert.NotEqual(t, "Use fmt.Println here", result)
}

// --- Task 6.4: List rendering ---

func TestRender_ListItems(t *testing.T) {
	mv := newTestMV()

	tests := []struct {
		name  string
		input string
	}{
		{"dash", "- item one"},
		{"star", "* item two"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.Render(tt.input, 80)
			stripped := ansi.Strip(result)
			// Emoji on: bullet should be •
			assert.Contains(t, stripped, "•")
		})
	}
}

func TestRender_ListItems_Accessible(t *testing.T) {
	mv := newAccessibleMV()
	result := mv.Render("- item one", 80)
	assert.Contains(t, result, "- item one")
	assert.NotContains(t, result, "•")
}

// --- Task 6.5: Bold rendering ---

func TestRender_Bold(t *testing.T) {
	mv := newTestMV()
	result := mv.Render("This is **bold** text", 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "bold")
	assert.NotContains(t, stripped, "**")
}

// --- Task 6.6: Italic rendering ---

func TestRender_Italic(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"star", "This is *italic* text"},
		{"underscore", "This is _italic_ text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := newTestMV()
			result := mv.Render(tt.input, 80)
			stripped := ansi.Strip(result)
			assert.Contains(t, stripped, "italic")
		})
	}
}

// --- Task 6.7: No-color mode ---

func TestRender_NoColor_ZeroANSIColorCodes(t *testing.T) {
	mv := newNoColorMV()

	inputs := []struct {
		name  string
		input string
	}{
		{"heading", "# Hello"},
		{"code block", "```\ncode\n```"},
		{"list", "- item"},
		{"bold", "**bold**"},
		{"italic", "*italic*"},
		{"inline code", "`code`"},
		{"mixed", "# Title\n- item\n**bold** and *italic*"},
	}

	// ANSI color codes use patterns like \x1b[38;... or \x1b[48;... for colors.
	// Bold (\x1b[1m), Faint (\x1b[2m), Reverse (\x1b[7m) are structural, NOT color codes.
	colorCodePattern := regexp.MustCompile(`\x1b\[(3[0-9]|4[0-9]|38;|48;|9[0-7]|10[0-7])`)

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.Render(tt.input, 80)
			matches := colorCodePattern.FindAllString(result, -1)
			assert.Empty(t, matches, "no-color mode should not emit ANSI color codes, got: %v in output: %q", matches, result)
		})
	}
}

// --- Task 6.8: Accessible mode ---

func TestRender_Accessible_TextLabels(t *testing.T) {
	mv := newAccessibleMV()

	result := mv.Render("# Title\n```\ncode\n```\n- item", 80)

	assert.Contains(t, result, "[H1]")
	assert.Contains(t, result, "[CODE]")
	assert.Contains(t, result, "[/CODE]")
	assert.Contains(t, result, "- item")
}

// --- Task 6.9: Width truncation ---

func TestRender_WidthTruncation(t *testing.T) {
	mv := newNoColorMV() // NoColor for predictable width.
	input := "This is a very long line that should be truncated"
	result := mv.Render(input, 20)
	// ansi.Truncate truncates to width, but may append ellipsis within that width.
	visibleWidth := ansi.StringWidth(result)
	assert.LessOrEqual(t, visibleWidth, 20, "visible width should fit within 20 columns")
}

func TestRender_MinimumWidth(t *testing.T) {
	mv := newTestMV()
	// Width 0 should be clamped to 1.
	result := mv.Render("hello", 0)
	require.NotEmpty(t, result)
}

// --- Task 6.10: Pass-through ---

func TestRender_Passthrough(t *testing.T) {
	mv := newNoColorMV()

	tests := []struct {
		name  string
		input string
	}{
		{"table", "| col1 | col2 |"},
		{"image", "![alt](image.png)"},
		{"html", "<div>hello</div>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.Render(tt.input, 80)
			stripped := ansi.Strip(result)
			assert.Equal(t, tt.input, stripped, "unsupported elements should pass through as plain text")
		})
	}
}

// --- Task 6.11: No-emoji mode ---

func TestRender_NoEmoji_BulletIsDash(t *testing.T) {
	mv := newNoEmojiMV()
	result := mv.Render("- item", 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "- item")
	assert.NotContains(t, stripped, "•")
}

// --- Task 6.13: Mixed content ---

func TestRender_MixedContent(t *testing.T) {
	mv := newTestMV()
	input := strings.Join([]string{
		"# Title",
		"",
		"Some **bold** and *italic* text.",
		"",
		"```",
		"code block",
		"```",
		"",
		"- list item",
		"- another `inline` item",
	}, "\n")

	result := mv.Render(input, 80)
	stripped := ansi.Strip(result)

	assert.Contains(t, stripped, "Title")
	assert.Contains(t, stripped, "bold")
	assert.Contains(t, stripped, "italic")
	assert.Contains(t, stripped, "code block")
	assert.Contains(t, stripped, "•")
	assert.Contains(t, stripped, "list item")
	assert.Contains(t, stripped, "inline")
	// Inline code backticks should be stripped in styled mode.
	assert.NotContains(t, stripped, "`inline`", "inline code delimiters should be removed in styled rendering")
}

// --- Additional edge case tests ---

func TestRender_CodeBlock_NoInlineFormatting(t *testing.T) {
	mv := newTestMV()
	input := "```\n**not bold** *not italic*\n```"
	result := mv.Render(input, 80)
	stripped := ansi.Strip(result)
	// Inside code blocks, ** and * should be preserved literally.
	assert.Contains(t, stripped, "**not bold** *not italic*")
}

func TestRender_MultipleCodeBlocks(t *testing.T) {
	mv := newTestMV()
	input := "```\nblock1\n```\ntext\n```\nblock2\n```"
	result := mv.Render(input, 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "block1")
	assert.Contains(t, stripped, "text")
	assert.Contains(t, stripped, "block2")
}

func TestNewMarkdownView(t *testing.T) {
	theme := tui.DefaultTheme()
	config := tui.RenderConfig{Color: tui.ColorTrueColor}
	mv := NewMarkdownView(theme, config)
	require.NotNil(t, mv)
	assert.Equal(t, tui.ColorTrueColor, mv.renderConfig.Color)
}

// --- Review Patch Tests ---

func TestRender_NoColor_CodeBlock_HasReverse(t *testing.T) {
	mv := newNoColorMV()
	input := "```\ncode line\n```"
	result := mv.Render(input, 80)
	// Reverse ANSI escape is \x1b[7m — should be present for code block in NoColor mode.
	assert.Contains(t, result, "\x1b[7m", "NoColor code block should use Reverse styling")
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "code line")
}

func TestRender_NoColor_InlineCode_HasReverse(t *testing.T) {
	mv := newNoColorMV()
	result := mv.Render("Use `code` here", 80)
	// Inline code in NoColor should use CodePath.Resolve(ColorNone) = Reverse.
	assert.Contains(t, result, "\x1b[7m", "NoColor inline code should use Reverse styling")
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "code")
}

func TestRender_Accessible_InlineCode_BacktickWrapped(t *testing.T) {
	mv := newAccessibleMV()
	result := mv.Render("Use `code` here", 80)
	assert.Contains(t, result, "`code`", "accessible mode should wrap inline code with backticks")
}

func TestRender_EmptyBoldDelimiter(t *testing.T) {
	mv := newTestMV()
	// **** should pass through as literal text, not create empty bold span.
	result := mv.Render("****", 80)
	stripped := ansi.Strip(result)
	assert.Equal(t, "****", stripped, "empty bold delimiters should pass through")
}

func TestRender_EmptyItalicDelimiter(t *testing.T) {
	mv := newTestMV()
	// ** (two stars) should pass through as literal, not create empty italic span.
	result := mv.Render("**", 80)
	stripped := ansi.Strip(result)
	assert.Equal(t, "**", stripped, "empty italic delimiters should pass through")
}

func TestRender_ItalicNearBold(t *testing.T) {
	mv := newTestMV()
	// *bold** should not incorrectly parse as italic containing "bold*".
	result := mv.Render("*bold**", 80)
	stripped := ansi.Strip(result)
	assert.Equal(t, "*bold**", stripped, "mismatched delimiters should pass through as literal")
}

// --- CR2: Word boundary and heading inline tests ---

func TestRender_LiteralUnderscoreInWords(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"snake_case", "Use snake_case naming"},
		{"pkg_name", "Import pkg_name here"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mv := newTestMV()
			result := mv.Render(tt.input, 80)
			stripped := ansi.Strip(result)
			assert.Equal(t, tt.input, stripped, "underscores inside words should not trigger italic")
		})
	}
}

func TestRender_LiteralAsteriskInWords(t *testing.T) {
	mv := newTestMV()
	result := mv.Render("Calculate 2*3*4", 80)
	stripped := ansi.Strip(result)
	assert.Equal(t, "Calculate 2*3*4", stripped, "asterisks inside words should not trigger italic")
}

func TestRender_HeadingWithInlineCode(t *testing.T) {
	mv := newTestMV()
	result := mv.Render("# Use `cmd`", 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Use")
	assert.Contains(t, stripped, "cmd")
	assert.NotContains(t, stripped, "`", "inline code backticks should be stripped in heading")
}

func TestRender_HeadingWithBold(t *testing.T) {
	mv := newTestMV()
	result := mv.Render("## **Warning**", 80)
	stripped := ansi.Strip(result)
	assert.Contains(t, stripped, "Warning")
	assert.NotContains(t, stripped, "**", "bold delimiters should be stripped in heading")
}

func TestRender_HeadingWithInlineCode_Accessible(t *testing.T) {
	mv := newAccessibleMV()
	result := mv.Render("# Use `cmd`", 80)
	assert.Contains(t, result, "[H1]")
	assert.Contains(t, result, "`cmd`", "accessible heading should preserve backtick-wrapped inline code")
}

func TestRender_LiteralUnderscoreInWords_NoColor(t *testing.T) {
	mv := newNoColorMV()
	result := mv.Render("Use snake_case naming", 80)
	stripped := ansi.Strip(result)
	assert.Equal(t, "Use snake_case naming", stripped, "underscores inside words should not trigger italic in no-color mode")
}
