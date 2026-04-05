package licensing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configFileName    = "config.json"
	reminderInterval  = 5
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

// ShowLoginPrompt displays the login prompt and returns the user's choice.
// Returns "1" for GitHub, "2" for Google, "s" for skip.
func ShowLoginPrompt() string {
	fmt.Println()
	fmt.Println("💡 Sign in for marketplace access and future Pro features:")
	fmt.Println("  [1] GitHub (recommended)")
	fmt.Println("  [2] Google")
	fmt.Println("  [s] Skip for now")
	fmt.Println()
	fmt.Print("Choose: ")

	var choice string
	if _, err := fmt.Scanln(&choice); err != nil {
		return "s"
	}
	return choice
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
	return os.WriteFile(filepath.Join(configDir, configFileName), data, filePermissions)
}
