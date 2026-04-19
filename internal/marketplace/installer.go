// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"siply.dev/siply/internal/plugins"
)

const (
	maxDownloadSize     = 200 << 20  // 200 MB
	maxExtractTotal     = 2000 << 20 // 2 GB cumulative extraction limit
	downloadHTTPTimeout = 5 * time.Minute
)

var downloadHTTPClient = func() *http.Client {
	return &http.Client{Timeout: downloadHTTPTimeout}
}

// InstallerFunc is the function signature for the LocalRegistry.Install method.
// Accepts a context and the path to the source directory.
type InstallerFunc func(ctx context.Context, sourceDir string) error

// Install downloads (or copies) the item to a local directory, verifies the
// SHA256 checksum (if item.SHA256 != ""), and calls registryInstall with the
// source directory.
//
// If item.Category == "skills" and a non-nil skillsInstall is provided, the
// item is installed to the skills directory instead of the plugins directory (AC#1).
//
// file:// URLs: source is treated as a local directory path — no extraction
// needed, the chosen installer is called directly with that path.
// https:// / http:// URLs: downloaded and extracted before install.
// empty URL: returns ErrNoDownloadURL immediately.
//
// skillsInstall is variadic (0 or 1 element) for backward compatibility with
// existing callers that do not pass it.
func Install(ctx context.Context, item Item, registryInstall InstallerFunc, skillsInstall ...InstallerFunc) error {
	// Route skills to the skills-dir installer when provided.
	installFn := registryInstall
	if item.Category == "skills" && len(skillsInstall) > 0 && skillsInstall[0] != nil {
		installFn = skillsInstall[0]
	}

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
		return installFn(ctx, localPath)
	}

	if strings.HasPrefix(item.DownloadURL, "https://") || strings.HasPrefix(item.DownloadURL, "http://") {
		if strings.HasPrefix(item.DownloadURL, "http://") {
			fmt.Fprintf(os.Stderr, "⚠ WARNING: download URL uses plaintext HTTP — connection is not encrypted: %s\n", item.DownloadURL)
		}
		tmpDir, err := downloadAndExtract(ctx, item.DownloadURL, item.SHA256)
		if err != nil {
			return fmt.Errorf("marketplace: remote install %q: %w", item.Name, err)
		}
		defer os.RemoveAll(tmpDir)
		return installFn(ctx, tmpDir)
	}

	return fmt.Errorf("marketplace: unsupported download URL scheme for item %q: %s", item.Name, item.DownloadURL)
}

// InstallBundle installs all components of a bundle sequentially.
// Pre-flight: validates all components exist in the index and are compatible.
// If any pre-flight check fails, no components are installed.
func InstallBundle(ctx context.Context, bundle Item, idx *Index, registryInstall InstallerFunc, siplyVersion string, w ...io.Writer) error {
	out := io.Writer(os.Stdout)
	if len(w) > 0 && w[0] != nil {
		out = w[0]
	}

	if len(bundle.Components) == 0 {
		return fmt.Errorf("%w: %q", ErrBundleEmptyComponents, bundle.Name)
	}

	// Check the bundle's own SiplyMin before checking components.
	if !isCompatible(bundle.SiplyMin, siplyVersion) {
		return fmt.Errorf("bundle %q requires siply >=%s, have %s", bundle.Name, bundle.SiplyMin, siplyVersion)
	}

	// Pre-flight: resolve all components and check compatibility.
	type resolved struct {
		item Item
		comp BundleComponent
	}
	items := make([]resolved, 0, len(bundle.Components))
	var preflightErrs []string

	for _, comp := range bundle.Components {
		item, err := FindByName(idx, comp.Name)
		if err != nil {
			preflightErrs = append(preflightErrs, fmt.Sprintf("  %s: %v", comp.Name, ErrBundleComponentNotFound))
			continue
		}
		if item.Category == "bundles" {
			preflightErrs = append(preflightErrs, fmt.Sprintf("  %s: nested bundles are not supported", comp.Name))
			continue
		}
		if !isCompatible(item.SiplyMin, siplyVersion) {
			preflightErrs = append(preflightErrs, fmt.Sprintf("  %s v%s: %v (requires siply >=%s, have %s)",
				comp.Name, item.Version, ErrBundleComponentIncompatible, item.SiplyMin, siplyVersion))
			continue
		}
		if comp.Version != "" && item.Version != comp.Version {
			fmt.Fprintf(out, "  ⚠ %s: bundle specifies v%s but index has v%s\n", comp.Name, comp.Version, item.Version)
		}
		items = append(items, resolved{item: *item, comp: comp})
	}

	if len(preflightErrs) > 0 {
		return fmt.Errorf("bundle %q install blocked — pre-flight failures:\n%s", bundle.Name, strings.Join(preflightErrs, "\n"))
	}

	// Sequential install.
	var succeeded []string
	for _, r := range items {
		fmt.Fprintf(out, "  Installing %s v%s... ", r.comp.Name, r.item.Version)
		if err := Install(ctx, r.item, registryInstall); err != nil {
			fmt.Fprintln(out, "❌")
			return fmt.Errorf("bundle %q: component %q failed (succeeded: %s): %w",
				bundle.Name, r.comp.Name, strings.Join(succeeded, ", "), err)
		}
		fmt.Fprintln(out, "✅")
		succeeded = append(succeeded, r.comp.Name)
	}

	fmt.Fprintf(out, "✅ Bundle %s installed (%d items)\n", bundle.Name, len(items))
	return nil
}

