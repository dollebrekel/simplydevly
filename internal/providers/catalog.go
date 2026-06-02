// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

import "sort"

// cloudModelCatalog is the curated, manually-maintained list of known cloud
// models per provider.
//
// This is intentionally a STATIC catalog rather than live provider-API
// discovery (see story 12.13 "Out of Scope"): every known model is shown — even
// for providers without a configured API key — so users can browse and test the
// full set from /model and Settings. Keep this list current by hand when
// providers ship new models.
//
// Ollama (local) models are discovered dynamically via GET /api/tags and are
// deliberately absent here.
var cloudModelCatalog = map[string][]string{
	"anthropic": {
		"claude-opus-4-8",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
	},
	"openai": {
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-5",
	},
	"openrouter": {
		"anthropic/claude-sonnet-4-6",
		"openai/gpt-4o",
		"x-ai/grok-2",
	},
	"kimi": {
		"kimi-k2",
		"kimi-k2-thinking",
		"moonshot-v1-128k",
	},
}

// CloudProviders returns the curated cloud provider names in deterministic
// (alphabetical) order. Ollama is excluded — local models are discovered
// dynamically, not curated here. Iterating the catalog map directly would yield
// a random order each run (see pattern: deterministic-map-iteration), so callers
// must use this helper for stable grouping/rendering.
func CloudProviders() []string {
	names := make([]string, 0, len(cloudModelCatalog))
	for name := range cloudModelCatalog {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CloudModels returns the curated known models for the given provider, sorted
// alphabetically. It returns a fresh copy (callers may sort/append freely) and
// nil for an unknown provider.
func CloudModels(provider string) []string {
	models, ok := cloudModelCatalog[provider]
	if !ok {
		return nil
	}
	out := make([]string, len(models))
	copy(out, models)
	sort.Strings(out)
	return out
}
