// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import (
	"testing"
)

func TestFormatRatingWithCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rating float64
		count  int
		want   string
	}{
		{0, 0, "—"},
		{4.5, 10, "⭐ 4.5 (10)"},
		{5.0, 1, "⭐ 5.0 (1)"},
		{3.2, 999, "⭐ 3.2 (999)"},
	}

	for _, tc := range tests {
		got := FormatRatingWithCount(tc.rating, tc.count)
		if got != tc.want {
			t.Errorf("FormatRatingWithCount(%f, %d) = %q, want %q", tc.rating, tc.count, got, tc.want)
		}
	}
}

func TestFormatReviewCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int
		want string
	}{
		{0, "no reviews"},
		{1, "1 review"},
		{2, "2 reviews"},
		{42, "42 reviews"},
	}

	for _, tc := range tests {
		got := FormatReviewCount(tc.n)
		if got != tc.want {
			t.Errorf("FormatReviewCount(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
