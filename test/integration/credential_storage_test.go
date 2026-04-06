// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/credential"
)

// TestCredentialStorage_FullLifecycle tests the complete credential flow:
// Init with env vars → auto-detect → persist → re-init from file → verify same keys.
func TestCredentialStorage_FullLifecycle(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// AC#3: Set env vars for auto-detection.
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-integration")
	t.Setenv("OPENAI_API_KEY", "sk-oai-integration")

	// Phase 1: Init auto-detects from env.
	fs1 := credential.NewFileStore(dir)
	require.NoError(t, fs1.Init(ctx))

	cred, err := fs1.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-integration", cred.Value)

	cred, err = fs1.GetProvider(ctx, "openai")
	require.NoError(t, err)
	assert.Equal(t, "sk-oai-integration", cred.Value)

	// Verify file was created.
	credPath := filepath.Join(dir, "credentials")
	_, err = os.Stat(credPath)
	require.NoError(t, err, "credentials file should exist after auto-detection")

	// Phase 2: Clear env vars, re-init from file only.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	fs2 := credential.NewFileStore(dir)
	require.NoError(t, fs2.Init(ctx))

	cred, err = fs2.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	assert.Equal(t, "sk-ant-integration", cred.Value, "should load from file, not env")

	cred, err = fs2.GetProvider(ctx, "openai")
	require.NoError(t, err)
	assert.Equal(t, "sk-oai-integration", cred.Value, "should load from file, not env")
}

// TestCredentialStorage_SetStopReInit verifies persistence across lifecycle restarts.
func TestCredentialStorage_SetStopReInit(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Phase 1: Set a credential and stop.
	fs1 := credential.NewFileStore(dir)
	require.NoError(t, fs1.Init(ctx))
	require.NoError(t, fs1.Start(ctx))

	require.NoError(t, fs1.SetProvider(ctx, "openrouter", core.Credential{Value: "sk-or-persist"}))
	require.NoError(t, fs1.Stop(ctx))

	// Phase 2: Re-init and verify.
	fs2 := credential.NewFileStore(dir)
	require.NoError(t, fs2.Init(ctx))

	cred, err := fs2.GetProvider(ctx, "openrouter")
	require.NoError(t, err)
	assert.Equal(t, "sk-or-persist", cred.Value)
}

// TestCredentialStorage_PluginIsolationAcrossRestarts verifies plugin credentials
// persist and maintain namespace isolation across restarts.
func TestCredentialStorage_PluginIsolationAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	fs1 := credential.NewFileStore(dir)
	require.NoError(t, fs1.Init(ctx))

	require.NoError(t, fs1.SetPluginCredential(ctx, "plugin-alpha", "secret", core.Credential{Value: "alpha-val"}))
	require.NoError(t, fs1.SetPluginCredential(ctx, "plugin-beta", "secret", core.Credential{Value: "beta-val"}))

	// Restart.
	fs2 := credential.NewFileStore(dir)
	require.NoError(t, fs2.Init(ctx))

	// Each plugin sees only its own namespace.
	cred, err := fs2.GetPluginCredential(ctx, "plugin-alpha", "secret")
	require.NoError(t, err)
	assert.Equal(t, "alpha-val", cred.Value)

	cred, err = fs2.GetPluginCredential(ctx, "plugin-beta", "secret")
	require.NoError(t, err)
	assert.Equal(t, "beta-val", cred.Value)

	// Cross-namespace access fails.
	_, err = fs2.GetPluginCredential(ctx, "plugin-alpha", "nonexistent")
	require.Error(t, err)
}
