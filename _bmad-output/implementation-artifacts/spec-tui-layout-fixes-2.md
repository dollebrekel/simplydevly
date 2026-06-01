---
title: 'TUI Layout Fixes R2 — Borderless Center, Input Dividers, Drag Regression, Streaming Status Bar'
type: 'bugfix'
created: '2026-06-01'
status: 'done'
baseline_commit: '8cd5469'
context:
  - '{project-root}/docs/go-best-practices.md'
  - '{project-root}/project-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Four TUI layout issues remain after the May 31 status-bar fix (commit 8cd5469): (A1) the center/main window still draws an outer border, (A2) the input field has no clear top+bottom demarcation from the output, (A3) panel divider drag-to-resize regressed — the `343021e` chat merge gated dragging behind `layoutLocked` and shipped it defaulting to `true`, so the plugin-box side is no longer draggable out of the box, and (C) the status bar disappears while AI output streams, because the multi-line agent-status block grows the center content past `MaxContentHeight` and `RenderBorder` does not clip on height.

**Approach:** Make the center REPL panel permanently borderless; bracket the input with a divider above (exists) and a new one below; restore drag by defaulting `layoutLocked` to `false`; and make `REPLPanel.View()` height-aware so the composed panel never exceeds its allotted height regardless of streaming agent-status growth, keeping the status bar's reserved line on-screen.

## Boundaries & Constraints

**Always:**
- `go test -race ./internal/tui/...` stays green
- lipgloss v2 / Bubble Tea v2 APIs (charm.land imports)
- Status bar renders on every frame when `layout.ShowStatusBar == true`, including during streaming
- Left/right plugin-box borders (rendered via `renderSlot`) are preserved — only the center loses its border
- Chat messages keep vertical role-based stacking (unchanged)

**Ask First:**
- Removing the center border such that Ctrl+B no longer re-enables a center border (center is now always borderless)
- Defaulting `layoutLocked` to `false` (previously flagged; now explicitly requested by the human)

**Never:**
- Break accessible mode rendering (`VerbosityAccessible`)
- Introduce new dependencies or change theme tokens / color values
- Modify the splash screen or chat-bubble stacking logic
- Truncate the input line or divider when bounding height — only the chat viewport may shrink

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Borderless center | Render center panel | No box-drawing border around center content | N/A |
| Input dividers | Render REPL | Horizontal rule directly above AND below the input line | N/A |
| Drag out of the box | Fresh start, plugin box present, click ±2px on its divider, drag | Plugin width changes, center reflows | Clamped by `clampSlotWidth` |
| Streaming status bar | Agent streaming, agent-status = header + N agents + tip (multi-line) | Status bar remains the last visible line; chat viewport shrinks to absorb the chrome | Viewport height clamps to ≥1 |
| Idle status bar | No agents running | Status bar visible, viewport uses full height | N/A |
| Narrow terminal | Width < 60 | Side panels auto-collapse, status bar compact (unchanged) | N/A |

</frozen-after-approval>

## Code Map

- `internal/tui/panels/repl.go:170` -- `hasBorder` initialised from `config.Borders`; center border origin (A1)
- `internal/tui/panels/repl.go:1037-1102` -- `View()`: composes viewport + agentStatus + top divider + overlay + input; add bottom divider (A2) and dynamic viewport sizing (C)
- `internal/tui/panels/repl.go:1104-1165` -- `SetSize()`: viewport height math (`height-4`, `-2` for border); adjust chrome reservation
- `internal/tui/panels/repl.go:1167-1173` -- `SetBordered()`: must not re-enable center border (A1)
- `internal/tui/panels/manager.go:132` -- `layoutLocked: true` default → set `false` (A3)
- `internal/tui/panels/manager.go:438-457` -- drag-start guard `if !m.layoutLocked` (verify path once default flips)
- `internal/tui/components/agentstatus.go:129-218` -- `Render()` returns header + per-agent lines + tip → multi-line growth that overflows height (C root cause)
- `internal/tui/render.go:187` -- `RenderBorder` wraps, never clips on height (C contributing cause)
- `internal/tui/app.go:687-708` -- panelManager render path; status bar appended after center; statusBar lock indicator init

## Tasks & Acceptance

