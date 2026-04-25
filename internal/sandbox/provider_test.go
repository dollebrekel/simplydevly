// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package sandbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.True(t, cfg.Enabled)
	assert.False(t, cfg.FailIfUnavailable)
	assert.False(t, cfg.AllowNetwork)
	assert.Equal(t, 512, cfg.MemoryLimitMB)
	assert.Equal(t, 100, cfg.MaxProcesses)
	assert.Empty(t, cfg.ExtraReadPaths)
	assert.Empty(t, cfg.ExtraWritePaths)
}

func TestSandboxOptions_Defaults(t *testing.T) {
	opts := SandboxOptions{
		WorkDir:       "/tmp/workspace",
		AllowNetwork:  false,
		MemoryLimitMB: 512,
		MaxProcesses:  100,
		Timeout:       30 * time.Second,
	}
	assert.Equal(t, "/tmp/workspace", opts.WorkDir)
	assert.False(t, opts.AllowNetwork)
	assert.Equal(t, 512, opts.MemoryLimitMB)
}

func TestSandboxResult_Fields(t *testing.T) {
	result := SandboxResult{
		ExitCode:   0,
		Stdout:     "hello\n",
		Stderr:     "",
		Duration:   100 * time.Millisecond,
		Killed:     false,
		KillReason: "",
	}
	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.False(t, result.Killed)
}

func TestSandboxResult_Killed(t *testing.T) {
	result := SandboxResult{
		ExitCode:   137,
		Killed:     true,
		KillReason: "OOM",
	}
	assert.True(t, result.Killed)
	assert.Equal(t, "OOM", result.KillReason)
}

func TestSandboxCaps_Platform(t *testing.T) {
	caps := SandboxCaps{
		PIDIsolation:        true,
		NetworkIsolation:    true,
		FilesystemIsolation: true,
		ResourceLimits:      false,
		Platform:            "linux",
	}
	assert.Equal(t, "linux", caps.Platform)
	assert.True(t, caps.PIDIsolation)
	assert.False(t, caps.ResourceLimits)
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtraReadPaths = []string{"/opt/tools"}
	cfg.ExtraWritePaths = []string{"/data/output"}
	err := ValidateConfig(cfg)
	require.NoError(t, err)
}

func TestValidateConfig_RelativeReadPath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtraReadPaths = []string{"relative/path"}
	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestValidateConfig_RelativeWritePath(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtraWritePaths = []string{"relative/path"}
	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute")
}

func TestValidateConfig_NegativeMemory(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MemoryLimitMB = -1
	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "memory_limit_mb")
}

func TestValidateConfig_NegativeProcesses(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxProcesses = -1
	err := ValidateConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_processes")
}

func TestBuildOptions_MergesPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtraReadPaths = []string{"/opt/a", "/opt/b"}
	cfg.ExtraWritePaths = []string{"/data/out"}
	cfg.AllowNetwork = true
	cfg.MemoryLimitMB = 256

	opts := BuildOptions(cfg, "/home/user/project")
	assert.Equal(t, "/home/user/project", opts.WorkDir)
	assert.True(t, opts.AllowNetwork)
	assert.Equal(t, 256, opts.MemoryLimitMB)
	assert.Equal(t, []string{"/opt/a", "/opt/b"}, opts.ReadOnlyMounts)
	assert.Equal(t, []string{"/data/out"}, opts.ReadWriteMounts)
}

func TestBuildOptions_DeduplicatesPaths(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ExtraReadPaths = []string{"/opt/a", "/opt/a"}

	opts := BuildOptions(cfg, "/workspace")
	assert.Equal(t, []string{"/opt/a"}, opts.ReadOnlyMounts)
}

func TestConfigFromCore_Defaults(t *testing.T) {
	cfg := ConfigFromCore(nil, nil, nil, nil, nil, nil, nil)
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestConfigFromCore_Overrides(t *testing.T) {
	enabled := false
	fail := true
	net := true
	mem := 256
	procs := 50

	cfg := ConfigFromCore(&enabled, &fail, []string{"/a"}, []string{"/b"}, &net, &mem, &procs)
	assert.False(t, cfg.Enabled)
	assert.True(t, cfg.FailIfUnavailable)
	assert.True(t, cfg.AllowNetwork)
	assert.Equal(t, 256, cfg.MemoryLimitMB)
	assert.Equal(t, 50, cfg.MaxProcesses)
	assert.Equal(t, []string{"/a"}, cfg.ExtraReadPaths)
	assert.Equal(t, []string{"/b"}, cfg.ExtraWritePaths)
}

func TestNewProvider_ReturnsProvider(t *testing.T) {
	p := NewProvider(DefaultConfig())
	assert.NotNil(t, p)
	caps := p.Capabilities()
	assert.NotEmpty(t, caps.Platform)
}

func TestSandboxProvider_Close(t *testing.T) {
	p := NewProvider(DefaultConfig())
	err := p.Close()
	assert.NoError(t, err)
}
