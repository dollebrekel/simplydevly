// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

// TokenUsage tracks token consumption for a request.
type TokenUsage struct {
	InputTokens              int
	OutputTokens             int
	CacheReadInputTokens     int
	CacheCreationInputTokens int
}

// TokenCounter estimates token counts and costs.
type TokenCounter interface {
	Count(text string, model string) (int, error)
	EstimateCost(usage TokenUsage, model string) (float64, error)
}
