// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"siply.dev/siply/internal/tui"
)

func TestRenderSplash_LineCount(t *testing.T) {
	out := RenderSplash(80, tui.ColorTrueColor)
	lines := strings.Split(out, "\n")
	assert.Equal(t, SplashHeight, len(lines), "splash should be exactly %d lines", SplashHeight)
}

func TestRenderSplash_BlankSeparator(t *testing.T) {
	out := RenderSplash(80, tui.ColorNone)
	lines := strings.Split(out, "\n")
	stripped := ansi.Strip(lines[6])
	assert.Equal(t, "", strings.TrimSpace(stripped), "line 7 (index 6) should be the blank separator between SIMPLY and DEVLY")
}

func TestRenderSplash_HorizontalCentering(t *testing.T) {
	out := RenderSplash(120, tui.ColorNone)
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		content := strings.TrimSpace(line)
		contentW := ansi.StringWidth(content)
		lineW := ansi.StringWidth(line)
		pad := lineW - contentW
		assert.Greater(t, pad, 0, "line %d should have leading spaces for centering at width 120", i)
	}
}

func TestRenderSplash_NarrowFallback(t *testing.T) {
	out := RenderSplash(40, tui.ColorNone)
	lines := strings.Split(out, "\n")
	assert.Equal(t, SplashFallbackHeight, len(lines), "narrow terminal should use text fallback with %d lines", SplashFallbackHeight)
	stripped := ansi.Strip(out)
	assert.Contains(t, stripped, "SIMPL")
	assert.Contains(t, stripped, "DEVL")
}

func TestSplashLines(t *testing.T) {
	assert.Equal(t, SplashHeight, SplashLines(120))
	assert.Equal(t, SplashFallbackHeight, SplashLines(40))
}

func TestRenderSplash_NoColorMode(t *testing.T) {
	out := RenderSplash(80, tui.ColorNone)
	assert.Equal(t, out, ansi.Strip(out), "no-color mode should produce no ANSI escape sequences")
}

func TestRenderSplash_TrueColorHasEscapes(t *testing.T) {
	out := RenderSplash(80, tui.ColorTrueColor)
	assert.NotEqual(t, out, ansi.Strip(out), "truecolor mode should contain ANSI escape sequences")
}

func TestRenderSplash_ContainsHalfBlocks(t *testing.T) {
	out := RenderSplash(80, tui.ColorNone)
	assert.True(t, strings.ContainsAny(out, "█▀▄"), "splash should contain half-block characters")
}

func TestRenderSplash_MinWidth(t *testing.T) {
	out := RenderSplash(1, tui.ColorNone)
	assert.NotEmpty(t, out, "should produce output even with width=1")
}

func TestTrimLetter(t *testing.T) {
	grid := [][]byte{
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
	}
	trimmed := trimLetter(grid)
	assert.Equal(t, 3, len(trimmed[0]), "trimmed I-like column should have width 3")
	assert.Equal(t, 3, len(trimmed[1]))
}

func TestGetWordPixels_Dimensions(t *testing.T) {
	grid := getWordPixels("SIMPLY")
	assert.Equal(t, pixelRows, len(grid.pixels))
	assert.Equal(t, pixelRows, len(grid.owners))
	assert.Greater(t, len(grid.pixels[0]), 0, "pixel rows should have non-zero width")
}

func TestGetWordPixels_Kerning(t *testing.T) {
	withKerning := getWordPixels("LY")
	noKerning := getWordPixels("LS")
	lyWidth := len(withKerning.pixels[0])
	lsWidth := len(noKerning.pixels[0])
	assert.Less(t, lyWidth, lsWidth, "LY with kerning -2 should be narrower than LS with default gap")
}
