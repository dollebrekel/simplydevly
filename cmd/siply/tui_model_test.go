// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	appstartup "siply.dev/siply/internal/app"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/tui"
)

// keyedProviders builds a hasKey predicate that reports true only for the named
// providers.
func keyedProviders(names ...string) func(string) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(p string) bool { return set[p] }
}

func TestCloudModelOptions_PerProviderActiveAndDisabled(t *testing.T) {
	opts := cloudModelOptions("anthropic", "claude-sonnet-4-6", keyedProviders("anthropic"))

	if len(opts) == 0 {
		t.Fatal("expected cloud options across providers")
	}

	// All entries must be cloud, and each provider's models must be contiguous
	// (grouped) — providers appear in the fixed curated catalog order, not
	// alphabetical (the dedicated catalog test asserts the exact order).
	var lastProvider string
	seen := map[string]bool{}
	var anthropicActiveFound, openaiDisabledFound bool
	for _, o := range opts {
		if o.Kind != "cloud" {
			t.Errorf("expected cloud kind, got %q", o.Kind)
		}
		if o.Provider != lastProvider {
			if seen[o.Provider] {
				t.Errorf("provider %q is not contiguous — options not grouped/sorted", o.Provider)
			}
			seen[o.Provider] = true
			lastProvider = o.Provider
		}
		switch o.Provider {
		case "anthropic":
			if o.Disabled {
				t.Errorf("anthropic has a key — must not be disabled: %+v", o)
			}
			if o.Model == "claude-sonnet-4-6" {
				if !o.Active {
					t.Error("expected claude-sonnet-4-6 to be marked Active")
				}
				anthropicActiveFound = true
			}
		case "openai":
			if !o.Disabled {
				t.Errorf("openai has no key — must be disabled: %+v", o)
			}
			if o.Description != noAPIKeyHint {
				t.Errorf("expected %q hint for keyless provider, got %q", noAPIKeyHint, o.Description)
			}
			openaiDisabledFound = true
		}
	}

	if !anthropicActiveFound {
		t.Error("active anthropic model not found in options")
	}
	if !openaiDisabledFound {
		t.Error("expected keyless openai entries to be present (shown but disabled)")
	}
}

func TestCloudModelOptions_UnsupportedProviderDisabledEvenWithKey(t *testing.T) {
	opts := cloudModelOptions("", "", keyedProviders("google"))

	var found bool
	for _, o := range opts {
		if o.Provider != "google" {
			continue
		}
		found = true
		if !o.Disabled {
			t.Errorf("google has no runtime adapter and must be disabled: %+v", o)
		}
		if o.Description != unsupportedProviderHint {
			t.Errorf("expected %q hint for unsupported provider, got %q", unsupportedProviderHint, o.Description)
		}
	}
	if !found {
		t.Fatal("expected google catalog entries to remain visible")
	}
}

func TestCloudModelOptions_ActiveModelInjectedWhenMissing(t *testing.T) {
	// A date-suffixed variant that is not part of the curated catalog.
	const custom = "claude-sonnet-4-6-20250514"
	opts := cloudModelOptions("anthropic", custom, keyedProviders("anthropic"))

	var found bool
	for _, o := range opts {
		if o.Provider == "anthropic" && o.Model == custom {
			found = true
			if !o.Active {
				t.Error("injected active model should be marked Active")
			}
		}
	}
	if !found {
		t.Errorf("active model %q not injected into anthropic group", custom)
	}
}

func TestCloudModelOptions_NilHasKeyEnablesSupportedProvidersOnly(t *testing.T) {
	opts := cloudModelOptions("", "", nil)
	var anthropicFound, googleFound bool
	for _, o := range opts {
		switch o.Provider {
		case "anthropic":
			anthropicFound = true
			if o.Disabled {
				t.Errorf("nil hasKey must treat supported providers as usable, got disabled: %+v", o)
			}
		case "google":
			googleFound = true
			if !o.Disabled {
				t.Errorf("unsupported provider must remain disabled even with nil hasKey: %+v", o)
			}
		}
	}
	if !anthropicFound || !googleFound {
		t.Fatalf("expected anthropic and google options, got anthropic=%v google=%v", anthropicFound, googleFound)
	}
}

func TestActiveTUISelectionUsesLiveStartupModel(t *testing.T) {
	cfg := core.ProviderConfig{Default: "anthropic", Model: "claude-sonnet-4-6"}
	startup := &appstartup.Startup{
		ProviderName: "openai",
		Model:        appstartup.ModelSelection{Model: "gpt-4o"},
	}

	provider, model := activeTUISelection(tui.CLIFlags{}, startup, cfg)
	if provider != "openai" || model != "gpt-4o" {
		t.Fatalf("active selection = %s/%s, want openai/gpt-4o", provider, model)
	}
}

