// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package credential

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
)

func TestNewFileStore(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NotNil(t, fs)
	assert.Equal(t, dir, fs.configDir)
	assert.False(t, fs.loaded)
}

func TestInit_CreatesDirectory(t *testing.T) {
	// AC#1: Init creates ~/.siply/ directory
	dir := filepath.Join(t.TempDir(), "subdir", ".siply")
	fs := NewFileStore(dir)

	require.NoError(t, fs.Init(context.Background()))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	// AC#2: directory permissions 0700
	assert.Equal(t, os.FileMode(dirPermissions), info.Mode().Perm())
}

func TestHealth_BeforeInit(t *testing.T) {
	// Lifecycle: Health before Init returns error
	fs := NewFileStore(t.TempDir())
	err := fs.Health()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestHealth_AfterInit(t *testing.T) {
	// Lifecycle: Health after Init returns nil
	fs := NewFileStore(t.TempDir())
	require.NoError(t, fs.Init(context.Background()))
	assert.NoError(t, fs.Health())
}

func TestStartStop_NoOps(t *testing.T) {
	fs := NewFileStore(t.TempDir())
	require.NoError(t, fs.Init(context.Background()))
	assert.NoError(t, fs.Start(context.Background()))
	assert.NoError(t, fs.Stop(context.Background()))
}

func TestSetProvider_PersistsToFile(t *testing.T) {
	// AC#1: credentials stored in ~/.siply/credentials as YAML
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	err := fs.SetProvider(context.Background(), "anthropic", core.Credential{Value: "sk-ant-test"})
	require.NoError(t, err)

	// Verify file exists with correct permissions.
	path := filepath.Join(dir, "credentials")
	info, err := os.Stat(path)
	require.NoError(t, err)
	// AC#2: file permissions 600
	assert.Equal(t, os.FileMode(filePermissions), info.Mode().Perm())

	// Verify content is YAML with the key.
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "sk-ant-test")
}

