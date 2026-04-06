// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

// AgentConfig holds agent-level configuration that controls runtime behavior.
// This struct is the agent's own config contract, passed via dependency
// injection. It is independent of ConfigResolver (which wires it in Epic 3).
type AgentConfig struct {
	// ParallelTools enables concurrent tool execution when the provider
	// returns multiple tool calls in a single turn. Default: false (sequential).
	ParallelTools bool

	// MaxIterations limits the number of tool-call rounds per Run invocation.
	// Zero or negative values fall back to the package-level default.
	MaxIterations int
}

// effectiveMaxIterations returns MaxIterations if positive, otherwise the
// package-level default.
func (c AgentConfig) effectiveMaxIterations() int {
	if c.MaxIterations > 0 {
		return c.MaxIterations
	}
	return maxToolIterations
}
