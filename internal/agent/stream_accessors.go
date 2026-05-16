// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package agent

import "encoding/json"

// Text returns the streamed text chunk. This accessor allows external packages
// (like cmd/siply/run.go) to extract text from stream.text events published
// by the agent via the EventBus.
func (e *streamTextEvent) Text() string { return e.text }

// ToolName returns the tool name from a stream.tool_call event.
func (e *streamToolCallEvent) ToolName() string { return e.toolName }

// ToolID returns the provider tool call identifier, when available.
func (e *streamToolCallEvent) ToolID() string { return e.toolID }

// Input returns the JSON tool input from a stream.tool_call event.
func (e *streamToolCallEvent) Input() json.RawMessage { return e.input }
