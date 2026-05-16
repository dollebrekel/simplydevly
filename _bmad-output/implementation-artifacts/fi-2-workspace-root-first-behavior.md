# Story FI.2: Workspace-Root First Behavior voor `siply tui`

Status: review

## Story

As a Siply user,  
I want `siply tui` to treat my current directory as the active workspace root and remember local context per directory,  
so that search scope, settings, and git context stay predictable between sessions without breaking global defaults.

## Acceptance Criteria

1. **Given** `siply tui` starts in directory `X`, **when** startup workspace resolution runs, **then** active workspace root is `realpath(X)` (canonical absolute path).
2. **Given** search/discovery runs in default mode, **when** results are collected, **then** only files under current workspace root are scanned.
3. **Given** zero results inside workspace and user did not explicitly allow outside scope, **when** fallback is considered, **then** no outside-workspace scan is executed.
4. **Given** zero results inside workspace and user explicitly allows outside scope, **when** fallback runs, **then** outside-workspace search is executed and clearly labeled as expanded scope.
5. **Given** workspace override settings are saved in directory `X`, **when** `siply tui` is restarted from `X`, **then** same workspace entry and overrides are restored.
6. **Given** global settings exist and workspace overrides exist for subset keys, **when** effective config is built, **then** precedence is `defaults -> global -> workspace override` and only allowed keys can be overridden locally.
7. **Given** workspace A has local overrides, **when** user starts in workspace B without those overrides, **then** A overrides do not affect B.
8. **Given** workspace is in a git repo, **when** git link metadata is persisted, **then** stored link includes branch name plus repo identity/fingerprint.
9. **Given** persisted git link metadata is invalid on next launch (repo mismatch, missing branch, detached/invalid state), **when** restore runs, **then** Siply does not crash or auto-checkout; it warns and falls back safely.
10. **Given** user starts `siply tui` in the same directory on a later run, **when** workspace resolution runs, **then** the same workspace is reselected deterministically.

## Tasks / Subtasks

### Task 1: Canonical workspace-root resolution (AC: #1, #10)
- [x] Add/extend resolver for `workspaceRoot = EvalSymlinks(Abs(cwd))`.
- [x] Ensure path normalization prevents duplicate entries from symlink/relative variants.
- [x] Add regression tests for symlink and relative path inputs.

### Task 2: Strict workspace search scope + explicit fallback (AC: #2, #3, #4)
- [x] Enforce scope guard for discovery/search to stay under workspace root by default.
- [x] Add explicit flag/toggle path for outside-workspace fallback.
- [x] Require zero in-workspace hits before outside fallback can execute.
- [x] Add tests for blocked `../` path escapes and symlink escape attempts.

### Task 3: Per-directory workspace store (AC: #5, #10)
- [x] Add workspace store entry keyed by canonical absolute root path.
- [x] Persist minimal metadata: `root`, `created_at`, `last_used_at`, `overrides`, `linked_branch_state`.
- [x] Persist with atomic write semantics and clear recovery behavior on corrupt file.
- [x] Restore workspace deterministically on next launch from same directory.

### Task 4: Global vs workspace config layering (AC: #6, #7)
- [x] Implement merge order `defaults -> global -> workspace`.
- [x] Restrict workspace overrides to explicit allowlist keys.
- [x] Keep credentials/secrets global only (not duplicated into workspace store).
- [x] Add visibility metadata in runtime/UI model for setting source (`default|global|workspace`).

### Task 5: Safe git branch linkage per workspace (AC: #8, #9)
- [x] Persist branch metadata with repo identity (repo root + optional remote hash/fingerprint).
- [x] Validate linkage on restore (repo match, branch exists, non-destructive behavior).
- [x] Never auto-switch branch during startup; metadata sync + warning/relink path only.
- [x] Add tests for missing branch, repo mismatch, and detached HEAD scenarios.

### Task 6: Startup flow integration in `siply tui` (AC: #1-#10)
- [x] Wire startup sequence: resolve root -> load global -> load workspace entry -> validate git link -> build effective config.
- [x] Ensure same directory rehydrates same workspace before TUI becomes interactive.
- [x] Add focused integration tests for restart behavior across two directories.

## Dev Notes

### Decision Contract (agreed)
- Workspace is directory-first: current directory determines root.
- Default behavior is safety-first: no implicit outside-workspace scanning.
- Workspace overrides are local deltas, never full config duplication.
- Global configuration remains valid baseline everywhere.
- Git linkage is contextual metadata, not an instruction to mutate git state.

### Implementation Guidance
- Prefer a dedicated workspace store file under user config/state dir (not in repo).
- Use canonical path keys consistently in read/write and lookup.
- Store schema version to allow future migrations.
- Handle failures with actionable warnings, not hard crashes during startup.

### Suggested Data Shape (illustrative; actual store format is YAML in `workspace-roots.yaml`)
```yaml
version: 1
workspaces:
  /abs/path/project-a:
    root: /abs/path/project-a
    created_at: "2026-05-16T18:40:00Z"
    last_used_at: "2026-05-16T19:05:00Z"
    overrides:
      provider: openai
      model: gpt-5
    linked_branch_state:
      repo_root: /abs/path/project-a
      repo_fingerprint: sha256:...
      branch: feature/workspace-root
```

### Test Matrix (minimum)
- Unit: root canonicalization, store read/write, merge precedence, allowlist enforcement, git link validation.
- Integration: launch in dir A -> save override -> relaunch dir A restores; launch dir B isolated from A; explicit outside-scope fallback path only after zero hits.
- Regression: no automatic branch checkout, no global config mutation when saving workspace override.

## References

- [Source: _bmad-output/implementation-artifacts/4-3-workspace-management.md]
- [Source: project-context.md]
- [Source: Party-mode roundtable consensus (Winston, John, Amelia, Mary) on 2026-05-16]

## Dev Agent Record

### Agent Model Used

gpt-5

### Debug Log References

- Party-mode discussion context and agent outputs (2026-05-16)

### Completion Notes List

- Story created from agreed architecture + product decision package.
- Implemented a new workspace-root-first persistence layer (`workspace-roots.yaml`) with canonical root resolution, atomic writes, and corrupt-file recovery.
- Added deterministic per-directory restore semantics and allowlisted local override application helper with source metadata mapping.
- Wired `siply tui` startup to resolve and persist canonical workspace root and validate git link metadata non-destructively.
- Added regression tests for canonicalization, deterministic restore, corrupt recovery, and override allowlist behavior.
- Added strict workspace search scope enforcement with explicit outside-scope fallback only after zero in-workspace hits, plus path-escape regression tests.
- Added git-link validation tests for repo mismatch, missing branch, and detached HEAD behavior.
- Added integration coverage for restart behavior and workspace override isolation across two directories.

### File List

- `_bmad-output/implementation-artifacts/fi-2-workspace-root-first-behavior.md`
- `internal/workspace/root_store.go`
- `internal/workspace/root_store_test.go`
- `cmd/siply/tui.go`
- `internal/tools/search.go`
- `internal/tools/search_test.go`
- `test/integration/workspace_root_store_integration_test.go`

### Change Log

- 2026-05-16: Implemented workspace-root-first store foundation (canonical root resolution, persistence, git-link validation) and integrated startup usage in `siply tui`.
- 2026-05-16: Completed workspace-scoped search containment/fallback behavior and runtime provider layering with workspace override allowlist and source metadata.
