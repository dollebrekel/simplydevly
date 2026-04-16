// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"fmt"
	"slices"
	"strings"
)

// ValidCategories lists the valid marketplace item categories.
// Declared as var (not const) for testability.
var ValidCategories = []string{"plugins", "extensions", "skills", "configs", "bundles"}

// Search returns all items from idx whose name, description, author, or tags
// contain query (case-insensitive, leading/trailing whitespace ignored).
// If query is empty or whitespace-only, all items are returned as a copy.
// Returns nil if idx is nil.
func Search(idx *Index, query string) []Item {
	if idx == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		// Return a copy so callers cannot mutate the index's backing array.
		return append([]Item(nil), idx.Items...)
	}
	var results []Item
	for _, item := range idx.Items {
		if itemMatchesQuery(item, q) {
			results = append(results, item)
		}
	}
	return results
}

// itemMatchesQuery reports whether item matches the lowercase query string.
func itemMatchesQuery(item Item, q string) bool {
	if strings.Contains(strings.ToLower(item.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Description), q) {
		return true
	}
	if strings.Contains(strings.ToLower(item.Author), q) {
		return true
	}
	for _, tag := range item.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// FilterByCategory returns items in the given category.
// Returns ErrInvalidCategory if category is not in ValidCategories.
// Returns nil items (not an error) if idx is nil.
func FilterByCategory(idx *Index, category string) ([]Item, error) {
	if !isValidCategory(category) {
		return nil, fmt.Errorf("%w: %q — valid categories: %s",
			ErrInvalidCategory, category, strings.Join(ValidCategories, ", "))
	}
	if idx == nil {
		return nil, nil
	}
	var results []Item
	for _, item := range idx.Items {
		if item.Category == category {
			results = append(results, item)
		}
	}
	return results, nil
}

// isValidCategory reports whether the given category is in ValidCategories.
func isValidCategory(category string) bool {
	return slices.Contains(ValidCategories, category)
}

// FindByName returns the item with the given name (exact, case-insensitive).
// Returns ErrItemNotFound if the index is nil or no item matches.
func FindByName(idx *Index, name string) (*Item, error) {
	if idx == nil {
		return nil, ErrItemNotFound
	}
	needle := strings.ToLower(strings.TrimSpace(name))
	for _, item := range idx.Items {
		if strings.ToLower(item.Name) == needle {
			// Return a copy so callers cannot mutate the index.
			copy := item
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrItemNotFound, name)
}
