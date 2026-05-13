# ClaudeFast-Style Skill System SSOT

Status: draft for BMAD story creation
Owner intent: make Siply use the same style of skill activation as Claude Code / ClaudeFast / Tjoep, with one source of truth and token-conscious loading.

## Problem

Siply currently has a skill system, but it is not the ClaudeFast-style system the user expects.

Observed current behavior:

- Slash skill commands expand into full prompt text.
- The expanded skill prompt is submitted as normal user-visible chat text.
- The TUI can show that skill text in the chat area.
- Siply discovers skills from `~/.siply/skills` and `<workspace>/.siply/skills`.
- Siply expects each skill directory to contain `manifest.yaml` and `prompts.yaml` or `config.yaml`.
- Siply does not currently read Claude/Codex-style `SKILL.md` files as first-class skills.
- Siply does not currently read ClaudeFast `skill-rules.json` activation rules.
- Siply does not currently have a hidden/meta message flag in `core.Message`.

The user expects:

- A single source of truth for skill definitions and skill activation rules.
- Behavior similar to Claude Code / ClaudeFast / Tjoep.
- Skills should be discovered and recommended automatically from intent.
- Full skill content should be loaded only when needed.
- Skill context should be model-visible when needed, but not shown as normal user chat text.
- Token usage should be lower than always injecting full skill text.

## Current Code References

Skill loading:

- `internal/skills/loader.go`
- `cmd/siply/tui.go`
- `cmd/siply/run.go`
- `cmd/siply/skills.go`

Slash command dispatch:

- `internal/tui/panels/repl.go`
- `internal/commands/slash.go`
- `internal/tui/panels/slashoverlay.go`

Message model and agent flow:

- `internal/core/context.go`
- `internal/agent/agent.go`
- `internal/tui/messages.go`
- `internal/tui/app.go`

Extensibility hooks that may be reused:

- `internal/hooks`
- `plugins/session-intelligence`
- `plugins/context-distillation`

Relevant external comparison points in this workspace:

- Tjoep uses meta user messages for skill context.
- ClaudeFast uses `.claude/skills/skill-rules.json` plus a `UserPromptSubmit` hook to recommend skills from user intent.

## Desired Architecture

Siply should support a native skill activation layer with these parts:

1. Skill source registry
2. Activation rules index
3. On-demand skill content loader
4. Hidden/meta message injection
5. TUI display separation
6. Token-aware caching and deduplication

## Single Source Of Truth

The SSOT should be a project-readable file that maps skill identity, source files, activation triggers, and loading behavior.

Recommended native Siply file:

```text
.siply/skills.index.json
```

Recommended schema:

```json
{
  "version": 1,
  "sources": [
    {
      "type": "skill-md-directory",
      "path": ".siply/skills",
      "format": "SKILL.md"
    },
    {
      "type": "skill-md-directory",
      "path": ".agents/skills",
      "format": "SKILL.md"
    },
    {
      "type": "claudefast-rules",
      "path": ".claude/skills/skill-rules.json"
    }
  ],
  "skills": [
    {
      "name": "bmad-help",
      "source": ".siply/skills/bmad-help/SKILL.md",
      "description": "Answer BMad workflow questions and recommend the next skill.",
      "activation": {
        "keywords": ["bmad-help", "bmad help", "what next", "which bmad skill"],
        "intentPatterns": ["user asks what to do next in BMad", "user asks why a BMAD skill is not working"]
      },
      "loading": {
        "mode": "on_demand",
        "dedupePerSession": true,
        "visibleInChat": false
      }
    }
  ]
}
```

This file should become the runtime index. The skill body remains in `SKILL.md`; the SSOT points to it and defines how Siply activates it.

## Runtime Behavior

When the user submits a prompt:

1. Siply reads the active skill index.
2. Siply checks activation rules against the prompt.
3. Siply presents or applies a recommendation.
4. If a skill is selected or confidently matched, Siply loads the related `SKILL.md`.
5. Siply injects the skill instructions as a meta/hidden model message.
6. Siply shows a compact UI event such as `Loaded skill: bmad-help`, not the full skill text.
7. Siply records that the skill was loaded for this session to avoid repeated full injection.

## Message Model Requirement

