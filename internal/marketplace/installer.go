// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstallerFunc is the function signature for the LocalRegistry.Install method.
// Accepts a context and the path to the source directory.
type InstallerFunc func(ctx context.Context, sourceDir string) error

// Install downloads (or copies) the item to a local directory, verifies the
// SHA256 checksum (if item.SHA256 != ""), and calls registryInstall with the
// source directory.
//
// file:// URLs: source is treated as a local directory path — no extraction
// needed, registryInstall is called directly with that path.
// https:// / http:// URLs: deferred — returns advisory error.
// empty URL: returns ErrNoDownloadURL immediately.
func Install(ctx context.Context, item Item, registryInstall InstallerFunc) error {
	if strings.TrimSpace(item.DownloadURL) == "" {
		return fmt.Errorf("%w: %q — run 'siply marketplace sync' to fetch download metadata", ErrNoDownloadURL, item.Name)
	}

	if localPath, ok := strings.CutPrefix(item.DownloadURL, "file://"); ok {
		// P2: validate path — empty or relative paths are rejected to prevent
		// accidental CWD installs and path traversal from a malicious index.
		if strings.TrimSpace(localPath) == "" {
			return fmt.Errorf("marketplace: file:// URL has empty path for item %q", item.Name)
		}
		if !filepath.IsAbs(localPath) {
			return fmt.Errorf("marketplace: file:// URL must use an absolute path for item %q: %q", item.Name, localPath)
		}
		// SHA256 verification (opt-in: skip if SHA256 field is empty).
		if item.SHA256 != "" {
			if err := verifyDirChecksum(localPath, item.SHA256); err != nil {
				return err
			}
		}
		return registryInstall(ctx, localPath)
	}

	if strings.HasPrefix(item.DownloadURL, "https://") || strings.HasPrefix(item.DownloadURL, "http://") {
		return fmt.Errorf("marketplace: remote download not yet implemented — use 'siply marketplace sync' to fetch items")
	}

	return fmt.Errorf("marketplace: unsupported download URL scheme for item %q: %s", item.Name, item.DownloadURL)
}

// verifyDirChecksum computes the SHA256 hash of the manifest.yaml in the given
// directory and compares it to the expected hex-encoded digest.
// Returns ErrChecksumMismatch if they differ.
func verifyDirChecksum(dir, expectedHex string) error {
	manifestPath := filepath.Join(dir, "manifest.yaml")
	f, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Fall back to hashing all files in directory order.
			return verifyAllFilesChecksum(dir, expectedHex)
		}
		return fmt.Errorf("marketplace: open manifest for checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("marketplace: hash manifest: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(expectedHex) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedHex, got)
	}
	return nil
}

// verifyAllFilesChecksum hashes all files in the directory (sorted by path) and
// compares the combined digest to expectedHex. Used as a fallback when no manifest.yaml exists.
func verifyAllFilesChecksum(dir, expectedHex string) error {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// P7: skip symlinks — copyDir also skips them, so verifying symlink
		// targets would produce a checksum of content that never gets installed.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("marketplace: open file for checksum %s: %w", path, err)
		}
		defer f.Close()
		if _, err := io.Copy(h, f); err != nil {
			return fmt.Errorf("marketplace: hash file %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("marketplace: compute checksum: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(expectedHex) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedHex, got)
	}
	return nil
}
