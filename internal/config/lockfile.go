// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
)

// Lockfile represents a reproducible configuration snapshot.
// Field order is chosen for logical grouping and git-diffable output.
type Lockfile struct {
	Version     string           `json:"version"`
	GeneratedAt string           `json:"generated_at"`
	Config      core.Config      `json:"config"`
	Plugins     []LockfilePlugin `json:"plugins"`
}

// LockfilePlugin captures metadata for a single installed plugin.
type LockfilePlugin struct {
	Checksum        string `json:"checksum"`
	Name            string `json:"name"`
	Pinned          bool   `json:"pinned"`
	PinnedVersion   string `json:"pinned_version,omitempty"`
	PreviousVersion string `json:"previous_version,omitempty"`
	Tier            int    `json:"tier"`
	Version         string `json:"version"`
}

// MarshalLockfile serializes a Lockfile to indented, git-diffable JSON.
// Map keys are sorted alphabetically by encoding/json. Struct fields appear
// in declaration order via json tags.
func MarshalLockfile(lf *Lockfile) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(lf); err != nil {
		return nil, fmt.Errorf("lockfile: failed to marshal: %w", err)
	}
	// json.Encoder.Encode appends a newline; trim trailing whitespace for clean output.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// GenerateOptions configures lockfile generation.
type GenerateOptions struct {
	ConfigResolver core.ConfigResolver
	PluginRegistry core.PluginRegistry // nil-safe — plugins section empty if nil
	RegistryDir    string              // path to plugin registry directory for checksum calculation
}

// GenerateLockfile creates a Lockfile snapshot from the current configuration.
func GenerateLockfile(ctx context.Context, opts GenerateOptions) (*Lockfile, error) {
	if opts.ConfigResolver == nil {
		return nil, fmt.Errorf("lockfile: failed to generate: ConfigResolver is required")
	}

	cfg := opts.ConfigResolver.Config()
	if cfg == nil {
		return nil, fmt.Errorf("lockfile: failed to generate: ConfigResolver returned nil config")
	}

	lf := &Lockfile{
		Version:     "1",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Config:      *cfg,
		Plugins:     []LockfilePlugin{}, // empty slice, not null in JSON
	}

	if opts.PluginRegistry != nil {
		metas, err := opts.PluginRegistry.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("lockfile: failed to generate: listing plugins: %w", err)
		}
		plugins := make([]LockfilePlugin, len(metas))
		for i, m := range metas {
			plugins[i] = LockfilePlugin{
				Name:    m.Name,
				Version: m.Version,
				Tier:    m.Tier,
				}
			if opts.RegistryDir != "" {
				pluginDir := filepath.Join(opts.RegistryDir, m.Name)
				checksum, err := CalculatePluginChecksum(pluginDir, m.Tier)
				if err == nil {
					plugins[i].Checksum = checksum
				} else {
					slog.Warn("lockfile: checksum calculation failed, integrity verification disabled for plugin", "name", m.Name, "err", err)
				}
			}
		}
		sort.Slice(plugins, func(i, j int) bool {
			return plugins[i].Name < plugins[j].Name
		})
		lf.Plugins = plugins
	}

	return lf, nil
}

// WriteLockfile writes a lockfile to disk with git-shareable permissions.
func WriteLockfile(path string, lf *Lockfile) error {
	data, err := MarshalLockfile(lf)
	if err != nil {
		return fmt.Errorf("lockfile: failed to write %s: %w", path, err)
	}
	// Append final newline for POSIX compatibility.
	data = append(data, '\n')
	if err := fileutil.AtomicWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("lockfile: failed to write %s: %w", path, err)
	}
	return nil
}

// VerifyOptions configures lockfile verification.
type VerifyOptions struct {
	LockfilePath   string
	ConfigResolver core.ConfigResolver
	PluginRegistry core.PluginRegistry
	RegistryDir    string // path to plugin registry directory for checksum verification
}

// VerifyResult holds the outcome of a lockfile verification.
type VerifyResult struct {
	Match bool
	Diffs []VerifyDiff
}

// VerifyDiff describes a single mismatch between lockfile and current state.
type VerifyDiff struct {
	Field    string
	Expected string
	Actual   string
}

