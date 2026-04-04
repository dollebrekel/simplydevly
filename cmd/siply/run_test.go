package main

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/agent"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/routing"
)

func TestNewRunCmd_FlagParsing(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid task flag",
			args:    []string{"--task", "analyze code quality"},
			wantErr: true, // will fail on provider init, but flag parsing succeeds
		},
		{
			name:        "missing task flag",
			args:        []string{},
			wantErr:     true,
			errContains: "required flag",
		},
		{
			name:    "task with yolo flag",
			args:    []string{"--task", "test", "--yolo"},
			wantErr: true, // fails on provider init
		},
		{
			name:    "task with auto-accept flag",
			args:    []string{"--task", "test", "--auto-accept"},
			wantErr: true, // fails on provider init
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRunCmd()
			cmd.SetArgs(tt.args)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true

			err := cmd.Execute()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no ansi",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "red text",
			input: "\x1b[31mhello\x1b[0m",
			want:  "hello",
		},
		{
			name:  "bold green",
			input: "\x1b[1;32mSuccess\x1b[0m: done",
			want:  "Success: done",
		},
		{
			name:  "multiple sequences",
			input: "\x1b[36minfo\x1b[0m: \x1b[33mwarning\x1b[0m",
			want:  "info: warning",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBootstrapProvider(t *testing.T) {
	tests := []struct {
		name        string
		envProvider string
		wantErr     bool
		errContains string
	}{
		{
			name:        "default anthropic",
			envProvider: "",
			wantErr:     false,
		},
		{
			name:        "explicit anthropic",
			envProvider: "anthropic",
			wantErr:     false,
		},
		{
			name:        "openai",
			envProvider: "openai",
			wantErr:     false,
		},
		{
			name:        "ollama",
			envProvider: "ollama",
			wantErr:     false,
		},
		{
			name:        "openrouter",
			envProvider: "openrouter",
			wantErr:     false,
		},
		{
			name:        "unknown provider",
			envProvider: "groq",
			wantErr:     true,
			errContains: "unknown provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envProvider != "" {
				t.Setenv("SIPLY_PROVIDER", tt.envProvider)
			} else {
				t.Setenv("SIPLY_PROVIDER", "")
			}

			p, err := bootstrapProvider(nil)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

// testStreamTextEvent is a test double for EventBus collection testing.
type testStreamTextEvent struct {
	text string
	ts   time.Time
}

func (e *testStreamTextEvent) Type() string         { return "stream.text" }
func (e *testStreamTextEvent) Timestamp() time.Time { return e.ts }
func (e *testStreamTextEvent) Text() string         { return e.text }

func TestEventBusOutputCollection(t *testing.T) {
	bus := events.NewBus()
	ctx := context.Background()

	var output strings.Builder
	bus.Subscribe("stream.text", func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			output.WriteString(te.Text())
		}
	})

	require.NoError(t, bus.Publish(ctx, &testStreamTextEvent{text: "Hello ", ts: time.Now()}))
	require.NoError(t, bus.Publish(ctx, &testStreamTextEvent{text: "World!", ts: time.Now()}))

	assert.Equal(t, "Hello World!", output.String())
}

// mockProviderForRun implements core.Provider for integration testing.
type mockProviderForRun struct {
	responses [][]core.StreamEvent
	mu        sync.Mutex
	callIdx   int
}

func (m *mockProviderForRun) Init(_ context.Context) error  { return nil }
func (m *mockProviderForRun) Start(_ context.Context) error { return nil }
func (m *mockProviderForRun) Stop(_ context.Context) error  { return nil }
func (m *mockProviderForRun) Health() error                 { return nil }
func (m *mockProviderForRun) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{MaxContextTokens: 100000}
}
func (m *mockProviderForRun) Query(_ context.Context, _ core.QueryRequest) (<-chan core.StreamEvent, error) {
	m.mu.Lock()
	idx := m.callIdx
	m.callIdx++
	m.mu.Unlock()

	ch := make(chan core.StreamEvent, 100)
	go func() {
		defer close(ch)
		if idx < len(m.responses) {
			for _, ev := range m.responses[idx] {
				ch <- ev
			}
		}
	}()
	return ch, nil
}

