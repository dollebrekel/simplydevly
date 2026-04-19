// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

const maxRenderedSize = 1 << 20 // 1 MB output limit for rendered templates

// reservedCommands lists built-in slash commands that MUST NOT be overridable
// by installed skills (AC#6 / Dev Notes: Built-in Slash Commands).
var reservedCommands = map[string]bool{
	"help":        true,
	"yolo":        true,
	"auto-accept": true,
	"default":     true,
	"code":        true,
	"chat":        true,
	"plan":        true,
	"research":    true,
	"marketplace": true,
}

// ErrReservedCommand is returned when a slash command name conflicts with a built-in.
var ErrReservedCommand = errors.New("skills: slash command name is reserved")

// IsReservedCommand returns true if the given name conflicts with a built-in slash command.
func IsReservedCommand(name string) bool {
	return reservedCommands[name]
}

// SlashDispatcher resolves slash commands to skill prompt templates.
type SlashDispatcher struct {
	loader *SkillLoader
}

// NewSlashDispatcher creates a SlashDispatcher backed by the given SkillLoader.
func NewSlashDispatcher(loader *SkillLoader) *SlashDispatcher {
	if loader == nil {
		return nil
	}
	return &SlashDispatcher{loader: loader}
}

// IsSlashCommand returns true if input starts with "/" followed by a known skill name
// that is not a reserved built-in command (AC#2).
func (d *SlashDispatcher) IsSlashCommand(input string) bool {
	if d == nil || d.loader == nil {
		return false
	}
	name, _ := parseSlashInput(input)
	if name == "" || reservedCommands[name] {
		return false
	}
	_, err := d.loader.Get(name)
	return err == nil
}

// Dispatch parses `/<name> <args>`, finds the skill, and renders its prompt template
// with the args substituted into {{.input}} (AC#3).
// Returns the rendered prompt string or an error if the skill is not found or rendering fails.
func (d *SlashDispatcher) Dispatch(input string) (string, error) {
	if d == nil || d.loader == nil {
		return "", fmt.Errorf("skills: dispatcher not initialized")
	}

	name, args := parseSlashInput(input)
	if name == "" {
		return "", fmt.Errorf("skills: empty slash command")
	}

	if reservedCommands[name] {
		return "", fmt.Errorf("%w: %s", ErrReservedCommand, name)
	}

	skill, err := d.loader.Get(name)
	if err != nil {
		return "", err
	}

	tmpl, err := pickTemplate(skill, name)
	if err != nil {
		return "", err
	}

	return renderTemplate(tmpl, args)
}

// parseSlashInput splits a slash command string into (name, args).
// "/<name>" → (name, "")
// "/<name> <rest>" → (name, rest)
// Input not starting with "/" → ("", "")
func parseSlashInput(input string) (name, args string) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return "", ""
	}
	trimmed = strings.TrimPrefix(trimmed, "/")
	parts := strings.SplitN(trimmed, " ", 2)
	name = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		args = strings.TrimSpace(parts[1])
	}
	return name, args
}

// pickTemplate selects the prompt template to use for a skill invocation.
// Priority: template key matching the skill name → first template in alphabetical order.
func pickTemplate(skill *Skill, name string) (PromptTemplate, error) {
	if len(skill.Prompts) == 0 {
		return PromptTemplate{}, fmt.Errorf("%w: %s", ErrNoPrompts, skill.Name)
	}

	// Prefer a prompt key matching the skill name.
	if pt, ok := skill.Prompts[name]; ok {
		return pt, nil
	}

	// Fall back to first prompt in alphabetical order (deterministic).
	keys := make([]string, 0, len(skill.Prompts))
	for k := range skill.Prompts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return skill.Prompts[keys[0]], nil
}

// limitedWriter wraps a bytes.Buffer and returns an error when the limit is exceeded.
type limitedWriter struct {
	buf   *bytes.Buffer
	limit int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.buf.Len()+len(p) > lw.limit {
		return 0, fmt.Errorf("skills: rendered template exceeds %d bytes", lw.limit)
	}
	return lw.buf.Write(p)
}

// renderTemplate renders the prompt template with the provided input text.
// Supports {{.input}}, {{.problem}}, and {{.context}} template variables.
func renderTemplate(pt PromptTemplate, inputText string) (string, error) {
	t, err := template.New("skill").Parse(pt.Template)
	if err != nil {
		return "", fmt.Errorf("skills: parse template: %w", err)
	}

	data := map[string]string{
		"input":   inputText,
		"problem": inputText,
		"context": inputText,
	}

	buf := &bytes.Buffer{}
	lw := &limitedWriter{buf: buf, limit: maxRenderedSize}
	if err := t.Execute(lw, data); err != nil {
		return "", fmt.Errorf("skills: render template: %w", err)
	}
	return buf.String(), nil
}
