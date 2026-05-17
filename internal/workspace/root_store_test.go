// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package workspace

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestResolveWorkspaceRoot_CanonicalizesSymlinkAndRelative(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	require.NoError(t, os.MkdirAll(real, 0o755))
	require.NoError(t, os.Symlink(real, link))

	got, err := ResolveWorkspaceRoot(filepath.Join(link, "..", "link"))
	require.NoError(t, err)
	expected, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestRootStore_ResolveFromCWD_DeterministicAcrossRuns(t *testing.T) {
	globalDir := t.TempDir()
	ws := t.TempDir()

	s1 := NewRootStore(globalDir)
	require.NoError(t, s1.Load(context.Background()))
	e1, err := s1.ResolveFromCWD(context.Background(), ws)
	require.NoError(t, err)

	s2 := NewRootStore(globalDir)
	require.NoError(t, s2.Load(context.Background()))
	e2, err := s2.ResolveFromCWD(context.Background(), ws)
	require.NoError(t, err)

	assert.Equal(t, e1.Root, e2.Root)
	assert.Equal(t, []string{e1.Root}, s2.WorkspaceRoots())
}

func TestRootStore_Load_CorruptRecovery(t *testing.T) {
	globalDir := t.TempDir()
	path := filepath.Join(globalDir, rootStoreFileName)
	require.NoError(t, os.MkdirAll(globalDir, 0o700))
	require.NoError(t, os.WriteFile(path, []byte(":\nbad: ["), 0o600))

	s := NewRootStore(globalDir)
	require.NoError(t, s.Load(context.Background()))
	assert.Empty(t, s.WorkspaceRoots())
}

func TestRootStore_SaveCreatesStoreDir(t *testing.T) {
	globalDir := filepath.Join(t.TempDir(), "missing", "siply")
	s := NewRootStore(globalDir)
	wsRoot := filepath.Join(t.TempDir(), "example")
	s.data.Workspaces[wsRoot] = &WorkspaceRootEntry{Root: wsRoot}

	require.NoError(t, s.Save(context.Background()))
	assert.FileExists(t, filepath.Join(globalDir, rootStoreFileName))
}

func TestInitializeWorkspace_PreservesExistingEntries(t *testing.T) {
	globalDir := t.TempDir()
	first := t.TempDir()
	second := t.TempDir()

	_, err := InitializeWorkspace(context.Background(), globalDir, first)
	require.NoError(t, err)
	_, err = InitializeWorkspace(context.Background(), globalDir, second)
	require.NoError(t, err)

	s := NewRootStore(globalDir)
	require.NoError(t, s.Load(context.Background()))
	roots := s.WorkspaceRoots()
	expectedFirst, err := ResolveWorkspaceRoot(first)
	require.NoError(t, err)
	expectedSecond, err := ResolveWorkspaceRoot(second)
	require.NoError(t, err)
	assert.Contains(t, roots, expectedFirst)
	assert.Contains(t, roots, expectedSecond)
}

func TestApplyOverrides_AllowlistAndSources(t *testing.T) {
	s := NewRootStore(t.TempDir())
	root := filepath.Join(t.TempDir(), "ws")
	s.data.Workspaces[root] = &WorkspaceRootEntry{
		Root: root,
		Overrides: map[string]any{
			"provider": "openai",
			"api_key":  "should-not-apply",
		},
	}
	effective := map[string]any{
		"provider": "anthropic",
		"model":    "sonnet",
		"api_key":  "global",
	}
	allow := map[string]struct{}{
		"provider": {},
		"model":    {},
	}

	sources := s.ApplyOverrides(root, effective, allow)
	assert.Equal(t, "openai", effective["provider"])
	assert.Equal(t, "global", effective["api_key"])
	assert.Equal(t, "workspace", sources["provider"])
	assert.Equal(t, "global", sources["api_key"])
}

func TestEffectiveProviderConfig_Allowlist(t *testing.T) {
	s := NewRootStore(t.TempDir())
	root := filepath.Join(t.TempDir(), "ws")
	s.data.Workspaces[root] = &WorkspaceRootEntry{
		Root: root,
		Overrides: map[string]any{
			"default":   "openai",
			"model":     "gpt-5",
			"local_url": "http://127.0.0.1:11434",
			"api_key":   "blocked",
		},
	}
	global := core.ProviderConfig{
		Default: "anthropic",
		Model:   "claude-sonnet",
	}
	effective, sources := s.EffectiveProviderConfig(root, global)
	assert.Equal(t, "openai", effective.Default)
	assert.Equal(t, "gpt-5", effective.Model)
	assert.Equal(t, "http://127.0.0.1:11434", effective.LocalURL)
	assert.Equal(t, "workspace", sources["default"])
	assert.Equal(t, "workspace", sources["model"])
	assert.Equal(t, "workspace", sources["local_url"])
}

func TestValidateGitLink_WarnsOnRepoMismatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	cmd := exec.Command("git", "-C", repo, "init")
	require.NoError(t, cmd.Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "init").Run())
	cmd = exec.Command("git", "-C", repo, "checkout", "-b", "feature/x")
	require.NoError(t, cmd.Run())

	s := NewRootStore(t.TempDir())
	entry := &WorkspaceRootEntry{
		Root: repo,
		LinkedBranchState: &GitLinkState{
			RepoRoot: "/different/repo",
			Branch:   "feature/x",
		},
	}
	warn := s.ValidateGitLink(repo, entry)
	assert.Contains(t, warn, "repo mismatch")
}