// mockToolExecutorForRun implements core.ToolExecutor for integration testing.
type mockToolExecutorForRun struct{}

func (m *mockToolExecutorForRun) Init(_ context.Context) error  { return nil }
func (m *mockToolExecutorForRun) Start(_ context.Context) error { return nil }
func (m *mockToolExecutorForRun) Stop(_ context.Context) error  { return nil }
func (m *mockToolExecutorForRun) Health() error                 { return nil }
func (m *mockToolExecutorForRun) ListTools() []core.ToolDefinition {
	return nil
}
func (m *mockToolExecutorForRun) GetTool(_ string) (core.ToolDefinition, error) {
	return core.ToolDefinition{}, nil
}
func (m *mockToolExecutorForRun) Execute(_ context.Context, _ core.ToolRequest) (core.ToolResponse, error) {
	return core.ToolResponse{Output: "ok"}, nil
}

// mockPermEvaluatorForRun implements core.PermissionEvaluator for integration testing.
type mockPermEvaluatorForRun struct{}

func (m *mockPermEvaluatorForRun) Init(_ context.Context) error  { return nil }
func (m *mockPermEvaluatorForRun) Start(_ context.Context) error { return nil }
func (m *mockPermEvaluatorForRun) Stop(_ context.Context) error  { return nil }
func (m *mockPermEvaluatorForRun) Health() error                 { return nil }
func (m *mockPermEvaluatorForRun) EvaluateAction(_ context.Context, _ core.Action) (core.ActionVerdict, error) {
	return core.Allow, nil
}
func (m *mockPermEvaluatorForRun) EvaluateCapabilities(_ context.Context, _ core.PluginMeta) (core.CapabilityVerdict, error) {
	return core.CapabilityVerdict{}, nil
}

func TestIntegration_AgentProcessesTaskAndWritesOutput(t *testing.T) {
	ctx := context.Background()

	// Use real providers.TextChunkEvent so the agent's processStream recognizes them.
	mockProv := &mockProviderForRun{
		responses: [][]core.StreamEvent{
			{
				&providers.TextChunkEvent{Text: "Analysis "},
				&providers.TextChunkEvent{Text: "complete."},
				&providers.DoneEvent{},
			},
		},
	}

	eventBus := events.NewBus()
	require.NoError(t, eventBus.Init(ctx))
	require.NoError(t, eventBus.Start(ctx))

	// Subscribe to stream.text to collect output (same pattern as run.go).
	var output strings.Builder
	eventBus.Subscribe("stream.text", func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			output.WriteString(te.Text())
		}
	})

	deps := agent.AgentDeps{
		Provider: mockProv,
		Tools:    &mockToolExecutorForRun{},
		Events:   eventBus,
		Tokens:   &agent.NoopTokenCounter{},
		Context:  agent.NewTruncationCompactor(),
		Status:   &agent.NoopStatusCollector{},
		Perm:     &mockPermEvaluatorForRun{},
	}

	ag := agent.NewAgent(deps)
	require.NoError(t, ag.Init(ctx))

	err := ag.Run(ctx, "analyze code quality")
	require.NoError(t, err)

	assert.Equal(t, "Analysis complete.", output.String())
}

