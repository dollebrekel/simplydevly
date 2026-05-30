---
title: 'TUI Layout Fixes — Panel Resize, Chat Flow, Status Bar, Slash Menu'
type: 'bugfix'
created: '2026-05-30'
status: 'done'
baseline_commit: '917cf94'
context:
  - '{project-root}/docs/go-best-practices.md'
  - '{project-root}/project-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Four TUI layout regressions break core usability: (1) panel dividers cannot be dragged to resize, (2) chat messages may render incorrectly instead of vertically stacked, (3) the status bar disappears when using the panelManager path, and (4) the `/` command menu renders below the panel border instead of inline above the input.

**Approach:** Debug each regression in its owning component — PanelManager drag state, REPL chat viewport, App status bar wiring, and SlashOverlay inline positioning — informed by codebase investigation of actual root causes.

## Boundaries & Constraints

**Always:**
- Preserve all existing test suites (`go test -race ./internal/tui/...` must pass)
- Follow lipgloss v2 / Bubble Tea v2 APIs (charm.land imports)
- Status bar must render on every frame when `layout.ShowStatusBar == true`
- Chat messages must stack vertically with role-based spacing (splash → 4 newlines, user↔assistant → 2 newlines)
- Panel resize must work via mouse drag on divider when `layoutLocked == false`

**Ask First:**
- Changing the slash overlay from external append to inline requires adjusting viewport height — confirm approach before implementing
- Any changes to `layoutLocked` default (currently `true` in `NewPanelManager`)

**Never:**
- Break accessible mode rendering (`VerbosityAccessible`)
- Introduce new dependencies
- Change theme tokens or color values
- Modify the splash screen component

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Panel drag resize | layoutLocked=false, click on left divider (±2px), drag right | `left.width` increases, center shrinks | Clamped by `clampSlotWidth` min/max |
| Panel drag while locked | layoutLocked=true, click on divider | No drag initiated | N/A |
| Chat vertical flow | 3 messages: user, assistant, user | Each message on own line(s), separated by `\n\n` | N/A |
| Status bar always visible | ShowStatusBar=true, panelManager path | Status bar string at bottom of View | Fallback text if statusBar==nil |
| Slash menu inline | Type `/` in input | Menu above input inside panel, chat viewport shrinks | Hidden when 0 items |
| Slash menu Tab-complete | Overlay visible, press Tab | Selected command inserted, overlay closes | Empty selection → no-op |
| Narrow terminal | Width < 60 | Side panels auto-collapse, status bar compact | N/A |

</frozen-after-approval>

## Code Map

- `internal/tui/panels/manager.go:427-476` -- Mouse drag detection: MouseClickMsg hit threshold (±2px), MouseMotionMsg delta application, `lastRenderedLeftW`/`lastRenderedRightW` set at line 514-515 from CalculateLayoutWithPanels output
- `internal/tui/panels/manager.go:132` -- `layoutLocked` defaults to `true` in NewPanelManager
- `internal/tui/panels/manager.go:550` -- `JoinHorizontal(lipgloss.Top, sections...)` for panel composition — confirmed NOT the cause of horizontal chat rendering
- `internal/tui/panels/manager.go:1332-1354` -- `clampSlotWidth` bounds checking against MinWidth/MaxWidth
- `internal/tui/panels/repl.go:686-772` -- `renderChat()` vertical message stacking with strings.Builder — confirmed correct `\n` separators
- `internal/tui/panels/repl.go:1034-1084` -- `View()` composes chatViewport + agent status + divider + textInput into panelView, then appends overlay BELOW panel at line 1075
- `internal/tui/panels/repl.go:1087-1136` -- `SetSize()` viewport height = height-4 (minus 2 more with borders)
- `internal/tui/panels/slashoverlay.go:216-237` -- `HandleKey("tab")` returns selected name via `s.SelectedName()`
- `internal/tui/panels/slashoverlay.go:283-293` -- `SetSize()` sets inner list to width-4, height-2
- `internal/tui/app.go:702-708` -- panelManager status bar path: NO fallback when `statusBar == nil` (unlike replPanel-only path at 738-751 which has fallback)
- `internal/tui/layout.go:83-101` -- `ShowStatusBar` driven by terminal height (≥15), `MaxContentHeight` reserves 1-2 lines for status bar