// VerifyLockfile compares a lockfile against the current configuration.
func VerifyLockfile(ctx context.Context, opts VerifyOptions) (*VerifyResult, error) {
	data, err := os.ReadFile(opts.LockfilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("lockfile: not found at %s — run 'siply lock' first", opts.LockfilePath)
		}
		return nil, fmt.Errorf("lockfile: failed to read %s: %w", opts.LockfilePath, err)
	}

	lf, err := ParseLockfile(data)
	if err != nil {
		return nil, err
	}

	if opts.ConfigResolver == nil {
		return nil, fmt.Errorf("lockfile: ConfigResolver is required for verification")
	}

	currentCfg := opts.ConfigResolver.Config()
	if currentCfg == nil {
		return nil, fmt.Errorf("lockfile: ConfigResolver returned nil config")
	}

	var diffs []VerifyDiff

	// Compare provider settings.
	if lf.Config.Provider.Default != currentCfg.Provider.Default {
		diffs = append(diffs, VerifyDiff{Field: "provider.default", Expected: lf.Config.Provider.Default, Actual: currentCfg.Provider.Default})
	}
	if lf.Config.Provider.Model != currentCfg.Provider.Model {
		diffs = append(diffs, VerifyDiff{Field: "provider.model", Expected: lf.Config.Provider.Model, Actual: currentCfg.Provider.Model})
	}

	// Compare routing settings.
	if ptrBoolStr(lf.Config.Routing.Enabled) != ptrBoolStr(currentCfg.Routing.Enabled) {
		diffs = append(diffs, VerifyDiff{Field: "routing.enabled", Expected: ptrBoolStr(lf.Config.Routing.Enabled), Actual: ptrBoolStr(currentCfg.Routing.Enabled)})
	}
	if lf.Config.Routing.DefaultProvider != currentCfg.Routing.DefaultProvider {
		diffs = append(diffs, VerifyDiff{Field: "routing.default_provider", Expected: lf.Config.Routing.DefaultProvider, Actual: currentCfg.Routing.DefaultProvider})
	}
	if lf.Config.Routing.PreprocessProvider != currentCfg.Routing.PreprocessProvider {
		diffs = append(diffs, VerifyDiff{Field: "routing.preprocess_provider", Expected: lf.Config.Routing.PreprocessProvider, Actual: currentCfg.Routing.PreprocessProvider})
	}
	if lf.Config.Routing.PreprocessModel != currentCfg.Routing.PreprocessModel {
		diffs = append(diffs, VerifyDiff{Field: "routing.preprocess_model", Expected: lf.Config.Routing.PreprocessModel, Actual: currentCfg.Routing.PreprocessModel})
	}

	// Compare session settings.
	if ptrIntStr(lf.Config.Session.RetentionCount) != ptrIntStr(currentCfg.Session.RetentionCount) {
		diffs = append(diffs, VerifyDiff{Field: "session.retention_count", Expected: ptrIntStr(lf.Config.Session.RetentionCount), Actual: ptrIntStr(currentCfg.Session.RetentionCount)})
	}

	// Compare telemetry settings.
	if ptrBoolStr(lf.Config.Telemetry.Enabled) != ptrBoolStr(currentCfg.Telemetry.Enabled) {
		diffs = append(diffs, VerifyDiff{Field: "telemetry.enabled", Expected: ptrBoolStr(lf.Config.Telemetry.Enabled), Actual: ptrBoolStr(currentCfg.Telemetry.Enabled)})
	}

	// Compare per-plugin config values (Config.Plugins map).
	diffs = append(diffs, comparePluginConfigs(lf.Config.Plugins, currentCfg.Plugins)...)

	// Plugin comparison when registry is available.
	if opts.PluginRegistry != nil {
		currentMetas, err := opts.PluginRegistry.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("lockfile: verification failed — listing plugins: %w", err)
		}
		currentMap := make(map[string]core.PluginMeta, len(currentMetas))
		for _, m := range currentMetas {
			currentMap[m.Name] = m
		}
		for _, lp := range lf.Plugins {
			cm, ok := currentMap[lp.Name]
			if !ok {
				diffs = append(diffs, VerifyDiff{
					Field:    fmt.Sprintf("plugin.%s", lp.Name),
					Expected: fmt.Sprintf("version=%s", lp.Version),
					Actual:   "not installed",
				})
				continue
			}
			if lp.Checksum != "" && opts.RegistryDir != "" {
				pluginDir := filepath.Join(opts.RegistryDir, lp.Name)
				currentChecksum, err := CalculatePluginChecksum(pluginDir, lp.Tier)
				if err == nil && currentChecksum != lp.Checksum {
					diffs = append(diffs, VerifyDiff{
						Field:    fmt.Sprintf("plugin.%s.checksum", lp.Name),
						Expected: lp.Checksum,
						Actual:   currentChecksum,
					})
				}
			}
			if lp.Version != cm.Version {
				diffs = append(diffs, VerifyDiff{
					Field:    fmt.Sprintf("plugin.%s.version", lp.Name),
					Expected: lp.Version,
					Actual:   cm.Version,
				})
			}
			delete(currentMap, lp.Name)
		}
		for name := range currentMap {
			diffs = append(diffs, VerifyDiff{
				Field:    fmt.Sprintf("plugin.%s", name),
				Expected: "not in lockfile",
				Actual:   "installed",
			})
		}
	} else if len(lf.Plugins) > 0 {
		// PluginRegistry is nil but lockfile has plugins — warn that plugin comparison was skipped.
		diffs = append(diffs, VerifyDiff{
			Field:    "plugins",
			Expected: fmt.Sprintf("%d plugins in lockfile", len(lf.Plugins)),
			Actual:   "plugin comparison skipped (no registry available)",
		})
	}

	return &VerifyResult{
		Match: len(diffs) == 0,
		Diffs: diffs,
	}, nil
}

