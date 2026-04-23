// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/fileutil"
)

// Compile-time interface compliance check.
var _ core.PluginRegistry = (*LocalRegistry)(nil)

// maxPluginFileSize is the maximum allowed size for any single file in a plugin (100MB).
const maxPluginFileSize = 100 << 20

// Sentinel errors for registry operations.
var (
	ErrAlreadyInstalled = errors.New("plugins: plugin already installed")
	ErrNotFound         = errors.New("plugins: plugin not found")
	ErrDevModeRemove    = errors.New("plugins: cannot remove dev-mode plugin")
)

// LocalRegistry manages plugins installed on the local filesystem.
type LocalRegistry struct {
	registryDir  string
	devPaths     map[string]string
	plugins      map[string]*Manifest
	eventBus     core.EventBus // optional, nil-safe
	tier2Loader  *Tier2Loader  // optional, nil-safe — set via SetTier2Loader
	siplyVersion string        // override for testing; empty = use GetSiplyVersion()
	mu           sync.RWMutex
}

// NewLocalRegistry creates a LocalRegistry that manages plugins in registryDir.
func NewLocalRegistry(registryDir string) *LocalRegistry {
	return &LocalRegistry{
		registryDir: registryDir,
		devPaths:    make(map[string]string),
		plugins:     make(map[string]*Manifest),
	}
}

// SetEventBus attaches an EventBus to the registry for publishing plugin lifecycle events.
// This is optional — if not set, no events are published.
func (r *LocalRegistry) SetEventBus(bus core.EventBus) {
	r.eventBus = bus
}

// SetTier2Loader attaches a Tier2Loader to the registry for loading Lua plugins.
// This is optional — if not set, Tier 2 plugins are loaded as metadata only.
func (r *LocalRegistry) SetTier2Loader(loader *Tier2Loader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tier2Loader = loader
}

// SetSiplyVersion overrides the siply version used for compatibility checks during Init.
// Primarily useful for testing. If not set, GetSiplyVersion() is used.
func (r *LocalRegistry) SetSiplyVersion(v string) {
	r.siplyVersion = v
}

// Init scans registryDir for installed plugins and loads their manifests.
// Invalid manifests are logged as warnings but do not block other plugins.
// Incompatible plugins (siply_min > current version) are skipped and a
// PluginDisabledEvent is published if an EventBus is attached.
func (r *LocalRegistry) Init(_ context.Context) error {
	if r.registryDir == "" {
		return fmt.Errorf("plugins: registryDir is empty, call NewLocalRegistry() first")
	}

	entries, err := os.ReadDir(r.registryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("plugins: read registry dir: %w", err)
	}

	r.mu.Lock()
	// NOTE: r.mu.Unlock() is called explicitly below, before publishing events,
	// to prevent deadlock if event handlers touch LocalRegistry.

	// Clear pre-existing state to ensure idempotent Init.
	r.plugins = make(map[string]*Manifest)
	var pendingDisabled []*events.PluginDisabledEvent

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(r.registryDir, entry.Name())
		m, err := LoadManifestFromDir(dir)
		if err != nil {
			slog.Warn("invalid manifest", "dir", dir, "err", err)
			continue
		}
		if entry.Name() != m.Metadata.Name {
			slog.Warn("plugins: directory name differs from manifest name, skipping",
				"dir", entry.Name(), "manifest_name", m.Metadata.Name)
			continue
		}
		// Skip incompatible plugins (siply_min > current version).
		siplyVersion := r.siplyVersion
		if siplyVersion == "" {
			siplyVersion = GetSiplyVersion()
		}
		if !IsCompatible(m.Metadata.SiplyMin, siplyVersion) {
			reason := FormatIncompatibleMessage(m.Metadata.Name, m.Metadata.Version, siplyVersion, m.Metadata.SiplyMin)
			slog.Warn("plugins: incompatible plugin skipped", "name", m.Metadata.Name, "reason", reason)
			pendingDisabled = append(pendingDisabled, events.NewPluginDisabledEvent(m.Metadata.Name, m.Metadata.Version, reason))
			continue
		}
		r.plugins[m.Metadata.Name] = m
		slog.Info("plugin loaded", "name", m.Metadata.Name, "version", m.Metadata.Version)
	}
	r.mu.Unlock()

	// Publish disabled events outside the lock to prevent deadlock if handlers touch LocalRegistry.
	if r.eventBus != nil {
		for _, evt := range pendingDisabled {
			if err := r.eventBus.Publish(context.Background(), evt); err != nil {
				slog.Warn("plugins: failed to publish PluginDisabledEvent", "name", evt.Name, "err", err)
			}
		}
	}

	return nil
}

