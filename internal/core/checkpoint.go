// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package core

import (
	"context"
	"encoding/json"
	"time"
)

// CheckpointManager manages conversation state checkpoints for rewind support.
type CheckpointManager interface {
	Checkpoint(ctx context.Context, step StepCheckpoint) error
	List(sessionID string) ([]CheckpointMeta, error)
	Load(sessionID string, step int) (*Checkpoint, error)
	Prune(maxBytes int64) error
	DeleteAfterStep(sessionID string, step int) error
	SessionID() string
	Close() error
}

// StepCheckpoint captures the full state at a single tool execution boundary.
type StepCheckpoint struct {
	SessionID    string            `json:"session_id"`
	StepNumber   int               `json:"step_number"`
	Timestamp    time.Time         `json:"timestamp"`
	ToolName     string            `json:"tool_name"`
	ToolInput    json.RawMessage   `json:"tool_input"`
	ToolOutput   string            `json:"tool_output"`
	ToolDuration time.Duration     `json:"tool_duration"`
	Messages     []Message         `json:"messages"`
	FileHashes   map[string]string `json:"file_hashes,omitempty"`
}

// Checkpoint is a full checkpoint including disk metadata.
type Checkpoint struct {
	StepCheckpoint
	DiskSize int64 `json:"disk_size"`
}

// CheckpointMeta is a lightweight view for listing checkpoints.
type CheckpointMeta struct {
	StepNumber   int       `json:"step_number"`
	Timestamp    time.Time `json:"timestamp"`
	ToolName     string    `json:"tool_name"`
	MessageCount int       `json:"message_count"`
	FileCount    int       `json:"file_count"`
	DiskSize     int64     `json:"disk_size"`
}

// CheckpointEvent is published via EventBus on checkpoint or rewind actions.
type CheckpointEvent struct {
	SessionID    string `json:"session_id"`
	StepNumber   int    `json:"step_number"`
	ToolName     string `json:"tool_name"`
	Action       string `json:"action"` // "checkpoint" or "rewind"
	DurationMS   int64  `json:"duration_ms"`
	MessageCount int    `json:"message_count"`
	FileCount    int    `json:"file_count"`
	Ts           time.Time
}

func (e *CheckpointEvent) Type() string         { return "checkpoint" }
func (e *CheckpointEvent) Timestamp() time.Time { return e.Ts }

// CheckpointConfig holds user-configurable checkpoint settings.
type CheckpointConfig struct {
	Enabled           *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	MaxStorageMB      *int  `yaml:"max_storage_mb,omitempty" json:"max_storage_mb,omitempty"`
	IncludeFileHashes *bool `yaml:"include_file_hashes,omitempty" json:"include_file_hashes,omitempty"`
	PruneOnStart      *bool `yaml:"prune_on_start,omitempty" json:"prune_on_start,omitempty"`
}
