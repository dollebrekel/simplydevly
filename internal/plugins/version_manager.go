// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"siply.dev/siply/internal/fileutil"
)

// Sentinel errors for version management operations.
var (
	ErrIncompatible     = errors.New("plugins: plugin requires newer siply version")
	ErrPluginPinned     = errors.New("plugins: plugin is pinned, skip update")
	ErrNoPreviousVersion = errors.New("plugins: no previous version to rollback to")
	ErrAlreadyLatest    = errors.New("plugins: plugin is already at latest version")
	ErrVersionNotFound  = errors.New("plugins: requested version not found")
)

// UpdateInfo holds information about a plugin's update status.
type UpdateInfo struct {
	Name       string
	Current    string
	Available  string
	Pinned     bool
	Compatible bool
}

// UpdateResult holds the outcome of a single plugin update operation.
type UpdateResult struct {
	Name    string
	From    string
	To      string
	Status  string // "updated", "skipped", "failed"
	Error   error
}

// VersionManager orchestrates plugin version lifecycle: update, rollback, pin.
// It composes LocalRegistry.Install + Remove for update operations.
type VersionManager struct {
	registry     *LocalRegistry
	backupDir    string // ~/.siply/plugins/.versions/
	siplyVersion string // injected siply version; falls back to GetSiplyVersion()
	mu           sync.RWMutex
	pinned       map[string]string // name → pinned version
	previous     map[string]string // name → previous version
}

// NewVersionManager creates a VersionManager backed by the given registry.
// backupDir is used for storing previous plugin versions for rollback.
func NewVersionManager(registry *LocalRegistry, backupDir string) *VersionManager {
	if registry == nil {
		return nil
	}
	return &VersionManager{
		registry:  registry,
		backupDir: backupDir,
		pinned:    make(map[string]string),
		previous:  make(map[string]string),
	}
}

// SetSiplyVersion overrides the siply version used for compatibility checks.
// This is primarily useful for testing. If not set, GetSiplyVersion() is used.
func (vm *VersionManager) SetSiplyVersion(v string) {
	vm.siplyVersion = v
}

// getSiplyVersion returns the effective siply version.
func (vm *VersionManager) getSiplyVersion() string {
	if vm.siplyVersion != "" {
		return vm.siplyVersion
	}
	return GetSiplyVersion()
}

// Check lists all installed plugins with their update status.
// In Phase 1, available version is determined from lockfile/local source comparison.
func (vm *VersionManager) Check(ctx context.Context) ([]UpdateInfo, error) {
	if vm == nil || vm.registry == nil {
		return nil, fmt.Errorf("plugins: version: check: VersionManager not initialized")
	}

	metas, err := vm.registry.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: version: check: %w", err)
	}

	siplyVersion := vm.getSiplyVersion()
	var infos []UpdateInfo

	vm.mu.RLock()
	defer vm.mu.RUnlock()

	for _, m := range metas {
		info := UpdateInfo{
			Name:    m.Name,
			Current: m.Version,
			Pinned:  vm.pinned[m.Name] != "",
		}

		// In Phase 1, we check compatibility but don't have a remote source
		// to determine available version. Available is empty until marketplace.
		pluginDir := filepath.Join(vm.registry.registryDir, m.Name)
		manifest, err := LoadManifestFromDir(pluginDir)
		if err == nil {
			info.Compatible = IsCompatible(manifest.Metadata.SiplyMin, siplyVersion)
		} else {
			info.Compatible = true // assume compatible if can't read manifest
		}

		infos = append(infos, info)
	}

	return infos, nil
}

