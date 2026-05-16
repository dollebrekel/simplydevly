// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package workspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
)

const rootStoreFileName = "workspace-roots.yaml"

// WorkspaceRootEntry stores per-directory workspace metadata keyed by canonical root.
type WorkspaceRootEntry struct {
	Root              string         `yaml:"root"`
	CreatedAt         time.Time      `yaml:"created_at"`
	LastUsedAt        time.Time      `yaml:"last_used_at"`
	Overrides         map[string]any `yaml:"overrides,omitempty"`
	LinkedBranchState *GitLinkState  `yaml:"linked_branch_state,omitempty"`
}

// GitLinkState persists non-destructive git linkage metadata.
type GitLinkState struct {
	RepoRoot        string `yaml:"repo_root"`
	RepoFingerprint string `yaml:"repo_fingerprint,omitempty"`
	Branch          string `yaml:"branch"`
}

type rootStoreFile struct {
	Version    int                           `yaml:"version"`
	Workspaces map[string]*WorkspaceRootEntry `yaml:"workspaces,omitempty"`
}

// RootStore persists workspace-root-first metadata in ~/.siply/workspace-roots.yaml.
type RootStore struct {
	globalDir string
	data      rootStoreFile
}

// ResolveWorkspaceRoot canonicalizes cwd using Abs+EvalSymlinks+Clean.
func ResolveWorkspaceRoot(cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve abs: %w", err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("workspace: resolve symlinks: %w", err)
	}
	return filepath.Clean(root), nil
}

func NewRootStore(globalDir string) *RootStore {
	return &RootStore{
		globalDir: globalDir,
		data: rootStoreFile{
			Version:    1,
			Workspaces: make(map[string]*WorkspaceRootEntry),
		},
	}
}

func (s *RootStore) path() string {
	return filepath.Join(s.globalDir, rootStoreFileName)
}

// Load reads the root store; parse failures are recovered by rotating corrupt file.
func (s *RootStore) Load(_ context.Context) error {
	if err := os.MkdirAll(s.globalDir, dirPermissions); err != nil {
		return fmt.Errorf("workspace: create root store dir: %w", err)
	}
	lock := fileutil.NewFileLock(s.path())
	if err := lock.ExclusiveLock(); err != nil {
		return fmt.Errorf("workspace: lock root store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	raw, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("workspace: read root store: %w", err)
	}
	if len(raw) == 0 {
		return nil
	}

	var rf rootStoreFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&rf); err != nil {
		badPath := fmt.Sprintf("%s.corrupt.%d", s.path(), time.Now().Unix())
		_ = os.Rename(s.path(), badPath)
		s.data = rootStoreFile{Version: 1, Workspaces: make(map[string]*WorkspaceRootEntry)}
		return nil
	}
	if rf.Version == 0 {
		rf.Version = 1
	}
	if rf.Workspaces == nil {
		rf.Workspaces = make(map[string]*WorkspaceRootEntry)
	}
	s.data = rf
	return nil
}