## Tasks & Acceptance

**Execution:**
- [x] `internal/tui/panels/manager.go` -- Debug drag detection: verified `lastRenderedLeftW`/`lastRenderedRightW` are set correctly from CalculateLayoutWithPanels (line 514-515), hit detection threshold (±2px) and delta application are correct, `clampSlotWidth` respects MinWidth/MaxWidth bounds. Mechanism works correctly when `layoutLocked=false`.
- [x] `internal/tui/panels/repl.go` -- Audited chat viewport rendering: `renderChat()` uses strings.Builder with correct `\n` separators, no JoinHorizontal anywhere in repl.go, `refreshChatViewport()` correctly sets content via `chatViewport.SetContent()`. Vertical stacking confirmed correct.
- [x] `internal/tui/app.go:702-708` -- Added fallback placeholder to panelManager status bar path: when `statusBar == nil`, writes muted "Ctrl+C to quit" hint (matching replPanel-only path pattern). Also moved the leading `\n` outside the `statusBar != nil` check so it's always written.
- [x] `internal/tui/panels/repl.go:1034-1084` -- Moved slash overlay from external append to inline rendering between divider and textInput inside the panel. `SetSize()` now reduces chatViewport height by overlayH when overlay is visible. Added `SetSize()` recalculation after every overlay Show/Hide toggle (in handleKey tab/esc/enter and updateOverlayVisibility). Hitmap registration updated for inline position.
- [x] `internal/tui/panels/slashoverlay.go` -- Verified: `HandleKey("tab")` returns selected name via `s.SelectedName()`, `list.Model` handles pagination for scrollable items, `CursorUp`/`CursorDown` navigate the list correctly.

**Acceptance Criteria:**
- Given layoutLocked=false and a terminal ≥120 columns with left panel registered, when the user clicks on the left divider (±2px) and drags right, then the left panel width increases and center content shrinks
- Given the REPL has 5 chat messages, when the panel renders, then each message appears on its own vertical block separated by appropriate newlines
- Given ShowStatusBar=true, when any render path executes (including panelManager path), then the status bar text appears as the last visible line
- Given the user types `/` in the input, when the slash overlay appears, then it renders between the chat viewport and the input line inside the panel border
- Given the slash overlay is visible and the user presses Tab, then the highlighted command name is inserted into the input

## Verification

**Commands:**
- `cd /home/dollebrekel/DolleMaster/02_TOOLS/reverseEngineer && go build ./...` -- expected: clean build
- `cd /home/dollebrekel/DolleMaster/02_TOOLS/reverseEngineer && go test -race -count=1 ./internal/tui/...` -- expected: all tests pass
- `cd /home/dollebrekel/DolleMaster/02_TOOLS/reverseEngineer && go vet ./internal/tui/...` -- expected: no issues

**Manual checks:**
- Run `siply` in a terminal ≥120 columns, toggle layoutLocked (Ctrl+Shift+L), verify divider drag resize works
- Send multiple messages and verify they stack vertically
- Confirm status bar is visible at bottom at all times
- Type `/` and verify command menu appears inline above input, is scrollable, and Tab-completes

## Suggested Review Order

**Status bar fix**

- Fallback placeholder + unconditional newline separator for panelManager path
  [`app.go:702`](../../internal/tui/app.go#L702)

**Slash overlay inline positioning**

- Core change: overlay rendered between divider and textInput inside panel content
  [`repl.go:1069`](../../internal/tui/panels/repl.go#L1069)

- Hitmap computed from panelView (post-wrapping) counting backward from end
  [`repl.go:1089`](../../internal/tui/panels/repl.go#L1089)

- SetSize reserves viewport height for overlay, caps overlayH to prevent squeeze
  [`repl.go:1137`](../../internal/tui/panels/repl.go#L1137)

**Overlay visibility → viewport recalculation**

- Tab/Esc handlers trigger SetSize to reclaim viewport space
  [`repl.go:296`](../../internal/tui/panels/repl.go#L296)

- updateOverlayVisibility tracks wasVisible, recalcs only on change
  [`repl.go:1209`](../../internal/tui/panels/repl.go#L1209)

- handleSubmit hides overlay with IsVisible guard + SetSize
  [`repl.go:410`](../../internal/tui/panels/repl.go#L410)
