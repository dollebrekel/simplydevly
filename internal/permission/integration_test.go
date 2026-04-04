package permission_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/tools"
)

// stubTool is a minimal tool for integration testing.
type stubTool struct {
	name        string
	destructive bool
}

func (s *stubTool) Name() string                 { return s.name }
func (s *stubTool) Description() string          { return "stub tool for integration test" }
func (s *stubTool) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (s *stubTool) Destructive() bool            { return s.destructive }
func (s *stubTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "executed", nil
}

func TestIntegration_DefaultMode_FileRead_Executes(t *testing.T) {
	eval, err := permission.NewEvaluator(permission.Config{Mode: permission.ModeDefault})
	require.NoError(t, err)

	reg := tools.NewRegistry(eval)
	require.NoError(t, reg.Register(&stubTool{name: "file_read"}))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:   "file_read",
		Input:  json.RawMessage(`{"path": "/tmp/test"}`),
		Source: "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, "executed", resp.Output)
	assert.False(t, resp.IsError)
}

func TestIntegration_DefaultMode_Bash_ConfirmationRequired(t *testing.T) {
	eval, err := permission.NewEvaluator(permission.Config{Mode: permission.ModeDefault})
	require.NoError(t, err)

	reg := tools.NewRegistry(eval)
	require.NoError(t, reg.Register(&stubTool{name: "bash"}))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:   "bash",
		Input:  json.RawMessage(`{"command": "ls"}`),
		Source: "agent",
	})
	// Ask verdict → "confirmation required", no error.
	assert.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Equal(t, "confirmation required", resp.Output)
}

func TestIntegration_YoloMode_Bash_Executes(t *testing.T) {
	eval, err := permission.NewEvaluator(permission.Config{Mode: permission.ModeYolo})
	require.NoError(t, err)

	reg := tools.NewRegistry(eval)
	require.NoError(t, reg.Register(&stubTool{name: "bash"}))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:   "bash",
		Input:  json.RawMessage(`{"command": "ls"}`),
		Source: "agent",
	})
	require.NoError(t, err)
	assert.Equal(t, "executed", resp.Output)
	assert.False(t, resp.IsError)
}

func TestIntegration_DefaultMode_Destructive_ConfirmationRequired(t *testing.T) {
	eval, err := permission.NewEvaluator(permission.Config{Mode: permission.ModeDefault})
	require.NoError(t, err)

	reg := tools.NewRegistry(eval)
	require.NoError(t, reg.Register(&stubTool{name: "file_write", destructive: true}))

	resp, err := reg.Execute(context.Background(), core.ToolRequest{
		Name:   "file_write",
		Input:  json.RawMessage(`{"path": "/etc/passwd"}`),
		Source: "agent",
	})
	// Destructive override → Ask → "confirmation required".
	assert.NoError(t, err)
	assert.True(t, resp.IsError)
	assert.Equal(t, "confirmation required", resp.Output)
}
