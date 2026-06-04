// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

import (
	"slices"
	"testing"
)

// wantProviderOrder is the FIXED, curated cloud provider order (NOT alphabetical).
var wantProviderOrder = []string{
	"anthropic", "openai", "google", "xai", "deepseek", "mistral", "kimi", "openrouter",
}

var wantCloudModelCatalog = map[string][]Model{
	"anthropic": {
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8", Category: "krachtigst"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Category: "balans"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Category: "snelst"},
	},
	"openai": {
		{ID: "gpt-5.5", Name: "GPT-5.5", Category: "krachtigst"},
		{ID: "gpt-5.4", Name: "GPT-5.4", Category: "balans"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini", Category: "snelst"},
	},
	"google": {
		{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", Category: "krachtigst"},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Category: "balans"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Category: "goedkoop"},
	},
	"xai": {
		{ID: "grok-4.3", Name: "Grok 4.3", Category: "krachtigst"},
	},
	"deepseek": {
		{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", Category: "redeneren"},
		{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", Category: "goedkoop"},
	},
	"mistral": {
		{ID: "mistral-large-latest", Name: "Mistral Large", Category: "krachtigst"},
		{ID: "codestral-latest", Name: "Codestral", Category: "coding"},
	},
	"kimi": {
		{ID: "kimi-k2.6", Name: "Kimi K2.6", Category: "krachtigst"},
		{ID: "kimi-k2.5", Name: "Kimi K2.5", Category: "balans"},
	},
	"openrouter": {
		{ID: "anthropic/claude-opus-4.8", Name: "Claude Opus 4.8", Category: "gateway"},
		{ID: "openai/gpt-5.5", Name: "GPT-5.5", Category: "gateway"},
		{ID: "google/gemini-3.5-flash", Name: "Gemini 3.5 Flash", Category: "gateway"},
		{ID: "x-ai/grok-4.3", Name: "Grok 4.3", Category: "gateway"},
		{ID: "deepseek/deepseek-v4-pro", Name: "DeepSeek V4 Pro", Category: "gateway"},
		{ID: "mistralai/mistral-large", Name: "Mistral Large", Category: "gateway"},
		{ID: "moonshotai/kimi-k2.6", Name: "Kimi K2.6", Category: "gateway"},
	},
}

func TestCloudProviders_FixedCatalogOrder(t *testing.T) {
	got := CloudProviders()
	if !slices.Equal(got, wantProviderOrder) {
		t.Fatalf("CloudProviders order = %v, want %v", got, wantProviderOrder)
	}

	// Order must be stable across calls (no map iteration underneath).
	if second := CloudProviders(); !slices.Equal(got, second) {
		t.Errorf("provider order not stable: %v vs %v", got, second)
	}

	// Explicitly assert the curated order is NOT alphabetical — that was the bug.
	if slices.IsSorted(got) {
		t.Errorf("providers must be in curated catalog order, not alphabetical: %v", got)
	}
}

func TestCloudModels_CatalogOrderNameAndCategory(t *testing.T) {
	for _, provider := range CloudProviders() {
		want, ok := wantCloudModelCatalog[provider]
		if !ok {
			t.Fatalf("no expected catalog entry for provider %q", provider)
		}
		got := CloudModels(provider)
		if !slices.Equal(got, want) {
			t.Fatalf("%s models = %+v, want %+v (approved catalog order, with name+category)", provider, got, want)
		}
		for _, m := range got {
			if m.ID == "" {
				t.Errorf("provider %q has a model with an empty ID: %+v", provider, m)
			}
			if m.Name == "" {
				t.Errorf("provider %q model %q has no display name", provider, m.ID)
			}
			if m.Category == "" {
				t.Errorf("provider %q model %q has no category", provider, m.ID)
			}
		}
	}
	if len(wantCloudModelCatalog) != len(wantProviderOrder) {
		t.Fatalf("expected catalog table has %d providers, want %d", len(wantCloudModelCatalog), len(wantProviderOrder))
	}
}

func TestCloudModels_ReturnedSliceIsIsolated(t *testing.T) {
	models := CloudModels("anthropic")
	if len(models) == 0 {
		t.Fatal("expected curated anthropic models")
	}

	// Mutating the returned slice must not corrupt the underlying catalog.
	models[0] = Model{ID: "MUTATED"}
	if again := CloudModels("anthropic"); again[0].ID == "MUTATED" {
		t.Error("CloudModels returned a slice aliased to the internal catalog")
	}
}

func TestCloudModels_UnknownAndOllamaReturnNil(t *testing.T) {
	if CloudModels("does-not-exist") != nil {
		t.Error("expected nil for an unknown provider")
	}
	// Ollama is dynamic (via /api/tags) and must NOT be part of the static catalog.
	if CloudModels("ollama") != nil {
		t.Error("ollama must not appear in the curated cloud catalog")
	}
}
