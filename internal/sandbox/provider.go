// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package sandbox

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnavailable = errors.New("sandbox: runtime not available on this platform")
	ErrKilled      = errors.New("sandbox: process killed due to resource limit")
)

// SandboxProvider abstracts OS-level process isolation.
// Platform-specific implementations live in sandbox_linux.go and sandbox_darwin.go.
type SandboxProvider interface {
	Execute(ctx context.Context, cmd string, opts SandboxOptions) (SandboxResult, error)
	Available() bool
	Capabilities() SandboxCaps
	Close() error
}

// SandboxOptions configures a single sandboxed execution.
type SandboxOptions struct {
	WorkDir         string
	ReadOnlyMounts  []string
	ReadWriteMounts []string
	AllowNetwork    bool
	Timeout         time.Duration
	MemoryLimitMB   int
	MaxProcesses    int
	Env             []string
}

// SandboxResult captures the outcome of a sandboxed command.
type SandboxResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	Duration   time.Duration
	Killed     bool
	KillReason string
}

// SandboxCaps describes what isolation capabilities the provider supports.
type SandboxCaps struct {
	PIDIsolation        bool
	NetworkIsolation    bool
	FilesystemIsolation bool
	ResourceLimits      bool
	Platform            string
}

// Config holds user-configurable sandbox settings read from ~/.siply/config.yaml.
type Config struct {
	Enabled            bool     `yaml:"enabled"`
	FailIfUnavailable  bool     `yaml:"fail_if_unavailable"`
	ExtraReadPaths     []string `yaml:"extra_read_paths"`
	ExtraWritePaths    []string `yaml:"extra_write_paths"`
	AllowNetwork       bool     `yaml:"allow_network"`
	MemoryLimitMB      int      `yaml:"memory_limit_mb"`
	MaxProcesses       int      `yaml:"max_processes"`
}

// DefaultConfig returns sandbox defaults for Pro users.
func DefaultConfig() Config {
	return Config{
		Enabled:           true,
		FailIfUnavailable: false,
		AllowNetwork:      false,
		MemoryLimitMB:     512,
		MaxProcesses:      100,
	}
}
