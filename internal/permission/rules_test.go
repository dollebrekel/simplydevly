package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/core"
)

func TestDefaultRules_NonEmpty(t *testing.T) {
	rules := defaultRules()
	assert.NotEmpty(t, rules, "defaultRules should return non-empty rules")
}

func TestRulesForMode(t *testing.T) {
	tests := []struct {
		mode Mode
		name string
	}{
		{ModeDefault, "default"},
		{ModeAutoAccept, "auto-accept"},
		{ModeYolo, "yolo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := rulesForMode(tt.mode)
			assert.NotEmpty(t, rules)
		})
	}
}

func TestEvaluateRules_FirstMatchWins(t *testing.T) {
	// Two rules for the same tool — first should win.
	rules := []Rule{
		{Tool: "bash", Action: ActionDeny},
		{Tool: "bash", Action: ActionAllow},
	}

	verdict := evaluateRules(rules, core.Action{Tool: "bash"}, ModeDefault)
	assert.Equal(t, core.Deny, verdict, "first matching rule should win")
}

func TestEvaluateRules_WildcardCatchAll(t *testing.T) {
	rules := []Rule{
		{Tool: "file_read", Action: ActionAllow},
		{Tool: "*", Action: ActionAsk},
	}

	// Known tool → specific rule.
	verdict := evaluateRules(rules, core.Action{Tool: "file_read"}, ModeDefault)
	assert.Equal(t, core.Allow, verdict)

	// Unknown tool → wildcard.
	verdict = evaluateRules(rules, core.Action{Tool: "something_else"}, ModeDefault)
	assert.Equal(t, core.Ask, verdict)
}

func TestEvaluateRules_NoMatchingRule_Ask(t *testing.T) {
	// Empty rule set — no matches.
	verdict := evaluateRules(nil, core.Action{Tool: "bash"}, ModeDefault)
	assert.Equal(t, core.Ask, verdict, "no matching rule → Ask (safe default)")
}

func TestEvaluateRules_DestructiveOverride(t *testing.T) {
	rules := []Rule{
		{Tool: "file_read", Action: ActionAllow},
	}

	// Non-destructive → Allow.
	verdict := evaluateRules(rules, core.Action{Tool: "file_read"}, ModeDefault)
	assert.Equal(t, core.Allow, verdict)

	// Destructive in non-Yolo → Ask (override).
	verdict = evaluateRules(rules, core.Action{Tool: "file_read", Destructive: true}, ModeDefault)
	assert.Equal(t, core.Ask, verdict)

	// Destructive in Yolo → normal evaluation (no override).
	verdict = evaluateRules(rules, core.Action{Tool: "file_read", Destructive: true}, ModeYolo)
	assert.Equal(t, core.Allow, verdict)
}

func TestMatchRule_DestructiveFilter(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name        string
		rule        Rule
		action      core.Action
		shouldMatch bool
	}{
		{
			"no filter, non-destructive",
			Rule{Tool: "bash", Action: ActionAsk},
			core.Action{Tool: "bash", Destructive: false},
			true,
		},
		{
			"no filter, destructive",
			Rule{Tool: "bash", Action: ActionAsk},
			core.Action{Tool: "bash", Destructive: true},
			true,
		},
		{
			"destructive=true filter, action destructive",
			Rule{Tool: "bash", Action: ActionAsk, Destructive: &trueVal},
			core.Action{Tool: "bash", Destructive: true},
			true,
		},
		{
			"destructive=true filter, action not destructive",
			Rule{Tool: "bash", Action: ActionAsk, Destructive: &trueVal},
			core.Action{Tool: "bash", Destructive: false},
			false,
		},
		{
			"destructive=false filter, action not destructive",
			Rule{Tool: "bash", Action: ActionAllow, Destructive: &falseVal},
			core.Action{Tool: "bash", Destructive: false},
			true,
		},
		{
			"wrong tool name",
			Rule{Tool: "git_push", Action: ActionAsk},
			core.Action{Tool: "bash"},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.shouldMatch, matchRule(tt.rule, tt.action))
		})
	}
}
