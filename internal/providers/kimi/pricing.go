// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package kimi

import (
	"siply.dev/siply/internal/core"
)

// Kimi (Moonshot AI) pricing constants (USD per million tokens).
// Source: https://platform.moonshot.cn/docs/pricing
const (
	// moonshot-v1-8k
	kimiV1_8kInputPerMToken  = 0.12
	kimiV1_8kOutputPerMToken = 0.12

	// moonshot-v1-32k
	kimiV1_32kInputPerMToken  = 0.24
	kimiV1_32kOutputPerMToken = 0.24

	// moonshot-v1-128k
	kimiV1_128kInputPerMToken  = 0.60
	kimiV1_128kOutputPerMToken = 0.60

	// Cache pricing multipliers relative to input price.
	kimiCacheReadMultiplier     = 0.10 // Cache read tokens cost 10% of input price.
	kimiCacheCreationMultiplier = 1.00 // Cache creation tokens cost 100% of input price.
)

type kimiModelPrice struct {
	InputPerMToken  float64
	OutputPerMToken float64
}

var kimiPrices = map[string]kimiModelPrice{
	"moonshot-v1-8k":   {InputPerMToken: kimiV1_8kInputPerMToken, OutputPerMToken: kimiV1_8kOutputPerMToken},
	"moonshot-v1-32k":  {InputPerMToken: kimiV1_32kInputPerMToken, OutputPerMToken: kimiV1_32kOutputPerMToken},
	"moonshot-v1-128k": {InputPerMToken: kimiV1_128kInputPerMToken, OutputPerMToken: kimiV1_128kOutputPerMToken},
}

// PricingTokenCounter implements core.TokenCounter with Kimi-specific
// cache-aware cost estimation.
type PricingTokenCounter struct{}

// Count returns 0 — token counting relies on provider-reported usage.
func (p *PricingTokenCounter) Count(_ string, _ string) (int, error) {
	return 0, nil
}

// EstimateCost calculates the cost of a Kimi request including cache pricing.
// Cache read tokens are priced at input_price * 0.10.
// Cache creation tokens are priced at input_price * 1.00.
func (p *PricingTokenCounter) EstimateCost(usage core.TokenUsage, model string) (float64, error) {
	price := lookupKimiPrice(model)
	if price.InputPerMToken == 0 && price.OutputPerMToken == 0 {
		return 0.0, nil
	}

	inputCost := float64(usage.InputTokens) * price.InputPerMToken / 1_000_000
	outputCost := float64(usage.OutputTokens) * price.OutputPerMToken / 1_000_000
	cacheReadCost := float64(usage.CacheReadInputTokens) * price.InputPerMToken * kimiCacheReadMultiplier / 1_000_000
	cacheCreationCost := float64(usage.CacheCreationInputTokens) * price.InputPerMToken * kimiCacheCreationMultiplier / 1_000_000

	return inputCost + outputCost + cacheReadCost + cacheCreationCost, nil
}

// lookupKimiPrice finds the pricing for a model.
func lookupKimiPrice(model string) kimiModelPrice {
	if p, ok := kimiPrices[model]; ok {
		return p
	}
	// Default to 128k pricing for unknown Kimi models.
	return kimiPrices["moonshot-v1-128k"]
}
