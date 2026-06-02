// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	SkillIndexVersion = 1
	MaxIndexFileSize  = 1 << 20 // 1 MiB
	MaxRulesFileSize  = 1 << 20 // 1 MiB
)

const (
	SkillKindLegacyPrompt = "legacy-prompt"
	SkillKindSkillMD      = "skill-md"
	SkillKindIndexed      = "indexed"
	SkillKindSmartPack    = "smart-pack"
)

var (
	ErrUnsupportedIndexVersion = errors.New("skills: unsupported index version")
	ErrInvalidIndexPath        = errors.New("skills: invalid index path")
	ErrDuplicateSkillName      = errors.New("skills: duplicate skill name")
)

type SkillIndex struct {
	Version          int                 `json:"version"`
	Sources          []SkillSource       `json:"sources,omitempty"`
	Skills           []IndexedSkill      `json:"skills,omitempty"`
	SmartPacks       []SmartPackRef      `json:"smartPacks,omitempty"`
	ClaudeFastRules  []ClaudeFastRuleRef `json:"-"`
	LoadedFrom       string              `json:"-"`
	Synthesized      bool                `json:"-"`
	ActivationTokens int                 `json:"-"`
}

type SkillSource struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Format string `json:"format,omitempty"`
}

type IndexedSkill struct {
	Name        string          `json:"name"`
	Source      string          `json:"source"`
	Kind        string          `json:"kind,omitempty"`
	Description string          `json:"description,omitempty"`
	Activation  ActivationRules `json:"activation,omitempty"`
	Loading     LoadingPolicy   `json:"loading,omitempty"`
	SmartPack   string          `json:"smartPack,omitempty"`
}

// ActivationRules contains deterministic FI-1 metadata only. Fuzzy matching,
// embeddings, and mini-LLM fallback are intentionally out of scope.
type ActivationRules struct {
	Commands       []string `json:"commands,omitempty"`
	Keywords       []string `json:"keywords,omitempty"`
	IntentPatterns []string `json:"intentPatterns,omitempty"`
}

type LoadingPolicy struct {
	Mode             string `json:"mode,omitempty"`
	DedupePerSession bool   `json:"dedupePerSession,omitempty"`
	VisibleInChat    bool   `json:"visibleInChat,omitempty"`
	MaxBodyBytes     int64  `json:"maxBodyBytes,omitempty"`
}

type SmartPackRef struct {
	Name               string   `json:"name"`
	Skills             []string `json:"skills,omitempty"`
	Tools              []string `json:"tools,omitempty"`
	Datasets           []string `json:"datasets,omitempty"`
	CompactDescription string   `json:"compactDescription,omitempty"`
}

type ClaudeFastRuleRef struct {
	Path string `json:"path"`
}

type IndexWarning struct {
	Path string
	Err  error
}

func (w IndexWarning) Error() string {
	if w.Path == "" {
		return w.Err.Error()
	}
	return fmt.Sprintf("%s: %v", w.Path, w.Err)
}

func (w IndexWarning) Unwrap() error {
	return w.Err
}

type LoadIndexOptions struct {
	ProjectRoot string
	HomeDir     string
	Loader      *SkillLoader
}

// LoadIndex loads .siply/skills.index.json from the project first, then falls
// back to ~/.siply/skills.index.json. If neither exists, it returns a
// synthesized in-memory index from currently loaded legacy skills.
func LoadIndex(ctx context.Context, opts LoadIndexOptions) (*SkillIndex, []IndexWarning, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("skills: load index canceled: %w", err)
	}

	normalized, err := normalizeLoadIndexOptions(opts)
	if err != nil {
		return nil, nil, err
	}

	projectIndex := filepath.Join(normalized.ProjectRoot, ".siply", "skills.index.json")
	idx, warnings, err := loadIndexFile(ctx, projectIndex, normalized.ProjectRoot)
	if err == nil {
		return idx, warnings, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, warnings, err
	}

	globalIndex := filepath.Join(normalized.HomeDir, ".siply", "skills.index.json")
	idx, warnings, err = loadIndexFile(ctx, globalIndex, normalized.HomeDir)
	if err == nil {
		return idx, warnings, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, warnings, err
	}

	return SynthesizeIndex(ctx, normalized.Loader), nil, nil
}

// SynthesizeIndex builds an in-memory index from existing legacy Siply skills
// and default native sources. It does not read full SKILL.md bodies.
func SynthesizeIndex(ctx context.Context, loader *SkillLoader) *SkillIndex {
	idx := &SkillIndex{
		Version: SkillIndexVersion,
		Sources: []SkillSource{
			{Type: "skill-md-directory", Path: ".siply/skills", Format: "SKILL.md"},
			{Type: "legacy-siply-directory", Path: ".siply/skills", Format: "manifest-prompts"},
			{Type: "claudefast-rules", Path: ".claude/skills/skill-rules.json"},
		},
		Synthesized: true,
	}
	if ctx.Err() != nil || loader == nil {
		return idx
	}

	for _, s := range loader.List() {
		idx.Skills = append(idx.Skills, IndexedSkill{
			Name:        s.Name,
			Source:      s.Dir,
			Kind:        s.Kind,
			Description: s.Description,
			Activation:  s.Activation,
			Loading: LoadingPolicy{
				Mode:             "on_demand",
				DedupePerSession: true,
				VisibleInChat:    true,
			},
		})
	}
	return idx
}

