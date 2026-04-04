package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const (
	defaultBashTimeout = 30 * time.Second
	maxBashTimeout     = 300 * time.Second
	maxOutputBytes     = 100 * 1024 // 100 KB
)

// BashTool executes shell commands.
type BashTool struct{}

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

	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	out, err := cmd.CombinedOutput()

	// Truncate large output.
	output := string(out)
	if len(out) > maxOutputBytes {
		output = string(out[:maxOutputBytes]) + fmt.Sprintf("\n... output truncated (exceeded %d bytes)", maxOutputBytes)
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return output, fmt.Errorf("bash: command timed out after %s", timeout)
		}
		// Non-zero exit code — return output with error indicator.
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Sprintf("%s\nexit code: %d", output, exitErr.ExitCode()), fmt.Errorf("bash: exit code %d", exitErr.ExitCode())
		}
		return output, fmt.Errorf("bash: %w", err)
	}

	return output, nil
}
