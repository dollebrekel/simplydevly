// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid manifest",
			file:    "testdata/valid_manifest.yaml",
			wantErr: false,
		},
		{
			name:    "valid tier 1 manifest",
			file:    "testdata/valid_tier1_manifest.yaml",
			wantErr: false,
		},
		{
			name:    "valid tier 3 manifest",
			file:    "testdata/valid_tier3_manifest.yaml",
			wantErr: false,
		},
		{
			name:    "empty file",
			file:    "testdata/invalid_empty.yaml",
			wantErr: true,
			errMsg:  "empty manifest",
		},
		{
			name:    "unknown fields rejected by strict parse",
			file:    "testdata/invalid_unknown_fields.yaml",
			wantErr: true,
			errMsg:  "invalid manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.file)
			require.NoError(t, err, "failed to read fixture file")

			m, err := ParseManifest(data)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, m)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, m)
			}
		})
	}
}

func TestParseManifest_ValidFields(t *testing.T) {
	data, err := os.ReadFile("testdata/valid_manifest.yaml")
	require.NoError(t, err)

	m, err := ParseManifest(data)
	require.NoError(t, err)

	assert.Equal(t, "siply/v1", m.APIVersion)
	assert.Equal(t, "Plugin", m.Kind)
	assert.Equal(t, "memory-default", m.Metadata.Name)
	assert.Equal(t, "1.0.0", m.Metadata.Version)
	assert.Equal(t, "1.0.0", m.Metadata.SiplyMin)
	assert.Equal(t, "Default memory backend", m.Metadata.Description)
	assert.Equal(t, "siply-dev", m.Metadata.Author)
	assert.Equal(t, "Apache-2.0", m.Metadata.License)
	assert.Equal(t, 3, m.Spec.Tier)
	assert.Equal(t, "read", m.Spec.Capabilities["filesystem"])
	assert.Equal(t, "none", m.Spec.Capabilities["network"])
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mod     func(*Manifest)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid manifest passes",
			mod:     func(m *Manifest) {},
			wantErr: false,
		},
		{
			name:    "nil manifest",
			mod:     nil, // special case
			wantErr: true,
			errMsg:  "nil manifest",
		},
		{
			name:    "wrong apiVersion",
			mod:     func(m *Manifest) { m.APIVersion = "siply/v2" },
			wantErr: true,
			errMsg:  "apiVersion",
		},
		{
			name:    "wrong kind",
			mod:     func(m *Manifest) { m.Kind = "Extension" },
			wantErr: true,
			errMsg:  "kind",
		},
		{
			name:    "empty name",
			mod:     func(m *Manifest) { m.Metadata.Name = "" },
			wantErr: true,
			errMsg:  "metadata.name is required",
		},
		{
			name:    "invalid name with uppercase",
			mod:     func(m *Manifest) { m.Metadata.Name = "MyPlugin" },
			wantErr: true,
			errMsg:  "metadata.name must match",
		},
		{
			name:    "invalid name with underscore",
			mod:     func(m *Manifest) { m.Metadata.Name = "my_plugin" },
			wantErr: true,
			errMsg:  "metadata.name must match",
		},
		{
			name:    "invalid name starting with number",
			mod:     func(m *Manifest) { m.Metadata.Name = "1plugin" },
			wantErr: true,
			errMsg:  "metadata.name must match",
		},
		{
			name:    "empty version",
			mod:     func(m *Manifest) { m.Metadata.Version = "" },
			wantErr: true,
			errMsg:  "metadata.version is required",
		},
		{
			name:    "non-semver version",
			mod:     func(m *Manifest) { m.Metadata.Version = "not-a-version" },
			wantErr: true,
			errMsg:  "metadata.version must be valid semver",
		},
		{
			name:    "empty siply_min",
			mod:     func(m *Manifest) { m.Metadata.SiplyMin = "" },
			wantErr: true,
			errMsg:  "metadata.siply_min is required",
		},
		{
			name:    "non-semver siply_min",
			mod:     func(m *Manifest) { m.Metadata.SiplyMin = "latest" },
			wantErr: true,
			errMsg:  "metadata.siply_min must be valid semver",
		},
		{
			name:    "empty description",
			mod:     func(m *Manifest) { m.Metadata.Description = "" },
			wantErr: true,
			errMsg:  "metadata.description is required",
		},
		{
			name:    "empty author",
			mod:     func(m *Manifest) { m.Metadata.Author = "" },
			wantErr: true,
			errMsg:  "metadata.author is required",
		},
		{
			name:    "empty license",
			mod:     func(m *Manifest) { m.Metadata.License = "" },
			wantErr: true,
			errMsg:  "metadata.license is required",
		},
		{
			name:    "tier 0",
			mod:     func(m *Manifest) { m.Spec.Tier = 0 },
			wantErr: true,
			errMsg:  "spec.tier must be 1, 2, or 3",
		},
		{
			name:    "tier 99",
			mod:     func(m *Manifest) { m.Spec.Tier = 99 },
			wantErr: true,
			errMsg:  "spec.tier must be 1, 2, or 3",
		},
		{
			name: "unknown capability key",
			mod: func(m *Manifest) {
				m.Spec.Capabilities = map[string]string{"teleport": "quantum"}
			},
			wantErr: true,
			errMsg:  "unknown key",
		},
		{
			name: "invalid capability value",
			mod: func(m *Manifest) {
				m.Spec.Capabilities = map[string]string{"filesystem": "execute"}
			},
			wantErr: true,
			errMsg:  "invalid value",
		},
		{
			name:    "semver with prerelease is valid",
			mod:     func(m *Manifest) { m.Metadata.Version = "1.0.0-alpha.1" },
			wantErr: false,
		},
		{
			name:    "semver with build metadata is valid",
			mod:     func(m *Manifest) { m.Metadata.Version = "1.0.0+build.123" },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nil manifest" {
				var m *Manifest
				err := m.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				return
			}

			m := validManifest()
			tt.mod(m)
			err := m.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	m := &Manifest{}
	err := m.Validate()
	require.Error(t, err)

	errStr := err.Error()
	assert.Contains(t, errStr, "apiVersion")
	assert.Contains(t, errStr, "kind")
	assert.Contains(t, errStr, "metadata.name")
	assert.Contains(t, errStr, "metadata.version")
}