// Update updates a plugin from a local source directory to a newer version.
func (vm *VersionManager) Update(ctx context.Context, name string, source string) error {
	if vm == nil || vm.registry == nil {
		return fmt.Errorf("plugins: version: update: VersionManager not initialized")
	}
	if name == "" {
		return fmt.Errorf("plugins: version: update: plugin name is empty")
	}
	if source == "" {
		return fmt.Errorf("plugins: version: update: source path is empty")
	}

	// Reject path traversal in name.
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("plugins: version: update: invalid plugin name %q", name)
	}

	// Check pin state under lock.
	vm.mu.RLock()
	pinnedVersion := vm.pinned[name]
	vm.mu.RUnlock()
	if pinnedVersion != "" {
		return fmt.Errorf("%w: %s is pinned at version %s", ErrPluginPinned, name, pinnedVersion)
	}

	// Load source manifest to validate.
	newManifest, err := LoadManifestFromDir(source)
	if err != nil {
		return fmt.Errorf("plugins: version: update: %w", err)
	}

	if newManifest.Metadata.Name != name {
		return fmt.Errorf("plugins: version: update: source manifest name %q does not match %q", newManifest.Metadata.Name, name)
	}

	// Get current installed version.
	currentManifest, err := vm.getCurrentManifest(name)
	if err != nil {
		return fmt.Errorf("plugins: version: update: %w", err)
	}

	// Check if already at latest.
	if CompareVersions(newManifest.Metadata.Version, currentManifest.Metadata.Version) <= 0 {
		return fmt.Errorf("%w: %s is at version %s, source is %s", ErrAlreadyLatest, name, currentManifest.Metadata.Version, newManifest.Metadata.Version)
	}

	// Check compatibility.
	siplyVersion := vm.getSiplyVersion()
	if !IsCompatible(newManifest.Metadata.SiplyMin, siplyVersion) {
		return fmt.Errorf("%w: %s", ErrIncompatible, FormatIncompatibleMessage(name, newManifest.Metadata.Version, siplyVersion, newManifest.Metadata.SiplyMin))
	}

	// Backup current version before update.
	currentVersion := currentManifest.Metadata.Version
	if err := vm.backupPlugin(name, currentVersion); err != nil {
		return fmt.Errorf("plugins: version: update: backup: %w", err)
	}

	// Remove old version and install new one.
	if err := vm.registry.Remove(ctx, name); err != nil {
		return fmt.Errorf("plugins: version: update: remove old: %w", err)
	}

	if err := vm.registry.Install(ctx, source); err != nil {
		// Attempt restore on failure.
		slog.Warn("update install failed, attempting restore", "name", name, "err", err)
		if restoreErr := vm.restorePlugin(name, currentVersion); restoreErr != nil {
			slog.Error("restore after failed update also failed", "name", name, "err", restoreErr)
		}
		return fmt.Errorf("plugins: version: update: install new: %w", err)
	}

	// Track previous version for rollback.
	vm.mu.Lock()
	vm.previous[name] = currentVersion
	vm.mu.Unlock()

	slog.Info("plugin updated", "name", name, "from", currentVersion, "to", newManifest.Metadata.Version)

	if err := vm.SaveState(); err != nil {
		slog.Warn("failed to persist version state after update", "err", err)
	}
	return nil
}