func TestIntegration_ExitCodes(t *testing.T) {
	ctx := context.Background()

	t.Run("success returns nil", func(t *testing.T) {
		mockProv := &mockProviderForRun{
			responses: [][]core.StreamEvent{
				{
					&providers.TextChunkEvent{Text: "ok"},
					&providers.DoneEvent{},
				},
			},
		}

		eventBus := events.NewBus()
		require.NoError(t, eventBus.Init(ctx))
		require.NoError(t, eventBus.Start(ctx))

		deps := agent.AgentDeps{
			Provider: mockProv,
			Tools:    &mockToolExecutorForRun{},
			Events:   eventBus,
			Tokens:   &agent.NoopTokenCounter{},
			Context:  agent.NewTruncationCompactor(),
			Status:   &agent.NoopStatusCollector{},
			Perm:     &mockPermEvaluatorForRun{},
		}

		ag := agent.NewAgent(deps)
		require.NoError(t, ag.Init(ctx))

		err := ag.Run(ctx, "test task")
		assert.NoError(t, err, "success should return nil (exit code 0)")
	})

	t.Run("error returns non-nil", func(t *testing.T) {
		mockProv := &mockProviderForRun{
			responses: [][]core.StreamEvent{
				{&providers.ErrorEvent{Err: assert.AnError}},
			},
		}

		eventBus := events.NewBus()
		require.NoError(t, eventBus.Init(ctx))
		require.NoError(t, eventBus.Start(ctx))

		deps := agent.AgentDeps{
			Provider: mockProv,
			Tools:    &mockToolExecutorForRun{},
			Events:   eventBus,
			Tokens:   &agent.NoopTokenCounter{},
			Context:  agent.NewTruncationCompactor(),
			Status:   &agent.NoopStatusCollector{},
			Perm:     &mockPermEvaluatorForRun{},
		}

		ag := agent.NewAgent(deps)
		require.NoError(t, ag.Init(ctx))

		err := ag.Run(ctx, "failing task")
		assert.Error(t, err, "failure should return non-nil (exit code non-zero)")
	})
}

func TestTTYDetection(t *testing.T) {
	// Verify the fd call works without panic. In test environments,
	// stdout is typically not a TTY.
	_ = os.Stdout.Fd()
}

