// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
)

const (
	configFileName   = "config.json"
	reminderInterval = 5
)

// promptConfig tracks login prompt state.
type promptConfig struct {
	SkipCount      int  `json:"skip_count"`
	ShowLoginHints bool `json:"show_login_hints"`
}

// ShouldShowLoginPrompt returns true if the login prompt should be shown.
// It checks if account.json exists and whether the user has dismissed the prompt.
func ShouldShowLoginPrompt(configDir string) bool {
	// Already logged in — no prompt needed.
	accountPath := filepath.Join(configDir, accountFileName)
	if _, err := os.Stat(accountPath); err == nil {
		return false
	}

	cfg := loadPromptConfig(configDir)

	// User permanently dismissed.
	if !cfg.ShowLoginHints {
		return false
	}

	// First run or every N sessions.
	return cfg.SkipCount == 0 || cfg.SkipCount%reminderInterval == 0
}

// RecordLoginSkip increments the skip counter.
func RecordLoginSkip(configDir string) error {
	cfg := loadPromptConfig(configDir)
	cfg.SkipCount++
	return savePromptConfig(configDir, cfg)
}

// DisableLoginPrompt permanently disables the login prompt.
func DisableLoginPrompt(configDir string) error {
	cfg := loadPromptConfig(configDir)
	cfg.ShowLoginHints = false
	return savePromptConfig(configDir, cfg)
}

// SelectProvider displays the provider selection menu and returns the chosen provider.
// Returns the provider, whether the user skipped, and any error.
func SelectProvider(header string) (core.AuthProvider, bool, error) {
	fmt.Println(header)
	fmt.Println()
	fmt.Println("  [1] GitHub (recommended)")
	fmt.Println("  [2] Google")
	fmt.Println("  [s] Skip")
	fmt.Println()
	fmt.Print("Choose: ")

	var choice string
	if _, err := fmt.Scanln(&choice); err != nil {
		return 0, false, fmt.Errorf("failed to read input: %w", err)
	}

	switch choice {
	case "1":
		return core.AuthGitHub, false, nil
	case "2":
		return core.AuthGoogle, false, nil
	case "s", "S":
		return 0, true, nil
	default:
		return 0, false, fmt.Errorf("invalid choice: %q", choice)
	}
}

// ShowLoginPrompt displays the first-run login prompt and returns the user's choice.
// Returns "1" for GitHub, "2" for Google, "s" for skip.
func ShowLoginPrompt() string {
	provider, skipped, err := SelectProvider("💡 Sign in for marketplace access and future Pro features:")
	if err != nil || skipped {
		return "s"
	}
	switch provider {
	case core.AuthGitHub:
		return "1"
	case core.AuthGoogle:
		return "2"
	default:
		return "s"
	}
}

func loadPromptConfig(configDir string) promptConfig {
	path := filepath.Join(configDir, configFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return promptConfig{ShowLoginHints: true}
	}
	var cfg promptConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return promptConfig{ShowLoginHints: true}
	}
	return cfg
}

func savePromptConfig(configDir string, cfg promptConfig) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("licensing: cannot create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(filepath.Join(configDir, configFileName), data, filePermissions)
}
