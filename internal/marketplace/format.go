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

// FormatVerified returns "✓" for verified items, empty string otherwise.
func FormatVerified(v bool) string {
	if v {
		return "✓"
	}
	return ""
}
