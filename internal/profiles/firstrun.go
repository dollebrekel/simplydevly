// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const firstRunMarkerFile = ".first-run-done"

// IsFirstRun returns true when the user has not yet completed first-run setup.
// First run is detected when ~/.siply/config.yaml is absent or the marker file
// ~/.siply/.first-run-done does not exist.
func IsFirstRun(homeDir string) bool {
	siplyDir := filepath.Join(homeDir, ".siply")
	markerPath := filepath.Join(siplyDir, firstRunMarkerFile)
	if _, err := os.Stat(markerPath); err == nil {
		return false
	}
	configPath := filepath.Join(siplyDir, "config.yaml")
	_, err := os.Stat(configPath)
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	// Permission error or other stat failure — treat as not first run
	// to avoid repeatedly prompting on broken filesystem state.
	return false
}

// WriteFirstRunMarker writes the .first-run-done marker so IsFirstRun returns false.
func WriteFirstRunMarker(homeDir string) error {
	siplyDir := filepath.Join(homeDir, ".siply")
	if err := os.MkdirAll(siplyDir, 0o755); err != nil {
		return fmt.Errorf("profiles: create .siply dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(siplyDir, firstRunMarkerFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("profiles: write first-run marker: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("profiles: sync first-run marker: %w", err)
	}
	return f.Close()
}

// RunFirstRunPrompt prints a profile selection prompt and reads a single character from r.
// Returns "minimal", "standard", or "" (skip).
func RunFirstRunPrompt(_ context.Context, w io.Writer, r io.Reader) (string, error) {
	fmt.Fprintln(w, "Welcome to siply! Choose a default profile:")
	fmt.Fprintln(w, "  [1] Minimal  — clean, no borders, compact")
	fmt.Fprintln(w, "  [2] Standard — full UI, borders, all status segments")
	fmt.Fprintln(w, "  [3] Skip     — configure manually later")
	fmt.Fprint(w, "Your choice [1/2/3]: ")

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("profiles: read first-run choice: %w", err)
		}
		return "", nil
	}

	switch strings.TrimSpace(scanner.Text()) {
	case "1":
		return "minimal", nil
	case "2":
		return "standard", nil
	default:
		return "", nil
	}
}
