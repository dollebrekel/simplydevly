// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
	"siply.dev/siply/internal/core"
)

const maxInjectedSprintStatusBytes = 256 * 1024

// Activator matches user prompts to loaded skills and injects model-visible
// hidden context. It is deterministic by design; no fuzzy search or LLM routing.
type Activator struct {
	loader *SkillLoader
	mu     sync.Mutex
	seen   map[string]struct{}
}

func NewActivator(loader *SkillLoader) *Activator {
	if loader == nil {
		return nil
	}
	return &Activator{
		loader: loader,
		seen:   make(map[string]struct{}),
	}
}

func (a *Activator) PreQueryHook(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
	if a == nil || a.loader == nil {
		return msgs, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	input := latestVisibleUserMessage(msgs)
	if strings.TrimSpace(input) == "" {
		return msgs, nil
	}

	var injections []core.Message
	for _, skill := range a.loader.List() {
		if !matchesSkill(input, skill) {
			continue
		}
		if a.alreadyInjected(skill.Name) {
			continue
		}
		body := strings.TrimSpace(skill.Body)
		if body == "" {
			rendered, err := renderLegacySkillBody(skill)
			if err != nil {
				return nil, err
			}
			body = rendered
		}
		injections = append(injections, core.Message{
			Role:    "system",
			Content: buildSkillContext(skill, body, input),
			Meta:    true,
			Hidden:  true,
			Source:  "skill:" + skill.Name,
		})
	}
	if len(injections) == 0 {
		return msgs, nil
	}
	return append(injections, msgs...), nil
}

func buildSkillContext(skill Skill, body, userInput string) string {
	skillRoot := skill.Dir
	if skillRoot == "" {
		skillRoot = skill.BodyPath
	}
	workspaceContext := buildWorkspaceContext(skill.Name)
	return fmt.Sprintf(`<activated_skill name=%q kind=%q source=%q skill_root=%q>
<runtime_instructions>
Use this skill as operating instructions for the current user request.
Do not print the workflow as shell, Python, pseudocode, or a transcript unless the skill explicitly asks for generated code.
Treat XML-like workflow markup as private execution instructions.
Never include raw workflow tags such as <activated_skill>, <skill_body>, <workflow>, <step>, <action>, <check>, <output>, <ask>, or <executing_skill> in user-facing answers.
When a workflow has an output or ask block, convert it into concise natural-language Markdown for the user.
Do not simulate files, projects, dates, statuses, counts, paths, session logs, or external vault contents.
When the workflow says to load, read, parse, search, or validate files, use the available tools against the current workspace.
Resolve {skill-root} to the skill_root attribute above. Resolve {project-root} to the active workspace root.
If workspace_context provides preloaded authoritative file content, use that content instead of guessing or using older paths from memory.
If a required file is missing, report the exact resolved path that was checked and the next action from the skill. Do not invent replacement data.
</runtime_instructions>
<user_request>
%s
</user_request>
%s
<skill_body>
%s
</skill_body>
</activated_skill>`, skill.Name, skill.Kind, skill.BodyPath, skillRoot, strings.TrimSpace(userInput), workspaceContext, strings.TrimSpace(body))
}

func buildWorkspaceContext(skillName string) string {
	if skillName != "bmad-sprint-status" {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Sprintf("<workspace_context>\nproject_root_error: %q\n</workspace_context>\n", err.Error())
	}
	statusPath := resolveSprintStatusPath(cwd)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return fmt.Sprintf(`<workspace_context>
project_root: %q
sprint_status_file: %q
sprint_status_error: %q
</workspace_context>
`, cwd, statusPath, err.Error())
	}
	if len(data) > maxInjectedSprintStatusBytes {
		return fmt.Sprintf(`<workspace_context>
project_root: %q
sprint_status_file: %q
sprint_status_error: "file too large to inject (%d bytes)"
</workspace_context>
`, cwd, statusPath, len(data))
	}
	return fmt.Sprintf(`<workspace_context>
project_root: %q
sprint_status_file: %q
<preloaded_file path=%q>
%s
</preloaded_file>
</workspace_context>
`, cwd, statusPath, statusPath, strings.TrimSpace(string(data)))
}

func resolveSprintStatusPath(projectRoot string) string {
	implementationArtifacts := filepath.Join(projectRoot, "_bmad-output", "implementation-artifacts")
	configPath := filepath.Join(projectRoot, "_bmad", "bmm", "config.yaml")
	if data, err := os.ReadFile(configPath); err == nil {
		var config map[string]any
		if yaml.Unmarshal(data, &config) == nil {
			if value, ok := config["implementation_artifacts"].(string); ok && strings.TrimSpace(value) != "" {
				implementationArtifacts = resolveProjectPath(projectRoot, value)
			}
		}
	}
	return filepath.Join(implementationArtifacts, "sprint-status.yaml")
}

func resolveProjectPath(projectRoot, value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "{project-root}", projectRoot)
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(projectRoot, value))
}

func latestVisibleUserMessage(msgs []core.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" && !msgs[i].Hidden && !msgs[i].Meta {
			return msgs[i].Content
		}
	}
	return ""
}

func (a *Activator) alreadyInjected(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.seen[name]; ok {
		return true
	}
	a.seen[name] = struct{}{}
	return false
}

func matchesSkill(input string, skill Skill) bool {
	trimmed := strings.TrimSpace(input)
	lower := strings.ToLower(trimmed)
	name := strings.ToLower(skill.Name)
	if lower == name || strings.HasPrefix(lower, name+" ") || strings.HasPrefix(lower, "$"+name) {
		return true
	}
	for _, command := range skill.Activation.Commands {
		command = strings.ToLower(strings.TrimSpace(command))
		if command != "" && (lower == command || strings.HasPrefix(lower, command+" ")) {
			return true
		}
	}
	for _, keyword := range skill.Activation.Keywords {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" && strings.Contains(lower, keyword) {
			return true
		}
	}
	for _, pattern := range skill.Activation.IntentPatterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern != "" && strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func renderLegacySkillBody(skill Skill) (string, error) {
	if len(skill.Prompts) == 0 {
		return "", fmt.Errorf("%w: %s", ErrNoPrompts, skill.Name)
	}
	tmpl, err := pickTemplate(&skill, skill.Name)
	if err != nil {
		return "", err
	}
	return tmpl.Template, nil
}
