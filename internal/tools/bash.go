// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/sandbox"
)

const (
	defaultBashTimeout = 30 * time.Second
	maxBashTimeout     = 300 * time.Second
	maxOutputBytes     = 100 * 1024 // 100 KB
)

// BashTool executes shell commands, optionally inside an OS-level sandbox.
type BashTool struct {
	Sandbox     sandbox.SandboxProvider
	SandboxCfg  sandbox.Config
	FeatureGate core.FeatureGate
	WorkDir     string
}

type bashInput struct {
	Command string `json:"command"`
	Timeout *int   `json:"timeout,omitempty"` // seconds
}

func (t *BashTool) Name() string        { return "bash" }
func (t *BashTool) Description() string { return "Execute a shell command" }
func (t *BashTool) Destructive() bool   { return true }
func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to execute"},"timeout":{"type":"integer","description":"Timeout in seconds (default 30, max 300)"}},"required":["command"]}`)
}

func (t *BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params bashInput
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("bash: invalid input: %w", err)
	}
	if params.Command == "" {
		return "", fmt.Errorf("bash: command is required")
	}

	timeout := defaultBashTimeout
	if params.Timeout != nil {
		timeout = time.Duration(*params.Timeout) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
		if timeout <= 0 {
			timeout = defaultBashTimeout
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use sandbox when available, enabled, and not feature-gated.
	if t.Sandbox != nil && t.Sandbox.Available() && t.sandboxAllowed(ctx) {
		return t.executeSandboxed(ctx, params.Command, timeout)
	}

	return t.executeDirect(ctx, params.Command, timeout)
}

// sandboxAllowed checks FeatureGate — returns false for Free users.
func (t *BashTool) sandboxAllowed(ctx context.Context) bool {
	if t.FeatureGate == nil {
		return true
	}
	return t.FeatureGate.Guard(ctx, "execution-sandbox") == nil
}

func (t *BashTool) executeSandboxed(ctx context.Context, command string, timeout time.Duration) (string, error) {
	workDir := t.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	opts := sandbox.BuildOptions(t.SandboxCfg, workDir)
	opts.Timeout = timeout

	result, err := t.Sandbox.Execute(ctx, command, opts)

	output := result.Stdout
	if result.Stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += result.Stderr
	}

	// Truncate large output.
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + fmt.Sprintf("\n... output truncated (exceeded %d bytes)", maxOutputBytes)
	}

	if err != nil {
		if result.Killed {
			return output, fmt.Errorf("bash: %s", result.KillReason)
		}
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return output, fmt.Errorf("bash: command timed out after %s", timeout)
			}
			return output, fmt.Errorf("bash: command canceled: %w", ctx.Err())
		}
		if result.ExitCode != 0 {
			return fmt.Sprintf("%s\nexit code: %d", output, result.ExitCode), fmt.Errorf("bash: exit code %d", result.ExitCode)
		}
		return output, fmt.Errorf("bash: %w", err)
	}

	return output, nil
}

func (t *BashTool) executeDirect(ctx context.Context, command string, timeout time.Duration) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()

	// Truncate large output.
	output := string(out)
	if len(out) > maxOutputBytes {
		output = string(out[:maxOutputBytes]) + fmt.Sprintf("\n... output truncated (exceeded %d bytes)", maxOutputBytes)
	}

	if err != nil {
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return output, fmt.Errorf("bash: command timed out after %s", timeout)
			}
			return output, fmt.Errorf("bash: command canceled: %w", ctx.Err())
		}
		// Non-zero exit code — return output with error indicator.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Sprintf("%s\nexit code: %d", output, exitErr.ExitCode()), fmt.Errorf("bash: exit code %d", exitErr.ExitCode())
		}
		return output, fmt.Errorf("bash: %w", err)
	}

	return output, nil
}
