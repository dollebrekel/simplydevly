// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package siplyui provides a public TUI component library for siply extension developers.
// All components are pure renderers — they implement Render(width, height int) string and HandleKey(string) bool.
// They do NOT depend on Bubble Tea (no tea.Model, no tea.Cmd).
//
// Usage:
//
//	theme := siplyui.DefaultTheme()
//	cfg := siplyui.DefaultRenderConfig()
//	tree := siplyui.NewTree(nodes, theme, cfg)
//	output := tree.Render(80, 24)
package siplyui
