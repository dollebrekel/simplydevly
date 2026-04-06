// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package permission

import "siply.dev/siply/internal/core"

// defaultRules returns the built-in rule set used when no custom rules are
// provided. These are the baseline rules; mode-specific behavior is
// handled by rulesForMode.
func defaultRules() []Rule {
	return []Rule{
		{Tool: "file_read", Action: ActionAllow},
		{Tool: "file_write", Action: ActionAllow},
		{Tool: "file_edit", Action: ActionAllow},
		{Tool: "search", Action: ActionAllow},
		{Tool: "web", Action: ActionAsk},
		{Tool: "bash", Action: ActionAsk},
		{Tool: "git_push", Action: ActionAsk},
		{Tool: "*", Action: ActionAsk}, // safe default for unknown tools
	}
}

// autoAcceptRules returns the rule set for auto-accept mode.
func autoAcceptRules() []Rule {
	return []Rule{
		{Tool: "file_read", Action: ActionAllow},
		{Tool: "file_write", Action: ActionAllow},
		{Tool: "file_edit", Action: ActionAllow},
		{Tool: "search", Action: ActionAllow},
		{Tool: "web", Action: ActionAllow},
		{Tool: "bash", Action: ActionAllow},
		{Tool: "git_push", Action: ActionAsk},
		{Tool: "*", Action: ActionAsk},
	}
}

// yoloRules returns the rule set for yolo mode: everything is allowed.
func yoloRules() []Rule {
	return []Rule{
		{Tool: "*", Action: ActionAllow},
	}
}

// rulesForMode returns the default rule set for the given mode.
func rulesForMode(mode Mode) []Rule {
	switch mode {
	case ModeDefault:
		return defaultRules()
	case ModeAutoAccept:
		return autoAcceptRules()
	case ModeYolo:
		return yoloRules()
	default:
		// Invalid mode — fall back to most restrictive (default) rules.
		return defaultRules()
	}
}

// evaluateRules matches an action against rules in order (first match wins).
// If action.Destructive is true and mode is not Yolo, the result is always Ask
// regardless of tool-specific rules.
func evaluateRules(rules []Rule, action core.Action, mode Mode) core.ActionVerdict {
	// Destructive override: in non-Yolo modes, destructive actions always Ask.
	if action.Destructive && mode != ModeYolo {
		return core.Ask
	}

	for _, r := range rules {
		if !matchRule(r, action) {
			continue
		}
		switch r.Action {
		case ActionAllow:
			return core.Allow
		case ActionAsk:
			return core.Ask
		case ActionDeny:
			return core.Deny
		default:
			// Unknown ActionType — skip this rule (fail-open to next rule).
			continue
		}
	}

	// No rule matched — safe default is Ask.
	return core.Ask
}

// matchRule checks if a rule applies to the given action.
func matchRule(r Rule, action core.Action) bool {
	// Check tool name (exact match or wildcard).
	if r.Tool != "*" && r.Tool != action.Tool {
		return false
	}

	// Check destructive filter if set.
	if r.Destructive != nil && *r.Destructive != action.Destructive {
		return false
	}

	return true
}
