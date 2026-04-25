// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package sandbox

import (
	"fmt"
	"path/filepath"
)

// ConfigFromCore converts core.SandboxConfig to sandbox.Config with defaults applied.
func ConfigFromCore(enabled *bool, failIfUnavailable *bool, extraRead, extraWrite []string, allowNetwork *bool, memMB, maxProcs *int) Config {
	cfg := DefaultConfig()
	if enabled != nil {
		cfg.Enabled = *enabled
	}
	if failIfUnavailable != nil {
		cfg.FailIfUnavailable = *failIfUnavailable
	}
	if extraRead != nil {
		cfg.ExtraReadPaths = extraRead
	}
	if extraWrite != nil {
		cfg.ExtraWritePaths = extraWrite
	}
	if allowNetwork != nil {
		cfg.AllowNetwork = *allowNetwork
	}
	if memMB != nil {
		cfg.MemoryLimitMB = *memMB
	}
	if maxProcs != nil {
		cfg.MaxProcesses = *maxProcs
	}
	return cfg
}

// ValidateConfig checks that config paths are valid.
func ValidateConfig(cfg Config) error {
	for _, p := range cfg.ExtraReadPaths {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("sandbox: extra_read_paths must be absolute: %q", p)
		}
	}
	for _, p := range cfg.ExtraWritePaths {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("sandbox: extra_write_paths must be absolute: %q", p)
		}
	}
	if cfg.MemoryLimitMB < 0 {
		return fmt.Errorf("sandbox: memory_limit_mb must be non-negative: %d", cfg.MemoryLimitMB)
	}
	if cfg.MaxProcesses < 0 {
		return fmt.Errorf("sandbox: max_processes must be non-negative: %d", cfg.MaxProcesses)
	}
	return nil
}

// BuildOptions creates SandboxOptions from Config, applying workspace and extra mounts.
func BuildOptions(cfg Config, workDir string) SandboxOptions {
	opts := SandboxOptions{
		WorkDir:       workDir,
		AllowNetwork:  cfg.AllowNetwork,
		MemoryLimitMB: cfg.MemoryLimitMB,
		MaxProcesses:  cfg.MaxProcesses,
	}

	// Merge extra paths (deduplicated).
	seen := make(map[string]bool)
	for _, p := range cfg.ExtraReadPaths {
		if !seen[p] {
			opts.ReadOnlyMounts = append(opts.ReadOnlyMounts, p)
			seen[p] = true
		}
	}
	for _, p := range cfg.ExtraWritePaths {
		if !seen[p] {
			opts.ReadWriteMounts = append(opts.ReadWriteMounts, p)
			seen[p] = true
		}
	}

	return opts
}
