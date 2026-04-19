// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package profiles

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

// ProfileLoader discovers and loads profiles from global and project-level directories.
// Project-level profiles override global profiles with the same name.
type ProfileLoader struct {
	globalDir  string
	projectDir string // empty string disables project-level loading
	mu         sync.RWMutex
	loaded     map[string]*Profile
}

// NewProfileLoader creates a ProfileLoader that scans globalDir and (optionally) projectDir.
// Pass an empty projectDir to disable project-level loading.
func NewProfileLoader(globalDir, projectDir string) *ProfileLoader {
	return &ProfileLoader{
		globalDir:  globalDir,
		projectDir: projectDir,
		loaded:     make(map[string]*Profile),
	}
}

// GlobalDir returns the path to the global profiles directory.
// Respects the SIPLY_HOME environment variable.
func GlobalDir(homeDir string) string {
	if v := os.Getenv("SIPLY_HOME"); v != "" {
		return filepath.Join(v, "profiles")
	}
	return filepath.Join(homeDir, ".siply", "profiles")
}

// LoadAll scans globalDir then projectDir, parsing each profile's manifest and profile.yaml.
// Project-level profiles override global ones with the same name.
func (l *ProfileLoader) LoadAll(_ context.Context) error {
	if l.globalDir == "" {
		return fmt.Errorf("profiles: globalDir is empty, call NewProfileLoader() first")
	}

	globalProfiles, err := l.loadDir(l.globalDir, "global")
	if err != nil {
		return err
	}

	loaded := make(map[string]*Profile, len(globalProfiles))
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

// Get returns a profile by name. Returns ErrProfileNotFound if missing.
func (l *ProfileLoader) Get(name string) (*Profile, error) {
	if name == "" {
		return nil, fmt.Errorf("profiles: name is required")
	}

	l.mu.RLock()
	profile, ok := l.loaded[name]
	l.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProfileNotFound, name)
	}
	cp := *profile
	return &cp, nil
}

// List returns all loaded profiles sorted by name (thread-safe).
func (l *ProfileLoader) List() []Profile {
	l.mu.RLock()
	defer l.mu.RUnlock()

	profiles := make([]Profile, 0, len(l.loaded))
	for _, p := range l.loaded {
		profiles = append(profiles, *p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles
}

// loadDir scans a directory for profile sub-directories and loads each one.
// Missing directories are silently ignored.
func (l *ProfileLoader) loadDir(dir, source string) ([]*Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("profiles: read dir %s: %w", dir, err)
	}

	var profiles []*Profile
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if strings.ContainsAny(entry.Name(), "/\\") || entry.Name() == ".." || entry.Name() == "." {
			continue
		}
		profileDir := filepath.Join(dir, entry.Name())
		profile, err := loadProfileFromDir(profileDir, source)
		if err != nil {
			slog.Warn("profiles: load failed", "dir", profileDir, "err", err)
			continue
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// loadProfileFromDir loads a single profile from its directory.
func loadProfileFromDir(dir, source string) (*Profile, error) {
	m, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf("profiles: load manifest from %s: %w", dir, err)
	}

	if m.Kind != "Profile" {
		return nil, fmt.Errorf("profiles: manifest in %s has kind %q, expected \"Profile\"", dir, m.Kind)
	}

	if m.Metadata.Name == "" || !namePattern.MatchString(m.Metadata.Name) {
		return nil, fmt.Errorf("profiles: manifest name %q in %s does not match naming convention", m.Metadata.Name, dir)
	}

	dirName := filepath.Base(dir)
	if dirName != m.Metadata.Name {
		return nil, fmt.Errorf("profiles: directory name %q differs from manifest name %q", dirName, m.Metadata.Name)
	}

	profilePath := filepath.Join(dir, "profile.yaml")
	data, err := readFileNoFollow(profilePath, plugins.MaxYAMLFileSize)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: no profile.yaml in %s", ErrNoProfile, dir)
		}
		return nil, fmt.Errorf("profiles: read profile.yaml from %s: %w", dir, err)
	}

	parsed, err := plugins.ParsePluginYAML(data)
	if err != nil {
		return nil, fmt.Errorf("profiles: parse profile.yaml in %s: %w", dir, err)
	}

	profile, err := parseProfileConfig(parsed)
	if err != nil {
		return nil, fmt.Errorf("profiles: invalid profile.yaml in %s: %w", dir, err)
	}

	profile.Name = m.Metadata.Name
	profile.Version = m.Metadata.Version
	profile.Description = m.Metadata.Description
	profile.Source = source
	profile.Dir = dir

	return profile, nil
}