// UpdateAll updates all non-pinned plugins from provided local sources.
func (vm *VersionManager) UpdateAll(ctx context.Context, sources map[string]string) ([]UpdateResult, error) {
	if vm == nil || vm.registry == nil {
		return nil, fmt.Errorf("plugins: version: update-all: VersionManager not initialized")
	}

	metas, err := vm.registry.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugins: version: update-all: %w", err)
	}

	var results []UpdateResult
	for _, m := range metas {
		source, ok := sources[m.Name]
		if !ok {
			continue
		}

		result := UpdateResult{Name: m.Name, From: m.Version}

		if err := vm.Update(ctx, m.Name, source); err != nil {
			if errors.Is(err, ErrPluginPinned) {
				result.Status = "skipped"
				result.Error = err
			} else if errors.Is(err, ErrAlreadyLatest) {
				result.Status = "skipped"
				result.Error = err
			} else {
				result.Status = "failed"
				result.Error = err
			}
		} else {
			result.Status = "updated"
			// Re-read the new version from registry.
			pluginDir := filepath.Join(vm.registry.registryDir, m.Name)
			if newManifest, err := LoadManifestFromDir(pluginDir); err == nil {
				result.To = newManifest.Metadata.Version
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// Rollback reverts a plugin to its previous version from backup.
func (vm *VersionManager) Rollback(ctx context.Context, name string) error {
	if vm == nil || vm.registry == nil {
		return fmt.Errorf("plugins: version: rollback: VersionManager not initialized")
	}
	if name == "" {
		return fmt.Errorf("plugins: version: rollback: plugin name is empty")
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("plugins: version: rollback: invalid plugin name %q", name)
	}

	// Get previous version.
	vm.mu.RLock()
	previousVersion := vm.previous[name]
	vm.mu.RUnlock()

	if previousVersion == "" {
		// Check backup directory for any backed up versions.
		versions, err := vm.listBackups(name)
		if err != nil || len(versions) == 0 {
			return fmt.Errorf("%w: %s", ErrNoPreviousVersion, name)
		}
		previousVersion = versions[len(versions)-1] // most recent backup
	}

	// Get current version for logging.
	currentManifest, err := vm.getCurrentManifest(name)
	if err != nil {
		return fmt.Errorf("plugins: version: rollback: %w", err)
	}
	currentVersion := currentManifest.Metadata.Version

	// Verify backup source exists and has valid manifest before removing current.
	backupSrcDir := filepath.Join(vm.backupDir, name, previousVersion)
	if _, err := LoadManifestFromDir(backupSrcDir); err != nil {
		return fmt.Errorf("plugins: version: rollback: backup %s@%s is invalid: %w", name, previousVersion, err)
	}

	// Backup current version before removal so we can recover on restore failure.
	if err := vm.backupPlugin(name, currentVersion); err != nil {
		slog.Warn("rollback: could not backup current version before removal", "name", name, "err", err)
	}

	// Remove current version.
	if err := vm.registry.Remove(ctx, name); err != nil {
		return fmt.Errorf("plugins: version: rollback: remove current: %w", err)
	}

	// Restore from backup.
	if err := vm.restorePlugin(name, previousVersion); err != nil {
		// Attempt to restore current version on failure.
		slog.Warn("rollback restore failed, attempting to re-restore current version", "name", name, "err", err)
		if restoreErr := vm.restorePlugin(name, currentVersion); restoreErr != nil {
			slog.Error("re-restore after failed rollback also failed", "name", name, "err", restoreErr)
		}
		return fmt.Errorf("plugins: version: rollback: restore: %w", err)
	}

	// Reload into registry.
	if err := vm.registry.Load(ctx, name); err != nil {
		return fmt.Errorf("plugins: version: rollback: reload: %w", err)
	}

	// Update tracking — clear previous since we just rolled back.
	vm.mu.Lock()
	vm.previous[name] = currentVersion // current becomes the "previous" after rollback
	vm.mu.Unlock()

	slog.Info("plugin rolled back", "name", name, "from", currentVersion, "to", previousVersion)

	if err := vm.SaveState(); err != nil {
		slog.Warn("failed to persist version state after rollback", "err", err)
	}
	return nil
}

// Pin pins a plugin to a specific version. If the plugin is at a different
// version, it must be updated to that version first (caller's responsibility).
func (vm *VersionManager) Pin(ctx context.Context, name string, version string) error {
	if vm == nil || vm.registry == nil {
		return fmt.Errorf("plugins: version: pin: VersionManager not initialized")
	}
	if name == "" {
		return fmt.Errorf("plugins: version: pin: plugin name is empty")
	}
	if version == "" {
		return fmt.Errorf("plugins: version: pin: version is empty")
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("plugins: version: pin: invalid plugin name %q", name)
	}

	// Verify plugin exists and check version.
	currentManifest, err := vm.getCurrentManifest(name)
	if err != nil {
		return fmt.Errorf("plugins: version: pin: %w", err)
	}

	// Normalize version comparison (strip "v" prefix).
	currentV := strings.TrimPrefix(currentManifest.Metadata.Version, "v")
	requestedV := strings.TrimPrefix(version, "v")

	if currentV != requestedV {
		return fmt.Errorf("%w: %s is at version %s, requested pin at %s — update first",
			ErrVersionNotFound, name, currentManifest.Metadata.Version, version)
	}

	vm.mu.Lock()
	vm.pinned[name] = version
	vm.mu.Unlock()

	slog.Info("plugin pinned", "name", name, "version", version)

	if err := vm.SaveState(); err != nil {
		slog.Warn("failed to persist version state after pin", "err", err)
	}
	return nil
}

// Unpin removes the pin from a plugin, allowing it to be updated again.
func (vm *VersionManager) Unpin(_ context.Context, name string) error {
	if vm == nil || vm.registry == nil {
		return fmt.Errorf("plugins: version: unpin: VersionManager not initialized")
	}
	if name == "" {
		return fmt.Errorf("plugins: version: unpin: plugin name is empty")
	}

	vm.mu.Lock()
	delete(vm.pinned, name)
	vm.mu.Unlock()

	slog.Info("plugin unpinned", "name", name)

	if err := vm.SaveState(); err != nil {
		slog.Warn("failed to persist version state after unpin", "err", err)
	}
	return nil
}

// IsCompatiblePlugin checks whether a plugin is compatible with the current siply version.
func (vm *VersionManager) IsCompatiblePlugin(_ context.Context, name string) (bool, string) {
	if vm == nil || vm.registry == nil {
		return false, "VersionManager not initialized"
	}

	pluginDir := filepath.Join(vm.registry.registryDir, name)
	manifest, err := LoadManifestFromDir(pluginDir)
	if err != nil {
		return false, fmt.Sprintf("cannot read manifest: %v", err)
	}

	siplyVersion := vm.getSiplyVersion()
	if !IsCompatible(manifest.Metadata.SiplyMin, siplyVersion) {
		return false, FormatIncompatibleMessage(name, manifest.Metadata.Version, siplyVersion, manifest.Metadata.SiplyMin)
	}
	return true, ""
}

// GetPinned returns the pinned version for a plugin, or empty if not pinned.
func (vm *VersionManager) GetPinned(name string) string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.pinned[name]
}

// GetPrevious returns the previous version for a plugin, or empty if none.
func (vm *VersionManager) GetPrevious(name string) string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.previous[name]
}

// LoadPinState loads pin and previous version state from provided maps.
func (vm *VersionManager) LoadPinState(pinned map[string]string, previous map[string]string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	for k, v := range pinned {
		vm.pinned[k] = v
	}
	for k, v := range previous {
		vm.previous[k] = v
	}
}

// versionState is the on-disk representation of pin and previous-version state.
type versionState struct {
	Pinned   map[string]string `json:"pinned,omitempty"`
	Previous map[string]string `json:"previous,omitempty"`
}

// stateFilePath returns the path to the version state file.
func (vm *VersionManager) stateFilePath() string {
	return filepath.Join(vm.backupDir, "version-state.json")
}

// SaveState persists pin and previous-version state to disk.
func (vm *VersionManager) SaveState() error {
	if vm.backupDir == "" {
		return fmt.Errorf("plugins: version: save-state: backupDir not configured")
	}

	vm.mu.RLock()
	state := versionState{
		Pinned:   make(map[string]string, len(vm.pinned)),
		Previous: make(map[string]string, len(vm.previous)),
	}
	for k, v := range vm.pinned {
		state.Pinned[k] = v
	}
	for k, v := range vm.previous {
		state.Previous[k] = v
	}
	vm.mu.RUnlock()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("plugins: version: save-state: marshal: %w", err)
	}

	if err := os.MkdirAll(vm.backupDir, 0755); err != nil {
		return fmt.Errorf("plugins: version: save-state: create dir: %w", err)
	}

	if err := fileutil.AtomicWriteFile(vm.stateFilePath(), data, 0644); err != nil {
		return fmt.Errorf("plugins: version: save-state: write: %w", err)
	}

	return nil
}