func TestResolveSelectionConfigPath(t *testing.T) {
	// AC2: a project config dir wins (no regression).
	if got := resolveSelectionConfigPath("/proj/.siply", "/home/u"); got != filepath.Join("/proj/.siply", "config.yaml") {
		t.Errorf("project path not preferred, got %q", got)
	}
	// AC1: no project dir → global fallback under home.
	if got := resolveSelectionConfigPath("", "/home/u"); got != filepath.Join("/home/u", ".siply", "config.yaml") {
		t.Errorf("global fallback path wrong, got %q", got)
	}
	// Neither available → empty (caller skips persistence).
	if got := resolveSelectionConfigPath("", ""); got != "" {
		t.Errorf("expected empty path when nothing resolvable, got %q", got)
	}
}

func TestPersistSelection_GlobalFallbackWritesHomeConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No Startup → no workspace/project config dir → must fall back to global.
	c := newTUIModelController(tuiModelControllerOptions{})
	if err := c.persistSelection(tui.ModelOption{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8"}); err != nil {
		t.Fatalf("persistSelection: %v", err)
	}

	path := filepath.Join(home, ".siply", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected global config written at %q: %v", path, err)
	}
	var doc struct {
		Provider struct {
			Default string `yaml:"default"`
			Model   string `yaml:"model"`
		} `yaml:"provider"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse global config: %v", err)
	}
	if doc.Provider.Default != "anthropic" || doc.Provider.Model != "claude-opus-4-8" {
		t.Errorf("unexpected persisted config: default=%q model=%q", doc.Provider.Default, doc.Provider.Model)
	}
}

func TestPersistSelection_LocalGlobalFallbackWritesLocalModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := newTUIModelController(tuiModelControllerOptions{})
	if err := c.persistSelection(tui.ModelOption{Kind: "local", Provider: "ollama", Model: "qwen3:32b"}); err != nil {
		t.Fatalf("persistSelection: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".siply", "config.yaml"))
	if err != nil {
		t.Fatalf("expected global config written: %v", err)
	}
	var doc struct {
		Provider struct {
			Default    string `yaml:"default"`
			LocalModel string `yaml:"local_model"`
		} `yaml:"provider"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse global config: %v", err)
	}
	if doc.Provider.Default != "ollama" || doc.Provider.LocalModel != "qwen3:32b" {
		t.Errorf("unexpected local config: default=%q local_model=%q", doc.Provider.Default, doc.Provider.LocalModel)
	}
}

func TestPersistSelection_LocalClearsStaleCloudModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := newTUIModelController(tuiModelControllerOptions{})
	if err := c.persistSelection(tui.ModelOption{Kind: "cloud", Provider: "openai", Model: "gpt-4o"}); err != nil {
		t.Fatalf("persist cloud selection: %v", err)
	}
	if err := c.persistSelection(tui.ModelOption{Kind: "local", Provider: "ollama", Model: "qwen3:32b"}); err != nil {
		t.Fatalf("persist local selection: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".siply", "config.yaml"))
	if err != nil {
		t.Fatalf("expected global config written: %v", err)
	}
	var doc struct {
		Provider struct {
			Default    string `yaml:"default"`
			Model      string `yaml:"model"`
			LocalModel string `yaml:"local_model"`
		} `yaml:"provider"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse global config: %v", err)
	}
	if doc.Provider.Default != "ollama" || doc.Provider.LocalModel != "qwen3:32b" || doc.Provider.Model != "" {
		t.Errorf("unexpected config after local selection: default=%q model=%q local_model=%q", doc.Provider.Default, doc.Provider.Model, doc.Provider.LocalModel)
	}
}

func TestPersistSelection_CloudClearsStaleLocalModel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := newTUIModelController(tuiModelControllerOptions{})
	if err := c.persistSelection(tui.ModelOption{Kind: "local", Provider: "ollama", Model: "qwen3:32b"}); err != nil {
		t.Fatalf("persist local selection: %v", err)
	}
	if err := c.persistSelection(tui.ModelOption{Kind: "cloud", Provider: "anthropic", Model: "claude-opus-4-8"}); err != nil {
		t.Fatalf("persist cloud selection: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".siply", "config.yaml"))
	if err != nil {
		t.Fatalf("expected global config written: %v", err)
	}
	var doc struct {
		Provider struct {
			Default    string `yaml:"default"`
			Model      string `yaml:"model"`
			LocalModel string `yaml:"local_model"`
		} `yaml:"provider"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse global config: %v", err)
	}
	if doc.Provider.Default != "anthropic" || doc.Provider.Model != "claude-opus-4-8" || doc.Provider.LocalModel != "" {
		t.Errorf("unexpected config after cloud selection: default=%q model=%q local_model=%q", doc.Provider.Default, doc.Provider.Model, doc.Provider.LocalModel)
	}
}
