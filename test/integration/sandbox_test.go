// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package integration

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/sandbox"
)

func skipIfSandboxUnavailable(t *testing.T) {
	t.Helper()
	p := sandbox.NewProvider(sandbox.DefaultConfig())
	if !p.Available() {
		t.Skip("sandbox runtime not available (bwrap/sandbox-exec missing or not supported)")
	}
}

func TestSandbox_EndToEnd_EchoCommand(t *testing.T) {
	skipIfSandboxUnavailable(t)

	p := sandbox.NewProvider(sandbox.DefaultConfig())
	defer p.Close()

	workDir := t.TempDir()
	opts := sandbox.SandboxOptions{
		WorkDir:       workDir,
		AllowNetwork:  false,
		MemoryLimitMB: 512,
		MaxProcesses:  100,
		Timeout:       10 * time.Second,
	}

	result, err := p.Execute(context.Background(), "echo hello", opts)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "hello")
	assert.False(t, result.Killed)
}

func TestSandbox_FilesystemIsolation_WriteOutsideWorkspace(t *testing.T) {
	skipIfSandboxUnavailable(t)

	p := sandbox.NewProvider(sandbox.DefaultConfig())
	defer p.Close()

	workDir := t.TempDir()
	opts := sandbox.SandboxOptions{
		WorkDir: workDir,
		Timeout: 10 * time.Second,
	}

	// Attempt to write to /etc should fail inside the sandbox.
	result, err := p.Execute(context.Background(), "touch /etc/sandbox-test-file", opts)
	// On Linux with bwrap, the write should be denied.
	// On macOS with Seatbelt, the write should be denied.
	if err == nil {
		assert.NotEqual(t, 0, result.ExitCode, "writing to /etc should fail in sandbox")
	}
}

func TestSandbox_WorkspaceWrite(t *testing.T) {
	skipIfSandboxUnavailable(t)

	p := sandbox.NewProvider(sandbox.DefaultConfig())
	defer p.Close()

	workDir := t.TempDir()
	opts := sandbox.SandboxOptions{
		WorkDir: workDir,
		Timeout: 10 * time.Second,
	}

	result, err := p.Execute(context.Background(), "echo test > "+workDir+"/sandbox-test.txt && cat "+workDir+"/sandbox-test.txt", opts)
	require.NoError(t, err)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Stdout, "test")
}

func TestSandbox_NetworkIsolation(t *testing.T) {
	skipIfSandboxUnavailable(t)
	if runtime.GOOS != "linux" {
		t.Skip("network isolation test only reliable on Linux with --unshare-net")
	}

	p := sandbox.NewProvider(sandbox.DefaultConfig())
	defer p.Close()

	workDir := t.TempDir()
	opts := sandbox.SandboxOptions{
		WorkDir:      workDir,
		AllowNetwork: false,
		Timeout:      5 * time.Second,
	}

	// curl should fail when network is denied.
	result, err := p.Execute(context.Background(), "curl -s --max-time 2 http://1.1.1.1", opts)
	if err == nil {
		assert.NotEqual(t, 0, result.ExitCode, "network request should fail when AllowNetwork=false")
	}
}

func TestSandbox_Capabilities(t *testing.T) {
	p := sandbox.NewProvider(sandbox.DefaultConfig())
	caps := p.Capabilities()

	switch runtime.GOOS {
	case "linux":
		assert.Equal(t, "linux", caps.Platform)
		assert.True(t, caps.PIDIsolation)
		assert.True(t, caps.NetworkIsolation)
		assert.True(t, caps.FilesystemIsolation)
	case "darwin":
		assert.Equal(t, "darwin", caps.Platform)
		assert.True(t, caps.NetworkIsolation)
		assert.True(t, caps.FilesystemIsolation)
	default:
		assert.Equal(t, "unsupported", caps.Platform)
		assert.False(t, caps.FilesystemIsolation)
	}
}
