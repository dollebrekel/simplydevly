// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package menu

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultKeyBindings_Returns5Categories(t *testing.T) {
	cats := DefaultKeyBindings()
	require.Len(t, cats, 5)
}

func TestDefaultKeyBindings_CategoryNames(t *testing.T) {
	cats := DefaultKeyBindings()
	expected := []string{"Navigation", "AI Agent", "Extensions", "Git", "Terminal"}
	for i, name := range expected {
		assert.Equal(t, name, cats[i].Name, "category %d name", i)
	}
}

func TestDefaultKeyBindings_EachCategoryHasBindings(t *testing.T) {
	cats := DefaultKeyBindings()
	for _, cat := range cats {
		assert.NotEmpty(t, cat.Bindings, "category %q should have at least 1 binding", cat.Name)
	}
}

func TestDefaultKeyBindings_BindingFieldsPopulated(t *testing.T) {
	cats := DefaultKeyBindings()
	for _, cat := range cats {
		for _, kb := range cat.Bindings {
			assert.NotEmpty(t, kb.Key, "binding in %q should have Key", cat.Name)
			assert.NotEmpty(t, kb.Action, "binding in %q should have Action", cat.Name)
			assert.Equal(t, cat.Name, kb.Category, "binding Category should match parent")
		}
	}
}

func TestDefaultKeyBindings_NavigationHas5Bindings(t *testing.T) {
	cats := DefaultKeyBindings()
	assert.Len(t, cats[0].Bindings, 5)
}

func TestDefaultKeyBindings_AIAgentHas3Bindings(t *testing.T) {
	cats := DefaultKeyBindings()
	assert.Len(t, cats[1].Bindings, 3)
}

func TestDefaultKeyBindings_ExtensionsHas2Bindings(t *testing.T) {
	cats := DefaultKeyBindings()
	assert.Len(t, cats[2].Bindings, 2)
}

func TestDefaultKeyBindings_TerminalHas4Bindings(t *testing.T) {
	cats := DefaultKeyBindings()
	assert.Len(t, cats[4].Bindings, 4)
}
