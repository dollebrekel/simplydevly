// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// maxYAMLFileSize is the maximum allowed size for any single YAML file in a plugin (1MB per NFR12).
const maxYAMLFileSize = 1 << 20 // 1MB

// maxYAMLFilesPerPlugin is the maximum number of YAML files allowed per plugin (NFR12).
const maxYAMLFilesPerPlugin = 100

// Sentinel errors for Tier1Loader operations.
var (
	ErrNotTier1        = errors.New("plugins: plugin is not Tier 1")
	ErrTooManyFiles    = errors.New("plugins: plugin exceeds maximum YAML file count")
	ErrFileTooLarge    = errors.New("plugins: YAML file exceeds 1MB size limit")
	ErrPluginNotLoaded = errors.New("plugins: plugin not loaded")
)

// ConfigMerger is the interface for integrating plugin config into the project config.
// It is defined here and implemented in the config package (dependency inversion:
// plugins does not import config).
type ConfigMerger interface {
	MergePluginConfig(pluginName string, pluginConfig map[string]any) error
	RemovePluginConfig(pluginName string) error
}

// Tier1Plugin represents a loaded Tier 1 YAML config plugin.
type Tier1Plugin struct {
	Manifest *Manifest
	Config   map[string]any // parsed plugin config.yaml content
	Files    []string       // all YAML files in plugin dir (excluding manifest.yaml)
}

// Tier1Loader loads and manages Tier 1 YAML config plugins.
// No code execution occurs during loading — plugins consist entirely of YAML files.
type Tier1Loader struct {
	registry     *LocalRegistry
	configMerger ConfigMerger
	mu           sync.RWMutex
	loaded       map[string]*Tier1Plugin // name → loaded plugin
}

// NewTier1Loader creates a new Tier1Loader backed by the given registry and config merger.
func NewTier1Loader(registry *LocalRegistry, merger ConfigMerger) *Tier1Loader {
	return &Tier1Loader{
		registry:     registry,
		configMerger: merger,
		loaded:       make(map[string]*Tier1Plugin),
	}
}

// Load loads a Tier 1 plugin by name from the registry, merging its config.yaml into
// the project config. The plugin must already be installed via LocalRegistry.Install.
// No code execution occurs at any point.
func (l *Tier1Loader) Load(ctx context.Context, name string) error {
	if l.registry == nil {
		return fmt.Errorf("plugins: tier1: registry is nil, call NewTier1Loader() first")
	}
	if l.configMerger == nil {
		return fmt.Errorf("plugins: tier1: configMerger is nil, call NewTier1Loader() first")
	}

	// Determine the plugin directory (dev mode overrides registry dir).
	pluginDir, err := l.pluginDir(name)
	if err != nil {
		return err
	}

	// Load and validate manifest — disk I/O without holding loaded mutex.
	manifest, err := LoadManifestFromDir(pluginDir)
	if err != nil {
		return fmt.Errorf("plugins: tier1: load manifest %s: %w", name, err)
	}

	// Verify Tier 1 — return sentinel error if not.
	if manifest.Spec.Tier != 1 {
		return fmt.Errorf("%w: %s has tier %d", ErrNotTier1, name, manifest.Spec.Tier)
	}

	// Scan plugin directory for YAML files (skip manifest.yaml).
	yamlFiles, err := scanYAMLFiles(pluginDir)
	if err != nil {
		return fmt.Errorf("plugins: tier1: scan files %s: %w", name, err)
	}

	// Enforce maximum file count (NFR12) — count excludes manifest.yaml.
	if len(yamlFiles) > maxYAMLFilesPerPlugin {
		return fmt.Errorf("%w: %s has %d files (max %d)", ErrTooManyFiles, name, len(yamlFiles), maxYAMLFilesPerPlugin)
	}

	// Parse all YAML files; only config.yaml is merged into project config.
	var pluginConfig map[string]any
	for _, f := range yamlFiles {
		data, err := readFileWithSizeLimit(f, maxYAMLFileSize)
		if err != nil {
			return fmt.Errorf("plugins: tier1: read file %s in %s: %w", filepath.Base(f), name, err)
		}

		isConfigFile := filepath.Base(f) == "config.yaml" || filepath.Base(f) == "config.yml"
		parsed, err := parsePluginYAML(data)
		if err != nil {
			return fmt.Errorf("plugins: tier1: parse %s in %s: %w", filepath.Base(f), name, err)
		}

		if isConfigFile {
			pluginConfig = parsed
		}
	}

	// Merge config.yaml content into project config via ConfigMerger.
	if pluginConfig != nil {
		if err := l.configMerger.MergePluginConfig(name, pluginConfig); err != nil {
			return fmt.Errorf("plugins: tier1: merge config %s: %w", name, err)
		}
	}

	// Update loaded map — narrow mutex to map operation only (not held during I/O above).
	plugin := &Tier1Plugin{
		Manifest: manifest,
		Config:   pluginConfig,
		Files:    yamlFiles,
	}
	l.mu.Lock()
	l.loaded[name] = plugin
	l.mu.Unlock()

	slog.Info("tier1 plugin loaded", "name", name, "version", manifest.Metadata.Version)
	return nil
}