func normalizeLoadIndexOptions(opts LoadIndexOptions) (LoadIndexOptions, error) {
	if opts.ProjectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return opts, fmt.Errorf("skills: get working directory: %w", err)
		}
		opts.ProjectRoot = wd
	}
	projectRoot, err := filepath.Abs(opts.ProjectRoot)
	if err != nil {
		return opts, fmt.Errorf("skills: resolve project root: %w", err)
	}
	opts.ProjectRoot = filepath.Clean(projectRoot)

	if opts.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return opts, fmt.Errorf("skills: get home directory: %w", err)
		}
		opts.HomeDir = home
	}
	homeDir, err := filepath.Abs(opts.HomeDir)
	if err != nil {
		return opts, fmt.Errorf("skills: resolve home directory: %w", err)
	}
	opts.HomeDir = filepath.Clean(homeDir)
	return opts, nil
}

func loadIndexFile(ctx context.Context, path, baseDir string) (*SkillIndex, []IndexWarning, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("skills: load index canceled: %w", err)
	}
	if err := ensureContainedPath(path, baseDir); err != nil {
		return nil, nil, err
	}

	data, err := readFileNoFollow(path, MaxIndexFileSize)
	if err != nil {
		return nil, nil, err
	}

	var idx SkillIndex
	if err := decodeStrictJSON(data, &idx); err != nil {
		return nil, nil, fmt.Errorf("skills: parse index %s: %w", path, err)
	}
	idx.LoadedFrom = path

	if err := validateIndex(&idx, baseDir); err != nil {
		return nil, nil, fmt.Errorf("skills: validate index %s: %w", path, err)
	}

	warnings := loadClaudeFastRuleRefs(ctx, &idx, baseDir)
	return &idx, warnings, nil
}

func validateIndex(idx *SkillIndex, baseDir string) error {
	if idx.Version != SkillIndexVersion {
		return fmt.Errorf("%w: %d", ErrUnsupportedIndexVersion, idx.Version)
	}

	seen := make(map[string]struct{}, len(idx.Skills))
	for _, source := range idx.Sources {
		if strings.TrimSpace(source.Path) == "" {
			return fmt.Errorf("%w: source path is required", ErrInvalidIndexPath)
		}
		if _, err := containedPath(baseDir, source.Path); err != nil {
			return err
		}
	}

	for _, skill := range idx.Skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			return fmt.Errorf("skills: indexed skill name is required")
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("%w: %s", ErrDuplicateSkillName, name)
		}
		seen[name] = struct{}{}
		if strings.TrimSpace(skill.Source) == "" {
			return fmt.Errorf("%w: source is required for skill %q", ErrInvalidIndexPath, name)
		}
		if _, err := containedPath(baseDir, skill.Source); err != nil {
			return fmt.Errorf("skill %q: %w", name, err)
		}
	}
	return nil
}

func loadClaudeFastRuleRefs(ctx context.Context, idx *SkillIndex, baseDir string) []IndexWarning {
	var warnings []IndexWarning
	for _, source := range idx.Sources {
		if source.Type != "claudefast-rules" {
			continue
		}
		if err := ctx.Err(); err != nil {
			warnings = append(warnings, IndexWarning{Path: source.Path, Err: err})
			continue
		}

		path, err := containedPath(baseDir, source.Path)
		if err != nil {
			warnings = append(warnings, IndexWarning{Path: source.Path, Err: err})
			continue
		}
		data, err := readFileNoFollow(path, MaxRulesFileSize)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			warnings = append(warnings, IndexWarning{Path: source.Path, Err: err})
			continue
		}

		var raw any
		if err := decodeStrictJSON(data, &raw); err != nil {
			warnings = append(warnings, IndexWarning{Path: source.Path, Err: fmt.Errorf("parse ClaudeFast rules: %w", err)})
			continue
		}
		idx.ClaudeFastRules = append(idx.ClaudeFastRules, ClaudeFastRuleRef{Path: source.Path})
	}
	return warnings
}

func decodeStrictJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("unexpected trailing data after JSON")
	}
	return nil
}

func containedPath(baseDir, candidate string) (string, error) {
	if filepath.IsAbs(candidate) {
		return cleanContainedPath(baseDir, candidate)
	}
	return cleanContainedPath(baseDir, filepath.Join(baseDir, candidate))
}

func ensureContainedPath(candidate, baseDir string) error {
	_, err := cleanContainedPath(baseDir, candidate)
	return err
}

func cleanContainedPath(baseDir, candidate string) (string, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("%w: resolve base %q: %v", ErrInvalidIndexPath, baseDir, err)
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("%w: resolve path %q: %v", ErrInvalidIndexPath, candidate, err)
	}
	baseAbs = filepath.Clean(baseAbs)
	candidateAbs = filepath.Clean(candidateAbs)

	rel, err := filepath.Rel(baseAbs, candidateAbs)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrInvalidIndexPath, candidate)
	}
	if rel == "." {
		return candidateAbs, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %s escapes %s", ErrInvalidIndexPath, candidate, baseAbs)
	}
	return candidateAbs, nil
}
