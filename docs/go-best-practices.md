# Go Best Practices — AI Agent Reference

> **Purpose:** AI-optimized reference document for specialized Go sub-agents.
> **Format:** Each pattern is self-contained. Agents load ONLY the section matching their domain/tags.
> **Source:** All examples are from the siply.dev codebase.
> **Maintained by:** Code review process — new patterns added as they are discovered.

---

## Document Structure

```
Section: {domain}
  Pattern: {name}
    Tags: [tag1, tag2, ...]
    ... pattern content ...
```

**Domains:** `backend`, `api`, `frontend-tui`, `ux`, `shared`
**Load strategy:** Agent reads ONLY patterns matching its domain + relevant tags.

---

## Section: shared

Patterns that apply across ALL Go domains. Every agent MUST load this section.

---

### Pattern: context-cancellation-goroutines

**Tags:** `concurrency`, `goroutine`, `context`, `streaming`, `error-handling`
**Domain:** shared
**Severity:** critical
**Discovered in:** `internal/providers/anthropic/adapter.go`, `internal/agent/agent.go`

#### Problem Summary

When a goroutine sends events on a channel and the caller cancels via `context.Context`, the goroutine must stop. Without a `select` on `ctx.Done()`, the goroutine blocks forever on `ch <-` if nobody reads, causing a goroutine leak. For HTTP streaming, the response body must also be closed to unblock the scanner.

#### Where It Happened

- `internal/providers/anthropic/adapter.go` — SSE goroutine blocked on channel send after cancellation
- `internal/agent/agent.go` — nil stream channel blocked forever

#### Why It Went Wrong

The initial implementation sent events with `ch <- event` without checking if the context was still active. When the consumer stopped reading (due to cancellation), the send blocked indefinitely. The goroutine leaked, holding the HTTP connection open.

#### Bad Example

```go
go func() {
    for scanner.Scan() {
        event := parseEvent(scanner.Text())
        ch <- event  // BLOCKS if nobody reads — goroutine leak!
    }
    close(ch)
}()
```

#### Good Example

Source: `internal/providers/anthropic/adapter.go:148-194`

```go
func (a *Adapter) readStream(ctx context.Context, body io.ReadCloser, ch chan<- core.StreamEvent) {
    defer close(ch)
    defer body.Close()

    // Close body on context cancel to unblock scanner.
    done := make(chan struct{})
    defer close(done)
    go func() {
        select {
        case <-ctx.Done():
            body.Close()
        case <-done:
        }
    }()

    // ... parsing logic ...

    // Select before every channel send:
    if event != nil {
        select {
        case ch <- event:
        case <-ctx.Done():
            return
        }
    }
}
```

#### Rule

Every goroutine that sends on a channel MUST use `select` with `ctx.Done()`. For HTTP streaming, close `resp.Body` on cancel to unblock the scanner.

#### Detection Signals

- Bare `ch <- value` inside a goroutine (no `select`)
- `go func()` that reads from `io.Reader` without a cancellation path
- HTTP response body not closed on context cancellation
- Missing `defer close(ch)` in goroutine

#### What The Agent Must Do Instead

1. Wrap every channel send in `select { case ch <- event: case <-ctx.Done(): return }`
2. Spawn a helper goroutine that closes `resp.Body` on `<-ctx.Done()`
3. Use a `done` channel to prevent the helper goroutine from leaking when the stream ends normally
4. Always `defer close(ch)` at the top of the goroutine

#### Scope

Applies to: any goroutine that sends on a channel, any HTTP streaming reader, any long-running background operation.
Does NOT apply to: synchronous functions, buffered channels with guaranteed capacity.

---

### Pattern: transport-timeout-not-client-timeout

**Tags:** `api`, `http`, `streaming`, `timeout`, `configuration`
**Domain:** shared
**Severity:** critical
**Discovered in:** `internal/providers/anthropic/adapter.go`

#### Problem Summary

Go's `http.Client.Timeout` applies to the ENTIRE request lifecycle — including reading the response body. For streaming responses (SSE, NDJSON) that can last minutes, a 30-second client timeout kills the stream mid-response. Use `Transport.ResponseHeaderTimeout` instead, which only limits the time waiting for response headers.

#### Where It Happened

- `internal/providers/anthropic/adapter.go` — 30s client timeout killed streaming after 30 seconds

#### Why It Went Wrong

The developer used `http.Client{Timeout: 30 * time.Second}` assuming it only applied to connection setup. Go's documentation is not obvious about this behavior — `Timeout` covers the entire round-trip including body reads.

#### Bad Example

```go
client := &http.Client{
    Timeout: 30 * time.Second,  // Kills streaming responses after 30s!
}
```

#### Good Example

Source: `internal/providers/anthropic/adapter.go:55-63`

```go
a.client = &http.Client{
    Transport: &http.Transport{
        DialContext: (&net.Dialer{
            Timeout: dialTimeout,               // 10s — connection only
        }).DialContext,
        TLSHandshakeTimeout:   tlsHandshakeTimeout,   // 10s
        ResponseHeaderTimeout: responseHeaderTimeout,  // 30s — headers only, body unlimited
    },
}
```

#### Rule

NEVER use `http.Client.Timeout` for streaming HTTP. Use `Transport.ResponseHeaderTimeout` for header deadline and `Transport.DialContext` with `net.Dialer.Timeout` for connection deadline.

#### Detection Signals

- `&http.Client{Timeout: ...}` in code that handles streaming responses
- Any HTTP client used with SSE, NDJSON, or chunked transfer encoding that has a `Timeout` field set
- Streaming that "randomly" disconnects after a fixed period

#### What The Agent Must Do Instead

1. Set `Transport.ResponseHeaderTimeout` for the maximum time to wait for first response headers
2. Set `net.Dialer.Timeout` for TCP connection timeout
3. Set `Transport.TLSHandshakeTimeout` for TLS negotiation
4. Leave `http.Client.Timeout` at zero (no overall timeout)
5. Use `context.Context` with deadlines for per-request control

#### Scope

Applies to: all HTTP clients that handle streaming responses (SSE, NDJSON, chunked).
Does NOT apply to: simple request/response HTTP calls where `Client.Timeout` is fine.

---

### Pattern: nil-guard-interface-methods

**Tags:** `error-handling`, `nil-safety`, `defensive`, `interface`
**Domain:** shared
**Severity:** high
**Discovered in:** `internal/providers/events.go`, `internal/providers/anthropic/adapter.go`, `internal/agent/agent.go`

#### Problem Summary

In Go, struct fields can be `nil`. Calling a method on a nil field causes a panic. This pattern appeared 4 times across 2 stories — on `ErrorEvent.Err`, `Adapter.client`, empty message slices, and nil stream channels.

#### Where It Happened

- `internal/providers/events.go` — `ErrorEvent.Error()` panicked on nil `Err`
- `internal/providers/anthropic/adapter.go` — `Query()` panicked on nil `client` (Init not called)
- `internal/agent/context_manager.go` — `Compact()` panicked on empty messages slice
- `internal/agent/agent.go` — nil stream channel blocked forever

#### Why It Went Wrong

The developer assumed callers would always use the correct initialization sequence. In practice, methods can be called before `Init()`, with empty inputs, or when upstream errors produced nil values.

#### Bad Example

```go
func (e *ErrorEvent) Error() string {
    return e.Err.Error()  // PANIC if e.Err == nil
}

func (a *Adapter) Query(ctx context.Context, req QueryRequest) (<-chan StreamEvent, error) {
    resp, err := a.client.Do(buildReq(req))  // PANIC if a.client == nil
```

#### Good Example

Source: `internal/providers/events.go:41-46`, `internal/providers/anthropic/adapter.go:67-69`

```go
func (e *ErrorEvent) Error() string {
    if e.Err == nil {
        return "unknown error"
    }
    return e.Err.Error()
}

func (a *Adapter) Query(ctx context.Context, req QueryRequest) (<-chan StreamEvent, error) {
    if a.client == nil {
        return nil, fmt.Errorf("anthropic: adapter not initialized, call Init() first")
    }
```

#### Rule

Every public method that uses a struct field: add a nil/zero guard clause at the top BEFORE using the field.

#### Detection Signals

- Method that dereferences a pointer field without nil check
- Method that indexes a slice without length check
- Method that uses a `map` field without nil check
- Public method with no guard clauses at top

#### What The Agent Must Do Instead

1. First line(s) of every public method: check all required fields are non-nil/non-zero
2. Return a descriptive `fmt.Errorf("package: description, call Init() first")` for uninitialized state
3. For slices: check `len(slice) == 0` before indexing
4. For channels: check `ch == nil` before ranging

#### Scope

Applies to: all public methods on structs with pointer/interface/slice/map/channel fields.
Does NOT apply to: private helper functions called only from validated contexts.

---

### Pattern: scanner-buffer-size

