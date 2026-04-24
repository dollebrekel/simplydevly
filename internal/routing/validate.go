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

// ValidateConfig checks the routing configuration for common issues and logs
// warnings. It does not return errors — misconfiguration degrades gracefully.
func ValidateConfig(cfg core.RoutingConfig, providerCount int, providerNames []string) {
	enabled := cfg.Enabled != nil && *cfg.Enabled
	if !enabled {
		return
	}

	if providerCount <= 1 && len(cfg.Rules) > 0 {
		slog.Warn("routing enabled with rules but only 1 provider configured — routing will be bypassed")
	}

	if cfg.PreferCheapest != nil && *cfg.PreferCheapest && len(cfg.Pricing) == 0 {
		slog.Warn("routing prefer_cheapest enabled but no pricing configured — cost-based routing unavailable")
	}

	knownProviders := make(map[string]bool, len(providerNames))
	for _, name := range providerNames {
		knownProviders[name] = true
	}

	seen := make(map[string]bool)
	for _, rule := range cfg.Rules {
		if seen[rule.Category] {
			slog.Warn("duplicate routing rule for category", "category", rule.Category)
		}
		seen[rule.Category] = true

		if !validCategories[rule.Category] {
			slog.Warn("routing rule references unknown category", "category", rule.Category)
		}

		if len(providerNames) > 0 && !knownProviders[rule.Provider] {
			slog.Warn("routing rule references unknown provider", "provider", rule.Provider, "category", rule.Category)
		}
	}
}
