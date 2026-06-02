// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/app"
	"siply.dev/siply/internal/config"
)

// resolveStartupSelection mirrors the provider-then-model resolution that
// app.Start performs, so we can exercise the full cold-start path without
// booting providers/credentials.
func resolveStartupSelection(t *testing.T, globalDir, projectDir string) (string, app.ModelSelection) {
	t.Helper()
	t.Setenv("SIPLY_MODEL", "")
	t.Setenv("SIPLY_PROVIDER", "")

	l := config.NewLoader(config.LoaderOptions{GlobalDir: globalDir, ProjectDir: projectDir})
	require.NoError(t, l.Init(context.Background()))
	providerCfg := l.Config().Provider

	providerName := app.ResolveProviderName(app.ProviderSelectionInput{
		ConfigProvider: providerCfg.Default,
	})
	preferLocal := strings.EqualFold(strings.TrimSpace(providerName), "ollama")
	model := app.ResolveModelSelection(app.ModelSelectionInput{
		PreferLocal:  preferLocal,
		MergedConfig: providerCfg,
	})
	return providerName, model
}

// TestColdStart_LocalSelectionSurvivesGlobalCloudModel is the regression for
// the "local jumps to cloud model" bug: after picking a local model (project
// config has default=ollama + local_model, no model), a leaked global cloud
// provider.model must NOT win on the next cold start.
func TestColdStart_LocalSelectionSurvivesGlobalCloudModel(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	// Global: cloud default + cloud model (as in the real ~/.siply/config.yaml).
	writeYAML(t, filepath.Join(globalDir, "config.yaml"), `
provider:
  default: kimi
  model: kimi-k2.6
  local_model: qwen3.6:27b-q8_0
  local_url: http://localhost:11434
`)
	// Project after a LOCAL /model pick: default=ollama + local_model, no model.
	writeYAML(t, filepath.Join(projectDir, "config.yaml"), `
provider:
  default: ollama
  local_model: qwen3.6:27b-q8_0
`)

	providerName, model := resolveStartupSelection(t, globalDir, projectDir)

	assert.Equal(t, "ollama", providerName)
	assert.Equal(t, "qwen3.6:27b-q8_0", model.Model, "local choice must survive, not the leaked cloud model")
	assert.Equal(t, app.ModelSourceLocalPreference, model.Source)
}

// TestColdStart_CloudSelectionUnchanged confirms the cloud path is untouched.
func TestColdStart_CloudSelectionUnchanged(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()

	writeYAML(t, filepath.Join(globalDir, "config.yaml"), `
provider:
  default: anthropic
  model: claude-opus
`)
	writeYAML(t, filepath.Join(projectDir, "config.yaml"), `
provider:
  default: openai
  model: gpt-4o
`)

	providerName, model := resolveStartupSelection(t, globalDir, projectDir)

	assert.Equal(t, "openai", providerName)
	assert.Equal(t, "gpt-4o", model.Model)
	assert.Equal(t, app.ModelSourceConfig, model.Source)
}
