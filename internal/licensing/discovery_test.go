// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMockMarketServer starts a test server returning the given repos JSON.
func setupMockMarketServer(t *testing.T, repos []marketRepo, statusCode int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user/repos", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if repos != nil {
			json.NewEncoder(w).Encode(repos)
		} else {
			w.Write([]byte("[]"))
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	origURL := MarketBaseURL
	MarketBaseURL = "http://" + listener.Addr().String()
	t.Cleanup(func() { MarketBaseURL = origURL })
}

// createFakeGitRepo creates a directory with .git/config containing a GitHub remote.
func createFakeGitRepo(t *testing.T, base, name, remoteURL string) string {
	t.Helper()
	repoDir := filepath.Join(base, name)
	gitDir := filepath.Join(repoDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	config := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = ` + remoteURL + `
	fetch = +refs/heads/*:refs/remotes/origin/*
`
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0644))
	return repoDir
}

func TestDiscoverReposMatchesRemotes(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fake local git repos.
	repo1 := createFakeGitRepo(t, tmpDir, "myapp", "https://github.com/testuser/myapp.git")
	repo2 := createFakeGitRepo(t, tmpDir, "backend", "git@github.com:testuser/backend.git")
	createFakeGitRepo(t, tmpDir, "unrelated", "https://gitlab.com/other/unrelated.git")

	// Mock market API returning GitHub repos.
	setupMockMarketServer(t, []marketRepo{
		{FullName: "testuser/myapp", Language: "Go", Size: 100},
		{FullName: "testuser/backend", Language: "Python", Size: 200},
		{FullName: "testuser/no-local-clone", Language: "Rust", Size: 50},
	}, http.StatusOK)

	// Override scanRoots to only scan our temp dir.
	origRoots := scanRoots
	scanRoots = func() []string { return []string{tmpDir} }
	t.Cleanup(func() { scanRoots = origRoots })

	v := &licenseValidator{
		cached: &accountData{
			AuthProvider: "github",
			Token:        "test-token",
		},
	}

	repos, err := v.DiscoverRepos(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 2)

	// Build map for easier assertion.
	byName := make(map[string]string)
	for _, r := range repos {
		byName[r.GitHubFullName] = r.LocalPath
	}

	assert.Equal(t, repo1, byName["testuser/myapp"])
	assert.Equal(t, repo2, byName["testuser/backend"])
	assert.NotContains(t, byName, "testuser/no-local-clone")
}

func TestDiscoverReposSkipsExistingSiply(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := createFakeGitRepo(t, tmpDir, "myapp", "https://github.com/testuser/myapp.git")

	// Create .siply/ directory in repo.
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, ".siply"), 0755))

	setupMockMarketServer(t, []marketRepo{
		{FullName: "testuser/myapp", Language: "Go", Size: 100},
	}, http.StatusOK)

	origRoots := scanRoots
	scanRoots = func() []string { return []string{tmpDir} }
	t.Cleanup(func() { scanRoots = origRoots })

	v := &licenseValidator{
		cached: &accountData{
			AuthProvider: "github",
			Token:        "test-token",
		},
	}

	repos, err := v.DiscoverRepos(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.True(t, repos[0].HasSiplyConfig, "should report HasSiplyConfig=true for existing .siply/")
}

func TestDiscoverReposNoRepoScope(t *testing.T) {
	// Market returns 403 when no repo scope was granted.
	setupMockMarketServer(t, nil, http.StatusForbidden)

	v := &licenseValidator{
		cached: &accountData{
			AuthProvider: "github",
			Token:        "test-token",
		},
	}

	repos, err := v.DiscoverRepos(context.Background())
	assert.NoError(t, err, "should not error on denied repo scope")
	assert.Empty(t, repos, "should return empty list when repo scope denied")
}

func TestDiscoverReposNotLoggedIn(t *testing.T) {
	v := &licenseValidator{cached: nil}

	repos, err := v.DiscoverRepos(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, repos)
}

func TestDiscoverReposGoogleProvider(t *testing.T) {
	v := &licenseValidator{
		cached: &accountData{
			AuthProvider: "google",
			Token:        "test-token",
		},
	}

	repos, err := v.DiscoverRepos(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, repos, "Google provider should return empty — repo discovery is GitHub only")
}

func TestWorkspaceCreation(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myapp")
	require.NoError(t, os.MkdirAll(repoDir, 0755))

	err := SetupWorkspace(repoDir, "Go")
	require.NoError(t, err)

	// Verify .siply/config.yaml exists.
	configPath := filepath.Join(repoDir, ".siply", "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "provider:")
	assert.Contains(t, string(data), "default: anthropic")
	assert.Contains(t, string(data), "# Language: Go")
}

func TestWorkspaceCreationSkipsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myapp")
	siplyDir := filepath.Join(repoDir, ".siply")
	require.NoError(t, os.MkdirAll(siplyDir, 0755))

	// Write custom config.
	customConfig := "custom: true\n"
	require.NoError(t, os.WriteFile(filepath.Join(siplyDir, "config.yaml"), []byte(customConfig), 0644))

	// hasSiplyConfig should return true.
	assert.True(t, hasSiplyConfig(repoDir))

	// SetupWorkspace overwrites — caller (login.go) is responsible for checking HasSiplyConfig.
	// This test verifies hasSiplyConfig works correctly for the skip logic.
}

func TestParseGitHubRemoteHTTPS(t *testing.T) {
	tmpDir := t.TempDir()
	gitConfigPath := filepath.Join(tmpDir, "config")

	config := `[remote "origin"]
	url = https://github.com/user/repo.git
`
	require.NoError(t, os.WriteFile(gitConfigPath, []byte(config), 0644))

	assert.Equal(t, "user/repo", parseGitHubRemote(gitConfigPath))
}

func TestParseGitHubRemoteSSH(t *testing.T) {
	tmpDir := t.TempDir()
	gitConfigPath := filepath.Join(tmpDir, "config")

	config := `[remote "origin"]
	url = git@github.com:user/repo.git
`
	require.NoError(t, os.WriteFile(gitConfigPath, []byte(config), 0644))

	assert.Equal(t, "user/repo", parseGitHubRemote(gitConfigPath))
}

func TestParseGitHubRemoteHTTPSNoGitSuffix(t *testing.T) {
	tmpDir := t.TempDir()
	gitConfigPath := filepath.Join(tmpDir, "config")

	config := `[remote "origin"]
	url = https://github.com/user/repo
`
	require.NoError(t, os.WriteFile(gitConfigPath, []byte(config), 0644))

	assert.Equal(t, "user/repo", parseGitHubRemote(gitConfigPath))
}

func TestParseGitHubRemoteNonGitHub(t *testing.T) {
	tmpDir := t.TempDir()
	gitConfigPath := filepath.Join(tmpDir, "config")

	config := `[remote "origin"]
	url = https://gitlab.com/user/repo.git
`
	require.NoError(t, os.WriteFile(gitConfigPath, []byte(config), 0644))

	assert.Equal(t, "", parseGitHubRemote(gitConfigPath))
}

func TestParseGitHubRemoteNoOrigin(t *testing.T) {
	tmpDir := t.TempDir()
	gitConfigPath := filepath.Join(tmpDir, "config")

	config := `[remote "upstream"]
	url = https://github.com/user/repo.git
`
	require.NoError(t, os.WriteFile(gitConfigPath, []byte(config), 0644))

	assert.Equal(t, "", parseGitHubRemote(gitConfigPath), "should only match remote 'origin'")
}

func TestEstimateLines(t *testing.T) {
	assert.Equal(t, 2500, estimateLines(100))
	assert.Equal(t, 0, estimateLines(0))
}
