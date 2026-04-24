// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

// BenchmarkView_DockOnly benchmarks the dock-only rendering path (no overlays).
// AC: <16ms frame time for dock-only rendering.
func BenchmarkView_DockOnly(b *testing.B) {
	m := NewPanelManager(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Borders:   tui.BorderUnicode,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	})

	// Left panel with realistic content.
	leftContent := generateTreeContent(30)
	require.NoError(b, m.Register(core.PanelConfig{
		Name:        "tree",
		Position:    core.PanelLeft,
		MinWidth:    25,
		MaxWidth:    50,
		Collapsible: true,
		ContentFunc: func() string { return leftContent },
	}))

	// Right panel with realistic content.
	rightContent := generatePreviewContent(40)
	require.NoError(b, m.Register(core.PanelConfig{
		Name:        "preview",
		Position:    core.PanelRight,
		MinWidth:    30,
		MaxWidth:    60,
		Collapsible: true,
		ContentFunc: func() string { return rightContent },
	}))

	m.left.width = 30
	m.right.width = 35

	centerContent := strings.Repeat("$ siply run --verbose\nProcessing request...\n", 10)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = m.View(160, 40, centerContent)
	}
}

// BenchmarkView_WithOverlay benchmarks rendering with one active overlay.
func BenchmarkView_WithOverlay(b *testing.B) {
	m := NewPanelManager(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Borders:   tui.BorderUnicode,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	})

	// Dock panels.
	leftContent := generateTreeContent(30)
	require.NoError(b, m.Register(core.PanelConfig{
		Name:        "tree",
		Position:    core.PanelLeft,
		MinWidth:    25,
		MaxWidth:    50,
		ContentFunc: func() string { return leftContent },
	}))
	m.left.width = 30

	// Overlay panel.
	overlayContent := "Search results:\n  1. config.go\n  2. main.go\n  3. render.go"
	require.NoError(b, m.Register(core.PanelConfig{
		Name:        "search-overlay",
		Position:    core.PanelOverlay,
		MinWidth:    30,
		OverlayX:    10,
		OverlayY:    5,
		OverlayZ:    10,
		ContentFunc: func() string { return overlayContent },
	}))
	require.NoError(b, m.Activate("search-overlay"))

	centerContent := strings.Repeat("$ siply run\nOK\n", 10)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = m.View(120, 30, centerContent)
	}
}

// BenchmarkView_DockOnly_Unicode benchmarks dock rendering with CJK/emoji content.
func BenchmarkView_DockOnly_Unicode(b *testing.B) {
	m := NewPanelManager(tui.DefaultTheme(), tui.RenderConfig{
		Color:     tui.ColorTrueColor,
		Borders:   tui.BorderUnicode,
		Emoji:     true,
		Motion:    tui.MotionSpinner,
		Verbosity: tui.VerbosityFull,
	})

	unicodeContent := "📁 项目文件\n  ├── 配置.yaml\n  ├── こんにちは.go\n  └── 안녕.md\n"
	unicodeContent = strings.Repeat(unicodeContent, 5)

	require.NoError(b, m.Register(core.PanelConfig{
		Name:        "unicode-tree",
		Position:    core.PanelLeft,
		MinWidth:    30,
		MaxWidth:    50,
		ContentFunc: func() string { return unicodeContent },
	}))
	m.left.width = 35

	centerContent := "Code 代码 🚀 ready!\n" + strings.Repeat("処理中...\n", 15)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = m.View(120, 30, centerContent)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func generateTreeContent(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		if i%5 == 0 {
			b.WriteString(fmt.Sprintf("📁 dir-%d/\n", i/5))
		} else {
			b.WriteString(fmt.Sprintf("  📄 file-%d.go\n", i))
		}
	}
	return b.String()
}

func generatePreviewContent(lines int) string {
	var b strings.Builder
	b.WriteString("// Preview: main.go\n")
	b.WriteString("package main\n\n")
	for i := 0; i < lines-3; i++ {
		b.WriteString(fmt.Sprintf("func handler%d() { /* ... */ }\n", i))
	}
	return b.String()
}
