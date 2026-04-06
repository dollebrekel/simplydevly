// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

// ConfigPolicy implements RoutingPolicy using static routing rules from RoutingConfig.
type ConfigPolicy struct {
	config RoutingConfig
}

// NewConfigPolicy creates a ConfigPolicy from the given configuration.
// cfg.DefaultProvider must be non-empty; if empty, Select will return empty
// provider names on fallback, causing routing failures downstream.
func NewConfigPolicy(cfg RoutingConfig) *ConfigPolicy {
	return &ConfigPolicy{config: cfg}
}

// Select picks a provider based on the task.category hint matched against rules.
// If no rule matches, the default provider is returned.
func (p *ConfigPolicy) Select(hints map[string]string) ProviderSelection {
	category := hints[HintKeyCategory]

	for _, rule := range p.config.Rules {
		if string(rule.Category) == category {
			return ProviderSelection{
				Provider: rule.Provider,
				Model:    rule.Model,
				Reason:   "matched rule for category " + category,
			}
		}
	}

	return ProviderSelection{
		Provider: p.config.DefaultProvider,
		Reason:   "no rule matched, using default provider",
	}
}
