// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package pipeline

import "context"

// Chain composes two stages into one: the output of s1 feeds the input of s2.
// The resulting stage respects context cancellation through both inner stages.
func Chain[A, B, C any](s1 Stage[A, B], s2 Stage[B, C]) Stage[A, C] {
	return func(ctx context.Context, in <-chan A) <-chan C {
		return s2(ctx, s1(ctx, in))
	}
}