func (s *RootStore) Save(_ context.Context) error {
	raw, err := yaml.Marshal(&s.data)
	if err != nil {
		return fmt.Errorf("workspace: marshal root store: %w", err)
	}
	lock := fileutil.NewFileLock(s.path())
	if err := lock.ExclusiveLock(); err != nil {
		return fmt.Errorf("workspace: lock root store: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	if err := fileutil.AtomicWriteFile(s.path(), raw, filePermissions); err != nil {
		return fmt.Errorf("workspace: write root store: %w", err)
	}
	return nil
}

// ResolveFromCWD gets/creates a canonical root entry and updates LastUsedAt.
func (s *RootStore) ResolveFromCWD(ctx context.Context, cwd string) (*WorkspaceRootEntry, error) {
	root, err := ResolveWorkspaceRoot(cwd)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	entry, ok := s.data.Workspaces[root]
	if !ok {
		entry = &WorkspaceRootEntry{
			Root:      root,
			CreatedAt: now,
		}
		s.data.Workspaces[root] = entry
	}
	entry.Root = root
	entry.LastUsedAt = now
	if err := s.Save(ctx); err != nil {
		return nil, err
	}
	return entry, nil
}

// ApplyOverrides applies allowlisted overrides (defaults->global already handled elsewhere).
func (s *RootStore) ApplyOverrides(root string, effective map[string]any, allowlist map[string]struct{}) map[string]string {
	sources := make(map[string]string)
	entry := s.data.Workspaces[root]
	if entry == nil || len(entry.Overrides) == 0 {
		for k := range effective {
			sources[k] = "global"
		}
		return sources
	}
	for key := range effective {
		sources[key] = "global"
	}
	for key, value := range entry.Overrides {
		if _, ok := allowlist[key]; !ok {
			continue
		}
		effective[key] = value
		sources[key] = "workspace"
	}
	return sources
}

func (s *RootStore) SetOverrides(root string, overrides map[string]any, allowlist map[string]struct{}) {
	entry := s.data.Workspaces[root]
	if entry == nil {
		entry = &WorkspaceRootEntry{
			Root:      root,
			CreatedAt: time.Now().UTC(),
		}
		s.data.Workspaces[root] = entry
	}
	if entry.Overrides == nil {
		entry.Overrides = make(map[string]any)
	}
	for key := range entry.Overrides {
		delete(entry.Overrides, key)
	}
	for key, value := range overrides {
		if _, ok := allowlist[key]; ok {
			entry.Overrides[key] = value
		}
	}
}

func (s *RootStore) WorkspaceRoots() []string {
	roots := make([]string, 0, len(s.data.Workspaces))
	for root := range s.data.Workspaces {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	return roots
}

func ProviderOverrideAllowlist() map[string]struct{} {
	return map[string]struct{}{
		"default":     {},
		"model":       {},
		"local_model": {},
		"local_url":   {},
	}
}

func (s *RootStore) EffectiveProviderConfig(root string, global core.ProviderConfig) (core.ProviderConfig, map[string]string) {
	effective := global
	sources := map[string]string{
		"default":     "global",
		"model":       "global",
		"local_model": "global",
		"local_url":   "global",
	}
	entry := s.data.Workspaces[root]
	if entry == nil || len(entry.Overrides) == 0 {
		return effective, sources
	}
	allow := ProviderOverrideAllowlist()
	for key, value := range entry.Overrides {
		if _, ok := allow[key]; !ok {
			continue
		}
		str, ok := value.(string)
		if !ok {
			continue
		}
		switch key {
		case "default":
			effective.Default = str
		case "model":
			effective.Model = str
		case "local_model":
			effective.LocalModel = str
		case "local_url":
			effective.LocalURL = str
		}
		sources[key] = "workspace"
	}
	return effective, sources
}

// RefreshGitLinkState updates per-workspace git metadata without mutating git state.
func (s *RootStore) RefreshGitLinkState(root string) {
	entry := s.data.Workspaces[root]
	if entry == nil {
		return
	}
	repoRoot, branch, ok := detectGitMetadata(root)
	if !ok {
		return
	}
	entry.LinkedBranchState = &GitLinkState{
		RepoRoot:        repoRoot,
		RepoFingerprint: repoFingerprint(repoRoot),
		Branch:          branch,
	}
}

// ValidateGitLink returns warning text when persisted link is invalid for current cwd.
func (s *RootStore) ValidateGitLink(cwd string, entry *WorkspaceRootEntry) string {
	if entry == nil || entry.LinkedBranchState == nil {
		return ""
	}
	repoRoot, branch, ok := detectGitMetadata(cwd)
	if !ok {
		return "git metadata unavailable; using workspace defaults"
	}
	if repoRoot != entry.LinkedBranchState.RepoRoot {
		return "workspace git link repo mismatch; using workspace defaults"
	}
	if branch == "" || strings.EqualFold(branch, "HEAD") {
		return "detached or invalid branch state; using workspace defaults"
	}
	if branch != entry.LinkedBranchState.Branch {
		return "workspace linked branch missing or changed; using workspace defaults"
	}
	return ""
}

func detectGitMetadata(dir string) (repoRoot, branch string, ok bool) {
	rootOut, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", false
	}
	branchOut, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", "", false
	}
	return strings.TrimSpace(string(rootOut)), strings.TrimSpace(string(branchOut)), true
}

func repoFingerprint(repoRoot string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(repoRoot)))
	return "sha256:" + hex.EncodeToString(sum[:])
}
