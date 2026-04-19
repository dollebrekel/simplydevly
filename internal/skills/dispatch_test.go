// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDispatcher(t *testing.T) *SlashDispatcher {
	t.Helper()
	loader := NewSkillLoader(testdataDir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	return NewSlashDispatcher(loader)
}

func TestSlashDispatcher_IsSlashCommand_ValidSkill(t *testing.T) {
	d := setupDispatcher(t)
	assert.True(t, d.IsSlashCommand("/valid-skill"))
	assert.True(t, d.IsSlashCommand("/valid-skill some input"))
}

func TestSlashDispatcher_IsSlashCommand_UnknownSkill(t *testing.T) {
	d := setupDispatcher(t)
	assert.False(t, d.IsSlashCommand("/nonexistent"))
}

func TestSlashDispatcher_IsSlashCommand_ReservedCommands(t *testing.T) {
	d := setupDispatcher(t)
	for _, reserved := range []string{"/help", "/yolo", "/code", "/chat", "/plan", "/research", "/marketplace", "/auto-accept", "/default"} {
		assert.False(t, d.IsSlashCommand(reserved), "should not match reserved command: %s", reserved)
	}
}

func TestSlashDispatcher_IsSlashCommand_NotSlash(t *testing.T) {
	d := setupDispatcher(t)
	assert.False(t, d.IsSlashCommand("no slash here"))
	assert.False(t, d.IsSlashCommand(""))
}

func TestSlashDispatcher_Dispatch_ValidSkillWithInput(t *testing.T) {
	d := setupDispatcher(t)
	result, err := d.Dispatch("/valid-skill some code here")
	require.NoError(t, err)
	assert.Contains(t, result, "some code here", "template variable {{.input}} should be substituted")
}

func TestSlashDispatcher_Dispatch_ValidSkillEmptyInput(t *testing.T) {
	d := setupDispatcher(t)
	result, err := d.Dispatch("/valid-skill")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}

func TestSlashDispatcher_Dispatch_UnknownSkill(t *testing.T) {
	d := setupDispatcher(t)
	_, err := d.Dispatch("/nonexistent-skill")
	assert.ErrorIs(t, err, ErrSkillNotFound)
}

func TestSlashDispatcher_Dispatch_ReservedCommand(t *testing.T) {
	d := setupDispatcher(t)
	_, err := d.Dispatch("/help")
	assert.ErrorIs(t, err, ErrReservedCommand)
}

func TestSlashDispatcher_TemplateRendering_InputVariable(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	writeTestSkill(t, skillDir, "my-skill")

	// Override the prompts file to use a simple template.
	prompts := "prompts:\n  my-skill:\n    name: My Skill\n    description: test\n    template: \"Input: {{.input}}\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "prompts.yaml"), []byte(prompts), 0600))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	d := NewSlashDispatcher(loader)

	result, err := d.Dispatch("/my-skill hello world")
	require.NoError(t, err)
	assert.Equal(t, "Input: hello world", result)
}

func TestSlashDispatcher_NilLoader(t *testing.T) {
	d := NewSlashDispatcher(nil)
	assert.Nil(t, d)
}

func TestParseSlashInput(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs string
	}{
		{"/code-review some code", "code-review", "some code"},
		{"/debug", "debug", ""},
		{"/skill with spaces and stuff", "skill", "with spaces and stuff"},
		{"not a slash command", "", ""},
		{"", "", ""},
		{"/  ", "", ""},
	}
	for _, tt := range tests {
		name, args := parseSlashInput(tt.input)
		assert.Equal(t, tt.wantName, name, "input: %q", tt.input)
		assert.Equal(t, tt.wantArgs, args, "input: %q", tt.input)
	}
}

func TestPickTemplate_MatchesSkillName(t *testing.T) {
	skill := &Skill{
		Name: "code-review",
		Prompts: map[string]PromptTemplate{
			"code-review": {Template: "review: {{.input}}"},
			"explain":     {Template: "explain: {{.input}}"},
		},
	}
	pt, err := pickTemplate(skill, "code-review")
	require.NoError(t, err)
	assert.Equal(t, "review: {{.input}}", pt.Template)
}

func TestPickTemplate_FallbackToFirst(t *testing.T) {
	skill := &Skill{
		Name: "my-skill",
		Prompts: map[string]PromptTemplate{
			"beta":  {Template: "beta template"},
			"alpha": {Template: "alpha template"},
		},
	}
	pt, err := pickTemplate(skill, "my-skill")
	require.NoError(t, err)
	// Alphabetically first is "alpha".
	assert.Equal(t, "alpha template", pt.Template)
}

func TestPickTemplate_NoPrompts(t *testing.T) {
	skill := &Skill{Name: "empty", Prompts: map[string]PromptTemplate{}}
	_, err := pickTemplate(skill, "empty")
	assert.ErrorIs(t, err, ErrNoPrompts)
}

func TestRenderTemplate_Substitution(t *testing.T) {
	pt := PromptTemplate{Template: "Review: {{.input}}\nContext: {{.context}}"}
	result, err := renderTemplate(pt, "my code")
	require.NoError(t, err)
	assert.Contains(t, result, "my code")
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	pt := PromptTemplate{Template: "{{.unclosed"}
	_, err := renderTemplate(pt, "input")
	assert.Error(t, err)
}

func TestRenderTemplate_EmptyInput(t *testing.T) {
	pt := PromptTemplate{Template: "Input: {{.input}}"}
	result, err := renderTemplate(pt, "")
	require.NoError(t, err)
	assert.Equal(t, "Input: ", result)
}

func TestIsSlashCommand_NoLoader(t *testing.T) {
	d := &SlashDispatcher{loader: nil}
	assert.False(t, d.IsSlashCommand("/anything"))
}

func TestDispatch_NoLoader(t *testing.T) {
	d := &SlashDispatcher{loader: nil}
	_, err := d.Dispatch("/anything")
	assert.Error(t, err)
}

func TestSkillLoader_TemplateVariables(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "debug-skill")
	require.NoError(t, os.MkdirAll(skillDir, 0755))
	writeTestSkill(t, skillDir, "debug-skill")

	prompts := "prompts:\n  debug-skill:\n    name: Debug\n    template: \"Problem: {{.problem}} Input: {{.input}}\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "prompts.yaml"), []byte(prompts), 0600))

	loader := NewSkillLoader(dir, "")
	require.NoError(t, loader.LoadAll(context.Background()))
	d := NewSlashDispatcher(loader)

	result, err := d.Dispatch("/debug-skill error here")
	require.NoError(t, err)
	assert.Contains(t, result, "error here")
	assert.NotErrorIs(t, err, errors.New("any"))
}
