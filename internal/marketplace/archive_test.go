// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestPluginDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("name: test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func extractTarGz(t *testing.T, archivePath string) map[string]bool {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	files := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		files[hdr.Name] = true
	}
	return files
}

func TestPackageDir_ValidPlugin(t *testing.T) {
	t.Parallel()
	dir := createTestPluginDir(t)

	archivePath, sha, err := PackageDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(archivePath)

	if sha == "" {
		t.Error("expected non-empty SHA256")
	}

	files := extractTarGz(t, archivePath)
	for _, expected := range []string{"manifest.yaml", "README.md", "plugin.go"} {
		if !files[expected] {
			t.Errorf("expected %q in archive, got entries: %v", expected, files)
		}
	}
}

func TestPackageDir_SkipsGitDir(t *testing.T) {
	t.Parallel()
	dir := createTestPluginDir(t)

	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0644); err != nil {
		t.Fatal(err)
	}

	archivePath, _, err := PackageDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(archivePath)

	files := extractTarGz(t, archivePath)
	for name := range files {
		if name == ".git/" || name == ".git/HEAD" {
			t.Errorf("expected .git to be excluded, found %q", name)
		}
	}
}

func TestPackageDir_SkipsSymlinks(t *testing.T) {
	t.Parallel()
	dir := createTestPluginDir(t)

	target := filepath.Join(dir, "plugin.go")
	link := filepath.Join(dir, "link.go")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported on this platform")
	}

	archivePath, _, err := PackageDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(archivePath)

	files := extractTarGz(t, archivePath)
	if files["link.go"] {
		t.Error("expected symlink to be excluded")
	}
}

func TestPackageDir_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, _, err := PackageDir(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestPackageDir_SHA256Deterministic(t *testing.T) {
	t.Parallel()
	dir := createTestPluginDir(t)

	path1, sha1, err := PackageDir(dir)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	defer os.Remove(path1)

	path2, sha2, err := PackageDir(dir)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	defer os.Remove(path2)

	if sha1 != sha2 {
		t.Errorf("SHA256 not deterministic: %s vs %s", sha1, sha2)
	}
}

func TestPackageDir_PathTraversalGuardExists(t *testing.T) {
	t.Parallel()
	// filepath.WalkDir never produces ".." in relative paths from a valid base,
	// so the guard in PackageDir is defense-in-depth. This test verifies that a
	// normal directory packs successfully (the guard does not false-positive).
	// The guard itself is a strings.Contains(rel, "..") check at archive.go:69-71.
	dir := createTestPluginDir(t)

	archivePath, _, err := PackageDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(archivePath)

	files := extractTarGz(t, archivePath)
	for name := range files {
		if name == ".." || strings.Contains(name, "../") {
			t.Errorf("archive entry contains path traversal: %q", name)
		}
	}
}