**Tags:** `streaming`, `parsing`, `buffer`, `sse`, `api`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/providers/anthropic/stream.go`

#### Problem Summary

Go's `bufio.Scanner` has a default maximum line size of 64KB. SSE data lines from AI providers can exceed this — especially tool call responses with large JSON payloads. When a line exceeds the buffer, the scanner stops silently with `bufio.ErrTooLong`.

#### Where It Happened

- `internal/providers/anthropic/stream.go` — large tool call JSON exceeded 64KB default

#### Why It Went Wrong

The developer used `bufio.NewScanner(reader)` without setting a custom buffer. Go's default is not documented prominently.

#### Bad Example

```go
scanner := bufio.NewScanner(resp.Body)
// Default 64KB max — silently fails on large SSE data lines
```

#### Good Example

Source: `internal/providers/anthropic/stream.go:38-39`

```go
s := bufio.NewScanner(r)
s.Buffer(make([]byte, 64*1024), 1024*1024)  // Start 64KB, max 1MB
```

#### Rule

When parsing external streams with `bufio.Scanner`, always set an explicit buffer size with `scanner.Buffer()`.

#### Detection Signals

- `bufio.NewScanner(...)` without a following `scanner.Buffer(...)` call
- Scanner used to read from HTTP response bodies or network streams
- Silent data loss or unexpected EOF when processing large payloads

#### What The Agent Must Do Instead

1. Always call `scanner.Buffer(make([]byte, 64*1024), maxSize)` after `bufio.NewScanner()`
2. Choose `maxSize` based on expected data: 1MB for SSE/NDJSON, adjust for specific use cases
3. Check `scanner.Err()` after the scan loop to catch `bufio.ErrTooLong`

#### Scope

Applies to: any `bufio.Scanner` reading from network streams, pipes, or files with potentially long lines.
Does NOT apply to: scanners reading controlled internal data with known line length limits.

---

### Pattern: deterministic-map-iteration

**Tags:** `concurrency`, `map`, `ordering`, `testing`, `determinism`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/providers/openai/stream.go`, `internal/providers/openrouter/stream.go`

#### Problem Summary

Go map iteration order is intentionally randomized. When emitting events, building responses, or producing output from a map, the order changes between runs. This causes non-deterministic behavior and flaky tests.

#### Where It Happened

- `internal/providers/openai/stream.go:196` — `emitToolCalls()` iterated map in random order
- `internal/providers/openrouter/stream.go:181` — same pattern, same bug

#### Why It Went Wrong

The developer iterated `for _, tool := range activeTools` where `activeTools` is a `map[int]*toolAccumulator`. Each run produced tool call events in different order, causing test assertions to fail intermittently.

#### Bad Example

```go
func emitToolCalls(tools map[int]*ToolState, ch chan Event) {
    for _, tool := range tools {  // RANDOM order every run!
        ch <- ToolCallEvent{Name: tool.Name, Input: tool.JSON}
    }
}
```

#### Good Example

Source: `internal/providers/openai/stream.go:196-227`

```go
func (p *streamParser) emitToolCalls() (core.StreamEvent, error) {
    keys := make([]int, 0, len(p.activeTools))
    for k := range p.activeTools {
        keys = append(keys, k)
    }
    sort.Ints(keys)  // Deterministic order

    var events []core.StreamEvent
    for _, k := range keys {
        acc := p.activeTools[k]
        events = append(events, &providers.ToolCallEvent{
            ToolName: acc.Name,
            ToolID:   acc.ID,
            Input:    json.RawMessage(acc.JSONBuf.String()),
        })
    }
    // ...
}
```

#### Rule

When output order matters (events, logs, API responses, test assertions): extract map keys into a slice, sort it, then iterate the sorted keys.

#### Detection Signals

- `for _, v := range someMap` where the output is user-visible or tested for order
- Flaky tests that pass sometimes and fail sometimes
- Non-deterministic log output or event ordering

#### What The Agent Must Do Instead

1. Extract keys: `keys := make([]T, 0, len(m)); for k := range m { keys = append(keys, k) }`
2. Sort keys: `sort.Ints(keys)` or `sort.Strings(keys)` or `slices.Sort(keys)`
3. Iterate sorted keys: `for _, k := range keys { v := m[k]; ... }`
4. If order truly doesn't matter, add a comment: `// Order-independent iteration`

#### Scope

Applies to: any map iteration where output is observable (events, responses, logs, test assertions).
Does NOT apply to: internal processing where order is irrelevant (counting, aggregation, existence checks).

---

### Pattern: mutex-shared-state

**Tags:** `concurrency`, `goroutine`, `state`, `race-condition`, `sync`
**Domain:** shared
**Severity:** critical
**Discovered in:** `internal/agent/agent.go`, `internal/permission/evaluator.go`

#### Problem Summary

When multiple goroutines read and write the same struct field (e.g., `cancel`, `running`, `config.Mode`), Go's race detector flags a data race. This can cause corrupted state, panics, or silent incorrect behavior.

#### Where It Happened

- `internal/agent/agent.go` — `Stop()` and `Run()` both accessed `a.cancel` without synchronization
- `internal/permission/evaluator.go` — `SetMode()` and `EvaluateAction()` both accessed `config.Mode`

#### Why It Went Wrong

The developer assumed `Stop()` and `Run()` would never be called concurrently. In practice, signal handlers call `Stop()` while `Run()` is still executing.

#### Bad Example

```go
type Agent struct {
    cancel context.CancelFunc  // No protection!
}
func (a *Agent) Run(ctx context.Context, msg string) error {
    ctx, a.cancel = context.WithCancel(ctx)  // WRITE
}
func (a *Agent) Stop(_ context.Context) error {
    if a.cancel != nil { a.cancel() }         // READ + CALL — RACE!
}
```

#### Good Example

Source: `internal/agent/agent.go:33-112`

```go
type Agent struct {
    mu      sync.Mutex
    cancel  context.CancelFunc
    running bool
}

func (a *Agent) Run(ctx context.Context, msg string) error {
    a.mu.Lock()
    if a.running {
        a.mu.Unlock()
        return fmt.Errorf("agent: Run already in progress")
    }
    a.running = true
    ctx, cancel := context.WithCancel(ctx)
    a.cancel = cancel
    a.mu.Unlock()
    defer func() {
        cancel()
        a.mu.Lock()
        a.cancel = nil
        a.running = false
        a.mu.Unlock()
    }()
    // ...
}

func (a *Agent) Stop(_ context.Context) error {
    a.mu.Lock()
    cancel := a.cancel
    a.mu.Unlock()
    if cancel != nil { cancel() }
    return nil
}
```

#### Rule

Every struct field accessed by multiple goroutines MUST be protected by `sync.Mutex` (or `sync.RWMutex` for read-heavy access). Always run tests with `-race` flag.

#### Detection Signals

- Struct field written in one method and read in another, both callable concurrently
- Missing `sync.Mutex` in struct that has `Start`/`Stop`/`Run` methods
- `go test -race` failures
- Struct accessed from HTTP handlers and background goroutines

#### What The Agent Must Do Instead

1. Add `mu sync.Mutex` to the struct
2. Lock before write, unlock after write (or use `defer a.mu.Unlock()`)
3. For read-heavy patterns: use `sync.RWMutex` with `RLock`/`RUnlock` for reads
4. Copy the value under lock, then use the copy outside the lock (avoid holding lock during slow operations)
5. Always run `go test -race ./...` to verify

#### Scope

Applies to: any struct with methods callable from multiple goroutines (servers, agent loops, background tasks).
Does NOT apply to: structs used only within a single goroutine, local variables.

---

### Pattern: lifecycle-init-start-stop

**Tags:** `lifecycle`, `initialization`, `cleanup`, `defer`, `error-handling`
**Domain:** shared
**Severity:** high
**Discovered in:** `cmd/siply/run.go`, `internal/routing/provider.go`

#### Problem Summary

Go has no built-in lifecycle management. Components with `Init()`, `Start()`, `Stop()` methods must be called in the correct order, and `Stop()` must always run — even on errors. The `defer Stop()` must use `context.Background()`, not the caller's context which may already be cancelled.

#### Where It Happened

- `cmd/siply/run.go` — `Start()` never called, `Stop()` never deferred
- `cmd/siply/run.go` — deferred `Stop()` used caller context (already cancelled on shutdown)
- `internal/routing/provider.go` — partial Init failure leaked initialized providers

#### Why It Went Wrong

The developer wired `Init()` but forgot `Start()` and `defer Stop()`. When `Stop()` was added, it used the same `ctx` parameter — which was already cancelled when the shutdown path ran, causing `Stop()` to fail immediately.

#### Bad Example

```go
func runTask(ctx context.Context) error {
    agent.Init(ctx)
    // Start() never called!
    err := agent.Run(ctx, task)
    // Stop() never called — resources leak!
    return err
}

// Or: Stop with caller context
defer agent.Stop(ctx)  // ctx is cancelled on shutdown — Stop() fails!
```

#### Good Example

Source: `cmd/siply/run.go:100-161`

```go
// Init all components
for _, c := range components {
    if err := c.lc.Init(ctx); err != nil {
        return fmt.Errorf("run: init %s: %w", c.name, err)
    }
}
// Start all components
for _, c := range components {
    if err := c.lc.Start(ctx); err != nil {
        return fmt.Errorf("run: start %s: %w", c.name, err)
    }
}
// Stop in REVERSE order with context.Background()
defer func() {
    stopCtx := context.Background()
    for i := len(components) - 1; i >= 0; i-- {
        _ = components[i].lc.Stop(stopCtx)
    }
}()
```

Source: `internal/routing/provider.go:45-59` — partial failure rollback:

```go
func (r *RoutingProvider) Init(ctx context.Context) error {
    var initialized []core.Provider
    for name, p := range r.providers {
        if err := p.Init(ctx); err != nil {
            for _, ip := range initialized {
                _ = ip.Stop(ctx)
            }
            return fmt.Errorf("routing: init provider %q: %w", name, err)
        }
        initialized = append(initialized, p)
    }
    return nil
}
```

#### Rule

Always: `Init()` then `Start()` then `defer Stop(context.Background())`. For multi-component init: rollback on partial failure. Stop in reverse order.

#### Detection Signals

- `Init()` called without corresponding `Start()` or `defer Stop()`
- `Stop()` using the same context as the request (not `context.Background()`)
- Multiple components initialized without rollback on partial failure
- Resource leaks on error paths

#### What The Agent Must Do Instead

