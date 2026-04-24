// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
)

func TestParseTUIFlags_OfflineFlag(t *testing.T) {
	root := testRootCmd()
	var gotOffline bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotOffline = flags.Offline
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--offline"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotOffline)
}

func TestParseTUIFlags_OfflineEnvVar(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "1")

	root := testRootCmd()
	var gotOffline bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotOffline = flags.Offline
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotOffline)
}

func TestParseTUIFlags_OfflineEnvVarTrue(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "true")

	root := testRootCmd()
	var gotOffline bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotOffline = flags.Offline
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, gotOffline)
}

func TestParseTUIFlags_NoOfflineByDefault(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "")

	root := testRootCmd()
	var gotOffline bool
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotOffline = flags.Offline
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui"})

	err := root.Execute()
	require.NoError(t, err)
	assert.False(t, gotOffline)
}

func TestParseTUIFlags_ModelOverride(t *testing.T) {
	root := testRootCmd()
	var gotModel string
	tuiCmd := &cobra.Command{
		Use: "tui",
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return err
			}
			gotModel = flags.ModelOverride
			return nil
		},
	}
	root.AddCommand(tuiCmd)
	root.SetArgs([]string{"tui", "--offline", "--model", "codellama:13b"})

	err := root.Execute()
	require.NoError(t, err)
	assert.Equal(t, "codellama:13b", gotModel)
}

func TestIsOfflineMode_Flag(t *testing.T) {
	root := testRootCmd()
	root.SetArgs([]string{"--offline"})
	_ = root.Execute()

	assert.True(t, isOfflineMode(root))
}

func TestIsOfflineMode_EnvVar(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "1")

	root := testRootCmd()
	root.SetArgs([]string{})
	_ = root.Execute()

	assert.True(t, isOfflineMode(root))
}

func TestIsOfflineMode_EnvVarTrue(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "true")

	root := testRootCmd()
	root.SetArgs([]string{})
	_ = root.Execute()

	assert.True(t, isOfflineMode(root))
}

func TestIsOfflineMode_NotSet(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "")

	root := testRootCmd()
	root.SetArgs([]string{})
	_ = root.Execute()

	assert.False(t, isOfflineMode(root))
}

func TestWithOfflineGuard_BlocksInOfflineMode(t *testing.T) {
	root := testRootCmd()
	var ran bool
	guardedCmd := withOfflineGuard(&cobra.Command{
		Use: "cloud-cmd",
		RunE: func(_ *cobra.Command, _ []string) error {
			ran = true
			return nil
		},
	})
	root.AddCommand(guardedCmd)
	root.SetArgs([]string{"--offline", "cloud-cmd"})

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "internet connection")
	assert.False(t, ran)
}

func TestWithOfflineGuard_AllowsWithoutOffline(t *testing.T) {
	t.Setenv("SIPLY_OFFLINE", "")

	root := testRootCmd()
	var ran bool
	guardedCmd := withOfflineGuard(&cobra.Command{
		Use: "cloud-cmd",
		RunE: func(_ *cobra.Command, _ []string) error {
			ran = true
			return nil
		},
	})
	root.AddCommand(guardedCmd)
	root.SetArgs([]string{"cloud-cmd"})

	err := root.Execute()
	require.NoError(t, err)
	assert.True(t, ran)
}

func TestResolveOfflineModel_ExplicitOverride(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "")
	cfg := core.ProviderConfig{OfflineModel: "deepseek-coder:6.7b"}
	model := providers.ResolveOfflineModel("codellama:13b", cfg)
	assert.Equal(t, "codellama:13b", model)
}

func TestResolveOfflineModel_ConfigFallback(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "")
	cfg := core.ProviderConfig{OfflineModel: "deepseek-coder:6.7b"}
	model := providers.ResolveOfflineModel("", cfg)
	assert.Equal(t, "deepseek-coder:6.7b", model)
}

func TestResolveOfflineModel_Default(t *testing.T) {
	t.Setenv("SIPLY_MODEL", "")
	cfg := core.ProviderConfig{}
	model := providers.ResolveOfflineModel("", cfg)
	assert.Equal(t, providers.DefaultOfflineModel, model)
}

func TestOfflineProviderConfig_Defaults(t *testing.T) {
	cfg := core.ProviderConfig{
		Default:      "anthropic",
		OfflineModel: "",
		OfflineURL:   "",
	}
	assert.Empty(t, cfg.OfflineModel)
	assert.Empty(t, cfg.OfflineURL)
}

func TestOfflineProviderConfig_CustomValues(t *testing.T) {
	cfg := core.ProviderConfig{
		Default:      "anthropic",
		OfflineModel: "qwen2.5-coder:7b",
		OfflineURL:   "http://myserver:11434",
	}
	assert.Equal(t, "qwen2.5-coder:7b", cfg.OfflineModel)
	assert.Equal(t, "http://myserver:11434", cfg.OfflineURL)
}

func TestBuildProvider_Ollama(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	os.MkdirAll(home+"/.siply", 0o700)

	provider, err := buildProvider("ollama", nil)
	assert.NotNil(t, provider)
	assert.NoError(t, err)
}
