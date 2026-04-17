// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxArchiveSize    = 200 << 20 // 200MB total archive limit
	maxPluginFileSize = 100 << 20 // 100MB per file (matches registry.go)
)

// PackageDir creates a .tar.gz archive of the given directory.
// Returns path to temp archive file and its SHA256 hex digest.
// Caller is responsible for removing the temp file.
func PackageDir(dir string) (archivePath string, sha256hex string, err error) {
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		return "", "", fmt.Errorf("marketplace: read dir: %w", readErr)
	}
	if len(entries) == 0 {
		return "", "", fmt.Errorf("marketplace: directory is empty: %s", dir)
	}

	tmpFile, err := os.CreateTemp("", "siply-publish-*.tar.gz")
	if err != nil {
		return "", "", fmt.Errorf("marketplace: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	defer func() {
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	mw := io.MultiWriter(tmpFile, hasher)
	gw, _ := gzip.NewWriterLevel(mw, gzip.DefaultCompression)
	gw.Header.ModTime = time.Time{}
	tw := tar.NewWriter(gw)

	var totalSize int64

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkEntryErr error) error {
		if walkEntryErr != nil {
			return walkEntryErr
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}

		if rel == "." {
			return nil
		}

		if strings.Contains(rel, "..") {
			return fmt.Errorf("marketplace: path traversal detected: %s", rel)
		}

		// Skip .git directory.
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Skip symlinks.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return fmt.Errorf("marketplace: stat %s: %w", rel, infoErr)
		}

		if d.IsDir() {
			header := &tar.Header{
				Name:     rel + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
			return tw.WriteHeader(header)
		}

		if info.Size() > maxPluginFileSize {
			return fmt.Errorf("marketplace: file %s exceeds 100MB limit", rel)
		}

		totalSize += info.Size()
		if totalSize > maxArchiveSize {
			return fmt.Errorf("marketplace: total archive size exceeds 200MB limit")
		}

		header := &tar.Header{
			Name:     rel,
			Size:     info.Size(),
			Mode:     0644,
			ModTime:  info.ModTime(),
			Typeflag: tar.TypeReg,
		}
		if writeErr := tw.WriteHeader(header); writeErr != nil {
			return writeErr
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return fmt.Errorf("marketplace: open %s: %w", rel, openErr)
		}
		defer f.Close()

		_, copyErr := io.Copy(tw, f)
		return copyErr
	})

	if walkErr != nil {
		return "", "", walkErr
	}

	if err = tw.Close(); err != nil {
		return "", "", fmt.Errorf("marketplace: close tar: %w", err)
	}
	if err = gw.Close(); err != nil {
		return "", "", fmt.Errorf("marketplace: close gzip: %w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return "", "", fmt.Errorf("marketplace: close file: %w", err)
	}

	return tmpPath, hex.EncodeToString(hasher.Sum(nil)), nil
}
