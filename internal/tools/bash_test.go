// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
