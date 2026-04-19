// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsFirstRun_TrueWhenNoConfig(t *testing.T) {
	home := t.TempDir()
	assert.True(t, IsFirstRun(home))
}

func TestIsFirstRun_FalseAfterMarkerExists(t *testing.T) {
	home := t.TempDir()
	siplyDir := filepath.Join(home, ".siply")
	require.NoError(t, os.MkdirAll(siplyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(siplyDir, firstRunMarkerFile), []byte{}, 0o644))
	assert.False(t, IsFirstRun(home))
}

func TestIsFirstRun_FalseWhenConfigExists(t *testing.T) {
	home := t.TempDir()
	siplyDir := filepath.Join(home, ".siply")
	require.NoError(t, os.MkdirAll(siplyDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(siplyDir, "config.yaml"), []byte("provider:\n  default: anthropic\n"), 0o644))
	assert.False(t, IsFirstRun(home))
}

func TestWriteFirstRunMarker(t *testing.T) {
	home := t.TempDir()
	require.True(t, IsFirstRun(home))
	require.NoError(t, WriteFirstRunMarker(home))
	assert.False(t, IsFirstRun(home))
}

func TestRunFirstRunPrompt_Minimal(t *testing.T) {
	var buf strings.Builder
	choice, err := RunFirstRunPrompt(context.Background(), &buf, strings.NewReader("1\n"))
	require.NoError(t, err)
	assert.Equal(t, "minimal", choice)
}

func TestRunFirstRunPrompt_Standard(t *testing.T) {
	var buf strings.Builder
	choice, err := RunFirstRunPrompt(context.Background(), &buf, strings.NewReader("2\n"))
	require.NoError(t, err)
	assert.Equal(t, "standard", choice)
}

func TestRunFirstRunPrompt_SkipOnOtherInput(t *testing.T) {
	var buf strings.Builder
	choice, err := RunFirstRunPrompt(context.Background(), &buf, strings.NewReader("3\n"))
	require.NoError(t, err)
	assert.Equal(t, "", choice)
}

func TestRunFirstRunPrompt_EmptyInputReturnsEmpty(t *testing.T) {
	var buf strings.Builder
	choice, err := RunFirstRunPrompt(context.Background(), &buf, strings.NewReader(""))
	require.NoError(t, err)
	assert.Equal(t, "", choice)
}