// Unload removes a loaded Tier 1 plugin and its config contribution from the project config.
func (l *Tier1Loader) Unload(_ context.Context, name string) error {
	if l.configMerger == nil {
		return fmt.Errorf("plugins: tier1: configMerger is nil, call NewTier1Loader() first")
	}

	// Use a single write lock for the entire operation to prevent TOCTOU races.
	l.mu.Lock()
	_, loaded := l.loaded[name]
	if !loaded {
		l.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, name)
	}
	delete(l.loaded, name)
	l.mu.Unlock()

	// Remove plugin config contribution — outside loaded mutex but after map update.
	if err := l.configMerger.RemovePluginConfig(name); err != nil {
		return fmt.Errorf("plugins: tier1: unload: remove config %s: %w", name, err)
	}

	slog.Info("tier1 plugin unloaded", "name", name)
	return nil
}

// List returns a snapshot of all currently loaded Tier 1 plugins (thread-safe).
func (l *Tier1Loader) List(_ context.Context) []Tier1Plugin {
	l.mu.RLock()
	defer l.mu.RUnlock()

	plugins := make([]Tier1Plugin, 0, len(l.loaded))
	for _, p := range l.loaded {
		plugins = append(plugins, *p)
	}
	return plugins
}

// IsLoaded returns true if the named plugin is currently loaded (thread-safe).
func (l *Tier1Loader) IsLoaded(name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.loaded[name]
	return ok
}

// pluginDir returns the effective directory for a plugin, respecting dev mode paths.
func (l *Tier1Loader) pluginDir(name string) (string, error) {
	if l.registry.registryDir == "" {
		return "", fmt.Errorf("plugins: tier1: registry not initialised (empty registryDir)")
	}

	// Reject path traversal attempts (e.g., "../etc").
	if strings.ContainsAny(name, "/\\") || name == ".." || strings.Contains(name, "..") {
		return "", fmt.Errorf("plugins: tier1: invalid plugin name %q: path traversal not allowed", name)
	}

	// Check for dev mode path under registry's read lock.
	l.registry.mu.RLock()
	devPath, isDev := l.registry.devPaths[name]
	l.registry.mu.RUnlock()

	if isDev {
		return devPath, nil
	}

	dir := filepath.Join(l.registry.registryDir, name)

	// Path containment check — ensure resolved path stays within the registry directory.
	cleanDir := filepath.Clean(dir)
	cleanBase := filepath.Clean(l.registry.registryDir)
	if !strings.HasPrefix(cleanDir, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("plugins: tier1: invalid plugin name %q: path escapes registry", name)
	}

	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return "", fmt.Errorf("plugins: tier1: stat plugin dir %s: %w", name, err)
	}
	return dir, nil
}

// scanYAMLFiles returns all .yaml/.yml files in dir, excluding manifest.yaml.
// Only the top-level directory is scanned (no recursion).
func scanYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("plugins: read dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "manifest.yaml" || name == "manifest.yml" {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}

// readFileWithSizeLimit reads a file, returning ErrFileTooLarge if it exceeds the limit.
func readFileWithSizeLimit(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// Read limit+1 bytes to detect oversized files without stat (TOCTOU-safe).
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: %s", ErrFileTooLarge, path)
	}
	return data, nil
}

