// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

// Model is one curated cloud model: its real API ID, a human-readable display
// name, and a short capability category (e.g. "krachtigst", "balans", "snelst").
// The picker surfaces Name + Category so users never see a raw, cryptic API ID.
type Model struct {
	ID       string
	Name     string
	Category string
}

// providerModels groups one provider's curated models in catalog (display)
// order — the order is meaningful and is preserved verbatim by CloudModels.
type providerModels struct {
	Provider string
	Models   []Model
}

// cloudModelCatalog is the curated, manually-maintained catalog of known cloud
// models, grouped per provider in a FIXED, curated order (deliberately NOT
// alphabetical): Anthropic → OpenAI → Google → xAI → DeepSeek → Mistral → Kimi →
// OpenRouter. Within each provider, models are listed in intended display order.
//
// This is intentionally a STATIC catalog rather than live provider-API discovery
// (see story 12.13 "Out of Scope"): every known model is shown — even for
// providers without a configured API key — so users can browse and test the full
// set from /model and Settings. Keep this list current by hand when providers
// ship new models, using the verified IDs from the approved model table.
//
// IDs are the direct provider-API identifiers (Anthropic/OpenAI/etc. use
// hyphenated or dotted IDs as the provider documents them). OpenRouter is a
// gateway: its IDs are provider-prefixed routing slugs (e.g.
// anthropic/claude-opus-4.8) and differ from the direct-API IDs.
//
// Ollama (local) models are discovered dynamically via GET /api/tags and are
// deliberately absent here.
var cloudModelCatalog = []providerModels{
	{Provider: "anthropic", Models: []Model{
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8", Category: "krachtigst"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Category: "balans"},
		{ID: "claude-haiku-4-5", Name: "Claude Haiku 4.5", Category: "snelst"},
	}},
	{Provider: "openai", Models: []Model{
		{ID: "gpt-5.5", Name: "GPT-5.5", Category: "krachtigst"},
		{ID: "gpt-5.4", Name: "GPT-5.4", Category: "balans"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini", Category: "snelst"},
	}},
	{Provider: "google", Models: []Model{
		{ID: "gemini-3.5-flash", Name: "Gemini 3.5 Flash", Category: "krachtigst"},
		{ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", Category: "balans"},
		{ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", Category: "goedkoop"},
	}},
	{Provider: "xai", Models: []Model{
		{ID: "grok-4.3", Name: "Grok 4.3", Category: "krachtigst"},
	}},
	{Provider: "deepseek", Models: []Model{
		{ID: "deepseek-v4-pro", Name: "DeepSeek V4 Pro", Category: "redeneren"},
		{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", Category: "goedkoop"},
	}},
	{Provider: "mistral", Models: []Model{
		{ID: "mistral-large-latest", Name: "Mistral Large", Category: "krachtigst"},
		{ID: "codestral-latest", Name: "Codestral", Category: "coding"},
	}},
	{Provider: "kimi", Models: []Model{
		{ID: "kimi-k2.6", Name: "Kimi K2.6", Category: "krachtigst"},
		{ID: "kimi-k2.5", Name: "Kimi K2.5", Category: "balans"},
	}},
	// OpenRouter mirrors each upstream provider's flagship as a gateway routing
	// slug. Slugs are dotted and provider-prefixed (distinct from direct-API IDs).
	{Provider: "openrouter", Models: []Model{
		{ID: "anthropic/claude-opus-4.8", Name: "Claude Opus 4.8", Category: "gateway"},
		{ID: "openai/gpt-5.5", Name: "GPT-5.5", Category: "gateway"},
		{ID: "google/gemini-3.5-flash", Name: "Gemini 3.5 Flash", Category: "gateway"},
		{ID: "x-ai/grok-4.3", Name: "Grok 4.3", Category: "gateway"},
		{ID: "deepseek/deepseek-v4-pro", Name: "DeepSeek V4 Pro", Category: "gateway"},
		{ID: "mistralai/mistral-large", Name: "Mistral Large", Category: "gateway"},
		{ID: "moonshotai/kimi-k2.6", Name: "Kimi K2.6", Category: "gateway"},
	}},
}

// CloudProviders returns the curated cloud provider names in the FIXED catalog
// order (NOT alphabetical). Ollama is excluded — local models are discovered
// dynamically, not curated here. Callers must use this helper for stable
// grouping/rendering; the order is the curated catalog order, never sorted.
func CloudProviders() []string {
	names := make([]string, 0, len(cloudModelCatalog))
	for _, pm := range cloudModelCatalog {
		names = append(names, pm.Provider)
	}
	return names
}

// CloudModels returns the curated known models for the given provider in catalog
// (display) order — never alphabetically sorted. It returns a fresh copy (callers
// may sort/append freely without corrupting the catalog) and nil for an unknown
// provider.
func CloudModels(provider string) []Model {
	for _, pm := range cloudModelCatalog {
		if pm.Provider == provider {
			out := make([]Model, len(pm.Models))
			copy(out, pm.Models)
			return out
		}
	}
	return nil
}
