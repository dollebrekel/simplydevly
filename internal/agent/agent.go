package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/routing"
)

const maxToolIterations = 10

// AgentDeps holds all dependencies injected into the Agent.
type AgentDeps struct {
	Provider core.Provider
	Tools    core.ToolExecutor
	Events   core.EventBus
	Tokens   core.TokenCounter
	Context  core.ContextManager
	Status   core.StatusCollector
	Perm     core.PermissionEvaluator
}

// Agent implements the main AI agent loop: user query → provider call →
// tool execution → response. It manages conversation history and publishes
// events for every significant action.
type Agent struct {
	deps    AgentDeps
	history []core.Message
	logger  *TransparencyLogger
	mu      sync.Mutex // protects cancel and running
	cancel  context.CancelFunc
	running bool
}

// NewAgent creates an Agent with all dependencies injected.
func NewAgent(deps AgentDeps) *Agent {
	return &Agent{
		deps:   deps,
		logger: NewTransparencyLogger(deps.Events),
	}
}

// Init validates that all required dependencies are non-nil.
func (a *Agent) Init(_ context.Context) error {
	if a.deps.Provider == nil {
		return fmt.Errorf("agent: provider is required")
	}
	if a.deps.Tools == nil {
		return fmt.Errorf("agent: tool executor is required")
	}
	if a.deps.Events == nil {
		return fmt.Errorf("agent: event bus is required")
	}
	if a.deps.Tokens == nil {
		return fmt.Errorf("agent: token counter is required")
	}
	if a.deps.Context == nil {
		return fmt.Errorf("agent: context manager is required")
	}
	if a.deps.Status == nil {
		return fmt.Errorf("agent: status collector is required")
	}
	if a.deps.Perm == nil {
		return fmt.Errorf("agent: permission evaluator is required")
	}
	return nil
}

// Start is a no-op for the agent.
func (a *Agent) Start(_ context.Context) error { return nil }

// Stop cancels any active query.
func (a *Agent) Stop(_ context.Context) error {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Health returns nil — the agent is stateless between queries.
func (a *Agent) Health() error { return nil }

// Run executes one user turn: sends the user message through the provider,
// handles any tool calls, and loops until the provider returns text-only
// or max iterations is reached.
func (a *Agent) Run(ctx context.Context, userMessage string) error {
	// Enforce single-flight: only one Run at a time.
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

	// Build turn on a local copy so failures don't pollute persistent history.
	localHistory := append([]core.Message(nil), a.history...)
	localHistory = append(localHistory, core.Message{
		Role:    "user",
		Content: userMessage,
	})

	// Multi-turn loop: keep calling the provider until no more tool calls.
	for range maxToolIterations {

		if err := ctx.Err(); err != nil {
			return err
		}

		// Re-check compaction before every provider call.
		localHistory = a.compactIfNeeded(ctx, localHistory)

		// Build query request. Default category is "primary" for all turns.
		tools := a.deps.Tools.ListTools()
		hints := map[string]string{routing.HintKeyCategory: string(routing.CategoryPrimary)}
		req := buildQueryRequest(localHistory, "", tools, hints)

		a.logger.LogQueryStart(ctx, len(localHistory))

		// Call provider.
		stream, err := a.deps.Provider.Query(ctx, req)
		if err != nil {
			return fmt.Errorf("agent: provider query: %w", err)
		}
		if stream == nil {
			return fmt.Errorf("agent: provider returned nil stream")
		}

		// Process stream events.
		text, toolCalls, usage, err := a.processStream(ctx, stream)
		if err != nil {
			return err
		}

		// Log query completion.
		if usage != nil {
			cost, _ := a.deps.Tokens.EstimateCost(*usage, "")
			a.logger.LogQueryComplete(ctx, usage.InputTokens, usage.OutputTokens, cost)

			a.deps.Status.Publish(core.StatusUpdate{
				Source: "agent",
				Metrics: map[string]any{
					"tokens_in":  usage.InputTokens,
					"tokens_out": usage.OutputTokens,
					"cost":       cost,
				},
				Timestamp: time.Now(),
			})
		}

		// Append assistant message to local history.
		assistantMsg := core.Message{
			Role:    "assistant",
			Content: text,
		}
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}
		localHistory = append(localHistory, assistantMsg)

		// If no tool calls, commit to persistent history and we're done.
		if len(toolCalls) == 0 {
			a.history = localHistory
			return nil
		}

		// Execute pending tools and append results.
		resultMsgs, err := a.executePendingTools(ctx, toolCalls)
		if err != nil {
			return err
		}
		localHistory = append(localHistory, resultMsgs...)
	}

	// Max iterations reached — still commit history so conversation is not lost.
	a.history = localHistory
	return fmt.Errorf("agent: max tool iterations (%d) reached", maxToolIterations)
}

