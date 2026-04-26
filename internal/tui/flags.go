// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

// CLIFlags carries parsed TUI-related CLI flag values.
type CLIFlags struct {
	NoColor          bool
	NoEmoji          bool
	NoBorders        bool
	NoMotion         bool
	Accessible       bool
	LowBandwidth     bool
	Minimal          bool
	Standard         bool
	Local            bool
	OllamaAvailable  bool
	ModelOverride    string
	ConfigProfile    string // Profile from config file ("minimal", "standard", or "")
}
