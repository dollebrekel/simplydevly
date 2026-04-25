// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SeatbeltSandbox uses macOS sandbox-exec for process isolation.
type SeatbeltSandbox struct {
	cfg        Config
	sandboxBin string
}

func NewProvider(cfg Config) SandboxProvider {
	bin, err := exec.LookPath("sandbox-exec")
	if err != nil {
		slog.Warn("sandbox unavailable, sandbox-exec not found in PATH")
		return &SeatbeltSandbox{cfg: cfg}
	}
	return &SeatbeltSandbox{cfg: cfg, sandboxBin: bin}
}

func (s *SeatbeltSandbox) Available() bool {
	return s.sandboxBin != ""
}

func (s *SeatbeltSandbox) Capabilities() SandboxCaps {
	return SandboxCaps{
		PIDIsolation:        false,
		NetworkIsolation:    true,
		FilesystemIsolation: true,
		ResourceLimits:      true,
		Platform:            "darwin",
	}
}

func (s *SeatbeltSandbox) Close() error { return nil }

func (s *SeatbeltSandbox) Execute(ctx context.Context, cmd string, opts SandboxOptions) (SandboxResult, error) {
	if !s.Available() {
		return SandboxResult{}, ErrUnavailable
	}

	profile := s.generateProfile(opts)

	// Write profile to temp file.
	profileFile, err := os.CreateTemp("", "siply-sandbox-*.sb")
	if err != nil {
		return SandboxResult{}, fmt.Errorf("sandbox: create profile file: %w", err)
	}
	defer os.Remove(profileFile.Name())

	if _, err := profileFile.WriteString(profile); err != nil {
		profileFile.Close()
		return SandboxResult{}, fmt.Errorf("sandbox: write profile: %w", err)
	}
	profileFile.Close()

	// Build sandboxed command with resource limits via ulimit.
	wrappedCmd := cmd
	if opts.MemoryLimitMB > 0 {
		limitKB := opts.MemoryLimitMB * 1024
		wrappedCmd = fmt.Sprintf("ulimit -v %d; %s", limitKB, cmd)
	}

	start := time.Now()
	proc := exec.CommandContext(ctx, s.sandboxBin, "-f", profileFile.Name(), "bash", "-c", wrappedCmd)

	var stdout, stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr
	proc.Env = opts.Env
	if len(proc.Env) == 0 {
		proc.Env = os.Environ()
	}

	execErr := proc.Run()
	duration := time.Since(start)

	result := SandboxResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	// Check stderr for Seatbelt violation format: "process(pid) deny(1) operation path".
	if stderrStr := stderr.String(); strings.Contains(stderrStr, "deny(") {
		slog.Warn("sandbox: Seatbelt violation detected", "stderr", stderrStr)
	}

	if execErr != nil {
		if exitErr, ok := execErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			if result.ExitCode == -1 || result.ExitCode == 137 || result.ExitCode == 9 {
				result.Killed = true
				result.KillReason = "process killed (signal or resource limit)"
				return result, ErrKilled
			}
		}
		if ctx.Err() != nil {
			result.Killed = true
			result.KillReason = "context deadline exceeded"
		}
		return result, execErr
	}

	return result, nil
}

// sensitiveHomeDirs are paths under $HOME hidden from the sandbox to prevent credential exfiltration.
var sensitiveHomeDirs = []string{
	".ssh", ".gnupg", ".aws", ".azure", ".gcloud",
	".kube", ".docker", ".config/gcloud", ".netrc",
}

func (s *SeatbeltSandbox) generateProfile(opts SandboxOptions) string {
	var sb strings.Builder

	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n")
	sb.WriteString("(allow process*)\n")
	sb.WriteString("(allow process-exec)\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")

	// System read paths.
	roSysPaths := []string{
		"/usr", "/bin", "/sbin",
		"/Library", "/System",
		"/private/var/db",
		"/dev",
	}
	// Homebrew on Apple Silicon.
	if _, err := os.Stat("/opt/homebrew"); err == nil {
		roSysPaths = append(roSysPaths, "/opt/homebrew")
	}

	sb.WriteString("(allow file-read*\n")
	for _, p := range roSysPaths {
		fmt.Fprintf(&sb, "  (subpath %q)\n", p)
	}
	sb.WriteString(")\n")

	// Home directory: deny sensitive paths first, then allow read-only.
	// Deny takes precedence over allow in SBPL.
	home, err := os.UserHomeDir()
	if err == nil {
		for _, rel := range sensitiveHomeDirs {
			p := filepath.Join(home, rel)
			if _, err := os.Stat(p); err == nil {
				fmt.Fprintf(&sb, "(deny file-read* (subpath %q))\n", p)
			}
		}
		fmt.Fprintf(&sb, "(allow file-read-data (subpath %q))\n", home)
	}

	// Workspace: read-write.
	if opts.WorkDir != "" {
		fmt.Fprintf(&sb, "(allow file-write* (subpath %q))\n", opts.WorkDir)
	}

	// Private tmp.
	sb.WriteString("(allow file-write* (subpath \"/private/tmp\"))\n")

	// Extra mounts.
	for _, p := range opts.ReadOnlyMounts {
		fmt.Fprintf(&sb, "(allow file-read* (subpath %q))\n", p)
	}
	for _, p := range opts.ReadWriteMounts {
		fmt.Fprintf(&sb, "(allow file-read* (subpath %q))\n", p)
		fmt.Fprintf(&sb, "(allow file-write* (subpath %q))\n", p)
	}

	// Network.
	if opts.AllowNetwork {
		sb.WriteString("(allow network*)\n")
	}

	return sb.String()
}