func TestNewRunCmd_RoutingFlag(t *testing.T) {
	cmd := newRunCmd()
	cmd.SetArgs([]string{"--task", "test", "--routing"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Will fail on provider init, but flag parsing succeeds.
	err := cmd.Execute()
	require.Error(t, err)
	// The error should NOT be about the flag itself.
	assert.NotContains(t, err.Error(), "unknown flag")
}

func TestRoutingEnabledViaEnvVar(t *testing.T) {
	t.Setenv("SIPLY_ROUTING_ENABLED", "true")

	cmd := newRunCmd()
	cmd.SetArgs([]string{"--task", "test"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	// Will fail on provider init, but the env var path should be exercised.
	// The --routing flag is NOT set; SIPLY_ROUTING_ENABLED enables it instead.
	err := cmd.Execute()
	require.Error(t, err)
	// If routing env var were ignored, we'd get a plain provider error.
	// With routing enabled but no preprocess provider, we still get a provider error
	// (routing falls through to primary), so just confirm it doesn't panic.
}

func TestBootstrapRouting_NoPreprocessProvider(t *testing.T) {
	t.Setenv("SIPLY_PREPROCESS_PROVIDER", "")

	primary := &mockProviderForRun{}
	eventBus := events.NewBus()

	p, err := bootstrapRouting(nil, primary, eventBus)
	require.NoError(t, err)
	// Should return the primary provider as-is.
	assert.Equal(t, primary, p)
}

func TestBootstrapRouting_WithPreprocessProvider(t *testing.T) {
	t.Setenv("SIPLY_PROVIDER", "anthropic")
	t.Setenv("SIPLY_PREPROCESS_PROVIDER", "ollama")
	t.Setenv("SIPLY_PREPROCESS_MODEL", "llama3.2")

	primary := &mockProviderForRun{}
	eventBus := events.NewBus()

	p, err := bootstrapRouting(nil, primary, eventBus)
	require.NoError(t, err)
	// Should return a RoutingProvider (different from primary).
	assert.NotEqual(t, primary, p)
	assert.NotNil(t, p)
}

func TestBootstrapRouting_SameProviderDifferentModel(t *testing.T) {
	t.Setenv("SIPLY_PROVIDER", "anthropic")
	t.Setenv("SIPLY_PREPROCESS_PROVIDER", "anthropic")
	t.Setenv("SIPLY_PREPROCESS_MODEL", "claude-haiku-4-5-20251001")

	primary := &mockProviderForRun{}
	eventBus := events.NewBus()

	p, err := bootstrapRouting(nil, primary, eventBus)
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestBootstrapRouting_UnknownPreprocessProvider(t *testing.T) {
	t.Setenv("SIPLY_PREPROCESS_PROVIDER", "nonexistent")

	primary := &mockProviderForRun{}
	eventBus := events.NewBus()

	_, err := bootstrapRouting(nil, primary, eventBus)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}

func TestIntegration_RoutingWithMockProviders(t *testing.T) {
	ctx := context.Background()

	cheapProvider := &mockProviderForRun{
		responses: [][]core.StreamEvent{
			{
				&providers.TextChunkEvent{Text: "cheap response"},
				&providers.DoneEvent{},
			},
		},
	}
	expensiveProvider := &mockProviderForRun{
		responses: [][]core.StreamEvent{
			{
				&providers.TextChunkEvent{Text: "expensive response"},
				&providers.DoneEvent{},
			},
		},
	}

	eventBus := events.NewBus()
	require.NoError(t, eventBus.Init(ctx))
	require.NoError(t, eventBus.Start(ctx))

	// Track routing events.
	var routingEvents []string
	eventBus.Subscribe("routing.decision", func(_ context.Context, ev core.Event) {
		routingEvents = append(routingEvents, ev.Type())
	})

	// Create routing provider with two providers.
	rp := routing.NewRoutingProvider(routing.RoutingProviderConfig{
		Providers: map[string]core.Provider{
			"expensive": expensiveProvider,
			"cheap":     cheapProvider,
		},
		Policy: routing.NewConfigPolicy(routing.RoutingConfig{
			Rules: []routing.RoutingRule{
				{Category: routing.CategoryPreprocess, Provider: "cheap", Model: "llama3.2"},
				{Category: routing.CategoryPrimary, Provider: "expensive"},
			},
			DefaultProvider: "expensive",
			Enabled:         true,
		}),
		DefaultProvider: "expensive",
		EventBus:        eventBus,
	})

	// Preprocess call should go to cheap provider.
	req := core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "summarize"}},
		Hints:    map[string]string{"task.category": "preprocess"},
	}
	stream, err := rp.Query(ctx, req)
	require.NoError(t, err)
	// Drain stream.
	for range stream {
	}

	assert.Equal(t, 1, len(routingEvents))

	// Primary call should go to expensive provider.
	req = core.QueryRequest{
		Messages: []core.Message{{Role: "user", Content: "generate code"}},
		Hints:    map[string]string{"task.category": "primary"},
	}
	stream, err = rp.Query(ctx, req)
	require.NoError(t, err)
	for range stream {
	}

	assert.Equal(t, 2, len(routingEvents))
}

func TestIntegration_RoutingDisabled_SingleProviderWorks(t *testing.T) {
	ctx := context.Background()

	provider := &mockProviderForRun{
		responses: [][]core.StreamEvent{
			{
				&providers.TextChunkEvent{Text: "response"},
				&providers.DoneEvent{},
			},
		},
	}

	eventBus := events.NewBus()
	require.NoError(t, eventBus.Init(ctx))
	require.NoError(t, eventBus.Start(ctx))

	// Single provider — routing should be bypassed.
	rp := routing.NewRoutingProvider(routing.RoutingProviderConfig{
		Providers:       map[string]core.Provider{"anthropic": provider},
		Policy:          routing.NewConfigPolicy(routing.RoutingConfig{Enabled: true}),
		DefaultProvider: "anthropic",
		EventBus:        eventBus,
	})

	var output strings.Builder
	eventBus.Subscribe("stream.text", func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			output.WriteString(te.Text())
		}
	})

	// Create agent with routing provider.
	deps := agent.AgentDeps{
		Provider: rp,
		Tools:    &mockToolExecutorForRun{},
		Events:   eventBus,
		Tokens:   &agent.NoopTokenCounter{},
		Context:  agent.NewTruncationCompactor(),
		Status:   &agent.NoopStatusCollector{},
		Perm:     &mockPermEvaluatorForRun{},
	}

	ag := agent.NewAgent(deps)
	require.NoError(t, ag.Init(ctx))

	err := ag.Run(ctx, "test task")
	require.NoError(t, err)
	assert.Equal(t, "response", output.String())
}
