// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"siply.dev/siply/internal/plugins"
)

// AgentConfigLoader discovers and loads agent configs from global and project-level directories.
// Project-level configs override global configs with the same name (AC#4).
type AgentConfigLoader struct {
	globalDir  string
	projectDir string // empty string disables project-level loading
	mu         sync.RWMutex
	loaded     map[string]*AgentProfile // name → profile
}

// NewAgentConfigLoader creates an AgentConfigLoader that scans globalDir and (optionally) projectDir.
// Pass an empty projectDir to disable project-level loading.
func NewAgentConfigLoader(globalDir, projectDir string) *AgentConfigLoader {
	return &AgentConfigLoader{
		globalDir:  globalDir,
		projectDir: projectDir,
		loaded:     make(map[string]*AgentProfile),
	}
}

// GlobalDir returns the path to the global agent configs directory.
// Respects the SIPLY_HOME environment variable.
func GlobalDir(homeDir string) string {
	if v := os.Getenv("SIPLY_HOME"); v != "" {
		return filepath.Join(v, "agents")
	}
	return filepath.Join(homeDir, ".siply", "agents")
}

// LoadAll scans globalDir then projectDir, parsing each agent config's manifest and config.yaml.
// Project-level configs override global ones with the same name.
func (l *AgentConfigLoader) LoadAll(_ context.Context) error {
	if l.globalDir == "" {
		return fmt.Errorf("agents: globalDir is empty, call NewAgentConfigLoader() first")
	}

	globalProfiles, err := l.loadDir(l.globalDir, "global")
	if err != nil {
		return err
	}

	loaded := make(map[string]*AgentProfile, len(globalProfiles))
	for _, p := range globalProfiles {
		loaded[p.Name] = p
	}

	if l.projectDir != "" {
		projectProfiles, err := l.loadDir(l.projectDir, "project")
		if err != nil {
			return err
		}
		for _, p := range projectProfiles {
			loaded[p.Name] = p
		}
	}

	l.mu.Lock()
	l.loaded = loaded
	l.mu.Unlock()

	return nil
}

// Get returns an agent config by name. Returns ErrAgentConfigNotFound if missing.
func (l *AgentConfigLoader) Get(name string) (*AgentProfile, error) {
	if name == "" {
		return nil, fmt.Errorf("agents: name is required")
	}

	l.mu.RLock()
	profile, ok := l.loaded[name]
	l.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAgentConfigNotFound, name)
	}
	return profile, nil
}

// List returns all loaded agent configs sorted by name (thread-safe).
func (l *AgentConfigLoader) List() []AgentProfile {
	l.mu.RLock()
	defer l.mu.RUnlock()

	profiles := make([]AgentProfile, 0, len(l.loaded))
	for _, p := range l.loaded {
		profiles = append(profiles, *p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles
}

// loadDir scans a directory for agent config sub-directories and loads each one.
// Missing directories are silently ignored.
func (l *AgentConfigLoader) loadDir(dir, source string) ([]*AgentProfile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("agents: read dir %s: %w", dir, err)
	}

	var profiles []*AgentProfile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.ContainsAny(entry.Name(), "/\\") || entry.Name() == ".." || entry.Name() == "." {
			continue
		}
		agentDir := filepath.Join(dir, entry.Name())
		profile, err := loadAgentConfigFromDir(agentDir, source)
		if err != nil {
			slog.Warn("agents: load failed", "dir", agentDir, "err", err)
			continue
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// loadAgentConfigFromDir loads a single agent config from its directory.
func loadAgentConfigFromDir(dir, source string) (*AgentProfile, error) {
	m, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf("agents: load manifest from %s: %w", dir, err)
	}

	dirName := filepath.Base(dir)
	if dirName != m.Metadata.Name {
		return nil, fmt.Errorf("agents: directory name %q differs from manifest name %q", dirName, m.Metadata.Name)
	}

	configPath := filepath.Join(dir, "config.yaml")
	data, err := readFileNoFollow(configPath, plugins.MaxYAMLFileSize)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: no config.yaml in %s", ErrNoAgentConfig, dir)
		}
		return nil, fmt.Errorf("agents: read config.yaml from %s: %w", dir, err)
	}

	parsed, err := plugins.ParsePluginYAML(data)
	if err != nil {
		return nil, fmt.Errorf("agents: parse config.yaml in %s: %w", dir, err)
	}

	profile, err := parseAgentConfig(parsed)
	if err != nil {
		return nil, fmt.Errorf("agents: invalid config.yaml in %s: %w", dir, err)
	}

	profile.Name = m.Metadata.Name
	profile.Version = m.Metadata.Version
	profile.Description = m.Metadata.Description
	profile.Source = source
	profile.Dir = dir

	return profile, nil
}
