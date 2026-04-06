// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package workspace

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	filePermissions    = 0600
	dirPermissions     = 0700
	workspacesFileName = "workspaces.yaml"
	stateFileName      = "state.yaml"
)

// Workspace represents a single workspace bound to a git project.
type Workspace struct {
	Name      string
	RootDir   string
	GitRoot   string
	ConfigDir string // .siply/ directory inside the project
}

// workspaceEntry is the YAML-serializable format for a single workspace.
type workspaceEntry struct {
	RootDir    string `yaml:"root_dir"`
	GitRoot    string `yaml:"git_root"`
	LastActive *int64 `yaml:"last_active,omitempty"`
}

// workspacesFile is the YAML-serializable root structure for ~/.siply/workspaces.yaml.
type workspacesFile struct {
	ActiveWorkspace string                    `yaml:"active_workspace,omitempty"`
	Workspaces      map[string]workspaceEntry `yaml:"workspaces,omitempty"`
}

// workspaceState is lightweight state persisted per workspace.
type workspaceState struct {
	LastSession string `yaml:"last_session,omitempty"`
	LastActive  int64  `yaml:"last_active"`
}

// Manager manages workspaces with file-based persistence.
type Manager struct {
	globalDir   string
	mu          sync.RWMutex
	data        workspacesFile
	active      *Workspace
	initialized bool
}

// NewManager creates a new Manager that persists workspace data under globalDir.
func NewManager(globalDir string) *Manager {
	return &Manager{globalDir: globalDir}
}

// workspacesPath returns the full path to the workspaces registry file.
func (m *Manager) workspacesPath() string {
	return filepath.Join(m.globalDir, workspacesFileName)
}

// Init creates the config directory and loads known workspaces.
func (m *Manager) Init(_ context.Context) error {
	if err := os.MkdirAll(m.globalDir, dirPermissions); err != nil {
		return fmt.Errorf("workspace: failed to create config dir: %w", err)
	}
	return m.loadWorkspaces()
}

// Start is a no-op for Manager.
func (m *Manager) Start(_ context.Context) error { return nil }

// Stop persists workspace state.
func (m *Manager) Stop(_ context.Context) error {
	m.mu.RLock()
	ws := m.active
	m.mu.RUnlock()

	if ws != nil {
		return m.saveState(ws)
	}
	return nil
}

// Health returns an error if the manager has not been initialized.
func (m *Manager) Health() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return fmt.Errorf("workspace: not initialized")
	}
	return nil
}

// loadWorkspaces reads the workspaces registry from disk.
func (m *Manager) loadWorkspaces() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := m.workspacesPath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			m.data = workspacesFile{}
			m.initialized = true
			return nil
		}
		return fmt.Errorf("workspace: failed to stat workspaces file: %w", err)
	}

	// Self-heal wrong permissions.
	if info.Mode().Perm() != filePermissions {
		slog.Warn("workspace: workspaces file has wrong permissions, fixing",
			"current", fmt.Sprintf("%04o", info.Mode().Perm()),
			"expected", fmt.Sprintf("%04o", filePermissions))
		if err := os.Chmod(path, filePermissions); err != nil {
			return fmt.Errorf("workspace: failed to fix permissions: %w", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("workspace: failed to read workspaces file: %w", err)
	}

	if len(raw) == 0 {
		m.data = workspacesFile{}
		m.initialized = true
		return nil
	}

	var wf workspacesFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&wf); err != nil {
		return fmt.Errorf("workspace: failed to parse workspaces file: %w", err)
	}

	m.data = wf
	m.initialized = true

	// Rehydrate active workspace from persisted ActiveWorkspace field.
	if wf.ActiveWorkspace != "" {
		if entry, ok := wf.Workspaces[wf.ActiveWorkspace]; ok {
			m.active = entryToWorkspace(wf.ActiveWorkspace, entry)
		}
	}

	return nil
}