// processStream reads all events from the provider's stream channel and
// returns accumulated text, tool calls, and token usage.
func (a *Agent) processStream(ctx context.Context, stream <-chan core.StreamEvent) (string, []core.ToolCall, *core.TokenUsage, error) {
	var (
		text      string
		toolCalls []core.ToolCall
		usage     *core.TokenUsage
	)

	for {
		select {
		case <-ctx.Done():
			return "", nil, nil, ctx.Err()
		case ev, ok := <-stream:
			if !ok {
				// Channel closed — stream complete.
				return text, toolCalls, usage, nil
			}

			// Stream event publish errors are intentionally not logged here.
			// These are high-frequency fire-and-forget events (especially TextChunk);
			// TransparencyLogger handles error logging for lower-frequency agent events.
			switch e := ev.(type) {
			case *providers.TextChunkEvent:
				text += e.Text
				_ = a.deps.Events.Publish(ctx, &streamTextEvent{text: e.Text, ts: time.Now()})

			case *providers.ToolCallEvent:
				tc := core.ToolCall{
					ToolID:   e.ToolID,
					ToolName: e.ToolName,
					Input:    e.Input,
				}
				toolCalls = append(toolCalls, tc)
				_ = a.deps.Events.Publish(ctx, &streamToolCallEvent{
					toolName: e.ToolName,
					toolID:   e.ToolID,
					ts:       time.Now(),
				})

			case *providers.ThinkingEvent:
				_ = a.deps.Events.Publish(ctx, &streamThinkingEvent{thinking: e.Thinking, ts: time.Now()})

			case *providers.UsageEvent:
				u := e.Usage
				usage = &u

			case *providers.ErrorEvent:
				return "", nil, nil, fmt.Errorf("agent: provider stream: %w", e.Err)

			case *providers.DoneEvent:
				_ = a.deps.Events.Publish(ctx, &streamDoneEvent{ts: time.Now()})
			}
		}
	}
}

// executePendingTools runs each pending tool call and returns result messages.
func (a *Agent) executePendingTools(ctx context.Context, toolCalls []core.ToolCall) ([]core.Message, error) {
	var results []core.Message

	for _, tc := range toolCalls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		start := time.Now()

		req := core.ToolRequest{
			Name:   tc.ToolName,
			Input:  tc.Input,
			Source: "agent",
		}

		resp, err := a.deps.Tools.Execute(ctx, req)
		duration := time.Since(start)

		// Log tool execution regardless of outcome.
		a.logger.LogToolExecution(ctx, &ToolExecutedEvent{
			ToolName: tc.ToolName,
			ToolID:   tc.ToolID,
			Input:    tc.Input,
			Output:   resp.Output,
			IsError:  resp.IsError || err != nil,
			Duration: duration,
		})

		if err != nil {
			if errors.Is(err, core.ErrPermissionDenied) {
				// Permission denied — tell the provider.
				results = append(results, core.Message{
					Role:   "user",
					ToolID: tc.ToolID,
					ToolResults: []core.ToolResult{{
						ToolID:  tc.ToolID,
						Content: "Permission denied: user declined this action",
						IsError: true,
					}},
				})
				continue
			}

			// Tool not found — tell the provider.
			if errors.Is(err, core.ErrToolNotFound) {
				results = append(results, core.Message{
					Role:   "user",
					ToolID: tc.ToolID,
					ToolResults: []core.ToolResult{{
						ToolID:  tc.ToolID,
						Content: fmt.Sprintf("Tool not found: %s", tc.ToolName),
						IsError: true,
					}},
				})
				continue
			}

			// Other executor-level error — send as tool result so provider can adapt.
			results = append(results, core.Message{
				Role:   "user",
				ToolID: tc.ToolID,
				ToolResults: []core.ToolResult{{
					ToolID:  tc.ToolID,
					Content: fmt.Sprintf("Tool error: %s", err.Error()),
					IsError: true,
				}},
			})
			continue
		}

		results = append(results, core.Message{
			Role:   "user",
			ToolID: tc.ToolID,
			ToolResults: []core.ToolResult{{
				ToolID:  tc.ToolID,
				Content: resp.Output,
				IsError: resp.IsError,
			}},
		})
	}

	return results, nil
}

// compactIfNeeded checks if context compaction is needed and returns the
// (possibly compacted) message slice.
func (a *Agent) compactIfNeeded(ctx context.Context, messages []core.Message) []core.Message {
	caps := a.deps.Provider.Capabilities()
	if caps.MaxContextTokens <= 0 {
		return messages
	}

	if !a.deps.Context.ShouldCompact(messages, caps.MaxContextTokens) {
		return messages
	}

	compacted, err := a.deps.Context.Compact(ctx, messages)
	if err != nil {
		slog.Warn("context compaction failed", "error", err)
		return messages
	}
	return compacted
}

// Stream events published to EventBus during processing.

type streamTextEvent struct {
	text string
	ts   time.Time
}

func (e *streamTextEvent) Type() string         { return "stream.text" }
func (e *streamTextEvent) Timestamp() time.Time { return e.ts }

type streamToolCallEvent struct {
	toolName string
	toolID   string
	ts       time.Time
}

func (e *streamToolCallEvent) Type() string         { return "stream.tool_call" }
func (e *streamToolCallEvent) Timestamp() time.Time { return e.ts }

type streamThinkingEvent struct {
	thinking string
	ts       time.Time
}

func (e *streamThinkingEvent) Type() string         { return "stream.thinking" }
func (e *streamThinkingEvent) Timestamp() time.Time { return e.ts }

type streamDoneEvent struct {
	ts time.Time
}

func (e *streamDoneEvent) Type() string         { return "stream.done" }
func (e *streamDoneEvent) Timestamp() time.Time { return e.ts }
