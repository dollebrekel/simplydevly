// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/sandbox"
)

func TestBash_SuccessfulCommand(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: "echo hello"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", output)
}

func TestBash_NonZeroExitCode(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: "exit 42"})

	output, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit code 42")
	assert.Contains(t, output, "exit code: 42")
}

func TestBash_Timeout(t *testing.T) {
	tool := &BashTool{}
	timeout := 1
	input, _ := json.Marshal(bashInput{Command: "sleep 10", Timeout: &timeout})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestBash_OutputTruncation(t *testing.T) {
	tool := &BashTool{}
	// Generate > 100KB of output.
	input, _ := json.Marshal(bashInput{Command: "head -c 110000 /dev/zero | tr '\\0' 'A'"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "truncated")
	assert.LessOrEqual(t, len(output), maxOutputBytes+200) // some room for truncation message
}

func TestBash_EmptyCommand(t *testing.T) {
	tool := &BashTool{}
	input, _ := json.Marshal(bashInput{Command: ""})

	_, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command is required")
}

func TestBash_Properties(t *testing.T) {
	tool := &BashTool{}
	assert.Equal(t, "bash", tool.Name())
	assert.True(t, tool.Destructive())
}

// TestBash_SandboxUnavailable_FallsThroughToDirect verifies that when a sandbox
// provider is set but not available, execution falls back to direct execution.
func TestBash_SandboxUnavailable_FallsThroughToDirect(t *testing.T) {
	tool := &BashTool{
		Sandbox:    &mockSandbox{available: false},
		SandboxCfg: sandbox.DefaultConfig(),
	}
	input, _ := json.Marshal(bashInput{Command: "echo fallback"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "fallback\n", output)
}

// TestBash_SandboxAvailable_UsesSandbox verifies that when sandbox is available,
// it delegates to the sandbox provider.
func TestBash_SandboxAvailable_UsesSandbox(t *testing.T) {
	mock := &mockSandbox{
		available: true,
		result: sandbox.SandboxResult{
			ExitCode: 0,
			Stdout:   "sandboxed output\n",
		},
	}
	tool := &BashTool{
		Sandbox:    mock,
		SandboxCfg: sandbox.DefaultConfig(),
	}
	input, _ := json.Marshal(bashInput{Command: "echo test"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "sandboxed output\n", output)
	assert.True(t, mock.executeCalled)
}

// TestBash_NilSandbox_UsesDirectExecution ensures nil sandbox works (current behavior).
func TestBash_NilSandbox_UsesDirectExecution(t *testing.T) {
	tool := &BashTool{Sandbox: nil}
	input, _ := json.Marshal(bashInput{Command: "echo direct"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "direct\n", output)
}

// TestBash_SandboxKilled verifies proper error handling when sandbox kills a process.
func TestBash_SandboxKilled(t *testing.T) {
	mock := &mockSandbox{
		available: true,
		result: sandbox.SandboxResult{
			ExitCode:   137,
			Killed:     true,
			KillReason: "OOM killer",
			Stdout:     "partial output",
		},
		err: sandbox.ErrKilled,
	}
	tool := &BashTool{
		Sandbox:    mock,
		SandboxCfg: sandbox.DefaultConfig(),
	}
	input, _ := json.Marshal(bashInput{Command: "memory-hog"})

	output, err := tool.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OOM killer")
	assert.Contains(t, output, "partial output")
}

// TestBash_SandboxOutputFormat verifies sandboxed output matches unsandboxed format.
func TestBash_SandboxOutputFormat(t *testing.T) {
	mock := &mockSandbox{
		available: true,
		result: sandbox.SandboxResult{
			ExitCode: 1,
			Stdout:   "error output",
			Stderr:   "stderr msg",
		},
	}
	// Simulate non-zero exit from sandbox (non-killed).
	tool := &BashTool{
		Sandbox:    mock,
		SandboxCfg: sandbox.DefaultConfig(),
	}
	input, _ := json.Marshal(bashInput{Command: "failing cmd"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, output, "error output")
	assert.Contains(t, output, "stderr msg")
}

// TestBash_FeatureGated verifies that a gated sandbox falls back to direct execution.
func TestBash_FeatureGated(t *testing.T) {
	mock := &mockSandbox{available: true}
	tool := &BashTool{
		Sandbox:     mock,
		SandboxCfg:  sandbox.DefaultConfig(),
		FeatureGate: &mockFeatureGate{gated: true},
	}
	input, _ := json.Marshal(bashInput{Command: "echo gated"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "gated\n", output)
	assert.False(t, mock.executeCalled)
}

// TestBash_FeatureAllowed verifies that an allowed feature uses sandbox.
func TestBash_FeatureAllowed(t *testing.T) {
	mock := &mockSandbox{
		available: true,
		result:    sandbox.SandboxResult{Stdout: "allowed\n"},
	}
	tool := &BashTool{
		Sandbox:     mock,
		SandboxCfg:  sandbox.DefaultConfig(),
		FeatureGate: &mockFeatureGate{gated: false},
	}
	input, _ := json.Marshal(bashInput{Command: "echo test"})

	output, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "allowed\n", output)
	assert.True(t, mock.executeCalled)
}

// TestBash_SandboxOptionsPopulated verifies that SandboxOptions are built from config.
func TestBash_SandboxOptionsPopulated(t *testing.T) {
	mock := &mockSandbox{
		available:   true,
		captureOpts: true,
		result:      sandbox.SandboxResult{Stdout: "ok\n"},
	}
	cfg := sandbox.DefaultConfig()
	cfg.MemoryLimitMB = 256
	cfg.MaxProcesses = 50

	tool := &BashTool{
		Sandbox:    mock,
		SandboxCfg: cfg,
		WorkDir:    "/test/workspace",
	}
	input, _ := json.Marshal(bashInput{Command: "echo test"})

	_, err := tool.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "/test/workspace", mock.lastOpts.WorkDir)
	assert.Equal(t, 256, mock.lastOpts.MemoryLimitMB)
	assert.Equal(t, 50, mock.lastOpts.MaxProcesses)
}

// mockSandbox implements sandbox.SandboxProvider for testing.
type mockSandbox struct {
	available     bool
	result        sandbox.SandboxResult
	err           error
	executeCalled bool
	captureOpts   bool
	lastOpts      sandbox.SandboxOptions
}

func (m *mockSandbox) Execute(_ context.Context, _ string, opts sandbox.SandboxOptions) (sandbox.SandboxResult, error) {
	m.executeCalled = true
	if m.captureOpts {
		m.lastOpts = opts
	}
	return m.result, m.err
}

func (m *mockSandbox) Available() bool { return m.available }

func (m *mockSandbox) Capabilities() sandbox.SandboxCaps {
	return sandbox.SandboxCaps{Platform: "test", FilesystemIsolation: true}
}

func (m *mockSandbox) Close() error { return nil }

// TestBash_SandboxTimeout verifies timeout is propagated correctly.
func TestBash_SandboxTimeout(t *testing.T) {
	mock := &mockSandbox{
		available: true,
		result: sandbox.SandboxResult{
			Killed:     true,
			KillReason: "context deadline exceeded",
		},
		err: context.DeadlineExceeded,
	}
	tool := &BashTool{
		Sandbox:    mock,
		SandboxCfg: sandbox.DefaultConfig(),
	}
	timeout := 1
	input, _ := json.Marshal(bashInput{Command: "sleep 10", Timeout: &timeout})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := tool.Execute(ctx, input)
	require.Error(t, err)
}

// mockFeatureGate implements core.FeatureGate for testing.
type mockFeatureGate struct {
	gated bool
}

func (m *mockFeatureGate) Guard(_ context.Context, _ string) error {
	if m.gated {
		return fmt.Errorf("feature gated")
	}
	return nil
}

func (m *mockFeatureGate) GuardWithFallback(_ context.Context, id string) (core.GateResult, error) {
	return core.GateResult{Allowed: !m.gated, FeatureID: id}, nil
}

func (m *mockFeatureGate) Register(_ core.Feature) error    { return nil }
func (m *mockFeatureGate) List() []core.FeatureStatus        { return nil }
func (m *mockFeatureGate) Init(_ context.Context) error      { return nil }
func (m *mockFeatureGate) Start(_ context.Context) error     { return nil }
func (m *mockFeatureGate) Stop(_ context.Context) error      { return nil }
func (m *mockFeatureGate) Health() error                     { return nil }
