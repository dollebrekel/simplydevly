// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package siplyui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const maxOverlayLayers = 3

// OverlayLayer is a single layer in the overlay stack.
type OverlayLayer struct {
	Content func(width, height int) string
	OnKey   func(key string) bool // return true if key was consumed
	ZIndex  int
}

// Overlay is a pure renderer for stacking modal layers.
// Up to maxOverlayLayers layers can be pushed at once.
type Overlay struct {
	layers       []OverlayLayer
	theme        Theme
	renderConfig RenderConfig
}

// NewOverlay creates an Overlay with the given theme and render config.
func NewOverlay(theme Theme, config RenderConfig) *Overlay {
	return &Overlay{theme: theme, renderConfig: config}
}

// Push adds a layer to the top of the overlay stack.
// If max layers are already open, Push is a no-op.
func (o *Overlay) Push(layer OverlayLayer) {
	if len(o.layers) >= maxOverlayLayers {
		return
	}
	o.layers = append(o.layers, layer)
}

// Pop removes the topmost layer.
func (o *Overlay) Pop() {
	if len(o.layers) > 0 {
		o.layers = o.layers[:len(o.layers)-1]
	}
}

// Clear removes all layers.
func (o *Overlay) Clear() {
	o.layers = o.layers[:0]
}

// IsOpen returns true if at least one layer is open.
func (o *Overlay) IsOpen() bool { return len(o.layers) > 0 }

// LayerCount returns the current number of open layers.
func (o *Overlay) LayerCount() int { return len(o.layers) }

// Render renders the topmost layer centered on a dimmed background.
// Background dimensions are provided by width × height.
func (o *Overlay) Render(width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if len(o.layers) == 0 {
		return ""
	}

	top := o.layers[len(o.layers)-1]

	// Modal content area: 80% of outer, clamped to minimum 10×3.
	modalW := width * 8 / 10
	if modalW < 10 {
		modalW = 10
	}
	if modalW > width {
		modalW = width
	}
	modalH := height * 8 / 10
	if modalH < 3 {
		modalH = 3
	}
	if modalH > height {
		modalH = height
	}

	content := ""
	if top.Content != nil {
		content = top.Content(modalW, modalH)
	}

	cs := o.renderConfig.RenderBorderedBox(o.theme, content, modalW)

	// Center the modal in the terminal.
	padLeft := (width - modalW) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	padTop := (height - strings.Count(cs, "\n") - 1) / 2
	if padTop < 0 {
		padTop = 0
	}

	var b strings.Builder
	prefix := strings.Repeat(" ", padLeft)
	lines := strings.Split(cs, "\n")
	for i := 0; i < padTop; i++ {
		b.WriteByte('\n')
	}
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(prefix + ansi.Truncate(line, width-padLeft, ""))
	}
	return b.String()
}

// HandleKey routes the key to the topmost layer.
// Esc always pops the top layer. When the overlay is open, all keys are
// consumed (focus trap) — returns true even if the layer doesn't handle the key.
func (o *Overlay) HandleKey(key string) bool {
	if len(o.layers) == 0 {
		return false
	}
	top := o.layers[len(o.layers)-1]
	if top.OnKey != nil && top.OnKey(key) {
		return true
	}
	if key == "esc" {
		o.Pop()
		return true
	}
	// Focus trap: consume all keys when overlay is open.
	return true
}

// RenderBorderedBox renders a bordered box using the theme's border token.
func (rc RenderConfig) RenderBorderedBox(theme Theme, content string, width int) string {
	if width < 2 {
		width = 2
	}
	inner := width - 2
	if inner < 1 {
		inner = 1
	}
	cs := rc.Color
	borderStyle := theme.Border.Resolve(cs)

	var b strings.Builder
	switch rc.Borders {
	case BorderNone:
		b.WriteString(content)
		return b.String()
	case BorderASCII:
		b.WriteString(borderStyle.Render("+"+strings.Repeat("-", inner)+"+") + "\n")
		for _, line := range strings.Split(content, "\n") {
			lw := ansi.StringWidth(line)
			pad := inner - lw
			if pad < 0 {
				pad = 0
			}
			b.WriteString(borderStyle.Render("|") + ansi.Truncate(line, inner, "") + strings.Repeat(" ", pad) + borderStyle.Render("|") + "\n")
		}
		b.WriteString(borderStyle.Render("+" + strings.Repeat("-", inner) + "+"))
	default: // unicode
		b.WriteString(borderStyle.Render("┌"+strings.Repeat("─", inner)+"┐") + "\n")
		for _, line := range strings.Split(content, "\n") {
			lw := ansi.StringWidth(line)
			pad := inner - lw
			if pad < 0 {
				pad = 0
			}
			b.WriteString(borderStyle.Render("│") + ansi.Truncate(line, inner, "") + strings.Repeat(" ", pad) + borderStyle.Render("│") + "\n")
		}
		b.WriteString(borderStyle.Render("└" + strings.Repeat("─", inner) + "┘"))
	}
	return b.String()
}
