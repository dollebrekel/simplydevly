// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package marketplace

import "fmt"

// FormatInstalls formats an install count with K/M suffixes.
// Zero is shown as "0" (a valid value for a newly listed item).
func FormatInstalls(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// FormatRating formats a rating as "⭐ N.N" or "—" if zero.
func FormatRating(r float64) string {
	if r == 0 {
		return "—"
	}
	return fmt.Sprintf("⭐ %.1f", r)
}

// FormatRatingWithCount formats a rating with count as "⭐ N.N (M)" or "—" if zero.
func FormatRatingWithCount(r float64, count int) string {
	if r == 0 {
		return "—"
	}
	return fmt.Sprintf("⭐ %.1f (%d)", r, count)
}

// FormatReviewCount formats a review count as "N reviews", "1 review", or "no reviews".
func FormatReviewCount(n int) string {
	switch {
	case n == 0:
		return "no reviews"
	case n == 1:
		return "1 review"
	default:
		return fmt.Sprintf("%d reviews", n)
	}
}

// FormatVerified returns "✓" for verified items, empty string otherwise.
func FormatVerified(v bool) string {
	if v {
		return "✓"
	}
	return ""
}
