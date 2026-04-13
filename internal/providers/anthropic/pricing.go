// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package anthropic

import (
	"strings"

	"siply.dev/siply/internal/core"
)

// modelPrice holds per-million-token pricing for a model.
type modelPrice struct {
	InputPerMToken  float64
	OutputPerMToken float64
}

// Known Anthropic model prices (USD per million tokens).
var anthropicPrices = map[string]modelPrice{
	"claude-sonnet-4":  {InputPerMToken: 3.00, OutputPerMToken: 15.00},
	"claude-opus-4":    {InputPerMToken: 15.00, OutputPerMToken: 75.00},
	"claude-haiku-4":   {InputPerMToken: 0.80, OutputPerMToken: 4.00},
}

// Anthropic cache pricing multipliers relative to input price.
const (
	cacheReadMultiplier    = 0.10 // Cache read tokens cost 10% of input price.
	cacheCreationMultiplier = 1.25 // Cache creation tokens cost 125% of input price.
)

// PricingTokenCounter implements core.TokenCounter with Anthropic-specific
// cache-aware cost estimation.
type PricingTokenCounter struct{}

// Count returns 0 — token counting relies on provider-reported usage.
func (p *PricingTokenCounter) Count(_ string, _ string) (int, error) {
	return 0, nil
}

// EstimateCost calculates the cost of a request including cache pricing.
// Cache read tokens are priced at input_price * 0.10.
// Cache creation tokens are priced at input_price * 1.25.
// Non-cache input tokens are priced at the standard input rate.
func (p *PricingTokenCounter) EstimateCost(usage core.TokenUsage, model string) (float64, error) {
	price := lookupAnthropicPrice(model)
	if price.InputPerMToken == 0 && price.OutputPerMToken == 0 {
		return 0.0, nil
	}

	inputCost := float64(usage.InputTokens) * price.InputPerMToken / 1_000_000
	outputCost := float64(usage.OutputTokens) * price.OutputPerMToken / 1_000_000
	cacheReadCost := float64(usage.CacheReadInputTokens) * price.InputPerMToken * cacheReadMultiplier / 1_000_000
	cacheCreationCost := float64(usage.CacheCreationInputTokens) * price.InputPerMToken * cacheCreationMultiplier / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheCreationCost, nil
}

// lookupAnthropicPrice finds the pricing for a model by trying progressively
// shorter name prefixes (strips date suffixes like -20250514).
func lookupAnthropicPrice(model string) modelPrice {
	// Exact match.
	if p, ok := anthropicPrices[model]; ok {
		return p
	}

	// Strip date suffix: "claude-sonnet-4-20250514" → "claude-sonnet-4"
	if stripped := stripDateSuffix(model); stripped != model {
		if p, ok := anthropicPrices[stripped]; ok {
			return p
		}
	}

	// Strip generation suffix: "claude-sonnet-4-6" → "claude-sonnet-4"
	if stripped := stripGenerationSuffix(model); stripped != model {
		if p, ok := anthropicPrices[stripped]; ok {
			return p
		}
	}

	return modelPrice{}
}

// stripDateSuffix removes a trailing date suffix (6+ digits after last hyphen).
func stripDateSuffix(model string) string {
	lastHyphen := strings.LastIndex(model, "-")
	if lastHyphen < 0 || lastHyphen >= len(model)-1 {
		return model
	}
	suffix := model[lastHyphen+1:]
	if len(suffix) < 6 {
		return model
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return model
		}
	}
	return model[:lastHyphen]
}

// stripGenerationSuffix removes a short trailing numeric suffix like "-6" in "claude-sonnet-4-6".
func stripGenerationSuffix(model string) string {
	lastHyphen := strings.LastIndex(model, "-")
	if lastHyphen < 0 || lastHyphen >= len(model)-1 {
		return model
	}
	suffix := model[lastHyphen+1:]
	if len(suffix) >= 6 {
		return model // This is a date suffix, not a generation suffix.
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return model
		}
	}
	return model[:lastHyphen]
}
