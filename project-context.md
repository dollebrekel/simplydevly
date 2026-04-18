# SimplyDevly (siply) — Project Context

## Project Info

- Module: `siply.dev/siply`
- Language: Go 1.26.1
- Test: `go test -race -parallel 4 ./...` (via `make test`)
- Lint: golangci-lint (CI enforced)
- No external HTTP client library — use `net/http` stdlib

## MANDATORY: Before Implementation

Load `docs/go-best-practices.md` BEFORE writing any code.

### Loading Strategy

1. **ALWAYS** load Section: `shared` (applies to all domains)
2. Load the section matching your story's domain:
   - Backend/core logic → Section: `backend`
   - API/HTTP/providers → Section: `api`
   - TUI/Bubble Tea → Section: `frontend-tui`
   - UX patterns → Section: `ux`
3. List which patterns (by name) are relevant to your story
4. Use these patterns as guardrails during implementation

### Key Conventions (Quick Reference)

- **Error wrapping:** always use `%w` for sentinel errors (`errors.Is()` compatibility)
- **Sentinel errors:** `var ErrXxx = errors.New("message")`
- **Context:** `cmd.Context()` in cobra handlers, never `context.Background()`
- **Text length:** `utf8.RuneCountInString()`, never `len()` for user-visible strings
- **HTTP streaming:** `select` with `ctx.Done()` on every channel send
- **File writes:** `fileutil.AtomicWriteFile()` for any config/state files