func TestValidateGitLink_WarnsOnMissingBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "init").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "checkout", "-b", "feature/current").Run())

	s := NewRootStore(t.TempDir())
	entry := &WorkspaceRootEntry{
		Root: repo,
		LinkedBranchState: &GitLinkState{
			RepoRoot: repo,
			Branch:   "feature/missing",
		},
	}
	warn := s.ValidateGitLink(repo, entry)
	assert.Contains(t, warn, "missing or changed")
}

func TestValidateGitLink_WarnsOnFingerprintChange(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "init").Run())

	s := NewRootStore(t.TempDir())
	s.data.Workspaces[repo] = &WorkspaceRootEntry{Root: repo}
	s.RefreshGitLinkState(repo)
	entry := s.data.Workspaces[repo]
	require.NotNil(t, entry.LinkedBranchState)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("changed"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "change").Run())

	warn := s.ValidateGitLink(repo, entry)
	assert.Contains(t, warn, "fingerprint changed")
}

func TestValidateGitLink_WarnsOnDetachedHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "init").Run())

	shaOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	sha := string(bytes.TrimSpace(shaOut))
	require.NoError(t, exec.Command("git", "-C", repo, "checkout", sha).Run())

	s := NewRootStore(t.TempDir())
	entry := &WorkspaceRootEntry{
		Root: repo,
		LinkedBranchState: &GitLinkState{
			RepoRoot: repo,
			Branch:   "main",
		},
	}
	warn := s.ValidateGitLink(repo, entry)
	assert.Contains(t, warn, "detached")
}

func TestRefreshGitLinkState_ClearsDetachedHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "init").Run())
	shaOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	require.NoError(t, exec.Command("git", "-C", repo, "checkout", string(bytes.TrimSpace(shaOut))).Run())

	s := NewRootStore(t.TempDir())
	s.data.Workspaces[repo] = &WorkspaceRootEntry{
		Root: repo,
		LinkedBranchState: &GitLinkState{
			RepoRoot: repo,
			Branch:   "main",
		},
	}
	s.RefreshGitLinkState(repo)
	assert.Nil(t, s.data.Workspaces[repo].LinkedBranchState)
}

func TestRefreshGitLinkState_SetsFingerprintHash(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.email", "test@example.com").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "config", "user.name", "test").Run())
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("x"), 0o644))
	require.NoError(t, exec.Command("git", "-C", repo, "add", "README.md").Run())
	require.NoError(t, exec.Command("git", "-C", repo, "commit", "-m", "init").Run())

	s := NewRootStore(t.TempDir())
	s.data.Workspaces[repo] = &WorkspaceRootEntry{Root: repo}
	s.RefreshGitLinkState(repo)
	entry := s.data.Workspaces[repo]
	require.NotNil(t, entry.LinkedBranchState)
	assert.True(t, strings.HasPrefix(entry.LinkedBranchState.RepoFingerprint, "sha256:"))
	assert.NotEqual(t, entry.LinkedBranchState.RepoRoot, entry.LinkedBranchState.RepoFingerprint)
}
