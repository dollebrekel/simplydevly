// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package skills provides skill loading, registry, and slash-command dispatch.
// Skills are Tier 1 YAML plugins stored in a dedicated skills/ directory.
// They inject prompt templates into the agent context via slash commands.
package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"siply.dev/siply/internal/plugins"
)

// Sentinel errors for skill operations.
var (
	ErrSkillNotFound = errors.New("skills: skill not found")
	ErrNoPrompts     = errors.New("skills: skill has no prompts")
)

// PromptTemplate holds a single named prompt template within a skill.
type PromptTemplate struct {
	Name        string
	Description string
	Template    string // raw Go template text with {{.input}}, {{.problem}}, {{.context}}
}

// Skill represents a loaded skill with its metadata and prompt templates.
type Skill struct {
	Name        string
	Version     string
	Description string
	Prompts     map[string]PromptTemplate
	Source      string // "global" or "project"
	Dir         string
}

// SkillLoader discovers and loads skills from the global and project-level skill directories.
// Project-level skills override global skills with the same name (AC#4).
type SkillLoader struct {
	globalDir  string
	projectDir string // empty string disables project-level loading
	mu         sync.RWMutex
	loaded     map[string]*Skill // name → skill
}

// NewSkillLoader creates a SkillLoader that scans globalDir and (optionally) projectDir.
// Pass an empty projectDir to disable project-level skill loading.
func NewSkillLoader(globalDir, projectDir string) *SkillLoader {
	return &SkillLoader{
		globalDir:  globalDir,
		projectDir: projectDir,
		loaded:     make(map[string]*Skill),
	}
}

// GlobalDir returns the path to the global skills directory.
// Respects the SIPLY_HOME environment variable (consistent with TD-1 cache path pattern).
func GlobalDir(homeDir string) string {
	if v := os.Getenv("SIPLY_HOME"); v != "" {
		return filepath.Join(v, "skills")
	}
	return filepath.Join(homeDir, ".siply", "skills")
}

// LoadAll scans globalDir then projectDir, parsing each skill's manifest and prompts.
// Project-level skills override global ones with the same name.
func (l *SkillLoader) LoadAll(_ context.Context) error {
	if l.globalDir == "" {
		return fmt.Errorf("skills: globalDir is empty, call NewSkillLoader() first")
	}

	// Load global skills first.
	globalSkills, err := l.loadDir(l.globalDir, "global")
	if err != nil {
		return err
	}

	loaded := make(map[string]*Skill, len(globalSkills))
	for _, s := range globalSkills {
		loaded[s.Name] = s
	}

	// Load project-level skills, overriding global ones with the same name.
	if l.projectDir != "" {
		projectSkills, err := l.loadDir(l.projectDir, "project")
		if err != nil {
			return err
		}
		for _, s := range projectSkills {
			loaded[s.Name] = s
		}
	}

	l.mu.Lock()
	l.loaded = loaded
	l.mu.Unlock()

	return nil
}

// Get returns a skill by name. Project-level skill takes precedence over global (AC#4).
func (l *SkillLoader) Get(name string) (*Skill, error) {
	if name == "" {
		return nil, fmt.Errorf("skills: name is required")
	}

	l.mu.RLock()
	skill, ok := l.loaded[name]
	l.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSkillNotFound, name)
	}
	return skill, nil
}

// List returns all loaded skills sorted by name (thread-safe, AC#5).
func (l *SkillLoader) List() []Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()

	skills := make([]Skill, 0, len(l.loaded))
	for _, s := range l.loaded {
		skills = append(skills, *s)
	}
	// Deterministic iteration order.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills
}

// loadDir scans a directory for skill sub-directories and loads each one.
// Missing directories are silently ignored (not an error — skills dir may not exist yet).
func (l *SkillLoader) loadDir(dir, source string) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: read dir %s: %w", dir, err)
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Reject path traversal in directory names.
		if strings.ContainsAny(entry.Name(), "/\\") || entry.Name() == ".." || entry.Name() == "." {
			continue
		}
		skillDir := filepath.Join(dir, entry.Name())
		skill, err := loadSkillFromDir(skillDir, source)
		if err != nil {
			slog.Warn("skills: load failed", "dir", skillDir, "err", err)
			continue
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

// loadSkillFromDir loads a single skill from its directory.
func loadSkillFromDir(dir, source string) (*Skill, error) {
	m, err := plugins.LoadManifestFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skills: load manifest from %s: %w", dir, err)
	}

	dirName := filepath.Base(dir)
	if dirName != m.Metadata.Name {
		return nil, fmt.Errorf("skills: directory name %q differs from manifest name %q", dirName, m.Metadata.Name)
	}

	prompts, err := loadPromptsFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skills: load prompts from %s: %w", dir, err)
	}

	return &Skill{
		Name:        m.Metadata.Name,
		Version:     m.Metadata.Version,
		Description: m.Metadata.Description,
		Prompts:     prompts,
		Source:      source,
		Dir:         dir,
	}, nil
}

// readFileNoFollow opens a file with O_NOFOLLOW (rejecting symlinks atomically)
// and reads up to maxSize bytes. Returns os.ErrNotExist if the file does not exist.
func readFileNoFollow(path string, maxSize int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ELOOP) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer f.Close()
	lr := io.LimitReader(f, maxSize+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("skills: file %s exceeds %d bytes", filepath.Base(path), maxSize)
	}
	return data, nil
}

func loadPromptsFromDir(dir string) (map[string]PromptTemplate, error) {
	// Try prompts.yaml first (O_NOFOLLOW rejects symlinks atomically).
	promptsPath := filepath.Join(dir, "prompts.yaml")
	data, err := readFileNoFollow(promptsPath, plugins.MaxYAMLFileSize)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read prompts.yaml: %w", err)
		}
		// Fall back to config.yaml.
		configPath := filepath.Join(dir, "config.yaml")
		data, err = readFileNoFollow(configPath, plugins.MaxYAMLFileSize)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: no prompts.yaml or config.yaml in %s", ErrNoPrompts, dir)
			}
			return nil, fmt.Errorf("read config.yaml: %w", err)
		}
	}

	parsed, err := plugins.ParsePluginYAML(data)
	if err != nil {
		return nil, fmt.Errorf("parse prompts YAML: %w", err)
	}

	return extractPrompts(parsed)
}

// extractPrompts extracts a PromptTemplate map from a parsed YAML document.
func extractPrompts(data map[string]any) (map[string]PromptTemplate, error) {
	if data == nil {
		return nil, fmt.Errorf("%w: empty YAML document", ErrNoPrompts)
	}

	promptsRaw, ok := data["prompts"]
	if !ok {
		return nil, fmt.Errorf("%w: no 'prompts' key in YAML", ErrNoPrompts)
	}

	promptsMap, ok := promptsRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("skills: 'prompts' must be a mapping, got %T", promptsRaw)
	}

	if len(promptsMap) == 0 {
		return nil, fmt.Errorf("%w: 'prompts' map is empty", ErrNoPrompts)
	}

	result := make(map[string]PromptTemplate, len(promptsMap))
	for key, val := range promptsMap {
		pm, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("skills: prompt entry %q must be a mapping", key)
		}
		pt := PromptTemplate{}
		if n, ok := pm["name"].(string); ok {
			pt.Name = n
		}
		if d, ok := pm["description"].(string); ok {
			pt.Description = d
		}
		if t, ok := pm["template"].(string); ok {
			pt.Template = t
		}
		result[key] = pt
	}
	return result, nil
}