// isCompatible is a package-local wrapper for plugins.IsCompatible to avoid
// import cycles in tests. It delegates to the plugins package.
var isCompatible = isCompatibleDefault

func isCompatibleDefault(siplyMin, currentVersion string) bool {
	return plugins.IsCompatible(siplyMin, currentVersion)
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

// downloadAndExtract fetches a tar.gz from url, optionally verifies SHA256,
// and extracts it to a temp directory. Caller must os.RemoveAll the returned dir.
func downloadAndExtract(ctx context.Context, url, expectedSHA256 string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("marketplace: create download request: %w", err)
	}

	resp, err := downloadHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("marketplace: download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("marketplace: download: unexpected status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "siply-download-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("marketplace: create temp file: %w", err)
	}
	tmpFilePath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFilePath)
	}()

	limited := io.LimitReader(resp.Body, maxDownloadSize+1)
	n, err := io.Copy(tmpFile, limited)
	if err != nil {
		return "", fmt.Errorf("marketplace: download stream: %w", err)
	}
	if n > maxDownloadSize {
		return "", fmt.Errorf("marketplace: download exceeds %d MB size limit", maxDownloadSize>>20)
	}

	if expectedSHA256 != "" {
		if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("marketplace: seek temp file: %w", err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, tmpFile); err != nil {
			return "", fmt.Errorf("marketplace: hash download: %w", err)
		}
		got := hex.EncodeToString(h.Sum(nil))
		if got != strings.ToLower(expectedSHA256) {
			return "", fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedSHA256, got)
		}
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("marketplace: seek temp file: %w", err)
	}

	extractDir, err := os.MkdirTemp("", "siply-extract-*")
	if err != nil {
		return "", fmt.Errorf("marketplace: create extract dir: %w", err)
	}

	if err := extractArchive(tmpFile, extractDir); err != nil {
		os.RemoveAll(extractDir)
		return "", fmt.Errorf("marketplace: extract: %w", err)
	}

	return extractDir, nil
}

// extractArchive extracts a tar.gz stream into destDir.
// Only regular files and directories are extracted; symlinks and hardlinks are skipped.
func extractArchive(r io.Reader, destDir string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip open: %w", err)
	}
	defer gr.Close()

	var totalBytes int64
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) && filepath.Clean(target) != filepath.Clean(destDir) {
			return fmt.Errorf("tar entry escapes destination: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("mkdir %s: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("mkdir parent %s: %w", hdr.Name, err)
			}
			perm := os.FileMode(hdr.Mode).Perm()&0755 | 0644
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
			if err != nil {
				return fmt.Errorf("create %s: %w", hdr.Name, err)
			}
			n, copyErr := io.Copy(f, io.LimitReader(tr, maxPluginFileSize+1))
			f.Close()
			if copyErr != nil {
				return fmt.Errorf("write %s: %w", hdr.Name, copyErr)
			}
			if n > maxPluginFileSize {
				return fmt.Errorf("marketplace: file %s exceeds %d MB size limit", hdr.Name, maxPluginFileSize>>20)
			}
			totalBytes += n
			if totalBytes > maxExtractTotal {
				return fmt.Errorf("marketplace: extracted content exceeds %d MB cumulative limit", maxExtractTotal>>20)
			}
		}
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
