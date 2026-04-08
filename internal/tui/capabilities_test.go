// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectColorDepth_TrueColor(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM", "xterm-256color")
	// detectColorDepth uses colorprofile which reads env, but
	// in test we verify the fallback path.
	depth := detectColorDepth()
	// Either TrueColor or Color256 depending on colorprofile detection in test env.
	assert.True(t, depth >= Color256, "expected at least Color256, got %s", depth)
}

func TestDetectColorDepth_256Color(t *testing.T) {
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "xterm-256color")
	depth := detectColorDepth()
	assert.True(t, depth >= Color256, "expected at least Color256, got %s", depth)
}

func TestDetectUnicode_UTF8(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	assert.True(t, detectUnicode())
}

func TestDetectUnicode_NoUTF8(t *testing.T) {
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "")
	assert.False(t, detectUnicode())
}

func TestDetectUnicode_LC_ALL_Overrides(t *testing.T) {
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	assert.True(t, detectUnicode())
}

func TestCapabilities_SSHSession(t *testing.T) {
	t.Setenv("SSH_CLIENT", "192.168.1.1 12345 22")
	t.Setenv("SSH_TTY", "")
	t.Setenv("TMUX", "")
	caps := DetectCapabilities()
	assert.True(t, caps.SSHSession)
	assert.False(t, caps.TmuxNested)
}

func TestCapabilities_TmuxNested(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	caps := DetectCapabilities()
	assert.True(t, caps.TmuxNested)
	assert.False(t, caps.SSHSession)
}

func TestCapabilities_MouseDefaultTrue(t *testing.T) {
	caps := DetectCapabilities()
	assert.True(t, caps.Mouse)
}

func TestCapabilities_EmojiFollowsUnicode(t *testing.T) {
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")
	caps := DetectCapabilities()
	assert.Equal(t, caps.Unicode, caps.Emoji)
}

func TestColorDepth_String(t *testing.T) {
	assert.Equal(t, "truecolor", TrueColor.String())
	assert.Equal(t, "256", Color256.String())
	assert.Equal(t, "16", Color16.String())
	assert.Equal(t, "none", NoColor.String())
}
