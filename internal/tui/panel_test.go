// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPanel(t *testing.T) {
	theme := DefaultTheme()
	config := RenderConfig{Borders: BorderUnicode, Color: ColorTrueColor}

	p := NewPanel("test", theme, config)

	assert.NotNil(t, p)
	assert.Equal(t, "test", p.title)
	assert.True(t, p.bordered)
	assert.False(t, p.focused)
}

func TestNewPanel_BorderNone(t *testing.T) {
	theme := DefaultTheme()
	config := RenderConfig{Borders: BorderNone}

	p := NewPanel("test", theme, config)

	assert.False(t, p.bordered)
}

func TestPanel_SetContent(t *testing.T) {
	p := NewPanel("test", DefaultTheme(), RenderConfig{Borders: BorderUnicode})
	p.SetContent("hello world")
	assert.Equal(t, "hello world", p.content)
}

func TestPanel_SetFocused(t *testing.T) {
	p := NewPanel("test", DefaultTheme(), RenderConfig{Borders: BorderUnicode})
	p.SetFocused(true)
	assert.True(t, p.focused)
	p.SetFocused(false)
	assert.False(t, p.focused)
}

func TestPanel_SetBordered(t *testing.T) {
	p := NewPanel("test", DefaultTheme(), RenderConfig{Borders: BorderUnicode})
	assert.True(t, p.bordered)
	p.SetBordered(false)
	assert.False(t, p.bordered)
}

func TestPanel_SetSize(t *testing.T) {
	p := NewPanel("test", DefaultTheme(), RenderConfig{Borders: BorderUnicode})
	p.SetSize(80, 24)
	assert.Equal(t, 80, p.width)
	assert.Equal(t, 24, p.height)
}

func TestPanel_SetSize_ClampsNegative(t *testing.T) {
	p := NewPanel("test", DefaultTheme(), RenderConfig{Borders: BorderUnicode})
	p.SetSize(-5, 0)
	assert.Equal(t, 1, p.width)
	assert.Equal(t, 1, p.height)
}

func TestPanel_Render_Borderless(t *testing.T) {
	p := NewPanel("test", DefaultTheme(), RenderConfig{Borders: BorderNone})
	p.SetContent("raw content")
	result := p.Render()
	assert.Equal(t, "raw content", result)
}

func TestPanel_Render_UnfocusedBordered(t *testing.T) {
	config := RenderConfig{Borders: BorderUnicode, Color: ColorNone}
	p := NewPanel("title", DefaultTheme(), config)
	p.SetContent("body")
	p.SetSize(30, 10)
	p.SetFocused(false)

	result := p.Render()
	assert.Contains(t, result, "┌")
	assert.Contains(t, result, "title")
	assert.Contains(t, result, "body")
}

func TestPanel_Render_FocusedBordered(t *testing.T) {
	config := RenderConfig{Borders: BorderUnicode, Color: ColorNone}
	p := NewPanel("title", DefaultTheme(), config)
	p.SetContent("body")
	p.SetSize(30, 10)
	p.SetFocused(true)

	result := p.Render()
	assert.Contains(t, result, "┌")
	assert.Contains(t, result, "title")
	assert.Contains(t, result, "body")
}

func TestPanel_Render_ASCIIBorder(t *testing.T) {
	config := RenderConfig{Borders: BorderASCII, Color: ColorNone}
	p := NewPanel("title", DefaultTheme(), config)
	p.SetContent("body")
	p.SetSize(30, 10)

	result := p.Render()
	assert.Contains(t, result, "+")
	assert.Contains(t, result, "title")
	assert.NotContains(t, result, "┌")
}

func TestPanel_Render_AccessibleMode(t *testing.T) {
	config := RenderConfig{Borders: BorderNone, Verbosity: VerbosityAccessible}
	p := NewPanel("title", DefaultTheme(), config)
	p.SetContent("body")
	p.SetSize(30, 10)

	result := p.Render()
	// Borderless returns raw content.
	assert.Equal(t, "body", result)
	assert.NotContains(t, result, "┌")
}

func TestRenderBorderFocused_Unicode(t *testing.T) {
	config := RenderConfig{Borders: BorderUnicode, Color: ColorNone}
	result := RenderBorderFocused("test", "hello", config, DefaultTheme(), 20)

	assert.Contains(t, result, "┌")
	assert.Contains(t, result, "test")
	assert.Contains(t, result, "hello")
}

func TestRenderBorderFocused_ASCII(t *testing.T) {
	config := RenderConfig{Borders: BorderASCII, Color: ColorNone}
	result := RenderBorderFocused("test", "hello", config, DefaultTheme(), 20)

	assert.Contains(t, result, "+")
	assert.Contains(t, result, "test")
}

func TestRenderBorderFocused_None(t *testing.T) {
	config := RenderConfig{Borders: BorderNone}
	result := RenderBorderFocused("test", "hello", config, DefaultTheme(), 20)

	assert.Contains(t, result, "== test ==")
}
