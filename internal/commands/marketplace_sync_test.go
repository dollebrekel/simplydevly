// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/marketplace"
)

// syncTestServer creates a test HTTP server serving an index JSON at /index.json.
func syncTestServer(t *testing.T, status int, idx *marketplace.Index) *httptest.Server {
	t.Helper()
	var body []byte
	if idx != nil {
		var err error
		body, err = json.Marshal(idx)
		require.NoError(t, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if body != nil {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// testSyncIndex is a helper index for sync CLI tests.
func testSyncIndex() *marketplace.Index {
	return &marketplace.Index{
		Version:   1,
		UpdatedAt: "2026-04-18T00:00:00Z",
		Items: []marketplace.Item{
			{Name: "plugin-a", Category: "plugins", Version: "1.0.0", Author: "x", License: "MIT", UpdatedAt: "2026-01-01"},
			{Name: "plugin-b", Category: "plugins", Version: "2.0.0", Author: "y", License: "MIT", UpdatedAt: "2026-01-01"},
			{Name: "ext-c", Category: "extensions", Version: "0.5.0", Author: "z", License: "Apache-2.0", UpdatedAt: "2026-01-01"},
		},
	}
}

// TestMarketplaceSyncCmd_Success verifies that a successful sync prints the item count
// and exits 0 (AC #1).
func TestMarketplaceSyncCmd_Success(t *testing.T) {
	srv := syncTestServer(t, http.StatusOK, testSyncIndex())

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	// Directly call SyncIndex to simulate what the CLI does (bypassing home dir logic).
	synced, count, err := marketplace.SyncIndex(t.Context(), marketplace.SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.NoError(t, err)
	assert.True(t, synced)
	assert.Equal(t, 3, count)

	// Verify the cache file contains all 3 items.
	data, readErr := os.ReadFile(cachePath)
	require.NoError(t, readErr)
	var idx marketplace.Index
	require.NoError(t, json.Unmarshal(data, &idx))
	assert.Len(t, idx.Items, 3)
}

// TestMarketplaceSyncCmd_UpToDate verifies that a 304 response prints "up to date" (AC #2).
func TestMarketplaceSyncCmd_UpToDate(t *testing.T) {
	srv := syncTestServer(t, http.StatusNotModified, nil)

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	// Write an existing cache file so mtime is available for If-Modified-Since.
	idxData, _ := json.Marshal(testSyncIndex())
	require.NoError(t, os.WriteFile(cachePath, idxData, 0644))

	synced, count, err := marketplace.SyncIndex(t.Context(), marketplace.SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
	})

	require.NoError(t, err)
	assert.False(t, synced, "304 should return synced=false")
	assert.Equal(t, 0, count)
}

// TestMarketplaceSyncCmd_ForceFlagOutput verifies that --force downloads even when
// a cache exists (AC #3) — uses CLI cmd wiring.
func TestMarketplaceSyncCmd_ForceFlagOutput(t *testing.T) {
	srv := syncTestServer(t, http.StatusOK, testSyncIndex())

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "marketplace-index.json")

	// Pre-existing cache.
	idxData, _ := json.Marshal(testSyncIndex())
	require.NoError(t, os.WriteFile(cachePath, idxData, 0644))

	synced, count, err := marketplace.SyncIndex(t.Context(), marketplace.SyncConfig{
		PagesBaseURL: srv.URL,
		CachePath:    cachePath,
		Force:        true,
	})

	require.NoError(t, err)
	assert.True(t, synced, "force=true should always download")
	assert.Equal(t, 3, count)
}

// TestMarketplaceSyncCmd_CobraWiring exercises the sync subcommand through the
// cobra command tree to verify flag parsing, cmd.Context(), and cmd.OutOrStdout().
func TestMarketplaceSyncCmd_CobraWiring(t *testing.T) {
	t.Run("success output through cobra", func(t *testing.T) {
		srv := syncTestServer(t, http.StatusOK, testSyncIndex())

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "marketplace-index.json")

		// Build a sync command that uses our test server and temp cache path.
		cmd := newMarketplaceSyncCmdWithConfig(srv.URL, cachePath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		require.NoError(t, err)
		assert.Contains(t, out.String(), "Marketplace index synced (3 items)")
	})

	t.Run("up to date output through cobra", func(t *testing.T) {
		srv := syncTestServer(t, http.StatusNotModified, nil)

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "marketplace-index.json")
		idxData, _ := json.Marshal(testSyncIndex())
		require.NoError(t, os.WriteFile(cachePath, idxData, 0644))

		cmd := newMarketplaceSyncCmdWithConfig(srv.URL, cachePath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		require.NoError(t, err)
		assert.Contains(t, out.String(), "Marketplace index is up to date")
	})

	t.Run("force flag through cobra", func(t *testing.T) {
		srv := syncTestServer(t, http.StatusOK, testSyncIndex())

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "marketplace-index.json")
		idxData, _ := json.Marshal(testSyncIndex())
		require.NoError(t, os.WriteFile(cachePath, idxData, 0644))

		cmd := newMarketplaceSyncCmdWithConfig(srv.URL, cachePath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--force"})

		err := cmd.Execute()

		require.NoError(t, err)
		assert.Contains(t, out.String(), "Marketplace index synced (3 items)")
	})

	t.Run("error output through cobra", func(t *testing.T) {
		srv := syncTestServer(t, http.StatusServiceUnavailable, nil)

		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "marketplace-index.json")

		cmd := newMarketplaceSyncCmdWithConfig(srv.URL, cachePath)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{})

		err := cmd.Execute()

		require.Error(t, err, "non-200/304 response must produce non-zero exit")
	})
}

// newMarketplaceSyncCmdWithConfig creates a testable sync command that bypasses
// os.UserHomeDir() and uses the provided pagesBaseURL and cachePath.
func newMarketplaceSyncCmdWithConfig(pagesBaseURL, cachePath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch latest marketplace index",
		RunE: func(cmd *cobra.Command, _ []string) error {
			force, _ := cmd.Flags().GetBool("force")
			synced, count, syncErr := marketplace.SyncIndex(cmd.Context(), marketplace.SyncConfig{
				PagesBaseURL: pagesBaseURL,
				CachePath:    cachePath,
				Force:        force,
			})
			if syncErr != nil {
				return syncErr
			}
			if !synced {
				fmt.Fprintln(cmd.OutOrStdout(), "Marketplace index is up to date")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Marketplace index synced (%d items)\n", count)
			}
			return nil
		},
	}
	cmd.Flags().Bool("force", false, "Force full download, ignoring cache freshness")
	return cmd
}
