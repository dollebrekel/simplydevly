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
