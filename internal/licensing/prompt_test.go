// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldShowLoginPrompt_FirstRun(t *testing.T) {
	configDir := t.TempDir()
	assert.True(t, ShouldShowLoginPrompt(configDir), "should show on first run")
}

func TestShouldShowLoginPrompt_AlreadyLoggedIn(t *testing.T) {
	configDir := t.TempDir()
	// Create account.json.
	require.NoError(t, os.WriteFile(filepath.Join(configDir, accountFileName), []byte(`{}`), filePermissions))

	assert.False(t, ShouldShowLoginPrompt(configDir), "should not show when logged in")
}

func TestShouldShowLoginPrompt_SkipAndReminder(t *testing.T) {
	configDir := t.TempDir()

	// First run shows prompt.
	assert.True(t, ShouldShowLoginPrompt(configDir))

	// Skip 1-4 times should not show.
	for i := 0; i < 4; i++ {
		require.NoError(t, RecordLoginSkip(configDir))
	}
	assert.False(t, ShouldShowLoginPrompt(configDir), "should not show before 5 skips")

	// 5th skip should trigger reminder.
	require.NoError(t, RecordLoginSkip(configDir))
	assert.True(t, ShouldShowLoginPrompt(configDir), "should show after 5 skips")
}

func TestDisableLoginPrompt(t *testing.T) {
	configDir := t.TempDir()

	assert.True(t, ShouldShowLoginPrompt(configDir))

	require.NoError(t, DisableLoginPrompt(configDir))

	assert.False(t, ShouldShowLoginPrompt(configDir), "should not show after disable")
}

func TestRecordLoginSkip_PersistsCounter(t *testing.T) {
	configDir := t.TempDir()

	require.NoError(t, RecordLoginSkip(configDir))
	require.NoError(t, RecordLoginSkip(configDir))
	require.NoError(t, RecordLoginSkip(configDir))

	data, err := os.ReadFile(filepath.Join(configDir, configFileName))
	require.NoError(t, err)

	var cfg promptConfig
	require.NoError(t, json.Unmarshal(data, &cfg))
	assert.Equal(t, 3, cfg.SkipCount)
	assert.True(t, cfg.ShowLoginHints)
}
