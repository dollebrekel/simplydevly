// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package licensing

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"siply.dev/siply/internal/core"
)

// marketRepo is the JSON response from the simply-market proxy for GitHub repos.
type marketRepo struct {
	FullName string `json:"full_name"`
	Language string `json:"language"`
	Size     int    `json:"size"` // KB from GitHub API
}

// httpClient is the HTTP client for market API calls. Overridable for testing.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// scanRoots returns the directories to scan for local git repos. Overridable for testing.
var scanRoots = defaultScanRoots

func defaultScanRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	roots := []string{home}
	for _, sub := range []string{"projects", "work", "src", "dev"} {
		p := filepath.Join(home, sub)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			roots = append(roots, p)
		}
	}
	return roots
}

// maxScanDepth limits directory traversal to avoid scanning the entire filesystem.
const maxScanDepth = 3

// DiscoverRepos matches GitHub repos against local git remotes.
// Returns empty list (no error) if not logged in with GitHub or repo scope was denied (AC #8).
func (v *licenseValidator) DiscoverRepos(ctx context.Context) ([]core.DiscoveredRepo, error) {
	if v.cached == nil || v.cached.AuthProvider != "github" {
		return nil, nil
	}

	ghRepos, err := fetchMarketRepos(ctx, v.cached.Token)
	if err != nil {
		return nil, fmt.Errorf("licensing: failed to fetch repos: %w", err)
	}
	if len(ghRepos) == 0 {
		return nil, nil
	}

	// Build lookup: lowercase "user/repo" -> marketRepo.
	repoMap := make(map[string]marketRepo, len(ghRepos))
	for _, r := range ghRepos {
		repoMap[strings.ToLower(r.FullName)] = r
	}

	localRepos := scanLocalGitRepos()

	// Skip $HOME itself — dotfile managers (yadm, bare git) make it a repo,
	// and creating .siply/ there conflicts with the siply config dir.
	home, _ := os.UserHomeDir()
	delete(localRepos, home)

	var discovered []core.DiscoveredRepo
	for localPath, remoteFullName := range localRepos {
		if mr, ok := repoMap[strings.ToLower(remoteFullName)]; ok {
			discovered = append(discovered, core.DiscoveredRepo{
				GitHubFullName: mr.FullName,
				LocalPath:      localPath,
				Language:       mr.Language,
				LinesOfCode:    estimateLines(mr.Size),
				HasSiplyConfig: hasSiplyConfig(localPath),
			})
		}
	}
	return discovered, nil
}

// fetchMarketRepos calls the simply-market proxy to list the user's GitHub repos.
// Returns empty list on 401/403 (no repo scope — AC #8).
func fetchMarketRepos(ctx context.Context, token string) ([]marketRepo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", MarketBaseURL+"/api/user/repos", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return nil, nil // No repo scope or invalid token — return empty, no error.
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("market API returned %d", resp.StatusCode)
	}

	// Cap response body at 10 MB to prevent OOM from malicious/misconfigured server.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}

	var repos []marketRepo
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("licensing: failed to parse repos response: %w", err)
	}
	return repos, nil
}

// scanLocalGitRepos walks common directories and returns map[localPath]githubFullName.
func scanLocalGitRepos() map[string]string {
	repos := make(map[string]string)
	seen := make(map[string]bool)

	for _, root := range scanRoots() {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		// Resolve symlinks on root paths too (e.g. ~/projects → /data/projects).
		realRoot, err := filepath.EvalSymlinks(absRoot)
		if err != nil {
			continue
		}
		walkGitRepos(realRoot, 0, repos, seen)
	}
	return repos
}

func walkGitRepos(dir string, depth int, repos map[string]string, seen map[string]bool) {
	if depth > maxScanDepth || seen[dir] {
		return
	}
	seen[dir] = true

	gitConfigPath := filepath.Join(dir, ".git", "config")
	if fullName := parseGitHubRemote(gitConfigPath); fullName != "" {
		repos[dir] = fullName
		return // Don't recurse into git repos.
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		childPath := filepath.Join(dir, e.Name())
		// Use os.Stat (not Lstat) to follow symlinks.
		info, err := os.Stat(childPath)
		if err != nil || !info.IsDir() {
			continue
		}
		// Resolve to real path for loop detection via seen map.
		realPath, err := filepath.EvalSymlinks(childPath)
		if err != nil {
			continue
		}
		if seen[realPath] {
			continue
		}
		walkGitRepos(realPath, depth+1, repos, seen)
	}
}

// githubRemoteRe matches GitHub remote URLs in both HTTPS and SSH formats.
var githubRemoteRe = regexp.MustCompile(`github\.com[:/]([^/\s]+/[^/.\s]+?)(?:\.git)?(?:\s|$)`)

// parseGitHubRemote reads .git/config and extracts the GitHub full name from remote "origin".
func parseGitHubRemote(gitConfigPath string) string {
	f, err := os.Open(gitConfigPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inOrigin := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inOrigin = false
			continue
		}
		if inOrigin && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			if m := githubRemoteRe.FindStringSubmatch(url); len(m) > 1 {
				return m[1]
			}
		}
	}
	return ""
}

func hasSiplyConfig(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, ".siply"))
	return err == nil
}

// estimateLines converts GitHub repo size (KB) to approximate lines of code.
func estimateLines(sizeKB int) int {
	return sizeKB * 25
}

// --- Workspace setup (AC #4-#6) ---

// SetupWorkspace creates .siply/config.yaml in a repo directory with language defaults.
func SetupWorkspace(repoPath, language string) error {
	siplyDir := filepath.Join(repoPath, ".siply")
	if err := os.MkdirAll(siplyDir, 0755); err != nil {
		return fmt.Errorf("licensing: failed to create .siply directory: %w", err)
	}

	configPath := filepath.Join(siplyDir, "config.yaml")
	content := languageDefaults(language)
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("licensing: failed to write config.yaml: %w", err)
	}
	return nil
}

func languageDefaults(language string) string {
	return fmt.Sprintf(`# .siply/config.yaml (auto-generated by repo discovery)
provider:
  default: anthropic
# Language: %s
`, language)
}
