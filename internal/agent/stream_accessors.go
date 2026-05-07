// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

// Text returns the streamed text chunk. This accessor allows external packages
// (like cmd/siply/run.go) to extract text from stream.text events published
// by the agent via the EventBus.
func (e *streamTextEvent) Text() string { return e.text }

// ToolName returns the tool name from a stream.tool_call event.
func (e *streamToolCallEvent) ToolName() string { return e.toolName }
