// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package checkpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RewindFiles restores workspace files to the state recorded in a checkpoint's file manifest.
// Creates a git stash as a safety net before restoring. Uses the checkpoint timestamp to
// find the closest git commit for each file.
func RewindFiles(step int, workDir string, fileHashes map[string]string, checkpointTime time.Time) error {
	if len(fileHashes) == 0 {
		return nil
	}

	// Safety net: stash current workspace state.
	stashMsg := fmt.Sprintf("siply-checkpoint-rewind-step-%d", step)
	stashCmd := exec.Command("git", "stash", "push", "-m", stashMsg)
	stashCmd.Dir = workDir
	if out, err := stashCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("file rewind: git stash failed (not a git repo?): %s: %w", strings.TrimSpace(string(out)), err)
	}

	var restored, skipped int
	for filePath, expectedHash := range fileHashes {
		currentHash, err := hashFile(filePath)
		if err != nil || currentHash == expectedHash {
			continue
		}

		commit, err := FindClosestCommit(workDir, filePath, checkpointTime)
		if err != nil {
			slog.Warn("file rewind: no git history for file", "file", filePath, "error", err)
			skipped++
			continue
		}

		restoreCmd := exec.Command("git", "checkout", commit, "--", filePath)
		restoreCmd.Dir = workDir
		if out, err := restoreCmd.CombinedOutput(); err != nil {
			slog.Warn("file rewind: restore failed", "file", filePath, "error", strings.TrimSpace(string(out)))
			skipped++
			continue
		}
		restored++
	}

	// Warn about untracked files created after checkpoint.
	untrackedCmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	untrackedCmd.Dir = workDir
	untrackedOut, _ := untrackedCmd.Output()
	if len(untrackedOut) > 0 {
		lines := strings.Split(strings.TrimSpace(string(untrackedOut)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			slog.Info("file rewind: untracked files exist (not deleted)", "count", len(lines))
			for _, f := range lines {
				slog.Debug("file rewind: untracked", "file", f)
			}
		}
	}

	slog.Info("file rewind complete", "restored", restored, "skipped", skipped, "step", step)
	return nil
}

// FindClosestCommit finds the closest git commit before the given timestamp for a file.
func FindClosestCommit(workDir, filePath string, before time.Time) (string, error) {
	cmd := exec.Command("git", "log",
		"--before="+before.Format(time.RFC3339),
		"-1", "--format=%H", "--", filePath,
	)
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find commit: %w", err)
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "", fmt.Errorf("no commits found before %s for %s", before.Format(time.RFC3339), filePath)
	}
	return commit, nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