// ptrBoolStr converts a *bool to a display string.
func ptrBoolStr(b *bool) string {
	if b == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%t", *b)
}

// ptrIntStr converts a *int to a display string.
func ptrIntStr(i *int) string {
	if i == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *i)
}

// ParseLockfile deserializes JSON into a Lockfile with strict validation.
// Unknown fields and trailing data are rejected.
func ParseLockfile(data []byte) (*Lockfile, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var lf Lockfile
	if err := dec.Decode(&lf); err != nil {
		return nil, fmt.Errorf("lockfile: failed to parse: %w", err)
	}

	// Reject trailing data after the JSON object.
	if dec.More() {
		return nil, fmt.Errorf("lockfile: trailing data after JSON object")
	}

	// Validate version field.
	if lf.Version == "" {
		return nil, fmt.Errorf("lockfile: missing required field: version")
	}
	if lf.Version != "1" {
		return nil, fmt.Errorf("lockfile: unsupported version %q (supported: \"1\")", lf.Version)
	}

	return &lf, nil
}

// CalculatePluginChecksum computes a SHA-256 checksum for a plugin.
// For Tier 1: checksum of manifest.yaml content.
// For Tier 3: checksum of the plugin binary (first executable file found).
// For other tiers: checksum of manifest.yaml.
func CalculatePluginChecksum(pluginDir string, tier int) (string, error) {
	if pluginDir == "" {
		return "", fmt.Errorf("lockfile: pluginDir is empty")
	}

	if tier == 3 {
		// Tier 3: hash the plugin binary.
		pluginName := filepath.Base(pluginDir)
		binaryPath := filepath.Join(pluginDir, pluginName)
		if _, err := os.Stat(binaryPath); err != nil {
			// Try with .exe suffix on Windows.
			binaryPath = filepath.Join(pluginDir, pluginName+".exe")
			if _, err := os.Stat(binaryPath); err != nil {
				// Fall back to manifest if binary not found.
				return checksumFile(filepath.Join(pluginDir, "manifest.yaml"))
			}
		}
		return checksumFile(binaryPath)
	}

	// Tier 1 and other tiers: hash manifest.yaml.
	return checksumFile(filepath.Join(pluginDir, "manifest.yaml"))
}

// checksumFile computes a SHA-256 hex digest of a file.
func checksumFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("lockfile: read file for checksum: %w", err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// comparePluginConfigs compares per-plugin config maps and returns diffs.
func comparePluginConfigs(expected, actual map[string]any) []VerifyDiff {
	var diffs []VerifyDiff

	for key, ev := range expected {
		av, ok := actual[key]
		if !ok {
			diffs = append(diffs, VerifyDiff{
				Field:    fmt.Sprintf("config.plugins.%s", key),
				Expected: fmt.Sprintf("%v", ev),
				Actual:   "<missing>",
			})
			continue
		}
		if fmt.Sprintf("%v", ev) != fmt.Sprintf("%v", av) {
			diffs = append(diffs, VerifyDiff{
				Field:    fmt.Sprintf("config.plugins.%s", key),
				Expected: fmt.Sprintf("%v", ev),
				Actual:   fmt.Sprintf("%v", av),
			})
		}
	}
	for key := range actual {
		if _, ok := expected[key]; !ok {
			diffs = append(diffs, VerifyDiff{
				Field:    fmt.Sprintf("config.plugins.%s", key),
				Expected: "<missing>",
				Actual:   fmt.Sprintf("%v", actual[key]),
			})
		}
	}

	return diffs
}