**Execution:**
- [x] `internal/tui/panels/repl.go` -- A1: center REPL panel forced borderless — `hasBorder=false` at construction, `p.SetBordered(false)`, and `SetBordered` made a no-op so Ctrl+B never re-enables a center border. `hasBorder` branches in `View`/`SetSize` now never fire.
- [x] `internal/tui/panels/repl.go` -- A2: `View()` renders a full-width divider directly below `textInput`, reusing the existing `borderStyle`/`divChar`/`divW`. Overlay hitmap `linesAfterOverlay` bumped to account for the extra line.
- [x] `internal/tui/panels/repl.go` -- C: added `chromeHeight(overlayLines)` mirroring `View()`'s layout; `SetSize` now sets `chatViewport` height = inner − chrome (clamped ≥1). Recomputation is triggered from the Update handlers that change chrome (UserEcho/AgentStatusUpdate/AgentDone call `SetSize`) rather than mutating in `View()`, per the `render-no-side-effects` rule.
- [x] `internal/tui/panels/manager.go:132` -- A3: `layoutLocked` defaults to `false`; status-bar default lock segment changed to 🔓 (`statusline.go`) so the indicator matches.
- [x] `internal/tui/panels/repl_test.go` -- added `TestREPL_CenterIsBorderless`, `TestREPL_InputHasDividersAboveAndBelow`, `TestREPL_ViewNeverExceedsHeight_DuringStreaming`; updated `TestViewport_SetSizeAllocatesCorrectly` and the lock-default tests in `manager_test.go` for the new behavior.

**Acceptance Criteria:**
- Given the TUI renders, when the center panel is shown, then it has no surrounding box-drawing border while left/right plugin boxes keep theirs
- Given the REPL renders, when inspected, then a horizontal rule appears immediately above and immediately below the input line
- Given a fresh launch with a plugin box and `layoutLocked` untouched, when the user drags the plugin-box divider (±2px), then its width changes — no keybinding required first
- Given the agent is streaming with a multi-line agent-status block, when frames render, then the status bar stays the last visible line and the chat viewport shrinks to fit

## Design Notes

C is the real fix: today `chatViewport` height is fixed in `SetSize` assuming ~1 chrome line, but `agentStatus.Render` emits `header + N agents + tip` during streaming. Since `RenderBorder` wraps (never clips), the panel grows past `MaxContentHeight` and shoves the status bar off-screen. Sizing the viewport from *measured* chrome each frame caps total height at `r.height` and naturally absorbs A2's extra divider line.

## Verification

**Commands:**
- `cd /home/dollebrekel/DolleMaster/02_TOOLS/reverseEngineer && make build` -- expected: clean build
- `cd /home/dollebrekel/DolleMaster/02_TOOLS/reverseEngineer && go test -race -count=1 ./internal/tui/...` -- expected: all pass
- `cd /home/dollebrekel/DolleMaster/02_TOOLS/reverseEngineer && go vet ./internal/tui/...` -- expected: no issues

**Manual checks:**
- `make build && ./bin/siply tui` (NOT bare `siply`): confirm no center border; divider above+below input; drag the plugin-box side resizes without pressing Ctrl+Shift+L; send a prompt and confirm the status bar stays put while output streams

## Suggested Review Order

**C — Streaming status bar (the core fix; start here)**

- Entry point: viewport sized from measured chrome so the panel never exceeds height
  [`repl.go:1127`](../../internal/tui/panels/repl.go#L1127)

- `chromeHeight` mirrors View()'s layout exactly so SetSize and View agree
  [`repl.go:1200`](../../internal/tui/panels/repl.go#L1200)

- `cappedAgentView` bounds the agent-status block so many sub-agents can't overflow
  [`repl.go:1217`](../../internal/tui/panels/repl.go#L1217)

- Chrome-changing Update handlers call SetSize (not bare refresh) — keeps View() pure
  [`repl.go:215`](../../internal/tui/panels/repl.go#L215)

**A1 — Borderless center**

- Center forced borderless at construction; SetBordered cannot re-enable it
  [`repl.go:155`](../../internal/tui/panels/repl.go#L155)
  [`repl.go:1249`](../../internal/tui/panels/repl.go#L1249)

**A2 — Input dividers**

- Bottom divider added below the input; overlay hitmap offset adjusted
  [`repl.go:1097`](../../internal/tui/panels/repl.go#L1097)

**A3 — Drag regression**

- `layoutLocked` defaults to false so dividers drag out of the box
  [`manager.go:134`](../../internal/tui/panels/manager.go#L134)

- Status-bar lock indicator default matches (🔓)
  [`statusline.go:72`](../../internal/tui/statusline/statusline.go#L72)

**Tests (peripheral)**

- New borderless / dividers / height-bound (idle, streaming, many-agents) cases
  [`repl_test.go:660`](../../internal/tui/panels/repl_test.go#L660)

- Lock-default tests inverted for the new unlocked default
  [`manager_test.go:692`](../../internal/tui/panels/manager_test.go#L692)
