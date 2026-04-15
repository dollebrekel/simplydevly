// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"siply.dev/siply/internal/core"
)

func TestBuildQueryRequest_DefaultSystemPrompt(t *testing.T) {
	messages := []core.Message{{Role: "user", Content: "hello"}}
	req := buildQueryRequest(messages, "", nil, nil, time.Time{})

	assert.Equal(t, defaultSystemPrompt, req.SystemPrompt)
	assert.Equal(t, messages, req.Messages)
	assert.Equal(t, defaultMaxTokens, req.MaxTokens)
}

func TestBuildQueryRequest_CustomSystemPrompt(t *testing.T) {
	req := buildQueryRequest(nil, "custom prompt", nil, nil, time.Time{})
	assert.Equal(t, "custom prompt", req.SystemPrompt)
}

func TestBuildQueryRequest_IncludesTools(t *testing.T) {
	tools := []core.ToolDefinition{
		{Name: "file_read", Description: "Read a file"},
		{Name: "bash", Description: "Run a command"},
	}

	req := buildQueryRequest(nil, "", tools, nil, time.Time{})
	assert.Len(t, req.Tools, 2)
	assert.Equal(t, "file_read", req.Tools[0].Name)
	assert.Equal(t, "bash", req.Tools[1].Name)
}

func TestBuildQueryRequest_MaxTokensDefault(t *testing.T) {
	req := buildQueryRequest(nil, "", nil, nil, time.Time{})
	assert.Equal(t, 4096, req.MaxTokens)
}

func TestBuildQueryRequest_PassesHints(t *testing.T) {
	hints := map[string]string{"task.category": "preprocess"}
	req := buildQueryRequest(nil, "", nil, hints, time.Time{})
	assert.Equal(t, hints, req.Hints)
}

func TestBuildQueryRequest_NilHints(t *testing.T) {
	req := buildQueryRequest(nil, "", nil, nil, time.Time{})
	assert.Nil(t, req.Hints)
}

func TestBuildQueryRequest_TaskStartTimePropagated(t *testing.T) {
	ts := time.Date(2026, 4, 14, 9, 0, 0, 0, time.UTC)
	req := buildQueryRequest(nil, "", nil, nil, ts)
	assert.Equal(t, ts, req.TaskStartTime)
}

func TestBuildQueryRequest_ZeroTaskStartTime(t *testing.T) {
	req := buildQueryRequest(nil, "", nil, nil, time.Time{})
	assert.True(t, req.TaskStartTime.IsZero())
}