func TestGetProvider_ReturnsStoredCredential(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	require.NoError(t, fs.SetProvider(context.Background(), "openai", core.Credential{Value: "sk-openai-test"}))

	cred, err := fs.GetProvider(context.Background(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "sk-openai-test", cred.Value)
}

func TestGetProvider_ErrorForUnknown(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	_, err := fs.GetProvider(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no key for provider "nonexistent"`)
}

func TestGetProvider_OllamaSpecialCase(t *testing.T) {
	// Ollama returns empty credential when no key stored (adapter uses default base URL).
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	cred, err := fs.GetProvider(context.Background(), "ollama")
	require.NoError(t, err)
	assert.Equal(t, "", cred.Value)
}

func TestGetProvider_OllamaWithStoredKey(t *testing.T) {
	// If Ollama has a stored key, return it instead of empty default.
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	require.NoError(t, fs.SetProvider(context.Background(), "ollama", core.Credential{Value: "custom-key"}))

	cred, err := fs.GetProvider(context.Background(), "ollama")
	require.NoError(t, err)
	assert.Equal(t, "custom-key", cred.Value)
}

func TestAutoDetectFromEnv(t *testing.T) {
	// AC#3: auto-detection from environment variables
	dir := t.TempDir()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-env")
	t.Setenv("OPENAI_API_KEY", "sk-openai-env")

	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	cred, err := fs.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-env", cred.Value)

	cred, err = fs.GetProvider(context.Background(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "sk-openai-env", cred.Value)

	// Verify persisted to file.
	path := filepath.Join(dir, "credentials")
	_, err = os.Stat(path)
	require.NoError(t, err, "credentials file should have been created from env detection")
}

func TestMissingFile_TriggersEnvDetection(t *testing.T) {
	// No credentials file → env detection runs
	dir := t.TempDir()
	t.Setenv("OPENROUTER_API_KEY", "sk-or-env")

	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	cred, err := fs.GetProvider(context.Background(), "openrouter")
	require.NoError(t, err)
	assert.Equal(t, "sk-or-env", cred.Value)
}

func TestExistingFile_SkipsEnvDetection(t *testing.T) {
	// If credentials file exists, env vars are NOT used (file takes precedence)
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	content := "providers:\n  anthropic:\n    value: \"sk-from-file\"\n"
	require.NoError(t, os.WriteFile(credFile, []byte(content), filePermissions))

	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env-should-not-be-used")

	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	cred, err := fs.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-from-file", cred.Value)
}

func TestPermissionsSelfHeal(t *testing.T) {
	// AC#2: permissions self-healed if wrong (e.g., 0644 → 0600)
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	content := "providers:\n  anthropic:\n    value: \"sk-test\"\n"
	require.NoError(t, os.WriteFile(credFile, []byte(content), 0644))

	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	info, err := os.Stat(credFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(filePermissions), info.Mode().Perm())
}

func TestSetProvider_ThenReInit(t *testing.T) {
	// Write then re-read: SetProvider persists, new FileStore reads it back
	dir := t.TempDir()

	fs1 := NewFileStore(dir)
	require.NoError(t, fs1.Init(context.Background()))
	require.NoError(t, fs1.SetProvider(context.Background(), "anthropic", core.Credential{Value: "sk-persist"}))

	fs2 := NewFileStore(dir)
	require.NoError(t, fs2.Init(context.Background()))

	cred, err := fs2.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-persist", cred.Value)
}

func TestCredentialWithExpiry(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, fs.SetProvider(context.Background(), "openai", core.Credential{
		Value:     "sk-expiring",
		ExpiresAt: &expires,
	}))

	// Re-init to verify persistence.
	fs2 := NewFileStore(dir)
	require.NoError(t, fs2.Init(context.Background()))

	cred, err := fs2.GetProvider(context.Background(), "openai")
	require.NoError(t, err)
	assert.Equal(t, "sk-expiring", cred.Value)
	require.NotNil(t, cred.ExpiresAt)
	assert.Equal(t, expires.Unix(), cred.ExpiresAt.Unix())
}

func TestPluginNamespaceIsolation(t *testing.T) {
	// AC#7: plugin X cannot read plugin Y's credentials
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	require.NoError(t, fs.SetPluginCredential(context.Background(), "plugin-a", "token", core.Credential{Value: "a-secret"}))
	require.NoError(t, fs.SetPluginCredential(context.Background(), "plugin-b", "token", core.Credential{Value: "b-secret"}))

	// Plugin A reads its own key.
	cred, err := fs.GetPluginCredential(context.Background(), "plugin-a", "token")
	require.NoError(t, err)
	assert.Equal(t, "a-secret", cred.Value)

	// Plugin B reads its own key.
	cred, err = fs.GetPluginCredential(context.Background(), "plugin-b", "token")
	require.NoError(t, err)
	assert.Equal(t, "b-secret", cred.Value)

	// Plugin A cannot read plugin B's key via cross-namespace access.
	_, err = fs.GetPluginCredential(context.Background(), "plugin-a", "b-only-key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `no key "b-only-key" for plugin "plugin-a"`)

	// Explicit cross-namespace: plugin-a tries to read plugin-b's "token" key — must fail.
	credA, err := fs.GetPluginCredential(context.Background(), "plugin-a", "token")
	require.NoError(t, err)
	assert.Equal(t, "a-secret", credA.Value, "plugin-a should see its own token")

	// Set a key only in plugin-b's namespace, verify plugin-a cannot see it.
	require.NoError(t, fs.SetPluginCredential(context.Background(), "plugin-b", "exclusive", core.Credential{Value: "b-exclusive"}))
	_, err = fs.GetPluginCredential(context.Background(), "plugin-a", "exclusive")
	require.Error(t, err, "plugin-a must not access plugin-b's exclusive key")
	assert.Contains(t, err.Error(), `no key "exclusive" for plugin "plugin-a"`)
}

func TestSetPluginCredential_PersistsToFile(t *testing.T) {
	dir := t.TempDir()
	fs1 := NewFileStore(dir)
	require.NoError(t, fs1.Init(context.Background()))

	require.NoError(t, fs1.SetPluginCredential(context.Background(), "my-plugin", "api_token", core.Credential{Value: "tok-123"}))

	// Re-init and verify.
	fs2 := NewFileStore(dir)
	require.NoError(t, fs2.Init(context.Background()))

	cred, err := fs2.GetPluginCredential(context.Background(), "my-plugin", "api_token")
	require.NoError(t, err)
	assert.Equal(t, "tok-123", cred.Value)
}

func TestUnknownYAMLFields_SilentlyIgnored(t *testing.T) {
	// B6: unknown fields in credentials file are silently ignored for forward compatibility.
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	content := "providers:\n  anthropic:\n    value: \"sk-test\"\n    unknown_field: \"oops\"\n"
	require.NoError(t, os.WriteFile(credFile, []byte(content), filePermissions))

	fs := NewFileStore(dir)
	err := fs.Init(context.Background())
	require.NoError(t, err, "unknown fields should be silently ignored")

	cred, err := fs.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-test", cred.Value)
}

func TestCorruptYAML_ProducesActionableError(t *testing.T) {
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credFile, []byte("{{invalid yaml"), filePermissions))

	fs := NewFileStore(dir)
	err := fs.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential: failed to parse credentials file")
}

func TestEmptyFile_TriggersEnvDetection(t *testing.T) {
	dir := t.TempDir()
	credFile := filepath.Join(dir, "credentials")
	require.NoError(t, os.WriteFile(credFile, []byte(""), filePermissions))

	t.Setenv("ANTHROPIC_API_KEY", "sk-empty-file-env")

	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	cred, err := fs.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-empty-file-env", cred.Value)
}

func TestMultipleProviders_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		envVar   string
		envValue string
	}{
		{name: "anthropic", provider: "anthropic", envVar: "ANTHROPIC_API_KEY", envValue: "sk-ant-td"},
		{name: "openai", provider: "openai", envVar: "OPENAI_API_KEY", envValue: "sk-oai-td"},
		{name: "openrouter", provider: "openrouter", envVar: "OPENROUTER_API_KEY", envValue: "sk-or-td"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv(tc.envVar, tc.envValue)

			fs := NewFileStore(dir)
			require.NoError(t, fs.Init(context.Background()))

			cred, err := fs.GetProvider(context.Background(), tc.provider)
			require.NoError(t, err)
			assert.Equal(t, tc.envValue, cred.Value)
		})
	}
}

func TestGetProvider_ExpiredCredentialReturnsError(t *testing.T) {
	// B5: expired credentials return ErrCredentialExpired instead of stale data.
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	expired := time.Now().Add(-1 * time.Hour)
	err := fs.SetProvider(context.Background(), "anthropic", core.Credential{
		Value:     "sk-expired",
		ExpiresAt: &expired,
	})
	require.NoError(t, err)

	_, err = fs.GetProvider(context.Background(), "anthropic")
	assert.ErrorIs(t, err, ErrCredentialExpired)
}

func TestGetProvider_NonExpiredCredentialSucceeds(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	future := time.Now().Add(24 * time.Hour)
	err := fs.SetProvider(context.Background(), "anthropic", core.Credential{
		Value:     "sk-valid",
		ExpiresAt: &future,
	})
	require.NoError(t, err)

	cred, err := fs.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-valid", cred.Value)
}

func TestGetProvider_NilExpiresAtSucceeds(t *testing.T) {
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	err := fs.SetProvider(context.Background(), "anthropic", core.Credential{
		Value: "sk-noexpiry",
	})
	require.NoError(t, err)

	cred, err := fs.GetProvider(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-noexpiry", cred.Value)
}

func TestGetPluginCredential_ExpiredReturnsError(t *testing.T) {
	// B5: expired plugin credentials return ErrCredentialExpired.
	dir := t.TempDir()
	fs := NewFileStore(dir)
	require.NoError(t, fs.Init(context.Background()))

	expired := time.Now().Add(-1 * time.Hour)
	err := fs.SetPluginCredential(context.Background(), "myplugin", "api_key", core.Credential{
		Value:     "expired-key",
		ExpiresAt: &expired,
	})
	require.NoError(t, err)

	_, err = fs.GetPluginCredential(context.Background(), "myplugin", "api_key")
	assert.ErrorIs(t, err, ErrCredentialExpired)
}