`core.Message` needs a way to distinguish normal user-visible messages from model-visible internal context.

Recommended fields:

```go
type Message struct {
    Role        string
    Content     string
    ToolCalls   []ToolCall
    ToolResults []ToolResult
    Meta        bool
    Hidden      bool
    Source      string
}
```

Expected behavior:

- `Meta=true` means the message is system/runtime context, not user-authored content.
- `Hidden=true` means the message should not be rendered as normal chat.
- Providers still receive the content when required.
- TUI history should show only a compact event, not the full hidden content.

## Token Strategy

Hiding content from the TUI does not save tokens by itself. Token savings come from:

- Loading only the skill metadata/index at startup.
- Loading full `SKILL.md` only when activation requires it.
- Deduplicating already-loaded skills per session.
- Reusing provider prompt caching where available.
- Keeping activation rules small and structured.
- Avoiding automatic injection of every installed skill.

## TUI Requirement

The TUI must separate three concepts:

- User chat messages
- Assistant chat messages
- Runtime events, such as loaded skills and hook recommendations

Skill content must not appear as a normal user message.

Slash command menu behavior should also be fixed so the command overlay does not cover or displace the REPL input. The overlay should render above the input or through the existing overlay compositor.

## Backward Compatibility

Siply should continue to support existing manifest/prompt skills:

- `manifest.yaml`
- `prompts.yaml`
- `config.yaml`

The new loader should add support for:

- `SKILL.md`
- `.siply/skills.index.json`
- optional ClaudeFast `.claude/skills/skill-rules.json`

Existing slash command syntax should continue to work.

## Acceptance Criteria

1. Siply can discover `SKILL.md` skills from `.siply/skills` and `.agents/skills`.
2. Siply can read `.siply/skills.index.json` as the native skill activation SSOT.
3. Siply can optionally import or reference ClaudeFast `.claude/skills/skill-rules.json`.
4. A matched skill can be loaded as model-visible hidden/meta context.
5. Hidden/meta skill content is not rendered as normal user chat in the TUI.
6. The TUI shows a compact runtime event when a skill is loaded.
7. Reusing the same skill in one session does not repeatedly inject the full content unless explicitly requested.
8. Existing `manifest.yaml` and `prompts.yaml` skill behavior remains supported.
9. `/skills` or equivalent listing shows both legacy Siply skills and indexed `SKILL.md` skills.
10. Tests cover loader discovery, activation matching, hidden message handling, and TUI rendering behavior.

## Suggested Story

Title: Add ClaudeFast-style skill activation SSOT and hidden skill context

As a Siply user,
I want Siply to load skills from a single source of truth with ClaudeFast-style activation,
so that skills work like Claude Code/Tjoep, avoid visible prompt dumping in chat, and reduce token use through on-demand loading.

Implementation order:

1. Add skill index types and parser for `.siply/skills.index.json`.
2. Add `SKILL.md` discovery support beside the existing manifest/prompt loader.
3. Add activation matching using keywords and intent patterns.
4. Add hidden/meta message support to the core message model and TUI rendering.
5. Route skill activation through the agent flow before provider calls.
6. Add session-level dedupe for loaded skill content.
7. Update `/skills` and slash command listing to include indexed skills.
8. Fix slash overlay positioning so it does not cover the input area.
9. Add focused unit and TUI tests.

## Suggested Next BMAD Prompt

Use this in a fresh session:

```text
bmad-create-story

Create a story for Simply Devly / Siply using docs/claudefast-skill-ssot.md as the single source of truth.
The story must cover ClaudeFast-style skill activation, SKILL.md discovery, .siply/skills.index.json, hidden/meta skill context, TUI non-visible skill injection, token-conscious loading, and backward compatibility with existing Siply manifest/prompts skills.
Keep the story implementable in one focused dev-story pass if possible, and split only if the blast radius is too large.
```

After the story is created and reviewed:

```text
bmad-dev-story

Implement the approved story for ClaudeFast-style skill activation SSOT in Simply Devly / Siply.
Use docs/claudefast-skill-ssot.md as the primary source of truth.
Do not remove existing manifest/prompts skill support.
Commit changes with an English commit message explaining what changed.
```