// Start is a no-op for LocalRegistry (lazy loading is in Story 6.3).
func (r *LocalRegistry) Start(_ context.Context) error { return nil }

// Stop clears all loaded plugin state.
func (r *LocalRegistry) Stop(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = make(map[string]*Manifest)
	r.devPaths = make(map[string]string)
	return nil
}

// Health checks that the registryDir is accessible.
func (r *LocalRegistry) Health() error {
	if r.registryDir == "" {
		return fmt.Errorf("plugins: registryDir not configured")
	}
	_, err := os.Stat(r.registryDir)
	if err != nil {
		return fmt.Errorf("plugins: registry dir not accessible: %w", err)
	}
	return nil
}

// Install copies a plugin from a local source directory into the registry.
func (r *LocalRegistry) Install(_ context.Context, source string) error {
	if r.registryDir == "" {
		return fmt.Errorf("plugins: registryDir is empty, call Init() first")
	}

	m, err := LoadManifestFromDir(source)
	if err != nil {
		return fmt.Errorf("plugins: install: %w", err)
	}

	name := m.Metadata.Name
	destDir := filepath.Join(r.registryDir, name)

	// Acquire file lock first to prevent cross-process TOCTOU.
	fl := fileutil.NewFileLock(r.registryDir)
	if err := fl.ExclusiveLock(); err != nil {
		return fmt.Errorf("plugins: install: acquire lock: %w", err)
	}
	defer func() { _ = fl.Unlock() }()

	// Check existence under file lock (prevents double-install across processes).
	r.mu.RLock()
	_, exists := r.plugins[name]
	r.mu.RUnlock()
	if exists {
		return fmt.Errorf("%w: %s", ErrAlreadyInstalled, name)
	}

	// Also check on-disk state (another process may have installed).
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("%w: %s", ErrAlreadyInstalled, name)
	}

	// Copy files (mutex NOT held during I/O).
	if err := copyDir(source, destDir); err != nil {
		os.RemoveAll(destDir) // Clean up partial copy.
		return fmt.Errorf("plugins: install: copy: %w", err)
	}

	// Update in-memory state.
	r.mu.Lock()
	r.plugins[name] = m
	r.mu.Unlock()

	slog.Info("plugin installed", "name", name, "version", m.Metadata.Version)
	return nil
}

// Load loads a single plugin by name into the registry.
func (r *LocalRegistry) Load(ctx context.Context, name string) error {
	if r.registryDir == "" {
		return fmt.Errorf("plugins: registryDir is empty, call Init() first")
	}

	// Read shared state under lock, then release for disk I/O.
	r.mu.RLock()
	devPath, isDev := r.devPaths[name]
	r.mu.RUnlock()

	var loadDir string
	if isDev {
		loadDir = devPath
	} else {
		loadDir = filepath.Join(r.registryDir, name)
		if _, err := os.Stat(loadDir); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", ErrNotFound, name)
			}
			return fmt.Errorf("plugins: load: stat: %w", err)
		}
	}

	// Disk I/O without holding mutex.
	m, err := LoadManifestFromDir(loadDir)
	if err != nil {
		if isDev {
			return fmt.Errorf("plugins: load dev: %w", err)
		}
		return fmt.Errorf("plugins: load: %w", err)
	}

	// Re-acquire lock to update map.
	r.mu.Lock()
	if _, already := r.plugins[name]; !already {
		r.plugins[name] = m
	}
	r.mu.Unlock()

	// Delegate to Tier2Loader for Lua plugins.
	if m.Spec.Tier == 2 && r.tier2Loader != nil {
		return r.tier2Loader.Load(ctx, name)
	}

	return nil
}