// MaxYAMLFileSize is the exported maximum allowed size for any single YAML file (1MB per NFR12).
// Other packages in the internal tree may use this constant directly.
const MaxYAMLFileSize = maxYAMLFileSize

// ParsePluginYAML is the exported version of parsePluginYAML, allowing other packages
// within the internal tree to reuse the same YAML security validation (no aliases, no
// custom tags, 1MB limit).
func ParsePluginYAML(data []byte) (map[string]any, error) {
	if int64(len(data)) > MaxYAMLFileSize {
		return nil, fmt.Errorf("YAML data exceeds %d bytes", MaxYAMLFileSize)
	}
	return parsePluginYAML(data)
}

// ReadFileWithSizeLimit is the exported version of readFileWithSizeLimit, allowing
// other packages within the internal tree to perform size-limited file reads.
func ReadFileWithSizeLimit(path string, limit int64) ([]byte, error) {
	return readFileWithSizeLimit(path, limit)
}

// parsePluginYAML parses YAML data from a plugin file, enforcing security restrictions:
//   - Rejects YAML alias nodes (*anchor references)
//   - Rejects YAML merge keys (<<:)
//   - Rejects non-standard custom type tags (!!python/, etc.)
//   - Enforces the 1MB size limit (caller must pre-check)
//
// All plugin YAML files (config, data, prompts, themes) are subject to the same
// security restrictions. The caller determines which files are merged into config.
func parsePluginYAML(data []byte) (map[string]any, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// First pass: parse into yaml.Node to inspect for forbidden constructs.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}

	// Walk node tree to reject aliases and custom types.
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		if err := validateYAMLNode(doc.Content[0]); err != nil {
			return nil, err
		}
	}

	// Second pass: decode into map[string]any.
	var result map[string]any
	dec := yaml.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	return result, nil
}

// maxYAMLDepth is the maximum allowed nesting depth for YAML validation and merge.
const maxYAMLDepth = 32

// validateYAMLNode recursively validates a YAML node tree, rejecting:
//   - AliasNode (YAML *anchor references)
//   - Mapping keys named "<<" (YAML merge key syntax)
//   - Non-standard !! custom type tags (!!python/, etc.)
//   - Nesting depth beyond maxYAMLDepth
func validateYAMLNode(node *yaml.Node) error {
	return validateYAMLNodeDepth(node, 0)
}

func validateYAMLNodeDepth(node *yaml.Node, depth int) error {
	if node == nil {
		return nil
	}
	if depth > maxYAMLDepth {
		return fmt.Errorf("plugins: YAML nesting depth exceeds maximum (%d)", maxYAMLDepth)
	}

	// Reject YAML alias nodes (*alias references).
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("plugins: YAML aliases are not allowed in plugin files")
	}

	// Reject non-standard custom type tags.
	if node.Tag != "" && !isAllowedYAMLTag(node.Tag) {
		return fmt.Errorf("plugins: custom YAML type tag %q is not allowed in plugin files", node.Tag)
	}

	// In mapping nodes, reject merge keys (<<:).
	if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			if keyNode.Value == "<<" {
				return fmt.Errorf("plugins: YAML merge keys (<<:) are not allowed in plugin files")
			}
		}
	}

	// Recurse into child nodes.
	for _, child := range node.Content {
		if err := validateYAMLNodeDepth(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// isAllowedYAMLTag returns true for empty tags and standard YAML core schema tags.
// Any tag starting with "!!" that is not in the standard set is rejected.
func isAllowedYAMLTag(tag string) bool {
	switch tag {
	case "", "!", "!!str", "!!int", "!!float", "!!bool", "!!null",
		"!!seq", "!!map", "!!timestamp", "!!binary", "!!omap", "!!pairs", "!!set":
		return true
	}
	// Reject all other !! prefixed tags (catches !!python/, !!js/, !!merge, etc.).
	if strings.HasPrefix(tag, "!!") {
		return false
	}
	// Short-form tags (single !) without a known type are allowed by default.
	return true
}
