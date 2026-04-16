// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadFixture is a test helper that loads the standard fixture index.
func loadFixture(t *testing.T) *Index {
	t.Helper()
	idx, err := LoadIndex(filepath.Join("testdata", "marketplace-index.json"))
	require.NoError(t, err)
	return idx
}

func TestSearch(t *testing.T) {
	idx := loadFixture(t)

	tests := []struct {
		name      string
		query     string
		wantNames []string // expected item names in results (subset check)
		wantMin   int      // minimum expected result count
		wantMax   int      // maximum expected result count (-1 = no upper bound)
	}{
		{
			name:    "empty query returns all items",
			query:   "",
			wantMin: 12,
			wantMax: -1,
		},
		{
			name:      "exact name match",
			query:     "memory-default",
			wantNames: []string{"memory-default"},
			wantMin:   1,
			wantMax:   -1,
		},
		{
			name:      "partial name match",
			query:     "memory",
			wantNames: []string{"memory-default"},
			wantMin:   1,
			wantMax:   -1,
		},
		{
			name:      "case-insensitive name match",
			query:     "MEMORY",
			wantNames: []string{"memory-default"},
			wantMin:   1,
			wantMax:   -1,
		},
		{
			name:      "matches on description",
			query:     "persistent context storage",
			wantNames: []string{"memory-default"},
			wantMin:   1,
			wantMax:   -1,
		},
		{
			name:    "matches on author",
			query:   "simplydevly",
			wantMin: 5, // multiple items by simplydevly
			wantMax: -1,
		},
		{
			name:      "matches on tag",
			query:     "docker",
			wantNames: []string{"docker-plugin"},
			wantMin:   1,
			wantMax:   -1,
		},
		{
			name:      "case-insensitive tag match",
			query:     "DOCKER",
			wantNames: []string{"docker-plugin"},
			wantMin:   1,
			wantMax:   -1,
		},
		{
			name:    "no match returns empty slice",
			query:   "zzznomatch999",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:      "matches bundle items",
			query:     "bundle",
			wantNames: []string{"fullstack-starter", "minimal-bundle"},
			wantMin:   2,
			wantMax:   -1,
		},
		{
			name:      "matches on partial description word",
			query:     "security",
			wantNames: []string{"code-review-skill"},
			wantMin:   1,
			wantMax:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Search(idx, tt.query)
			assert.GreaterOrEqual(t, len(got), tt.wantMin,
				"expected at least %d results for query %q, got %d", tt.wantMin, tt.query, len(got))
			if tt.wantMax >= 0 {
				assert.LessOrEqual(t, len(got), tt.wantMax,
					"expected at most %d results for query %q, got %d", tt.wantMax, tt.query, len(got))
			}

			// Verify expected item names are present.
			if len(tt.wantNames) > 0 {
				resultNames := make(map[string]bool, len(got))
				for _, item := range got {
					resultNames[item.Name] = true
				}
				for _, wantName := range tt.wantNames {
					assert.True(t, resultNames[wantName],
						"expected item %q in results for query %q", wantName, tt.query)
				}
			}
		})
	}
}

func TestSearch_NilIndex(t *testing.T) {
	result := Search(nil, "anything")
	assert.Nil(t, result)
}

func TestFilterByCategory(t *testing.T) {
	idx := loadFixture(t)

	tests := []struct {
		name     string
		category string
		wantErr  bool
		errIs    error
		wantMin  int
		wantAll  bool // all results must have this category
	}{
		{
			name:     "plugins category",
			category: "plugins",
			wantMin:  4,
			wantAll:  true,
		},
		{
			name:     "extensions category",
			category: "extensions",
			wantMin:  2,
			wantAll:  true,
		},
		{
			name:     "skills category",
			category: "skills",
			wantMin:  2,
			wantAll:  true,
		},
		{
			name:     "configs category",
			category: "configs",
			wantMin:  2,
			wantAll:  true,
		},
		{
			name:     "bundles category",
			category: "bundles",
			wantMin:  2,
			wantAll:  true,
		},
		{
			name:     "invalid category returns ErrInvalidCategory",
			category: "themes",
			wantErr:  true,
			errIs:    ErrInvalidCategory,
		},
		{
			name:     "empty string is invalid category",
			category: "",
			wantErr:  true,
			errIs:    ErrInvalidCategory,
		},
		{
			name:     "mixed-case invalid category",
			category: "Plugins",
			wantErr:  true,
			errIs:    ErrInvalidCategory,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterByCategory(idx, tt.category)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errIs != nil {
					assert.True(t, errors.Is(err, tt.errIs),
						"expected errors.Is(%v, %v)", err, tt.errIs)
				}
				return
			}
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(got), tt.wantMin)
			if tt.wantAll {
				for _, item := range got {
					assert.Equal(t, tt.category, item.Category,
						"item %q has category %q, expected %q", item.Name, item.Category, tt.category)
				}
			}
		})
	}
}

func TestFilterByCategory_NilIndex(t *testing.T) {
	// Nil index with valid category should return nil, no error.
	got, err := FilterByCategory(nil, "plugins")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFilterByCategory_ErrorMessageContainsValidCategories(t *testing.T) {
	_, err := FilterByCategory(loadFixture(t), "invalid")
	require.Error(t, err)
	errMsg := err.Error()
	for _, c := range ValidCategories {
		assert.Contains(t, errMsg, c,
			"error message should list valid category %q", c)
	}
}

func TestValidCategories_AllFivePresent(t *testing.T) {
	expected := []string{"plugins", "extensions", "skills", "configs", "bundles"}
	assert.Equal(t, expected, ValidCategories)
}

func TestFindByName_Found(t *testing.T) {
	idx := loadFixture(t)

	tests := []struct {
		name     string
		query    string
		wantName string
	}{
		{
			name:     "exact match",
			query:    "memory-default",
			wantName: "memory-default",
		},
		{
			name:     "case-insensitive match uppercase",
			query:    "MEMORY-DEFAULT",
			wantName: "memory-default",
		},
		{
			name:     "case-insensitive match mixed",
			query:    "Memory-Default",
			wantName: "memory-default",
		},
		{
			name:     "exact match prompt-basic",
			query:    "prompt-basic",
			wantName: "prompt-basic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, err := FindByName(idx, tt.query)
			require.NoError(t, err)
			require.NotNil(t, item)
			assert.Equal(t, tt.wantName, item.Name)
		})
	}
}

func TestFindByName_NotFound(t *testing.T) {
	idx := loadFixture(t)

	item, err := FindByName(idx, "nonexistent-item-xyz")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrItemNotFound),
		"expected ErrItemNotFound, got: %v", err)
	assert.Nil(t, item)
}

func TestFindByName_NilIndex(t *testing.T) {
	item, err := FindByName(nil, "memory-default")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrItemNotFound),
		"expected ErrItemNotFound for nil index, got: %v", err)
	assert.Nil(t, item)
}

func TestFindByName_ReturnsCopy(t *testing.T) {
	idx := loadFixture(t)

	item, err := FindByName(idx, "memory-default")
	require.NoError(t, err)
	require.NotNil(t, item)

	// Mutate the returned item — should not affect the index.
	originalName := item.Name
	item.Name = "mutated"

	item2, err := FindByName(idx, originalName)
	require.NoError(t, err)
	assert.Equal(t, originalName, item2.Name, "FindByName must return a copy, not a pointer to index data")
}