// saveWorkspaces writes the workspaces registry to disk. Caller must hold m.mu.
func (m *Manager) saveWorkspacesLocked() error {
	raw, err := yaml.Marshal(&m.data)
	if err != nil {
		return fmt.Errorf("workspace: failed to marshal workspaces: %w", err)
	}

	path := m.workspacesPath()
	if err := os.WriteFile(path, raw, filePermissions); err != nil {
		return fmt.Errorf("workspace: failed to write workspaces file: %w", err)
	}
	// Enforce permissions on existing files.
	if err := os.Chmod(path, filePermissions); err != nil {
		return fmt.Errorf("workspace: failed to set permissions on workspaces file: %w", err)
	}
	return nil
}

// Detect auto-detects a workspace from the current working directory's git root.
func (m *Manager) Detect(_ context.Context) (*Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("workspace: failed to get working directory: %w", err)
	}

	gitRoot, err := detectGitRoot(cwd)
	if err != nil {
		return nil, fmt.Errorf("workspace: failed to detect git root: %w", err)
	}
	if gitRoot == "" {
		return nil, nil // not in a git repo — no workspace
	}

	name := workspaceName(gitRoot)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already registered.
	if entry, ok := m.data.Workspaces[name]; ok {
		if entry.GitRoot == gitRoot {
			// Same workspace — activate and update last active.
			ws := entryToWorkspace(name, entry)
			m.activateWorkspace(ws)
			m.data.ActiveWorkspace = name
			now := time.Now().Unix()
			entry.LastActive = &now
			m.data.Workspaces[name] = entry
			if err := m.saveWorkspacesLocked(); err != nil {
				slog.Warn("workspace: failed to persist last active time", "error", err)
			}
			slog.Info("workspace: detected", "name", name, "root", gitRoot)
			return ws, nil
		}
		// Name collision: different git root has the same basename.
		// Generate a unique name and fall through to register as new.
		name = workspaceName(gitRoot) + "-" + filepath.Base(filepath.Dir(gitRoot))
		if _, collision := m.data.Workspaces[name]; collision {
			return nil, fmt.Errorf("workspace: name collision for %q — both %q and %q resolve to the same name", name, entry.GitRoot, gitRoot)
		}
	}

	// Register new workspace.
	ws := &Workspace{
		Name:      name,
		RootDir:   gitRoot,
		GitRoot:   gitRoot,
		ConfigDir: filepath.Join(gitRoot, ".siply"),
	}
	now := time.Now().Unix()
	if m.data.Workspaces == nil {
		m.data.Workspaces = make(map[string]workspaceEntry)
	}
	m.data.Workspaces[name] = workspaceEntry{
		RootDir:    ws.RootDir,
		GitRoot:    ws.GitRoot,
		LastActive: &now,
	}
	m.active = ws
	m.data.ActiveWorkspace = name
	if err := m.saveWorkspacesLocked(); err != nil {
		return nil, err
	}
	slog.Info("workspace: detected", "name", name, "root", gitRoot)
	return ws, nil
}

// Open opens a known workspace by name and sets it as active.
func (m *Manager) Open(_ context.Context, name string) (*Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.data.Workspaces[name]
	if !ok {
		return nil, fmt.Errorf("workspace: unknown workspace %q", name)
	}

	ws := entryToWorkspace(name, entry)
	m.active = ws
	m.data.ActiveWorkspace = name
	now := time.Now().Unix()
	entry.LastActive = &now
	m.data.Workspaces[name] = entry
	if err := m.saveWorkspacesLocked(); err != nil {
		return nil, fmt.Errorf("workspace: failed to persist workspace state: %w", err)
	}
	return ws, nil
}

