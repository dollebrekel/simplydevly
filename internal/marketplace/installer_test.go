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

// --- Bundle install tests ---

func TestInstallBundle_AllSuccess(t *testing.T) {
	orig := isCompatible
	isCompatible = func(_, _ string) bool { return true }
	defer func() { isCompatible = orig }()

	idx := &Index{Items: []Item{
		{Name: "memory-default", Version: "1.0.0", DownloadURL: "file:///tmp/fake", Category: "plugins"},
		{Name: "prompt-basic", Version: "1.0.0", DownloadURL: "file:///tmp/fake", Category: "plugins"},
	}}
	bundle := Item{
		Name:     "test-bundle",
		Category: "bundles",
		Components: []BundleComponent{
			{Name: "memory-default", Version: "1.0.0"},
			{Name: "prompt-basic", Version: "1.0.0"},
		},
	}

	installed := []string{}
	installer := func(_ context.Context, _ string) error {
		installed = append(installed, "ok")
		return nil
	}

	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.NoError(t, err)
	assert.Len(t, installed, 2)
}

func TestInstallBundle_ComponentNotFound(t *testing.T) {
	orig := isCompatible
	isCompatible = func(_, _ string) bool { return true }
	defer func() { isCompatible = orig }()

	idx := &Index{Items: []Item{
		{Name: "memory-default", Version: "1.0.0", Category: "plugins"},
	}}
	bundle := Item{
		Name:     "test-bundle",
		Category: "bundles",
		Components: []BundleComponent{
			{Name: "memory-default", Version: "1.0.0"},
			{Name: "nonexistent", Version: "1.0.0"},
		},
	}

	installer := func(_ context.Context, _ string) error { return nil }
	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight failures")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestInstallBundle_ComponentIncompatible(t *testing.T) {
	orig := isCompatible
	isCompatible = func(siplyMin, current string) bool {
		return siplyMin == "0.1.0"
	}
	defer func() { isCompatible = orig }()

	idx := &Index{Items: []Item{
		{Name: "memory-default", Version: "1.0.0", SiplyMin: "0.1.0", Category: "plugins"},
		{Name: "future-plugin", Version: "2.0.0", SiplyMin: "99.0.0", Category: "plugins"},
	}}
	bundle := Item{
		Name:     "test-bundle",
		SiplyMin: "0.1.0",
		Category: "bundles",
		Components: []BundleComponent{
			{Name: "memory-default", Version: "1.0.0"},
			{Name: "future-plugin", Version: "2.0.0"},
		},
	}

	installer := func(_ context.Context, _ string) error { return nil }
	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight failures")
	assert.Contains(t, err.Error(), "future-plugin")
	assert.Contains(t, err.Error(), "incompatible")
}

func TestInstallBundle_PartialFailure(t *testing.T) {
	orig := isCompatible
	isCompatible = func(_, _ string) bool { return true }
	defer func() { isCompatible = orig }()

	idx := &Index{Items: []Item{
		{Name: "memory-default", Version: "1.0.0", DownloadURL: "file:///tmp/fake", Category: "plugins"},
		{Name: "broken-plugin", Version: "1.0.0", DownloadURL: "file:///tmp/fake", Category: "plugins"},
	}}
	bundle := Item{
		Name:     "test-bundle",
		Category: "bundles",
		Components: []BundleComponent{
			{Name: "memory-default", Version: "1.0.0"},
			{Name: "broken-plugin", Version: "1.0.0"},
		},
	}

	callCount := 0
	installer := func(_ context.Context, _ string) error {
		callCount++
		if callCount == 2 {
			return errors.New("disk full")
		}
		return nil
	}

	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken-plugin")
	assert.Contains(t, err.Error(), "succeeded: memory-default")
}

func TestInstallBundle_EmptyComponents(t *testing.T) {
	idx := &Index{}
	bundle := Item{Name: "empty-bundle", Category: "bundles"}
	installer := func(_ context.Context, _ string) error { return nil }

	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBundleEmptyComponents)
}

func TestInstallBundle_NestedBundleRejected(t *testing.T) {
	orig := isCompatible
	isCompatible = func(_, _ string) bool { return true }
	defer func() { isCompatible = orig }()

	idx := &Index{Items: []Item{
		{Name: "inner-bundle", Version: "1.0.0", Category: "bundles"},
	}}
	bundle := Item{
		Name:     "outer-bundle",
		Category: "bundles",
		Components: []BundleComponent{
			{Name: "inner-bundle", Version: "1.0.0"},
		},
	}

	installer := func(_ context.Context, _ string) error { return nil }
	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested bundles")
}

func TestInstallBundle_BundleSiplyMinChecked(t *testing.T) {
	orig := isCompatible
	isCompatible = func(siplyMin, current string) bool {
		return siplyMin != "99.0.0"
	}
	defer func() { isCompatible = orig }()

	idx := &Index{Items: []Item{
		{Name: "memory-default", Version: "1.0.0", Category: "plugins"},
	}}
	bundle := Item{
		Name:     "future-bundle",
		SiplyMin: "99.0.0",
		Category: "bundles",
		Components: []BundleComponent{
			{Name: "memory-default", Version: "1.0.0"},
		},
	}

	installer := func(_ context.Context, _ string) error { return nil }
	err := InstallBundle(context.Background(), bundle, idx, installer, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires siply")
}