// List returns metadata for all loaded plugins.
func (r *LocalRegistry) List(_ context.Context) ([]core.PluginMeta, error) {
	if r.plugins == nil {
		return nil, fmt.Errorf("plugins: registry not initialized, call NewLocalRegistry() first")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	metas := make([]core.PluginMeta, 0, len(r.plugins))
	for _, m := range r.plugins {
		caps := make([]string, 0, len(m.Spec.Capabilities))
		for k := range m.Spec.Capabilities {
			caps = append(caps, k)
		}
		metas = append(metas, core.PluginMeta{
			Name:         m.Metadata.Name,
			Version:      m.Metadata.Version,
			Tier:         m.Spec.Tier,
			Capabilities: caps,
		})
	}
	return metas, nil
}

// Remove deletes a plugin from the registry. Dev-mode plugins cannot be removed.
func (r *LocalRegistry) Remove(_ context.Context, name string) error {
	if r.registryDir == "" {
		return fmt.Errorf("plugins: registryDir is empty, call Init() first")
	}

	// Check preconditions under read lock.
	r.mu.RLock()
	_, isDev := r.devPaths[name]
	_, exists := r.plugins[name]
	r.mu.RUnlock()

	if isDev {
		return fmt.Errorf("%w: %s", ErrDevModeRemove, name)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	// Acquire file lock (mutex NOT held during I/O).
	fl := fileutil.NewFileLock(r.registryDir)
	if err := fl.ExclusiveLock(); err != nil {
		return fmt.Errorf("plugins: remove: acquire lock: %w", err)
	}
	defer func() { _ = fl.Unlock() }()

	// Re-check after acquiring lock (another goroutine may have removed it).
	r.mu.RLock()
	_, isDev = r.devPaths[name]
	_, exists = r.plugins[name]
	r.mu.RUnlock()
	if isDev {
		return fmt.Errorf("%w: %s", ErrDevModeRemove, name)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}

	dir := filepath.Join(r.registryDir, name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("plugins: remove: %w", err)
	}

	// Update in-memory state.
	r.mu.Lock()
	delete(r.plugins, name)
	r.mu.Unlock()

	slog.Info("plugin removed", "name", name)
	return nil
}

// DevMode registers a local development path for a plugin.
// The plugin is loaded from the dev path, overriding registry dir.
func (r *LocalRegistry) DevMode(_ context.Context, path string) error {
	m, err := LoadManifestFromDir(path)
	if err != nil {
		return fmt.Errorf("plugins: devmode: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.devPaths[m.Metadata.Name] = path
	r.plugins[m.Metadata.Name] = m
	slog.Info("plugin dev mode", "name", m.Metadata.Name, "path", path)
	return nil
}

// copyDir copies all files from src to dst directory.
func copyDir(src, dst string) error {
	// Resolve to absolute paths and guard against overlapping src/dst.
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("plugins: resolve source path: %w", err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("plugins: resolve destination path: %w", err)
	}
	if strings.HasPrefix(absDst, absSrc+string(filepath.Separator)) ||
		strings.HasPrefix(absSrc, absDst+string(filepath.Separator)) ||
		absSrc == absDst {
		return fmt.Errorf("plugins: source and destination paths overlap")
	}

	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("plugins: create destination: %w", err)
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks to prevent path traversal and data exfiltration.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		// Path containment check.
		cleanTarget := filepath.Clean(target)
		cleanDst := filepath.Clean(dst)
		if cleanTarget != cleanDst && !strings.HasPrefix(cleanTarget, cleanDst+string(filepath.Separator)) {
			return fmt.Errorf("plugins: path traversal detected: %s", rel)
		}

		if d.IsDir() {
			dirInfo, err := d.Info()
			if err != nil {
				return fmt.Errorf("plugins: stat dir %s: %w", path, err)
			}
			return os.MkdirAll(target, dirInfo.Mode().Perm())
		}

		// Preserve original file permissions.
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("plugins: stat %s: %w", path, err)
		}

		// Guard against unbounded file reads (e.g., large binaries in malicious plugins).
		if info.Size() > maxPluginFileSize {
			return fmt.Errorf("plugins: file %s is %d bytes, exceeds %d byte limit", rel, info.Size(), maxPluginFileSize)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("plugins: read %s: %w", path, err)
		}

		return fileutil.AtomicWriteFile(target, data, info.Mode())
	})
}