// LoadState restores pin and previous-version state from disk.
func (vm *VersionManager) LoadState() error {
	if vm.backupDir == "" {
		return nil
	}

	data, err := os.ReadFile(vm.stateFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no state file yet — first run
		}
		return fmt.Errorf("plugins: version: load-state: read: %w", err)
	}

	var state versionState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("plugins: version: load-state: unmarshal: %w", err)
	}

	vm.mu.Lock()
	defer vm.mu.Unlock()
	for k, v := range state.Pinned {
		vm.pinned[k] = v
	}
	for k, v := range state.Previous {
		vm.previous[k] = v
	}
	return nil
}

// getCurrentManifest reads the manifest for a currently installed plugin.
func (vm *VersionManager) getCurrentManifest(name string) (*Manifest, error) {
	pluginDir := filepath.Join(vm.registry.registryDir, name)
	manifest, err := LoadManifestFromDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("read installed manifest %s: %w", name, err)
	}
	return manifest, nil
}

// backupPlugin copies a plugin directory to the versioned backup location.
func (vm *VersionManager) backupPlugin(name, version string) error {
	if vm.backupDir == "" {
		return fmt.Errorf("plugins: version: backup: backupDir not configured")
	}

	srcDir := filepath.Join(vm.registry.registryDir, name)
	dstDir := filepath.Join(vm.backupDir, name, version)

	// Acquire file lock to prevent concurrent backup corruption.
	fl := fileutil.NewFileLock(vm.backupDir)
	if err := fl.ExclusiveLock(); err != nil {
		return fmt.Errorf("plugins: version: backup: acquire lock: %w", err)
	}
	defer fl.Unlock()

	// Create backup directory.
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("plugins: version: backup: create dir: %w", err)
	}

	// Copy plugin files to backup.
	if err := copyDirContents(srcDir, dstDir); err != nil {
		os.RemoveAll(dstDir) // clean up partial backup
		return fmt.Errorf("plugins: version: backup: copy: %w", err)
	}

	slog.Info("plugin backed up", "name", name, "version", version)
	return nil
}

