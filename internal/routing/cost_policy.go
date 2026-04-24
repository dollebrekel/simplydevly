// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package routing

import (
	"fmt"
	"math"
	"sort"

	"siply.dev/siply/internal/core"
)

// HintKeyRequiresTools signals that the query needs tool-call support.
const HintKeyRequiresTools = "task.requires_tools"

// HintKeyRequiresVision signals that the query needs vision support.
const HintKeyRequiresVision = "task.requires_vision"

// HintKeyMinContext signals the minimum context window required.
const HintKeyMinContext = "task.min_context"

// CostPolicy implements RoutingPolicy by selecting the cheapest capable provider.
type CostPolicy struct {
	pricing         map[string]core.ProviderPricing
	capabilities    map[string]core.ProviderCapabilities
	defaultProvider string
	rules           []RoutingRule
}

// CostPolicyConfig holds construction parameters for CostPolicy.
type CostPolicyConfig struct {
	Pricing         map[string]core.ProviderPricing
	Capabilities    map[string]core.ProviderCapabilities
	DefaultProvider string
	Rules           []RoutingRule
}

// NewCostPolicy creates a CostPolicy from the given configuration.
func NewCostPolicy(cfg CostPolicyConfig) *CostPolicy {
	return &CostPolicy{
		pricing:         cfg.Pricing,
		capabilities:    cfg.Capabilities,
		defaultProvider: cfg.DefaultProvider,
		rules:           cfg.Rules,
	}
}

// Select picks the cheapest provider that satisfies the query requirements.
// If category-specific rules exist, those take precedence over cost sorting.
func (p *CostPolicy) Select(hints map[string]string) ProviderSelection {
	category := hints[HintKeyCategory]

	// Extract required capabilities from hints.
	required := parseRequiredCapabilities(hints)

	// Check explicit category rules first, but verify capabilities.
	for _, rule := range p.rules {
		if string(rule.Category) == category {
			if caps, ok := p.capabilities[rule.Provider]; ok && !meetsCapabilities(caps, required) {
				continue
			}
			return ProviderSelection{
				Provider: rule.Provider,
				Model:    rule.Model,
				Reason:   "matched rule for category " + category,
			}
		}
	}

	// Filter providers by capabilities.
	type candidate struct {
		name    string
		cost    float64
		pricing core.ProviderPricing
	}

	var candidates []candidate
	for name, caps := range p.capabilities {
		if !meetsCapabilities(caps, required) {
			continue
		}
		pricing, hasPricing := p.pricing[name]
		cost := pricing.InputPer1M + pricing.OutputPer1M
		if !hasPricing {
			cost = math.MaxFloat64
		}
		candidates = append(candidates, candidate{name: name, cost: cost, pricing: pricing})
	}

	if len(candidates) == 0 {
		return ProviderSelection{
			Provider: p.defaultProvider,
			Reason:   "no capable provider found, using default",
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cost < candidates[j].cost
	})

	best := candidates[0]
	reason := fmt.Sprintf("%s: $%.2f/$%.2f per 1M (input/output)",
		best.name, best.pricing.InputPer1M, best.pricing.OutputPer1M)
	if len(candidates) > 1 {
		runner := candidates[1]
		reason += fmt.Sprintf(" vs %s: $%.2f/$%.2f",
			runner.name, runner.pricing.InputPer1M, runner.pricing.OutputPer1M)
	}

	return ProviderSelection{
		Provider: best.name,
		Reason:   reason,
	}
}

func parseRequiredCapabilities(hints map[string]string) RequiredCapabilities {
	var req RequiredCapabilities
	if hints[HintKeyRequiresTools] == "true" {
		req.Tools = true
	}
	if hints[HintKeyRequiresVision] == "true" {
		req.Vision = true
	}
	if v := hints[HintKeyMinContext]; v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			req.MinContext = n
		}
	}
	return req
}

func meetsCapabilities(caps core.ProviderCapabilities, req RequiredCapabilities) bool {
	if req.Tools && !caps.SupportsToolCalls {
		return false
	}
	if req.Vision && !caps.SupportsVision {
		return false
	}
	if req.MinContext > 0 && caps.MaxContextTokens < req.MinContext {
		return false
	}
	return true
}
