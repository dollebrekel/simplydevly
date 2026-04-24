// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"log/slog"

	"siply.dev/siply/internal/core"
)

var validCategories = map[string]bool{
	string(CategoryPreprocess): true,
	string(CategoryPrimary):    true,
	string(CategoryReview):     true,
	string(CategoryChat):       true,
	string(CategoryVision):     true,
}

// ValidateConfig checks the routing configuration for common issues.
// Returns a slice of warning messages and also logs them via slog.Warn.
func ValidateConfig(cfg core.RoutingConfig, providerCount int, providerNames []string) []string {
	var warnings []string
	enabled := cfg.Enabled != nil && *cfg.Enabled
	if !enabled {
		return warnings
	}

	if providerCount <= 1 && len(cfg.Rules) > 0 {
		w := "routing enabled with rules but only 1 provider configured — routing will be bypassed"
		warnings = append(warnings, w)
		slog.Warn(w)
	}

	if cfg.PreferCheapest != nil && *cfg.PreferCheapest && len(cfg.Pricing) == 0 {
		w := "routing prefer_cheapest enabled but no pricing configured — cost-based routing unavailable"
		warnings = append(warnings, w)
		slog.Warn(w)
	}

	knownProviders := make(map[string]bool, len(providerNames))
	for _, name := range providerNames {
		knownProviders[name] = true
	}

	seen := make(map[string]bool)
	for _, rule := range cfg.Rules {
		if seen[rule.Category] {
			w := "duplicate routing rule for category: " + rule.Category
			warnings = append(warnings, w)
			slog.Warn("duplicate routing rule for category", "category", rule.Category)
		}
		seen[rule.Category] = true

		if !validCategories[rule.Category] {
			w := "routing rule references unknown category: " + rule.Category
			warnings = append(warnings, w)
			slog.Warn("routing rule references unknown category", "category", rule.Category)
		}

		if len(providerNames) > 0 {
			if !knownProviders[rule.Provider] {
				w := "routing rule references unknown provider: " + rule.Provider
				warnings = append(warnings, w)
				slog.Warn("routing rule references unknown provider", "provider", rule.Provider, "category", rule.Category)
			}
		} else {
			slog.Debug("provider names not supplied, skipping provider validation", "category", rule.Category)
		}
	}

	return warnings
}
