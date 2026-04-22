// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/pkg/siplyui"
)

// treeDepth returns the maximum nesting depth of a TreeNode slice.
func treeDepth(nodes []siplyui.TreeNode, depth int) int {
	max := depth
	for _, n := range nodes {
		if d := treeDepth(n.Children, depth+1); d > max {
			max = d
		}
	}
	return max
}

// TestBuildFileTree_BasicStructure verifies dirs appear before files and names are correct.
func TestBuildFileTree_BasicStructure(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# Test"), 0o644))

	nodes := buildFileTree(root, "", nil, 0)

	require.NotEmpty(t, nodes)
	assert.Equal(t, "pkg", nodes[0].Label)
	assert.Equal(t, "📁", nodes[0].Icon)
	assert.True(t, nodes[0].Expanded, "depth-0 dirs should be expanded")

	var fileLabels []string
	for _, n := range nodes[1:] {
		fileLabels = append(fileLabels, n.Label)
	}
	assert.Contains(t, fileLabels, "README.md")
	assert.Contains(t, fileLabels, "main.go")
}

// TestBuildFileTree_SkipsGitDir verifies .git and node_modules are excluded.
func TestBuildFileTree_SkipsGitDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "app.js"), []byte(""), 0o644))

	nodes := buildFileTree(root, "", nil, 0)

	for _, n := range nodes {
		assert.NotEqual(t, ".git", n.Label)
		assert.NotEqual(t, "node_modules", n.Label)
	}
	require.Len(t, nodes, 1)
	assert.Equal(t, "app.js", nodes[0].Label)
}

// TestBuildFileTree_MaxDepth verifies recursion stops at depth 10.
func TestBuildFileTree_MaxDepth(t *testing.T) {
	root := t.TempDir()
	cur := root
	for range 12 {
		cur = filepath.Join(cur, "level")
		require.NoError(t, os.Mkdir(cur, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(cur, "file.txt"), []byte(""), 0o644))
	}

	nodes := buildFileTree(root, "", nil, 0)
	depth := treeDepth(nodes, 0)
	// max recursion depth is 10, so max visible depth is 11 (level 0 = root children)
	assert.LessOrEqual(t, depth, 11, "tree depth should not exceed maxDepth+1")
}

// TestBuildFileTree_FileTypeIcons verifies icons are assigned per extension.
func TestBuildFileTree_FileTypeIcons(t *testing.T) {
	root := t.TempDir()
	wantIcons := map[string]string{
		"main.go":    "🐹",
		"app.js":     "🟨",
		"script.py":  "🐍",
		"notes.md":   "📝",
		"deploy.sh":  "🔧",
		"service.rs": "🦀",
		"readme.txt": "📄",
	}
	for name := range wantIcons {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(""), 0o644))
	}

	nodes := buildFileTree(root, "", nil, 0)

	byName := make(map[string]string)
	for _, n := range nodes {
		base := strings.SplitN(n.Label, " \x1b", 2)[0]
		byName[base] = n.Icon
	}
	for name, wantIcon := range wantIcons {
		assert.Equal(t, wantIcon, byName[name], "wrong icon for %s", name)
	}
}

// TestBuildFileTree_GitStatusAnnotation verifies git status indicators on labels.
func TestBuildFileTree_GitStatusAnnotation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"modified.go", "added.go", "untracked.go", "deleted.go", "clean.go"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(""), 0o644))
	}

	gitStatus := map[string]byte{
		"modified.go":  'M',
		"added.go":     'A',
		"untracked.go": '?',
		"deleted.go":   'D',
	}

	nodes := buildFileTree(root, "", gitStatus, 0)

	byBase := make(map[string]string)
	for _, n := range nodes {
		base := strings.SplitN(n.Label, " \x1b", 2)[0]
		byBase[base] = n.Label
	}

	assert.Contains(t, byBase["modified.go"], "[M]")
	assert.Contains(t, byBase["added.go"], "[A]")
	assert.Contains(t, byBase["untracked.go"], "[?]")
	assert.Contains(t, byBase["deleted.go"], "[D]")
	assert.Equal(t, "clean.go", byBase["clean.go"], "clean file should have no git indicator")
}

// TestBuildFileTree_SymlinkPrefixCollision verifies symlinks to prefix-collision dirs are excluded.
func TestBuildFileTree_SymlinkPrefixCollision(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "project")
	projectEvil := filepath.Join(base, "project-evil")

	require.NoError(t, os.MkdirAll(project, 0o755))
	require.NoError(t, os.MkdirAll(projectEvil, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectEvil, "secret.txt"), []byte("evil"), 0o644))

	// Create symlink inside project pointing to project-evil.
	require.NoError(t, os.Symlink(projectEvil, filepath.Join(project, "escape-link")))

	// Also create a valid file inside project.
	require.NoError(t, os.WriteFile(filepath.Join(project, "safe.txt"), []byte("safe"), 0o644))

	nodes := buildFileTree(project, "", nil, 0)

	var labels []string
	for _, n := range nodes {
		labels = append(labels, n.Label)
	}

	assert.Contains(t, labels, "safe.txt", "valid file should be included")
	assert.NotContains(t, labels, "escape-link", "symlink escaping project root should be excluded")
}

// TestBuildFileTree_SymlinkInsideProject verifies symlinks within project are kept.
func TestBuildFileTree_SymlinkInsideProject(t *testing.T) {
	project := t.TempDir()
	subdir := filepath.Join(project, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "target.txt"), []byte("data"), 0o644))

	// Symlink within project.
	require.NoError(t, os.Symlink(subdir, filepath.Join(project, "link-to-sub")))

	nodes := buildFileTree(project, "", nil, 0)

	var labels []string
	for _, n := range nodes {
		labels = append(labels, n.Label)
	}
	assert.Contains(t, labels, "link-to-sub", "symlink within project should be included")
}

// TestParseGitStatus_NonGitDir verifies empty result in non-git directories.
func TestParseGitStatus_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	result := parseGitStatus(dir)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// TestGitStatusIndicator_Colors verifies ANSI color codes per status byte.
func TestGitStatusIndicator_Colors(t *testing.T) {
	assert.Contains(t, gitStatusIndicator('M'), "\x1b[33m")
	assert.Contains(t, gitStatusIndicator('A'), "\x1b[32m")
	assert.Contains(t, gitStatusIndicator('?'), "\x1b[90m")
	assert.Contains(t, gitStatusIndicator('D'), "\x1b[31m")
	assert.Equal(t, "", gitStatusIndicator('X'))
}

// TestSkipDir verifies known dirs are excluded.
func TestSkipDir(t *testing.T) {
	assert.True(t, skipDir(".git"))
	assert.True(t, skipDir("node_modules"))
	assert.True(t, skipDir(".siply"))
	assert.True(t, skipDir("vendor"))
	assert.False(t, skipDir("src"))
	assert.False(t, skipDir("pkg"))
}

// TestFileIcon verifies icon per extension and directory flag.
func TestFileIcon(t *testing.T) {
	cases := []struct{ name string; isDir bool; want string }{
		{"dir", true, "📁"},
		{"main.go", false, "🐹"},
		{"app.js", false, "🟨"},
		{"app.ts", false, "🟨"},
		{"data.json", false, "📋"},
		{"config.yaml", false, "📄"},
		{"README.md", false, "📝"},
		{"script.py", false, "🐍"},
		{"lib.rs", false, "🦀"},
		{"api.proto", false, "📡"},
		{"run.sh", false, "🔧"},
		{"unknown.xyz", false, "📄"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, fileIcon(tc.name, tc.isDir))
		})
	}
}