// restorePlugin copies a backed-up plugin version back to the registry.
func (vm *VersionManager) restorePlugin(name, version string) error {
	if vm.backupDir == "" {
		return fmt.Errorf("plugins: version: restore: backupDir not configured")
	}

	srcDir := filepath.Join(vm.backupDir, name, version)
	dstDir := filepath.Join(vm.registry.registryDir, name)

	// Acquire shared lock to prevent reading a partially-written backup.
	fl := fileutil.NewFileLock(vm.backupDir)
	if err := fl.SharedLock(); err != nil {
		return fmt.Errorf("plugins: version: restore: acquire lock: %w", err)
	}
	defer fl.Unlock()

	if _, err := os.Stat(srcDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: backup not found for %s@%s", ErrNoPreviousVersion, name, version)
		}
		return fmt.Errorf("plugins: version: restore: stat backup: %w", err)
	}

	// Clean destination before restoring to prevent stale files.
	if err := os.RemoveAll(dstDir); err != nil {
		return fmt.Errorf("plugins: version: restore: clean destination: %w", err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("plugins: version: restore: create dir: %w", err)
	}

	// Copy from backup.
	if err := copyDirContents(srcDir, dstDir); err != nil {
		return fmt.Errorf("plugins: version: restore: copy: %w", err)
	}

	// Validate manifest after restore.
	if _, err := LoadManifestFromDir(dstDir); err != nil {
		os.RemoveAll(dstDir) // clean up invalid restore
		return fmt.Errorf("plugins: version: restore: manifest validation: %w", err)
	}

	slog.Info("plugin restored", "name", name, "version", version)
	return nil
}

// listBackups returns available backed-up versions for a plugin.
func (vm *VersionManager) listBackups(name string) ([]string, error) {
	if vm.backupDir == "" {
		return nil, fmt.Errorf("plugins: version: list-backups: backupDir not configured")
	}

	pluginBackupDir := filepath.Join(vm.backupDir, name)
	entries, err := os.ReadDir(pluginBackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugins: version: list-backups: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			versions = append(versions, entry.Name())
		}
	}
	// Sort by semver so the last element is the highest version.
	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i], versions[j]) < 0
	})
	return versions, nil
}

// copyDirContents copies all files from src to dst directory.
// Similar to copyDir in registry.go but operates on an existing destination.
func copyDirContents(src, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve source: %w", err)
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	if strings.HasPrefix(absDst, absSrc+string(filepath.Separator)) ||
		strings.HasPrefix(absSrc, absDst+string(filepath.Separator)) ||
		absSrc == absDst {
		return fmt.Errorf("source and destination paths overlap")
	}

	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip symlinks.
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
			return fmt.Errorf("path traversal detected: %s", rel)
		}

		if d.IsDir() {
			dirInfo, err := d.Info()
			if err != nil {
				return fmt.Errorf("stat dir %s: %w", path, err)
			}
			return os.MkdirAll(target, dirInfo.Mode().Perm())
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}

		if info.Size() > maxPluginFileSize {
			return fmt.Errorf("file %s exceeds %d byte limit", rel, maxPluginFileSize)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		return fileutil.AtomicWriteFile(target, data, info.Mode())
	})
}
