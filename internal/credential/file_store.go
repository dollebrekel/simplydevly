// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package credential

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"siply.dev/siply/internal/core"
)

const (
	filePermissions = 0600
	dirPermissions  = 0700
	credFileName    = "credentials"
)

// providerEntry is the YAML-serializable format for a single credential.
type providerEntry struct {
	Value     string `yaml:"value"`
	ExpiresAt *int64 `yaml:"expires_at,omitempty"` // Unix timestamp, nil = never expires
}

// credentialsFile is the YAML-serializable root structure for ~/.siply/credentials.
type credentialsFile struct {
	Providers map[string]providerEntry            `yaml:"providers,omitempty"`
	Plugins   map[string]map[string]providerEntry `yaml:"plugins,omitempty"`
}

// FileStore implements core.CredentialStore with file-based persistence.
// Credentials are stored as YAML in ~/.siply/credentials with 0600 permissions.
type FileStore struct {
	configDir string
	mu        sync.RWMutex
	data      credentialsFile
	loaded    bool
}

// NewFileStore creates a new FileStore that persists credentials under configDir.
func NewFileStore(configDir string) *FileStore {
	return &FileStore{configDir: configDir}
}

// credPath returns the full path to the credentials file.
func (s *FileStore) credPath() string {
	return filepath.Join(s.configDir, credFileName)
}

// Init creates the config directory and loads or detects credentials.
func (s *FileStore) Init(_ context.Context) error {
	if err := os.MkdirAll(s.configDir, dirPermissions); err != nil {
		return fmt.Errorf("credential: failed to create config dir: %w", err)
	}
	return s.loadOrDetect()
}

// Start is a no-op for FileStore.
func (s *FileStore) Start(_ context.Context) error { return nil }

// Stop is a no-op for FileStore.
func (s *FileStore) Stop(_ context.Context) error { return nil }

// Health returns an error if the store has not been initialized.
func (s *FileStore) Health() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		return fmt.Errorf("credential: store not initialized")
	}
	return nil
}

// loadOrDetect tries to load from file first, then falls back to env detection.
func (s *FileStore) loadOrDetect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loaded, err := s.loadFromFile()
	if err != nil {
		return err
	}

	if !loaded {
		s.detectFromEnv()
		if len(s.data.Providers) > 0 {
			if err := s.saveToFileLocked(); err != nil {
				return err
			}
		}
	}

	s.loaded = true
	return nil
}

// loadFromFile reads and parses the credentials YAML file.
// Returns (true, nil) if loaded, (false, nil) if file doesn't exist, or error.
func (s *FileStore) loadFromFile() (bool, error) {
	path := s.credPath()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = credentialsFile{}
			return false, nil
		}
		return false, fmt.Errorf("credential: failed to stat credentials file: %w", err)
	}

	// Self-heal wrong permissions.
	if info.Mode().Perm() != filePermissions {
		slog.Warn("credential: credentials file has wrong permissions, fixing",
			"current", fmt.Sprintf("%04o", info.Mode().Perm()),
			"expected", fmt.Sprintf("%04o", filePermissions))
		if err := os.Chmod(path, filePermissions); err != nil {
			return false, fmt.Errorf("credential: failed to fix permissions: %w", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("credential: failed to read credentials file: %w", err)
	}

	if len(raw) == 0 {
		s.data = credentialsFile{}
		return false, nil
	}

	var cf credentialsFile
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cf); err != nil {
		return false, fmt.Errorf("credential: failed to parse credentials file: %w", err)
	}

	s.data = cf
	return true, nil
}

// saveToFileLocked writes credentials to disk. Caller must hold s.mu.
func (s *FileStore) saveToFileLocked() error {
	raw, err := yaml.Marshal(&s.data)
	if err != nil {
		return fmt.Errorf("credential: failed to marshal credentials: %w", err)
	}

	path := s.credPath()
	if err := os.WriteFile(path, raw, filePermissions); err != nil {
		return fmt.Errorf("credential: failed to write credentials file: %w", err)
	}
	// Enforce permissions on existing files (os.WriteFile only sets mode on creation).
	if err := os.Chmod(path, filePermissions); err != nil {
		return fmt.Errorf("credential: failed to set permissions on credentials file: %w", err)
	}
	return nil
}

// detectFromEnv checks environment variables for known API keys.
func (s *FileStore) detectFromEnv() {
	if s.data.Providers == nil {
		s.data.Providers = make(map[string]providerEntry)
	}

	for provider, envVar := range envKeyMap {
		val := os.Getenv(envVar)
		if val != "" {
			s.data.Providers[provider] = providerEntry{Value: val}
			slog.Info("credential: auto-detected API key", "provider", provider)
		}
	}
}

// GetProvider returns the credential for the given provider name.
func (s *FileStore) GetProvider(_ context.Context, provider string) (core.Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if entry, ok := s.data.Providers[provider]; ok {
		return entryToCredential(entry), nil
	}

	// Ollama special case: no key needed.
	if provider == "ollama" {
		return core.Credential{Value: "unused"}, nil
	}

	return core.Credential{}, fmt.Errorf("credential: no key for provider %q", provider)
}

// SetProvider stores a credential for the given provider and persists to file.
func (s *FileStore) SetProvider(_ context.Context, provider string, cred core.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.Providers == nil {
		s.data.Providers = make(map[string]providerEntry)
	}
	s.data.Providers[provider] = credentialToEntry(cred)
	return s.saveToFileLocked()
}

// GetPluginCredential returns a credential scoped to the given plugin namespace.
func (s *FileStore) GetPluginCredential(_ context.Context, pluginName string, key string) (core.Credential, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if pluginKeys, ok := s.data.Plugins[pluginName]; ok {
		if entry, ok := pluginKeys[key]; ok {
			return entryToCredential(entry), nil
		}
	}

	return core.Credential{}, fmt.Errorf("credential: no key %q for plugin %q", key, pluginName)
}

// SetPluginCredential stores a credential scoped to the given plugin namespace and persists to file.
func (s *FileStore) SetPluginCredential(_ context.Context, pluginName string, key string, cred core.Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.Plugins == nil {
		s.data.Plugins = make(map[string]map[string]providerEntry)
	}
	if s.data.Plugins[pluginName] == nil {
		s.data.Plugins[pluginName] = make(map[string]providerEntry)
	}
	s.data.Plugins[pluginName][key] = credentialToEntry(cred)
	return s.saveToFileLocked()
}

// entryToCredential converts the YAML entry to a core.Credential.
func entryToCredential(e providerEntry) core.Credential {
	c := core.Credential{Value: e.Value}
	if e.ExpiresAt != nil {
		t := timeFromUnix(*e.ExpiresAt)
		c.ExpiresAt = &t
	}
	return c
}

// credentialToEntry converts a core.Credential to the YAML entry.
func credentialToEntry(c core.Credential) providerEntry {
	e := providerEntry{Value: c.Value}
	if c.ExpiresAt != nil {
		unix := c.ExpiresAt.Unix()
		e.ExpiresAt = &unix
	}
	return e
}

// timeFromUnix converts a Unix timestamp to time.Time.
func timeFromUnix(unix int64) time.Time {
	return time.Unix(unix, 0)
}
