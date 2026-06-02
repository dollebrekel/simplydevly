// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package providers

import (
	"slices"
	"sort"
	"testing"
)

func TestCloudProviders_Deterministic(t *testing.T) {
	first := CloudProviders()
	if len(first) == 0 {
		t.Fatal("expected at least one curated cloud provider")
	}
	if !sort.StringsAreSorted(first) {
		t.Errorf("CloudProviders not sorted deterministically: %v", first)
	}

	second := CloudProviders()
	if len(first) != len(second) {
		t.Fatalf("provider count not stable: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("provider order not stable at %d: %q vs %q", i, first[i], second[i])
		}
	}

	if !slices.Contains(first, "anthropic") {
		t.Errorf("expected anthropic in curated providers, got %v", first)
	}
}

func TestCloudModels_SortedIsolatedAndUnknown(t *testing.T) {
	models := CloudModels("anthropic")
	if len(models) == 0 {
		t.Fatal("expected curated anthropic models")
	}
	if !sort.StringsAreSorted(models) {
		t.Errorf("anthropic models not sorted: %v", models)
	}

	// Mutating the returned slice must not corrupt the underlying catalog.
	models[0] = "MUTATED"
	if again := CloudModels("anthropic"); again[0] == "MUTATED" {
		t.Error("CloudModels returned a slice aliased to the internal catalog")
	}

	if CloudModels("does-not-exist") != nil {
		t.Error("expected nil for an unknown provider")
	}
	// Ollama is dynamic (via /api/tags) and must NOT be part of the static catalog.
	if CloudModels("ollama") != nil {
		t.Error("ollama must not appear in the curated cloud catalog")
	}
}
