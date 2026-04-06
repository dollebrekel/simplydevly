// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package permission

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
)

func TestNewEvaluator_ValidModes(t *testing.T) {
	modes := []Mode{ModeDefault, ModeAutoAccept, ModeYolo}
	for _, m := range modes {
		e, err := NewEvaluator(Config{Mode: m})
		require.NoError(t, err, "mode %q should be valid", m)
		assert.Equal(t, m, e.Mode())
	}
}

func TestNewEvaluator_InvalidMode(t *testing.T) {
	_, err := NewEvaluator(Config{Mode: "invalid"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mode")
}

func TestEvaluator_DefaultMode(t *testing.T) {
	e, err := NewEvaluator(Config{Mode: ModeDefault})
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name   string
		action core.Action
		want   core.ActionVerdict
	}{
		{"file_read → Allow", core.Action{Tool: "file_read"}, core.Allow},
		{"file_write → Allow", core.Action{Tool: "file_write"}, core.Allow},
		{"file_edit → Allow", core.Action{Tool: "file_edit"}, core.Allow},
		{"bash → Ask", core.Action{Tool: "bash"}, core.Ask},
		{"git_push → Ask", core.Action{Tool: "git_push"}, core.Ask},
		{"web → Ask", core.Action{Tool: "web"}, core.Ask},
		{"unknown_tool → Ask", core.Action{Tool: "unknown_tool"}, core.Ask},
		{"destructive bash → Ask", core.Action{Tool: "bash", Destructive: true}, core.Ask},
		{"destructive file_write → Ask", core.Action{Tool: "file_write", Destructive: true}, core.Ask},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, err := e.EvaluateAction(ctx, tt.action)
			require.NoError(t, err)
			assert.Equal(t, tt.want, verdict)
		})
	}
}

func TestEvaluator_AutoAcceptMode(t *testing.T) {
	e, err := NewEvaluator(Config{Mode: ModeAutoAccept})
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name   string
		action core.Action
		want   core.ActionVerdict
	}{
		{"file_read → Allow", core.Action{Tool: "file_read"}, core.Allow},
		{"file_write → Allow", core.Action{Tool: "file_write"}, core.Allow},
		{"bash → Allow", core.Action{Tool: "bash"}, core.Allow},
		{"web → Allow", core.Action{Tool: "web"}, core.Allow},
		{"git_push → Ask", core.Action{Tool: "git_push"}, core.Ask},
		{"unknown_tool → Ask", core.Action{Tool: "unknown_tool"}, core.Ask},
		{"destructive bash → Ask", core.Action{Tool: "bash", Destructive: true}, core.Ask},
		{"destructive file_read → Ask", core.Action{Tool: "file_read", Destructive: true}, core.Ask},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, err := e.EvaluateAction(ctx, tt.action)
			require.NoError(t, err)
			assert.Equal(t, tt.want, verdict)
		})
	}
}

func TestEvaluator_YoloMode(t *testing.T) {
	e, err := NewEvaluator(Config{Mode: ModeYolo})
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name   string
		action core.Action
		want   core.ActionVerdict
	}{
		{"file_read → Allow", core.Action{Tool: "file_read"}, core.Allow},
		{"bash → Allow", core.Action{Tool: "bash"}, core.Allow},
		{"git_push → Allow", core.Action{Tool: "git_push"}, core.Allow},
		{"destructive bash → Allow", core.Action{Tool: "bash", Destructive: true}, core.Allow},
		{"destructive rm → Allow", core.Action{Tool: "rm", Destructive: true}, core.Allow},
		{"unknown_tool → Allow", core.Action{Tool: "anything"}, core.Allow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, err := e.EvaluateAction(ctx, tt.action)
			require.NoError(t, err)
			assert.Equal(t, tt.want, verdict)
		})
	}
}

func TestEvaluator_DestructiveOverride(t *testing.T) {
	ctx := context.Background()

	// In default mode, a normally-allowed tool becomes Ask when destructive.
	e, err := NewEvaluator(Config{Mode: ModeDefault})
	require.NoError(t, err)

	verdict, err := e.EvaluateAction(ctx, core.Action{Tool: "file_read", Destructive: true})
	require.NoError(t, err)
	assert.Equal(t, core.Ask, verdict, "destructive file_read in default → Ask")

	// In auto-accept mode, destructive also overrides to Ask.
	e2, err := NewEvaluator(Config{Mode: ModeAutoAccept})
	require.NoError(t, err)

	verdict, err = e2.EvaluateAction(ctx, core.Action{Tool: "file_read", Destructive: true})
	require.NoError(t, err)
	assert.Equal(t, core.Ask, verdict, "destructive file_read in auto-accept → Ask")

	// In yolo mode, destructive is still Allow.
	e3, err := NewEvaluator(Config{Mode: ModeYolo})
	require.NoError(t, err)

	verdict, err = e3.EvaluateAction(ctx, core.Action{Tool: "file_read", Destructive: true})
	require.NoError(t, err)
	assert.Equal(t, core.Allow, verdict, "destructive file_read in yolo → Allow")
}

func TestEvaluator_SetMode(t *testing.T) {
	e, err := NewEvaluator(DefaultConfig())
	require.NoError(t, err)
	assert.Equal(t, ModeDefault, e.Mode())

	require.NoError(t, e.SetMode(ModeYolo))
	assert.Equal(t, ModeYolo, e.Mode())

	require.NoError(t, e.SetMode(ModeAutoAccept))
	assert.Equal(t, ModeAutoAccept, e.Mode())

	err = e.SetMode("bogus")
	assert.Error(t, err)
	assert.Equal(t, ModeAutoAccept, e.Mode(), "mode unchanged after invalid SetMode")
}

func TestEvaluator_SetMode_Concurrent(t *testing.T) {
	e, err := NewEvaluator(DefaultConfig())
	require.NoError(t, err)

	modes := []Mode{ModeDefault, ModeAutoAccept, ModeYolo}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m := modes[idx%len(modes)]
			_ = e.SetMode(m)
			_ = e.Mode()
			_, _ = e.EvaluateAction(context.Background(), core.Action{Tool: "bash"})
		}(i)
	}

	wg.Wait()
	// No race condition = success. Mode must be one of the valid modes.
	assert.True(t, e.Mode().Valid())
}

func TestEvaluator_Lifecycle(t *testing.T) {
	e, err := NewEvaluator(DefaultConfig())
	require.NoError(t, err)

	ctx := context.Background()
	assert.NoError(t, e.Init(ctx))
	assert.NoError(t, e.Start(ctx))
	assert.NoError(t, e.Stop(ctx))
	assert.NoError(t, e.Health())
}

func TestEvaluator_EvaluateCapabilities(t *testing.T) {
	e, err := NewEvaluator(DefaultConfig())
	require.NoError(t, err)

	verdict, err := e.EvaluateCapabilities(context.Background(), core.PluginMeta{
		Name:    "test-plugin",
		Version: "1.0.0",
		Tier:    1,
	})
	assert.NoError(t, err)
	assert.Equal(t, core.CapabilityVerdict{}, verdict)
}