// Create creates a new workspace and registers it.
func (m *Manager) Create(_ context.Context, name, rootDir string) (*Workspace, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("workspace: failed to resolve path: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data.Workspaces[name]; exists {
		return nil, fmt.Errorf("workspace: workspace %q already exists", name)
	}

	gitRoot, err := detectGitRoot(absRoot)
	if err != nil {
		return nil, fmt.Errorf("workspace: failed to detect git root: %w", err)
	}
	if gitRoot == "" {
		return nil, fmt.Errorf("workspace: %q is not inside a git repository", absRoot)
	}

	ws := &Workspace{
		Name:      name,
		RootDir:   absRoot,
		GitRoot:   gitRoot,
		ConfigDir: filepath.Join(absRoot, ".siply"),
	}
	now := time.Now().Unix()
	if m.data.Workspaces == nil {
		m.data.Workspaces = make(map[string]workspaceEntry)
	}
	m.data.Workspaces[name] = workspaceEntry{
		RootDir:    ws.RootDir,
		GitRoot:    ws.GitRoot,
		LastActive: &now,
	}
	m.active = ws
	m.data.ActiveWorkspace = name
	if err := m.saveWorkspacesLocked(); err != nil {
		return nil, err
	}
	return ws, nil
}

// List returns all known workspaces sorted by name.
func (m *Manager) List(_ context.Context) []*Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Workspace, 0, len(m.data.Workspaces))
	for name, entry := range m.data.Workspaces {
		result = append(result, entryToWorkspace(name, entry))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Switch switches to a different workspace by name. Alias for Open.
func (m *Manager) Switch(ctx context.Context, name string) (*Workspace, error) {
	return m.Open(ctx, name)
}

// Active returns the currently active workspace (may be nil).
func (m *Manager) Active() *Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

// activateWorkspace sets a workspace as active and loads its state.
// Caller must hold m.mu.
func (m *Manager) activateWorkspace(ws *Workspace) {
	m.active = ws
	if state, err := m.loadState(ws); err == nil {
		_ = state // state loaded for future use (LastSession etc.)
	}
}

// ConfigDir returns the active workspace's .siply/ path, or empty string if no workspace.
func (m *Manager) ConfigDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active != nil {
		return m.active.ConfigDir
	}
	return ""
}

// loadState reads workspace state from the workspace's config dir.
func (m *Manager) loadState(ws *Workspace) (*workspaceState, error) {
	path := filepath.Join(ws.ConfigDir, stateFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &workspaceState{}, nil
		}
		return nil, fmt.Errorf("workspace: failed to read state file: %w", err)
	}
	if len(raw) == 0 {
		return &workspaceState{}, nil
	}
	var state workspaceState
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&state); err != nil {
		return nil, fmt.Errorf("workspace: failed to parse state file: %w", err)
	}
	return &state, nil
}

// saveState persists workspace state to the workspace's config dir.
func (m *Manager) saveState(ws *Workspace) error {
	if err := os.MkdirAll(ws.ConfigDir, dirPermissions); err != nil {
		return fmt.Errorf("workspace: failed to create workspace config dir: %w", err)
	}
	state := workspaceState{
		LastActive: time.Now().Unix(),
	}
	raw, err := yaml.Marshal(&state)
	if err != nil {
		return fmt.Errorf("workspace: failed to marshal state: %w", err)
	}
	path := filepath.Join(ws.ConfigDir, stateFileName)
	if err := os.WriteFile(path, raw, filePermissions); err != nil {
		return fmt.Errorf("workspace: failed to write state file: %w", err)
	}
	if err := os.Chmod(path, filePermissions); err != nil {
		return fmt.Errorf("workspace: failed to set permissions on state file: %w", err)
	}
	return nil
}

// detectGitRoot walks up from dir to find the .git/ directory.
// Returns empty string if no git root found (not an error).
func detectGitRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil // reached filesystem root
		}
		dir = parent
	}
}

// workspaceName derives a workspace name from the git root basename.
func workspaceName(gitRoot string) string {
	return filepath.Base(gitRoot)
}

// entryToWorkspace converts a registry entry to a Workspace.
func entryToWorkspace(name string, e workspaceEntry) *Workspace {
	return &Workspace{
		Name:      name,
		RootDir:   e.RootDir,
		GitRoot:   e.GitRoot,
		ConfigDir: filepath.Join(e.RootDir, ".siply"),
	}
}
