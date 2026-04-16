// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryDefaultPluginDir returns the path to the memory-default plugin fixture directory.
// Uses the real plugin directory relative to this test file.
func memoryDefaultPluginDir(t *testing.T) string {
	t.Helper()
	// From internal/marketplace/, up 3 levels to repo root, then plugins/memory-default.
	dir := filepath.Join("..", "..", "..", "plugins", "memory-default")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("memory-default plugin dir not found at %s: %v", dir, err)
	}
	return dir
}

// mockInstaller is a test double for InstallerFunc that records the sourceDir it was called with.
type mockInstaller struct {
	calledWith string
	returnErr  error
}

func (m *mockInstaller) Install(_ context.Context, sourceDir string) error {
	m.calledWith = sourceDir
	return m.returnErr
}

func TestInstall_EmptyURL(t *testing.T) {
	item := Item{Name: "test-item", DownloadURL: ""}
	mock := &mockInstaller{}

	err := Install(context.Background(), item, mock.Install)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoDownloadURL), "expected ErrNoDownloadURL, got: %v", err)
	assert.Empty(t, mock.calledWith, "installer must not be called when URL is empty")
}

func TestInstall_WhitespaceOnlyURL(t *testing.T) {
	item := Item{Name: "test-item", DownloadURL: "   "}
	mock := &mockInstaller{}

	err := Install(context.Background(), item, mock.Install)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoDownloadURL), "expected ErrNoDownloadURL for whitespace-only URL, got: %v", err)
}

func TestInstall_FileURL_Valid(t *testing.T) {
	pluginDir := memoryDefaultPluginDir(t)
	item := Item{
		Name:        "memory-default",
		DownloadURL: "file://" + pluginDir,
		SHA256:      "", // empty = skip verification
	}
	mock := &mockInstaller{}

	err := Install(context.Background(), item, mock.Install)

	require.NoError(t, err)
	assert.Equal(t, pluginDir, mock.calledWith, "installer must be called with the stripped local path")
}

func TestInstall_FileURL_ChecksumMismatch(t *testing.T) {
	pluginDir := memoryDefaultPluginDir(t)
	item := Item{
		Name:        "memory-default",
		DownloadURL: "file://" + pluginDir,
		SHA256:      "0000000000000000000000000000000000000000000000000000000000000000", // wrong hash
	}
	mock := &mockInstaller{}

	err := Install(context.Background(), item, mock.Install)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrChecksumMismatch), "expected ErrChecksumMismatch, got: %v", err)
	assert.Empty(t, mock.calledWith, "installer must not be called on checksum mismatch")
}

func TestInstall_FileURL_CorrectChecksum(t *testing.T) {
	pluginDir := memoryDefaultPluginDir(t)

	// Compute the real manifest.yaml checksum so we can pass a correct hash.
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	h := sha256.Sum256(data)
	correctHash := hex.EncodeToString(h[:])

	item := Item{
		Name:        "memory-default",
		DownloadURL: "file://" + pluginDir,
		SHA256:      correctHash,
	}
	mock := &mockInstaller{}

	err = Install(context.Background(), item, mock.Install)

	require.NoError(t, err)
	assert.Equal(t, pluginDir, mock.calledWith)
}

func TestInstall_HTTPSURL_NotImplemented(t *testing.T) {
	item := Item{
		Name:        "remote-item",
		DownloadURL: "https://example.com/remote-item-v1.0.tar.gz",
	}
	mock := &mockInstaller{}

	err := Install(context.Background(), item, mock.Install)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented", "advisory must mention not-yet-implemented")
	assert.Empty(t, mock.calledWith, "installer must not be called for remote URLs")
}

func TestInstall_HTTPURLNotImplemented(t *testing.T) {
	item := Item{
		Name:        "remote-item",
		DownloadURL: "http://example.com/remote-item-v1.0.tar.gz",
	}
	mock := &mockInstaller{}

	err := Install(context.Background(), item, mock.Install)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