1. Call `Init()` → check error → call `Start()` → check error → `defer Stop(context.Background())`
2. Use `context.Background()` in deferred Stop — NEVER the request context
3. For multi-component: track initialized list, rollback on failure
4. Stop in reverse order of start (stack unwinding)
5. Ignore errors from `Stop()` in cleanup paths (log but don't fail)

#### Scope

Applies to: all components implementing `Lifecycle` (Init/Start/Stop/Health) interface.
Does NOT apply to: simple structs without lifecycle requirements.

---

### Pattern: input-validation-guard-clauses

**Tags:** `validation`, `error-handling`, `defensive`, `api-boundary`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/providers/anthropic/adapter.go`, `internal/tools/file_edit.go`, `internal/tools/registry.go`

#### Problem Summary

Missing input validation causes cryptic errors deep in business logic. Guard clauses at function entry catch bad input early with descriptive, tool-name-prefixed error messages.

#### Where It Happened

- `internal/providers/anthropic/adapter.go` — empty Messages slice caused opaque API error
- `internal/tools/file_edit.go` — empty `old_string` caused confusing match behavior
- `internal/tools/registry.go` — missing default case in verdict switch

#### Why It Went Wrong

The developer went straight to business logic without validating inputs. An empty string, nil slice, or unexpected enum value caused failures far from the root cause, making debugging difficult.

#### Bad Example

```go
func (t *FileEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
    var params fileEditInput
    json.Unmarshal(input, &params)
    // params.OldString is "" — strings.Count returns every position!
    content := readFile(params.Path)
    content = strings.Replace(content, params.OldString, params.NewString, 1)
    // ... confusing behavior, no clear error
```

#### Good Example

Source: `internal/tools/file_edit.go:29-39`

```go
func (t *FileEditTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
    var params fileEditInput
    if err := json.Unmarshal(input, &params); err != nil {
        return "", fmt.Errorf("file_edit: invalid input: %w", err)
    }
    if params.Path == "" {
        return "", fmt.Errorf("file_edit: path is required")
    }
    if params.OldString == "" {
        return "", fmt.Errorf("file_edit: old_string must not be empty")
    }
    // Business logic only starts here — all inputs are validated
```

#### Rule

Every public function: validate ALL inputs at entry with guard clauses. Prefix errors with `package_or_tool_name:`. Return early on invalid input.

#### Detection Signals

- Function body starts with business logic, no validation
- `json.Unmarshal` without error check
- String parameters used without empty check
- Switch statement without default case
- Error messages without component prefix

#### What The Agent Must Do Instead

1. First lines: unmarshal and check error
2. Then: validate each required field (empty string, nil, out of range)
3. Error format: `fmt.Errorf("component: description: %w", err)`
4. Return immediately on validation failure — never continue to business logic
5. Switch statements: always include `default:` case

#### Scope

Applies to: all public functions, tool Execute methods, API handlers, interface implementations.
Does NOT apply to: private helper functions called only from already-validated contexts (but document the assumption).

---

## Section: api

Patterns specific to HTTP/gRPC API adapters and external service communication.

---

### Pattern: tool-json-size-limit

**Tags:** `api`, `streaming`, `validation`, `memory`, `security`
**Domain:** api
**Severity:** medium
**Discovered in:** `internal/providers/anthropic/stream.go`

#### Problem Summary

When accumulating tool call JSON from streaming chunks, unbounded accumulation can exhaust memory. A malicious or buggy provider could send infinite JSON input data.

#### Where It Happened

- `internal/providers/anthropic/stream.go` — no size limit on tool JSON accumulation

#### Why It Went Wrong

The SSE parser accumulated `partial_json` deltas in a `bytes.Buffer` without checking the total size. Each delta was appended unconditionally.

#### Bad Example

```go
func handleDelta(tb *toolBlock, partialJSON string) {
    tb.JSONBuf.WriteString(partialJSON)  // No limit — can grow forever!
}
```

#### Good Example

Source: `internal/providers/anthropic/stream.go:14,217-219`

```go
const maxToolJSONSize = 10 * 1024 * 1024 // 10MB

// Inside handleContentBlockDelta:
if tb.JSONBuf.Len()+len(cbd.Delta.PartialJSON) > maxToolJSONSize {
    return nil, fmt.Errorf("anthropic: tool call input exceeds maximum size (%d bytes)", maxToolJSONSize)
}
tb.JSONBuf.WriteString(cbd.Delta.PartialJSON)
```

#### Rule

Always set a maximum size when accumulating data from external sources. Check BEFORE writing, not after.

#### Detection Signals

- `bytes.Buffer` or `strings.Builder` growing from external input without size check
- Streaming accumulation loop without a break condition
- Missing `const max...Size` declaration near accumulation code

#### What The Agent Must Do Instead

1. Declare a `const maxSize` at package level
2. Check `buffer.Len() + len(newData) > maxSize` BEFORE writing
3. Return a descriptive error including the limit value
4. Also validate accumulated JSON with `json.Valid()` before using it

#### Scope

Applies to: any streaming accumulation from external sources (API responses, file uploads, plugin data).
Does NOT apply to: internal data structures with known bounded sizes.

---

### Pattern: multi-line-sse-data

**Tags:** `api`, `streaming`, `sse`, `parsing`
**Domain:** api
**Severity:** low
**Discovered in:** `internal/providers/anthropic/stream.go`

#### Problem Summary

Per the SSE specification, a single event can have multiple `data:` lines. These must be concatenated with newlines. Overwriting instead of accumulating causes data loss.

#### Where It Happened

- `internal/providers/anthropic/stream.go` — multi-line SSE data fields were overwritten

#### Why It Went Wrong

The parser stored the data line in a single variable: `data = line[6:]`. If an event had multiple `data:` lines, only the last one survived.

#### Bad Example

```go
case strings.HasPrefix(line, "data: "):
    data = line[6:]  // Overwrites previous data line!
```

#### Good Example

Source: `internal/providers/anthropic/stream.go` (accumulation pattern)

```go
case strings.HasPrefix(line, "data: "):
    if data.Len() > 0 {
        data.WriteByte('\n')  // SSE spec: join with newline
    }
    data.WriteString(line[6:])
```

#### Rule

SSE parsers must accumulate `data:` lines with newline separators, not overwrite.

#### Detection Signals

- SSE parser with `data = line[...]` (assignment, not append)
- Single `string` variable for SSE data instead of `strings.Builder` or `bytes.Buffer`

#### What The Agent Must Do Instead

1. Use `strings.Builder` or `bytes.Buffer` for SSE data accumulation
2. Join multiple `data:` lines with `\n` per SSE specification
3. Reset the buffer on empty line (event boundary)

#### Scope

Applies to: SSE stream parsers.
Does NOT apply to: NDJSON parsers (single-line JSON objects), OpenAI SSE (single-line data payloads in practice).

---

### Pattern: json-validation-before-use

**Tags:** `api`, `validation`, `json`, `tool-calls`
**Domain:** api
**Severity:** medium
**Discovered in:** `internal/providers/anthropic/stream.go`, `internal/providers/openai/stream.go`

#### Problem Summary

Tool call JSON accumulated from streaming chunks may be incomplete or malformed. Using it without validation causes downstream parsing failures with unhelpful error messages.

#### Where It Happened

- `internal/providers/anthropic/stream.go` — accumulated tool JSON emitted without validation

#### Why It Went Wrong

The parser emitted a `ToolCallEvent` with the accumulated JSON bytes immediately after the `content_block_stop` signal, without checking if the JSON was syntactically valid.

#### Bad Example

```go
case "content_block_stop":
    event := &providers.ToolCallEvent{
        Input: json.RawMessage(tb.JSONBuf.String()),  // May be invalid JSON!
    }
```

#### Good Example

Source: `internal/providers/openai/stream.go:206-209`

```go
inputJSON := acc.JSONBuf.String()
if !json.Valid([]byte(inputJSON)) {
    return nil, fmt.Errorf("openai: tool call %q produced invalid JSON input", acc.Name)
}
events = append(events, &providers.ToolCallEvent{
    Input: json.RawMessage(inputJSON),
})
```

#### Rule

Always validate accumulated JSON with `json.Valid()` before emitting it as `json.RawMessage`.

#### Detection Signals

- `json.RawMessage(someString)` without prior `json.Valid()` check
- Accumulated JSON bytes from streaming passed directly to consumers
- Missing error handling for malformed JSON in stream parsers

#### What The Agent Must Do Instead

1. After accumulation is complete, call `json.Valid([]byte(accumulated))`
2. If invalid: return error with tool name and context
3. Only then wrap in `json.RawMessage` and emit

#### Scope

Applies to: any code that accumulates JSON from streaming sources before passing it downstream.
Does NOT apply to: JSON parsed by `json.Unmarshal` (which validates implicitly).

---

### Pattern: deep-copy-before-merge

**Tags:** `map`, `pointer`, `mutation`, `merge`, `defensive`
**Domain:** shared
**Severity:** high
**Discovered in:** `internal/config/loader.go`, `internal/config/loader_test.go`

#### Problem Summary

When merging config layers (global → project → lockfile), `maps.Copy` on a base map mutates the original. Pointer fields (`*bool`, `*int`) share memory when copied by reference. Both cause invisible cross-layer contamination.

#### Bad Example

```go
// Map mutation: modifies base map!
maps.Copy(base.Plugins, override.Plugins)

// Pointer aliasing: both point to same bool!
merged.Routing.Enabled = override.Routing.Enabled  // *bool — shares memory
```

#### Good Example

Source: `internal/config/loader.go` (post-review fix)

```go
// Deep-copy base map first
merged := make(map[string]any, len(base.Plugins))
for k, v := range base.Plugins {
    merged[k] = v
}
maps.Copy(merged, override.Plugins)

// Copy pointed-to value, not pointer
if override.Routing.Enabled != nil {
    val := *override.Routing.Enabled
    merged.Routing.Enabled = &val
}
```

#### Rule

1. Always deep-copy maps before merging — never `maps.Copy` directly into the source.
2. Copy `*bool`, `*int`, `*time.Time` values, not pointers: `val := *src; dst = &val`.

#### Detection Signals

- `maps.Copy(base, overlay)` where `base` is reused later
- Pointer field assignment without dereferencing: `dst.Field = src.Field` where both are `*T`
- Tests that pass individually but fail together (shared state between test cases)

---

### Pattern: file-io-safety

**Tags:** `file`, `permissions`, `toctou`, `crash-safety`, `backup`
**Domain:** shared
**Severity:** high
**Discovered in:** `internal/config/loader.go`, `internal/credential/file_store.go`, `internal/workspace/manager.go`, `internal/storage/file.go`

#### Problem Summary

Five recurring file I/O issues discovered across Epic PB and Epic 4: (1) permissions not enforced after write, (2) TOCTOU race in stat-then-read, (3) writers assume parent dirs exist, (4) backup errors swallowed silently, (5) `.bak` suffix not rejected in storage paths.

#### Rules

**Rule 1: Always `os.Chmod` after `os.WriteFile`**
`os.WriteFile` only sets mode on file creation, not on existing files. Always follow with `os.Chmod`.

```go
// CORRECT — enforces permissions on existing files too
if err := os.WriteFile(path, data, 0600); err != nil { return err }
if err := os.Chmod(path, 0600); err != nil { return err }
```

Source: Epic PB.4 review (P6), established pattern in `licensing/validator.go:219-225`

**Rule 2: Open-then-stat for file reads**
Don't `os.Stat` then `os.ReadFile` — the file can change between calls (TOCTOU). Open the file first, then `Stat` the handle, then read with `io.LimitReader`.

```go
// CORRECT — no TOCTOU gap
f, err := os.Open(path)
if err != nil { return err }
defer f.Close()
info, err := f.Stat()
if err != nil { return err }
if info.Size() > maxSize { return fmt.Errorf("config: file exceeds limit") }
data, err := io.ReadAll(io.LimitReader(f, maxSize))
```

Source: Story 4-1 review (F3)

**Rule 3: `MkdirAll` in writers — never assume parent dirs exist**
Any function that creates a file must ensure its parent directory exists first.

```go
// CORRECT
if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { return err }
```

Source: Story 4-5 review — `NewTranscriptWriter` failed without parent dirs

**Rule 4: Backup errors must propagate**
If creating a `.bak` backup fails, the write operation must also fail. Don't swallow backup errors.

```go
// BAD — swallows backup error
_ = copyFile(path, path+".bak")
os.WriteFile(path, newData, 0644)

// GOOD — propagates backup failure
if err := copyFile(path, path+".bak"); err != nil {
    return fmt.Errorf("storage: backup failed: %w", err)
}
```

Source: Story 4-5 review — `backupIfExists` swallowed errors

**Rule 5: Reject `.bak` suffix in storage paths**
Storage paths are logical keys. Allowing `.bak` suffix lets users overwrite backup files directly.

```go
if strings.HasSuffix(path, ".bak") {
    return fmt.Errorf("storage: .bak suffix not allowed in paths")
}
```

Source: Story 4-5 review — `validatePath` missing `.bak` check

#### Detection Signals

- `os.WriteFile` without following `os.Chmod`
- `os.Stat` followed by `os.ReadFile` on same path
- File writer without `os.MkdirAll` for parent directory
- `_ = someBackupOperation()` (ignored error)
- Storage path validation missing `.bak` check

---

### Pattern: json-yaml-strict-validation

**Tags:** `json`, `yaml`, `validation`, `parsing`, `forward-compatibility`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/config/loader.go`, `internal/config/lockfile.go`

#### Problem Summary

Three JSON/YAML validation issues: (1) trailing data after valid JSON not detected, (2) version fields not validated on parse, (3) `bufio.Scanner` default 64KB buffer too small for JSONL entries.

#### Rules

**Rule 1: Detect JSON trailing data with `dec.More()`**

```go
dec := json.NewDecoder(bytes.NewReader(data))
dec.DisallowUnknownFields()
if err := dec.Decode(&result); err != nil { return err }
if dec.More() {
    return fmt.Errorf("lockfile: unexpected trailing data after JSON")
}
```

Source: Story 4-1 review (F8)

**Rule 2: Validate version/schema fields on parse**
Don't accept empty or unknown versions silently — reject them explicitly.

```go
if lf.Version == "" {
    return nil, fmt.Errorf("lockfile: missing version field")
}
if lf.Version != "1" {
    return nil, fmt.Errorf("lockfile: unsupported version %q", lf.Version)
}
```

Source: Story 4-4 review

**Rule 3: Set explicit `bufio.Scanner` buffer for JSONL (10MB)**
Default 64KB truncates long entries. Use 10MB for JSONL files.

```go
s := bufio.NewScanner(f)
s.Buffer(make([]byte, 64*1024), 10*1024*1024) // start 64KB, max 10MB
```

Source: Story 4-5 review — `ReadTranscript` truncated long entries

#### Detection Signals

- `json.NewDecoder` without `dec.More()` check after decode
- Struct with `Version` field parsed without validation
- `bufio.NewScanner` on JSONL file without explicit `Buffer()` call

---

### Pattern: cli-command-patterns

**Tags:** `cli`, `cobra`, `commands`, `error-handling`, `workspace`
**Domain:** shared
**Severity:** high
**Discovered in:** `cmd/siply/lock.go`, `cmd/siply/run.go`, `cmd/siply/workspaces.go`

#### Problem Summary

Four CLI command patterns discovered: (1) `os.Exit` in command handlers prevents deferred cleanup, (2) workspace must be detected before accessing `ConfigDir()`, (3) unknown `--workspace` name should create instead of error, (4) error prefixes must be consistent.

#### Rules

**Rule 1: Never `os.Exit(1)` in command handlers — use `return fmt.Errorf`**
Cobra handles exit codes. `os.Exit` skips all deferred cleanup (Stop, Close, etc.).

```go
// BAD
if !result.Match {
    fmt.Println("Mismatch")
    os.Exit(1)  // deferred cleanup NEVER runs!
}

// GOOD
if !result.Match {
    return fmt.Errorf("lockfile: verification failed — %d mismatches", len(result.Diffs))
}
```

Source: Story 4-4 review

**Rule 2: Always `wsMgr.Detect(ctx)` before `wsMgr.ConfigDir()`**
Workspace must be active before you can ask for its config directory.

```go
// CORRECT order
if _, err := wsMgr.Detect(ctx); err != nil { return err }
projectDir := wsMgr.ConfigDir() // now safe — workspace is active
```

Source: Story 4-4 review

**Rule 3: `--flag <name>` with unknown name → fallback to Create**
CLI UX: when a user specifies a name that doesn't exist, create it instead of erroring.

```go
ws, err := mgr.Open(ctx, name)
if err != nil {
    // Fallback: create workspace if it doesn't exist
    ws, err = mgr.Create(ctx, name, cwd)
    if err != nil { return err }
}
```

Source: Story 4-3 review (P8/D1) — AC#1 says "opens or creates"

**Rule 4: Consistent error prefix per package**
`workspace:` not `workspaces:`. One package = one prefix.

Source: Story 4-3 review (P4)

#### Detection Signals

- `os.Exit` inside a `RunE` function
- `ConfigDir()` called without prior `Detect()` or `Open()`
- CLI flag handler that only returns error on unknown name (no create fallback)
- Mixed error prefixes in same package (`workspace:` vs `workspaces:`)

---

### Pattern: state-persistence-and-nil-handling

**Tags:** `state`, `persistence`, `nil`, `registry`, `defensive`
**Domain:** shared
**Severity:** high
**Discovered in:** `internal/workspace/manager.go`, `cmd/siply/run.go`

#### Problem Summary

Two patterns: (1) state changes (active workspace, switch) must persist to disk or they're lost on restart, (2) functions that return `(result, nil)` where result can be nil require explicit nil handling by callers.

#### Rules

**Rule 1: Persist active state in registries**
If `Switch()` or `Open()` changes active state only in memory, the change is lost after restart.

```go
// BAD — ephemeral switch
func (m *Manager) Switch(ctx context.Context, name string) (*Workspace, error) {
    m.active = m.known[name]  // lost on restart!
    return m.active, nil
}

// GOOD — persistent switch
func (m *Manager) Switch(ctx context.Context, name string) (*Workspace, error) {
    m.active = m.known[name]
    m.registry.ActiveWorkspace = name
    return m.active, m.saveWorkspaces()  // persisted to disk
}
```

Source: Story 4-3 review (P3) — AC#5 violated

**Rule 2: Handle `(nil, nil)` returns explicitly**
When a function can legitimately return `nil, nil` (e.g., "no git root found, not an error"), the caller must check for nil result.

```go
// Function signature signals nil is possible
func (m *Manager) Detect(ctx context.Context) (*Workspace, error)

// Caller MUST handle nil result
ws, err := mgr.Detect(ctx)
if err != nil { return err }
if ws == nil {
    slog.Info("workspace: no git project detected")
    // continue without workspace — not an error
}
```

Source: Story 4-3 review (P7)

#### Detection Signals

- `Switch`/`Open`/`Activate` methods that don't call a save/persist function
- Function returning `(nil, nil)` where caller only checks `err != nil`
- State change in memory without corresponding file write

---

### Pattern: git-bound-constructor-validation

**Tags:** `git`, `workspace`, `validation`, `constructor`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/workspace/manager.go`

#### Problem Summary

When a constructor or factory is designed for git-bound objects (workspaces, repos), it must reject non-git directories instead of silently falling back.

#### Bad Example

```go
func (m *Manager) Create(ctx context.Context, name, rootDir string) (*Workspace, error) {
    absRoot, _ := filepath.Abs(rootDir)
    gitRoot, _ := detectGitRoot(absRoot)
    if gitRoot == "" {
        gitRoot = absRoot  // SILENT FALLBACK — violates "bound to git" constraint!
    }
    return &Workspace{GitRoot: gitRoot}, nil
}
```

#### Good Example

Source: `internal/workspace/manager.go` (post-review fix)

```go
func (m *Manager) Create(ctx context.Context, name, rootDir string) (*Workspace, error) {
    absRoot, err := filepath.Abs(rootDir)
    if err != nil { return nil, fmt.Errorf("workspace: invalid path: %w", err) }
    gitRoot, err := detectGitRoot(absRoot)
    if err != nil { return nil, fmt.Errorf("workspace: failed to detect git root: %w", err) }
    if gitRoot == "" {
        return nil, fmt.Errorf("workspace: %s is not inside a git repository", absRoot)
    }
    return &Workspace{GitRoot: gitRoot}, nil
}
```

#### Rule

When architecture specifies a binding constraint (e.g., "workspace bound to git project"), enforce it strictly. Never silently fall back to a weaker guarantee.

#### Detection Signals

- Constructor that catches an error and uses a fallback value instead of returning the error
- Architecture doc says "bound to X" but code accepts non-X inputs
- Empty string used as fallback for a required field

---

### Pattern: oauth-csrf-state

**Tags:** `auth`, `oauth`, `security`, `csrf`
**Domain:** shared
**Severity:** critical
**Discovered in:** `internal/auth/oauth.go`

#### Problem Summary

OAuth localhost callback servers must include a CSRF state parameter. Without it, an attacker can inject their own authorization code into the callback.

#### Rule

Every OAuth flow must:
1. Generate a random state parameter before redirecting to the auth provider
2. Store the state in memory (not in a cookie — localhost server)
3. Verify the state parameter in the callback matches what was sent
4. Reject callbacks with missing or mismatched state

```go
state := generateRandomState() // crypto/rand, 32 bytes, hex encoded
authURL := oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
// ... redirect user to authURL ...

// In callback handler:
if r.URL.Query().Get("state") != expectedState {
    http.Error(w, "Invalid state parameter", http.StatusForbidden)
    return
}
```

Source: Epic PB.4 review (P1)

#### Detection Signals

- `oauth2.Config.AuthCodeURL()` called without state parameter
- Callback handler that doesn't check `state` query parameter
- OAuth flow without `crypto/rand` import

---

### Pattern: no-speculative-interfaces

**Tags:** `interface`, `design`, `yagni`, `architecture`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/telemetry/collector.go`

#### Problem Summary

TelemetryCollector was designed with 4 unnecessary methods (Export, Subscribe, RecordSession + EventBus/FeatureGate dependencies). Team consensus: strip to `RecordStep` + `Flush` only. Build what you need now.

#### Rule

Don't add interface methods "because we might need them later." If zero callers exist today, the method doesn't belong in the interface today. When a caller appears, add the method then.

```go
// BAD — speculative methods with no callers
type TelemetryCollector interface {
    RecordStep(step StepRecord)
    Flush(ctx context.Context) error
    Export(ctx context.Context, format string) ([]byte, error)  // no caller!
    Subscribe(ch chan<- StepRecord)                              // no caller!
    RecordSession(session SessionRecord)                        // no caller!
}

// GOOD — minimal interface for current needs
type TelemetryCollector interface {
    RecordStep(step StepRecord)
    Flush(ctx context.Context) error
}
```

Source: Epic PB.7 simplification — team voted to remove speculative methods

#### Detection Signals

- Interface method with zero callers in the codebase
- Method added "for future use" or "so plugins can..."
- Interface with more methods than its concrete implementation actually uses

---

### Pattern: sentinel-errors-without-format-verbs

**Tags:** `errors`, `sentinel`, `style`
**Domain:** shared
**Severity:** low
**Discovered in:** `internal/licensing/validator.go`

#### Rule

Use `errors.New()` for sentinel errors (no format verbs). Use `fmt.Errorf()` only when you need `%s`, `%d`, `%w`, etc.

```go
// BAD — fmt.Errorf without format verbs
var ErrNotActivated = fmt.Errorf("license: not activated")

// GOOD — errors.New for static strings
var ErrNotActivated = errors.New("license: not activated")

// GOOD — fmt.Errorf when you need formatting
return fmt.Errorf("license: provider %q not found", name)
```

Source: Epic PB.6 review (P2)

---

### Pattern: named-struct-fields

**Tags:** `style`, `readability`, `maintenance`
**Domain:** shared
**Severity:** low
**Discovered in:** `internal/config/lockfile.go`

#### Rule

Always use named fields in struct literals. Positional literals break when fields are reordered or added.

```go
// BAD — positional, fragile, unreadable
diffs = append(diffs, VerifyDiff{"provider.model", "gpt-4", "claude"})

// GOOD — named, self-documenting, reorder-safe
diffs = append(diffs, VerifyDiff{
    Field:    "provider.model",
    Expected: "gpt-4",
    Actual:   "claude",
})
```

Source: Story 4-4 review

---

### Pattern: consistent-type-naming

**Tags:** `naming`, `style`, `consistency`
**Domain:** shared
**Severity:** low
**Discovered in:** `internal/core/config.go`

#### Rule

All config/options structs end with `Config` or `Options`. Don't abbreviate inconsistently.

```go
// BAD — inconsistent abbreviation
type RoutingCfg struct { ... }

// GOOD — consistent suffix
type RoutingConfig struct { ... }
```

Naming convention across the codebase: `ProviderConfig`, `SessionConfig`, `TelemetryConfig`, `RoutingConfig`, `LoaderOptions`, `GenerateOptions`.

Source: Story 4-1 review (F7)

---

### Pattern: cross-namespace-testing

**Tags:** `testing`, `isolation`, `security`, `plugins`
**Domain:** shared
**Severity:** medium
**Discovered in:** `internal/credential/file_store_test.go`

#### Rule

When testing namespace isolation (plugin credentials, plugin config, plugin state), test that plugin A **cannot** see plugin B's keys — even when those keys exist. Don't just test non-existent keys.

```go
// BAD — only tests non-existent key
_, err := store.GetPluginCredential(ctx, "plugin-a", "nonexistent")
require.Error(t, err)

// GOOD — tests real cross-namespace access
store.SetPluginCredential(ctx, "plugin-b", "secret", cred)
_, err := store.GetPluginCredential(ctx, "plugin-a", "secret") // plugin-a accessing plugin-b's key!
require.Error(t, err, "plugin-a should not see plugin-b's credentials")
```

Source: Story 4-2 review — plugin isolation test strengthened

---

## Section: backend

Patterns specific to core business logic, agent loop, and internal systems.

---

### Pattern: atomic-update-pattern

**Tags:** `file-safety`, `crash-recovery`, `plugins`
**Domain:** backend
**Severity:** high
**Discovered in:** Epic 6 stories 6-4, 6-9 — plugin update/rollback system

#### Rule

Multi-step file operations (install, update, rollback) must use staged renames for crash safety. If any step fails, restore from the step that succeeded.

```go
// BAD — direct overwrite, unrecoverable on crash
os.RemoveAll(targetDir)
os.Rename(newDir, targetDir)

// GOOD — staged rename with rollback
// Step 1: Install new to temp dir
tempDir := targetDir + ".new"
if err := installTo(tempDir); err != nil {
    os.RemoveAll(tempDir)
    return err
}
// Step 2: Rename old to backup
backupDir := targetDir + ".bak"
if err := os.Rename(targetDir, backupDir); err != nil {
    os.RemoveAll(tempDir)
    return err
}
// Step 3: Rename new to target
if err := os.Rename(tempDir, targetDir); err != nil {
    os.Rename(backupDir, targetDir) // restore from backup
    return err
}
// Step 4: Remove backup on success
os.RemoveAll(backupDir)
```

Use `os.Rename` for same-filesystem atomic moves. Cross-filesystem requires copy+rename.

Source: `internal/plugins/version_manager.go` — Story 6-9 Task 5

---

### Pattern: plugin-path-traversal

**Tags:** `security`, `validation`, `plugins`
**Domain:** backend
**Severity:** critical
**Discovered in:** Epic 6 stories 6-1, 6-2, 6-3 — plugin name and path validation

#### Rule

All user-provided plugin names and paths MUST be validated against path traversal before use. Reject path separators and parent directory references. After `filepath.Join`, verify the result is still within the expected parent directory.

```go
// BAD — plugin name used directly in path construction
pluginDir := filepath.Join(pluginsRoot, pluginName)

// GOOD — validate before use
func validatePluginName(name string) error {
    if strings.ContainsAny(name, "/\\") {
        return fmt.Errorf("plugin name contains path separator: %q", name)
    }
    if strings.Contains(name, "..") {
        return fmt.Errorf("plugin name contains parent traversal: %q", name)
    }
    return nil
}

// After joining, verify containment
pluginDir := filepath.Join(pluginsRoot, pluginName)
rel, err := filepath.Rel(pluginsRoot, pluginDir)
if err != nil || strings.HasPrefix(rel, "..") {
    return fmt.Errorf("plugin path escapes plugins directory")
}
```

Source: `internal/plugins/registry.go`, `internal/plugins/tier3_loader.go` — Stories 6-1, 6-3

---

### Pattern: grpc-timeout-pattern

**Tags:** `grpc`, `timeout`, `context`, `plugins`
**Domain:** backend
**Severity:** high
**Discovered in:** Epic 6 story 6-3 — Tier 3 plugin gRPC communication

#### Rule

Create operation-specific contexts with timeout for gRPC calls. On timeout: cancel context, kill the plugin process, and return a structured error. For cleanup operations after cancellation, use `context.Background()` — not the cancelled parent context.

```go
// BAD — unbounded gRPC call
resp, err := client.ExecuteTool(ctx, req)

// GOOD — operation-specific timeout
opCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()

resp, err := client.ExecuteTool(opCtx, req)
if err != nil {
    if errors.Is(opCtx.Err(), context.DeadlineExceeded) {
        // Kill unresponsive plugin process
        p.process.Kill()
        return nil, fmt.Errorf("plugin %s: tool execution timed out after 30s", p.Name())
    }
    return nil, fmt.Errorf("plugin %s: tool execution: %w", p.Name(), err)
}

// BAD — cleanup with cancelled context
client.Shutdown(ctx) // ctx may be cancelled

// GOOD — fresh context for cleanup
cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cleanupCancel()
client.Shutdown(cleanupCtx)
```

Source: `internal/plugins/tier3_loader.go` — Story 6-3

---

### Pattern: lazy-init-sync-once

**Tags:** `concurrency`, `initialization`, `performance`
**Domain:** backend
**Severity:** medium
**Discovered in:** Epic 6 story 6-6 — shell completion registry

#### Rule

Use `sync.Once` for deferred initialization that should happen at most once. Avoids startup cost by initializing on first use. Thread-safe by design.

```go
// BAD — eager initialization at startup
func NewRegistry() *Registry {
    r := &Registry{}
    r.completions = loadAllCompletions() // slow, may not be needed
    return r
}

// GOOD — lazy initialization with sync.Once
type Registry struct {
    once        sync.Once
    completions map[string]Completion
}

func (r *Registry) GetCompletions() map[string]Completion {
    r.once.Do(func() {
        r.completions = loadAllCompletions()
    })
    return r.completions
}
```

`sync.Once` guarantees the function runs exactly once, even under concurrent access. The first caller blocks; subsequent callers wait and then return immediately.

Source: `internal/completion/completion.go` — Story 6-6

---

### Pattern: version-comparison

**Tags:** `semver`, `validation`, `plugins`
**Domain:** backend
**Severity:** medium
**Discovered in:** Epic 6 story 6-4 — plugin update/rollback version checks

#### Rule

Normalize the v-prefix before comparison. Use `golang.org/x/mod/semver` — standard library extension. Validate input is valid semver before comparing — garbage input returns 0 (equal) silently.

```go
// BAD — string comparison for versions
if newVersion > oldVersion { ... }

// BAD — semver.Compare with unvalidated input
result := semver.Compare(a, b) // returns 0 for garbage input!

// GOOD — validate then compare
func CompareVersions(a, b string) (int, error) {
    // Normalize v-prefix
    if !strings.HasPrefix(a, "v") {
        a = "v" + a
    }
    if !strings.HasPrefix(b, "v") {
        b = "v" + b
    }
    // Validate before comparing
    if !semver.IsValid(a) {
        return 0, fmt.Errorf("invalid semver: %q", a)
    }
    if !semver.IsValid(b) {
        return 0, fmt.Errorf("invalid semver: %q", b)
    }
    return semver.Compare(a, b), nil
}
```

Import: `"golang.org/x/mod/semver"`

Source: `internal/plugins/version.go` — Story 6-4, fixed in Story 6-9 Task 7

---

## Section: frontend-tui

Patterns specific to terminal UI development with Bubble Tea.

> Patterns will be added here when TUI development begins.
> Expected categories: model updates, rendering performance, key event handling, component lifecycle.

---

## Section: ux

Patterns specific to user experience in terminal interfaces.

> Patterns will be added here when UX-specific Go patterns are discovered.
> Expected categories: terminal layout, accessibility, responsive design, color/theme handling.

---

## Section: shared (Epic 9 additions)

Patterns discovered during Epic 9 (Marketplace & Publishing) code reviews. These appeared in multiple stories and were caught by review, not by the developer — indicating the agent did not have access to prior learnings.

---

### Pattern: sentinel-error-wrapping

**Tags:** `errors`, `sentinel`, `wrapping`, `errors-is`
**Domain:** shared
**Severity:** critical
**Discovered in:** Epic 9 — Stories 9-1, 9-2, 9-5, 9-6 (appeared in 4 out of 7 stories)

#### Problem Summary

When wrapping a sentinel error with `fmt.Errorf`, omitting `%w` creates a new error string that breaks the `errors.Is()` chain. Callers can no longer match the sentinel — error handling silently fails.

#### Bad Example

```go
var ErrItemNotFound = errors.New("marketplace: item not found")

func findItem(name string) error {
    // ...
    return fmt.Errorf("marketplace: item %q not found", name) // NEW error — NOT ErrItemNotFound!
}

// Caller:
if errors.Is(err, ErrItemNotFound) { // NEVER true!
```

#### Good Example

```go
return fmt.Errorf("marketplace: item %q: %w", name, ErrItemNotFound)

// Caller:
if errors.Is(err, ErrItemNotFound) { // works!
```

#### Rule

When returning a sentinel error with additional context, ALWAYS use `%w` to wrap it. Never recreate the error message as a new string.

#### Detection Signals

- `fmt.Errorf(...)` that matches a sentinel's text but doesn't use `%w`
- `errors.Is()` checks that are unreachable
- Error messages duplicated between sentinel `var` and `fmt.Errorf` call

---

### Pattern: unicode-aware-string-operations

**Tags:** `unicode`, `string`, `validation`, `truncation`, `i18n`
**Domain:** shared
**Severity:** high
**Discovered in:** Epic 9 — Stories 9-6 (3 locations), 9-7 (3 locations)

#### Problem Summary

`len(s)` counts bytes, not characters. Byte-slicing `s[:n]` can split a multi-byte UTF-8 character, producing invalid output. For user-facing text validation and truncation, use rune-aware functions.

#### Bad Example

```go
if len(reviewText) > 2000 { return ErrReviewTooLong }

desc := text[:97] + "..." // splits multi-byte character!
```

#### Good Example

Source: Epic 9 review fixes

```go
if utf8.RuneCountInString(reviewText) > 2000 { return ErrReviewTooLong }

runes := []rune(text)
if len(runes) > 97 {
    desc = string(runes[:97]) + "..."
}
```

#### Rule

For user-facing text: use `utf8.RuneCountInString()` for length, `[]rune()` conversion for truncation. `len()` is only safe when you explicitly need byte count (network buffers, file I/O).

#### Detection Signals

- `len(userInput) > limit` in validation code
- `text[:n]` on user-provided or external strings
- Truncation functions without rune conversion

---

### Pattern: cmd-context-not-background

**Tags:** `cli`, `cobra`, `context`, `cancellation`
**Domain:** shared
**Severity:** high
**Discovered in:** Epic 9 — Stories 9-4, 9-5, 9-6

#### Problem Summary

In Cobra command `RunE` handlers, `context.Background()` ignores user cancellation (Ctrl+C). `cmd.Context()` inherits Cobra's signal handling and propagates cancellation correctly.

#### Bad Example

```go
func executePublish(cmd *cobra.Command, args []string) error {
    resp, err := client.Publish(context.Background(), req) // ignores Ctrl+C!
```

#### Good Example

```go
func executePublish(cmd *cobra.Command, args []string) error {
    resp, err := client.Publish(cmd.Context(), req) // respects Ctrl+C
```

#### Rule

In Cobra `RunE` handlers: ALWAYS use `cmd.Context()` for the request context. Reserve `context.Background()` only for deferred cleanup (`defer stop(context.Background())`).

#### Detection Signals

- `context.Background()` inside a Cobra RunE function body
- `context.TODO()` in production code (not tests)

---

### Pattern: tui-async-io

**Tags:** `bubbletea`, `async`, `http`, `io`, `tui`
**Domain:** frontend-tui
**Severity:** high
**Discovered in:** Epic 9 — Stories 9-3, 9-6, 9-7

#### Problem Summary

Bubble Tea's `Update()` runs on the main goroutine. Blocking I/O (HTTP calls, file reads) in `Update()` freezes the entire TUI until the operation completes. Always return a `tea.Cmd` for async operations.

#### Bad Example

```go
func (mb *MarketBrowser) Update(msg tea.Msg) tea.Cmd {
    case tea.KeyMsg:
        if msg.String() == "i" {
            reviews, _ := client.GetReviews(ctx, name, 1, 3) // BLOCKS UI!
            mb.reviews = reviews
```

#### Good Example

Source: Epic 9 review fixes

```go
func (mb *MarketBrowser) Update(msg tea.Msg) tea.Cmd {
    case tea.KeyMsg:
        if msg.String() == "i" {
            return fetchReviews(mb.client, name) // returns immediately
        }
    case ReviewsResultMsg:
        mb.reviews = msg.Reviews // handle async result
}

func fetchReviews(c *Client, name string) tea.Cmd {
    return func() tea.Msg {
        reviews, err := c.GetReviews(context.Background(), name, 1, 3)
        return ReviewsResultMsg{Reviews: reviews, Err: err}
    }
}
```

#### Rule

NEVER call blocking I/O inside `Update()`. Wrap it in a `tea.Cmd` (a `func() tea.Msg`). Handle the result in a subsequent `Update()` call via a custom message type.

#### Detection Signals

- HTTP client calls (`client.Do`, `client.Get`, `client.Post`) inside `Update()`
- `os.ReadFile` or file I/O inside `Update()`
- UI freezes on specific user actions

---

### Pattern: nil-slice-json-encoding

**Tags:** `json`, `encoding`, `nil`, `api`
**Domain:** shared
**Severity:** medium
**Discovered in:** Epic 9 — Stories 9-1, 9-2

#### Problem Summary

`json.Marshal(nil)` for a slice produces `null`, not `[]`. API consumers (JavaScript, Python) may crash or behave unexpectedly on `null` where an array is expected.

#### Bad Example

```go
type Item struct {
    Tags []string `json:"tags"`
}
// item.Tags is nil → {"tags": null}
```

#### Good Example

```go
if item.Tags == nil { item.Tags = []string{} }
// → {"tags": []}
```

#### Rule

Before JSON-encoding a struct for API output: initialize nil slices to empty (`[]T{}`). This applies to all `--json` flag output and API response bodies.

#### Detection Signals

- `json.Marshal` or `json.NewEncoder.Encode` on structs with slice fields
- API tests that check for `[]` but receive `null`
- `omitempty` on slices that should always be present in output

---

### Pattern: url-path-escape

**Tags:** `security`, `url`, `path-traversal`, `http`
**Domain:** api
**Severity:** critical
**Discovered in:** Epic 9 — Story 9-6 (4 endpoints)

#### Problem Summary

User-provided strings used directly in URL path segments allow path traversal. A malicious item name like `../../admin/delete` can redirect the request to an unintended endpoint.

#### Bad Example

```go
url := fmt.Sprintf("%s/api/v1/items/%s/rate", baseURL, itemName)
```

#### Good Example

Source: Epic 9 review fix

```go
url := fmt.Sprintf("%s/api/v1/items/%s/rate", baseURL, url.PathEscape(itemName))
```

#### Rule

ALWAYS use `url.PathEscape()` on user-provided values in URL path segments. Use `url.QueryEscape()` for query parameters.

#### Detection Signals

- `fmt.Sprintf` building URLs with `%s` for user-provided path segments
- `strings.Join` or `path.Join` building URLs with external input
- HTTP client code without `net/url` import

---

### Pattern: goroutine-pipe-cleanup

**Tags:** `goroutine`, `io`, `pipe`, `leak`, `cleanup`
**Domain:** shared
**Severity:** high
**Discovered in:** Epic 9 — Story 9-5

#### Problem Summary

`io.Pipe()` creates a synchronous in-memory pipe. If the writing goroutine errors without closing `pw`, the reading end blocks forever — goroutine leak plus hung HTTP request.

#### Bad Example

```go
pr, pw := io.Pipe()
go func() {
    err := writeMultipart(pw)
    if err != nil {
        return // pw never closed → pr.Read blocks forever!
    }
    pw.Close()
}()
http.Post(url, contentType, pr) // hangs if writer errors
```

#### Good Example

Source: Epic 9 review fix

```go
pr, pw := io.Pipe()
go func() {
    var err error
    defer func() {
        if err != nil {
            pw.CloseWithError(err) // signals error to reader
        } else {
            pw.Close()
        }
    }()
    err = writeMultipart(pw)
}()
```

#### Rule

When using `io.Pipe()` with goroutines: `defer pw.Close()` or `defer pw.CloseWithError(err)` on ALL paths. The reader MUST see EOF or an error — never leave it blocking.

#### Detection Signals

- `io.Pipe()` without `defer pw.Close()` in the writing goroutine
- Error paths in pipe writers that `return` without closing
- HTTP requests using `io.PipeReader` as body

---

### Pattern: gzip-determinism

**Tags:** `archive`, `hash`, `determinism`, `gzip`
**Domain:** shared
**Severity:** medium
**Discovered in:** Epic 9 — Story 9-5

#### Problem Summary

`gzip.NewWriter` embeds the current wall-clock time in the gzip header by default. This makes the compressed output non-deterministic — the same input produces different SHA256 hashes on different runs.

#### Bad Example

```go
gz := gzip.NewWriter(w)
// SHA256 of output differs every second!
```

#### Good Example

Source: Epic 9 review fix

```go
gz, _ := gzip.NewWriterLevel(w, gzip.DefaultCompression)
gz.Header.ModTime = time.Time{} // zero time → deterministic
```

#### Rule

When archive checksums matter (publish flows, integrity verification): set `gz.Header.ModTime` to zero time. Also set `gz.Header.OS` to `0xff` (unknown) for cross-platform determinism.

#### Detection Signals

- `gzip.NewWriter` followed by `sha256.Sum` on the output
- Flaky tests comparing archive hashes
- "checksum mismatch" errors that resolve on retry

---

### Pattern: double-cobra-output

**Tags:** `cli`, `cobra`, `error-handling`, `output`
**Domain:** shared
**Severity:** medium
**Discovered in:** Epic 9 — Stories 9-2, 9-6

#### Problem Summary

In Cobra `RunE` handlers, both printing an error message AND returning the error causes the user to see the error twice — once from your `fmt.Fprintf` and once from Cobra's automatic error printing.

#### Bad Example

```go
func executeInstall(cmd *cobra.Command, args []string) error {
    if err != nil {
        fmt.Fprintf(cmd.ErrOrStderr(), "Item not available: %v\n", err)
        return err // Cobra ALSO prints this!
    }
}
// User sees:
// Item not available: marketplace: item not found
// Error: marketplace: item not found
```

#### Good Example

```go
// Option A: return error only — let Cobra print it
return fmt.Errorf("item not available: %w", err)

// Option B: print custom message, return silent error
fmt.Fprintf(cmd.ErrOrStderr(), "Item not available: %v\n", err)
cmd.SilenceErrors = true
return err
```

#### Rule

In Cobra RunE: choose ONE output path. Either return the error (Cobra prints it) or print manually and silence Cobra's error output. Never both.

#### Detection Signals

- `fmt.Fprintf(cmd.ErrOrStderr()` followed by `return err` on the same path
- User-reported "error shows twice" in CLI output
- `cmd.SilenceErrors` not set when manual error printing is used

---

### Pattern: concurrent-operation-guard

**Tags:** `concurrency`, `tui`, `bubbletea`, `state`
**Domain:** frontend-tui
**Severity:** medium
**Discovered in:** Epic 9 — Stories 9-3, 9-6

#### Problem Summary

TUI components that trigger async operations (install, rate, HTTP fetch) via `tea.Cmd` have no built-in guard against duplicate invocation. Pressing Enter twice quickly triggers two concurrent installs.

#### Bad Example

```go
func (mb *MarketBrowser) installItem() tea.Cmd {
    item := mb.selectedItem()
    return func() tea.Msg {
        err := marketplace.Install(ctx, *item, mb.installer)
        return InstallResultMsg{Err: err}
    }
}
```

#### Good Example

Source: Epic 9 review fix

```go
func (mb *MarketBrowser) installItem() tea.Cmd {
    if mb.installing {
        return nil // already in progress
    }
    mb.installing = true
    item := mb.selectedItem()
    return func() tea.Msg {
        err := marketplace.Install(ctx, *item, mb.installer)
        return InstallResultMsg{Err: err}
    }
}

// In Update(), on InstallResultMsg:
mb.installing = false
```

#### Rule

Every TUI `tea.Cmd` that triggers a side effect (HTTP, file write, process spawn) MUST have a boolean guard. Set the guard before returning the Cmd, clear it when the result message arrives.

#### Detection Signals

- `tea.Cmd` returning functions that call external APIs without a guard flag
- Double-click or fast key repeat causing duplicate operations
- "already installed" or "409 conflict" errors from accidental double-submit

---

## Appendix: Pattern Index

Quick reference for agent skill loading. Format: `pattern-name | domain | tags | severity`

```
context-cancellation-goroutines    | shared  | concurrency,goroutine,context,streaming,error-handling | critical
transport-timeout-not-client-timeout | shared  | api,http,streaming,timeout,configuration              | critical
nil-guard-interface-methods        | shared  | error-handling,nil-safety,defensive,interface          | critical
scanner-buffer-size                | shared  | streaming,parsing,buffer,sse,api                      | medium
deterministic-map-iteration        | shared  | concurrency,map,ordering,testing,determinism           | medium
mutex-shared-state                 | shared  | concurrency,goroutine,state,race-condition,sync        | critical
lifecycle-init-start-stop          | shared  | lifecycle,initialization,cleanup,defer,error-handling  | high
input-validation-guard-clauses     | shared  | validation,error-handling,defensive,api-boundary       | medium
deep-copy-before-merge             | shared  | map,pointer,mutation,merge,defensive                   | high
file-io-safety                     | shared  | file,permissions,toctou,crash-safety,backup            | high
json-yaml-strict-validation        | shared  | json,yaml,validation,parsing,forward-compatibility    | medium
cli-command-patterns               | shared  | cli,cobra,commands,error-handling,workspace            | high
state-persistence-and-nil-handling | shared  | state,persistence,nil,registry,defensive               | high
git-bound-constructor-validation   | shared  | git,workspace,validation,constructor                   | medium
oauth-csrf-state                   | shared  | auth,oauth,security,csrf                               | critical
no-speculative-interfaces          | shared  | interface,design,yagni,architecture                    | medium
sentinel-errors-without-format-verbs | shared | errors,sentinel,style                                 | low
named-struct-fields                | shared  | style,readability,maintenance                          | low
consistent-type-naming             | shared  | naming,style,consistency                               | low
cross-namespace-testing            | shared  | testing,isolation,security,plugins                     | medium
tool-json-size-limit               | api     | api,streaming,validation,memory,security               | medium
multi-line-sse-data                | api     | api,streaming,sse,parsing                              | low
json-validation-before-use         | api     | api,validation,json,tool-calls                         | medium
atomic-update-pattern              | backend | file-safety,crash-recovery,plugins                     | high
plugin-path-traversal              | backend | security,validation,plugins                            | critical
grpc-timeout-pattern               | backend | grpc,timeout,context,plugins                           | high
lazy-init-sync-once                | backend | concurrency,initialization,performance                 | medium
version-comparison                 | backend | semver,validation,plugins                              | medium
sentinel-error-wrapping            | shared  | errors,sentinel,wrapping,errors-is                    | critical
unicode-aware-string-operations    | shared  | unicode,string,validation,truncation,i18n              | high
cmd-context-not-background         | shared  | cli,cobra,context,cancellation                         | high
tui-async-io                       | frontend-tui | bubbletea,async,http,io,tui                       | high
nil-slice-json-encoding            | shared  | json,encoding,nil,api                                  | medium
url-path-escape                    | api     | security,url,path-traversal,http                       | critical
goroutine-pipe-cleanup             | shared  | goroutine,io,pipe,leak,cleanup                         | high
gzip-determinism                   | shared  | archive,hash,determinism,gzip                          | medium
double-cobra-output                | shared  | cli,cobra,error-handling,output                        | medium
concurrent-operation-guard         | frontend-tui | concurrency,tui,bubbletea,state                  | medium
```

---

## Appendix: Known Deferred Items

Items identified in reviews but consciously deferred. Future patterns may emerge from these.

| Item | File | Reason Deferred |
|------|------|-----------------|
| Overlapping tool content blocks | `anthropic/stream.go` | Anthropic API doesn't send overlapping blocks currently |
| Bash tool buffers full output before truncation | `tools/bash.go` | Acceptable for current scope |
| Web tool no HTTP status code check | `tools/web.go` | Out of scope, will address when web tool is extended |
| Context compaction doesn't know token limit | `agent/context_manager.go` | Design choice — simple heuristic sufficient for now |
| NoopStatusCollector.Subscribe blocks | `agent/noop_status_collector.go` | No callers currently |
| ANSI stripping per-chunk vs buffer | `cmd/siply/run.go` | Dependent on streaming output refactor |
| Capabilities() union misleading per-request | `routing/provider.go` | Design consideration for multi-provider routing |
| Non-deterministic lifecycle ordering from maps | `routing/provider.go` | Pre-existing Go map iteration behavior |

---

## Section: frontend-tui

Patterns specific to the Bubble Tea TUI layer (`internal/tui/`, `internal/tui/components/`, `internal/tui/panels/`, `internal/tui/menu/`, `internal/tui/statusline/`).

---

### Pattern: ansi-safe-string-handling

**Tags:** `rendering`, `ansi`, `truncation`, `width`
**Domain:** frontend-tui
**Severity:** high
**Discovered in:** Epic 5 reviews — appeared in 8 out of 11 stories

#### Rule

NEVER use `len()` or byte slicing `[:n]` on styled strings. Styled strings contain ANSI escape codes that inflate byte length but have zero display width.

```go
// BAD — counts ANSI escape bytes as characters
if len(styledLine) > width {
    line = styledLine[:width]
}

// GOOD — ANSI-aware width and truncation
if ansi.StringWidth(styledLine) > width {
    line = ansi.Truncate(styledLine, width, "...")
}
```

For test assertions on styled output, always strip ANSI first:

```go
// BAD — assertion fails because styled string contains escape codes
assert.Contains(t, result, "Plugin installed")

// GOOD — strip ANSI for content assertions
stripped := ansi.Strip(result)
assert.Contains(t, stripped, "Plugin installed")
```

Import: `"github.com/charmbracelet/x/ansi"`

Source: Epic 5 — most consistent finding across all code reviews (stories 5.1–5.11)

---

### Pattern: import-cycle-prevention

**Tags:** `architecture`, `interfaces`, `packages`
**Domain:** frontend-tui
**Severity:** high
**Discovered in:** Epic 5 stories 5.3, 5.4, 5.5, 5.6, 5.7, 5.8

#### Rule

The parent `tui` package defines interfaces and message types. Child packages (`components`, `panels`, `menu`, `statusline`) implement them. NEVER import a child package from the parent.

```
tui/messages.go     → defines ActivityFeedRenderer, SubPanel, StatusRenderer, etc.
tui/components/     → implements ActivityFeedRenderer
tui/panels/         → implements SubPanel
tui/statusline/     → implements StatusRenderer
tui/menu/           → implements MenuOverlay
cmd/siply/tui.go    → wires concrete types to App via Set*() methods
```

```go
// BAD — parent imports child (circular dependency)
package tui
import "siply.dev/siply/internal/tui/components"

// GOOD — parent defines interface, child implements it
// tui/messages.go
type ActivityFeedRenderer interface {
    Render(width, height int) string
    HandleFeedEntry(msg FeedEntryMsg)
}

// tui/components/activityfeed.go
func (af *ActivityFeed) Render(width, height int) string { ... }
func (af *ActivityFeed) HandleFeedEntry(msg FeedEntryMsg) { ... }
```

Source: Epic 5 — repeated in every story that introduced a new sub-package

---

### Pattern: pure-renderer-not-tea-model

**Tags:** `architecture`, `bubbletea`, `rendering`
**Domain:** frontend-tui
**Severity:** medium
**Discovered in:** Epic 5 story 5.4 (StatusBar), established pattern for 5.5–5.11

#### Rule

TUI components that don't handle interactive input should be **pure renderers**, not full `tea.Model` implementations. Only the root `App` and interactive panels (e.g., `REPLPanel`) implement `tea.Model`.

```go
// GOOD — pure renderer for display-only components
type ActivityFeed struct { ... }
func (af *ActivityFeed) Render(width, height int) string { ... }
func (af *ActivityFeed) SetSize(width, height int) { ... }
func (af *ActivityFeed) HandleFeedEntry(msg FeedEntryMsg) { ... }

// GOOD — tea.Model only for interactive input components
type REPLPanel struct { ... }
func (r *REPLPanel) Init() tea.Cmd { ... }
func (r *REPLPanel) Update(msg tea.Msg) tea.Cmd { ... }  // SubPanel returns Cmd only
func (r *REPLPanel) View() string { ... }
```

Note: `SubPanel.Update()` returns `tea.Cmd` only (not `(tea.Model, tea.Cmd)`). Sub-models mutate via pointer receiver — the parent holds the pointer, not a value copy.

Source: Epic 5 — architectural decision at Story 5.4

---

### Pattern: dimension-clamping

**Tags:** `rendering`, `safety`, `layout`
**Domain:** frontend-tui
**Severity:** medium
**Discovered in:** Epic 5 stories 5.1, 5.3, 5.4, 5.6

#### Rule

Every `Render()` and `SetSize()` function MUST clamp width and height to minimum 1. Zero or negative values cause panics in string operations and lipgloss rendering.

```go
// BAD — no guard on zero/negative dimensions
func (c *Component) Render(width, height int) string {
    line := strings.Repeat(" ", width) // panics if width < 0
}

// GOOD — clamp to minimum 1
func (c *Component) Render(width, height int) string {
    if width < 1 || height < 1 {
        return ""
    }
    // safe to use width and height
}

func (c *Component) SetSize(width, height int) {
    if width < 1 { width = 1 }
    if height < 1 { height = 1 }
    c.width = width
    c.height = height
}
```

Source: Epic 5 — dimension-related panics caught in 4 code reviews

---

### Pattern: no-color-mode-completeness

**Tags:** `rendering`, `accessibility`, `nocolor`
**Domain:** frontend-tui
**Severity:** medium
**Discovered in:** Epic 5 stories 5.6, 5.7, 5.8, 5.9, 5.10

#### Rule

When `ColorNone` is active, ALL lipgloss styling must be skipped — not just foreground color. Structural styles (Bold, Faint, Reverse) are allowed but foreground/background colors are not. Third-party components (bubbles/list) may leak ANSI regardless — override their styles explicitly.

```go
// BAD — ColorNone still emits color from lipgloss
style := theme.Success.Resolve(cs) // Resolve handles ColorNone for Token styles...
line := style.Render(text)         // but third-party styles don't

// GOOD — override third-party list styles for no-color
if cs == tui.ColorNone {
    l.Styles.ActivePaginationDot = lipgloss.NewStyle()
    l.Styles.InactivePaginationDot = lipgloss.NewStyle()
    l.Styles.Title = lipgloss.NewStyle().Bold(true)
    l.Paginator.ActiveDot = "*"
    l.Paginator.InactiveDot = "."
}
```

For accessible mode tests, verify zero ANSI output:

```go
assert.Equal(t, result, ansi.Strip(result), "Accessible mode should have no ANSI codes")
```

Source: Epic 5 — ANSI leakage in no-color mode found in 5 stories

---

### Pattern: no-dead-message-types

**Tags:** `architecture`, `messages`, `dead-code`
**Domain:** frontend-tui
**Severity:** low
**Discovered in:** Epic 5 stories 5.7, 5.8, 5.9, 5.11

#### Rule

Every message type defined in `tui/messages.go` MUST have a corresponding handler in `app.go Update()`. Every getter or interface method MUST have at least one caller. Do not define types speculatively.

```go
// BAD — message type with no handler
type MarkdownRenderMsg struct { Content string }  // defined but never matched in Update()

// GOOD — every type has a consumer
type FeedbackMsg struct { ... }  // matched in app.go Update() case FeedbackMsg:
```

Checklist before adding a new message type:
1. Add type to `messages.go`
2. Add `case TypeName:` handler in `app.go Update()`
3. Add routing test in `app_test.go`
4. Verify interface method has implementer AND caller

Source: Epic 5 — dead message types removed in 5 code reviews

---

## Changelog

| Date | Patterns Added |
|------|----------------|
| 2026-04-05 | 11 patterns: 8 shared, 3 api |
| 2026-04-07 | 12 patterns: 12 shared (Epic PB retro + Epic 4 reviews) |
| 2026-04-10 | 6 patterns: 6 frontend-tui (Epic 5 retrospective) |
| 2026-04-11 | 5 patterns: 5 backend (Epic 6 review analysis) |
| 2026-04-18 | 10 patterns: 7 shared, 1 api, 2 frontend-tui (Epic 9 retrospective) |