func TestValidate_FromFixtures(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr bool
		errMsg  string
	}{
		{"valid", "testdata/valid_manifest.yaml", false, ""},
		{"valid tier1", "testdata/valid_tier1_manifest.yaml", false, ""},
		{"valid tier3", "testdata/valid_tier3_manifest.yaml", false, ""},
		{"missing name", "testdata/invalid_missing_name.yaml", true, "metadata.name"},
		{"bad version", "testdata/invalid_bad_version.yaml", true, "metadata.version"},
		{"bad tier", "testdata/invalid_bad_tier.yaml", true, "spec.tier"},
		{"bad capability", "testdata/invalid_bad_capability.yaml", true, "unknown key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			m, err := ParseManifest(data)
			if err != nil {
				// Strict parse failed — this counts as a validation error
				if tt.wantErr {
					return
				}
				t.Fatalf("unexpected parse error: %v", err)
			}

			err = m.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadManifestFromDir(t *testing.T) {
	t.Run("valid directory", func(t *testing.T) {
		dir := t.TempDir()
		copyFixture(t, "testdata/valid_manifest.yaml", filepath.Join(dir, "manifest.yaml"))

		m, err := LoadManifestFromDir(dir)
		require.NoError(t, err)
		assert.Equal(t, "memory-default", m.Metadata.Name)
	})

	t.Run("missing manifest.yaml", func(t *testing.T) {
		dir := t.TempDir()

		_, err := LoadManifestFromDir(dir)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrManifestNotFound)
	})

	t.Run("oversized file", func(t *testing.T) {
		dir := t.TempDir()
		bigData := strings.Repeat("a", maxManifestSize+1)
		err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(bigData), 0644)
		require.NoError(t, err)

		_, err = LoadManifestFromDir(dir)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrManifestTooLarge)
	})

	t.Run("empty pluginDir", func(t *testing.T) {
		_, err := LoadManifestFromDir("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pluginDir is empty")
	})

	t.Run("invalid manifest in directory", func(t *testing.T) {
		dir := t.TempDir()
		copyFixture(t, "testdata/invalid_bad_tier.yaml", filepath.Join(dir, "manifest.yaml"))

		_, err := LoadManifestFromDir(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec.tier")
	})
}

func TestErrorMessages_Actionable(t *testing.T) {
	// Verify error messages contain field name + expected format
	m := validManifest()
	m.Metadata.Name = "BAD_NAME"
	err := m.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata.name")
	assert.Contains(t, err.Error(), "^[a-z][a-z0-9-]{0,62}$")

	m2 := validManifest()
	m2.Metadata.Version = "bad"
	err = m2.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metadata.version")
	assert.Contains(t, err.Error(), "semver")
}

// validManifest returns a valid Manifest for test modification.
func validManifest() *Manifest {
	return &Manifest{
		APIVersion: "siply/v1",
		Kind:       "Plugin",
		Metadata: Metadata{
			Name:        "test-plugin",
			Version:     "1.0.0",
			SiplyMin:    "1.0.0",
			Description: "A test plugin",
			Author:      "test-author",
			License:     "MIT",
			Updated:     "2026-04-10",
		},
		Spec: Spec{
			Tier:         1,
			Capabilities: map[string]string{},
		},
	}
}

// copyFixture copies a fixture file to a destination path.
func copyFixture(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, data, 0644))
}
