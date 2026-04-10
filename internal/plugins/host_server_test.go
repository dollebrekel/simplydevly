// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
	"siply.dev/siply/internal/core"
)

// --- Mock implementations ---

type mockToolExecutorForHost struct {
	execFn func(ctx context.Context, req core.ToolRequest) (core.ToolResponse, error)
}

func (m *mockToolExecutorForHost) Init(_ context.Context) error              { return nil }
func (m *mockToolExecutorForHost) Start(_ context.Context) error             { return nil }
func (m *mockToolExecutorForHost) Stop(_ context.Context) error              { return nil }
func (m *mockToolExecutorForHost) Health() error                             { return nil }
func (m *mockToolExecutorForHost) ListTools() []core.ToolDefinition          { return nil }
func (m *mockToolExecutorForHost) GetTool(_ string) (core.ToolDefinition, error) {
	return core.ToolDefinition{}, nil
}
func (m *mockToolExecutorForHost) Execute(ctx context.Context, req core.ToolRequest) (core.ToolResponse, error) {
	if m.execFn != nil {
		return m.execFn(ctx, req)
	}
	return core.ToolResponse{Output: "default"}, nil
}

type mockCredentialStore struct {
	creds map[string]map[string]core.Credential // pluginName → key → credential
}

func (m *mockCredentialStore) Init(_ context.Context) error  { return nil }
func (m *mockCredentialStore) Start(_ context.Context) error { return nil }
func (m *mockCredentialStore) Stop(_ context.Context) error  { return nil }
func (m *mockCredentialStore) Health() error                 { return nil }
func (m *mockCredentialStore) GetProvider(_ context.Context, _ string) (core.Credential, error) {
	return core.Credential{}, errors.New("not implemented")
}
func (m *mockCredentialStore) SetProvider(_ context.Context, _ string, _ core.Credential) error {
	return errors.New("not implemented")
}
func (m *mockCredentialStore) GetPluginCredential(_ context.Context, pluginName string, key string) (core.Credential, error) {
	if m.creds != nil {
		if pCreds, ok := m.creds[pluginName]; ok {
			if cred, ok := pCreds[key]; ok {
				return cred, nil
			}
		}
	}
	return core.Credential{}, errors.New("not found")
}
func (m *mockCredentialStore) SetPluginCredential(_ context.Context, _ string, _ string, _ core.Credential) error {
	return nil
}

type mockStatusCollector struct {
	published []core.StatusUpdate
}

func (m *mockStatusCollector) Init(_ context.Context) error  { return nil }
func (m *mockStatusCollector) Start(_ context.Context) error { return nil }
func (m *mockStatusCollector) Stop(_ context.Context) error  { return nil }
func (m *mockStatusCollector) Health() error                 { return nil }
func (m *mockStatusCollector) Publish(update core.StatusUpdate) {
	m.published = append(m.published, update)
}
func (m *mockStatusCollector) Subscribe() (<-chan core.StatusUpdate, func()) {
	ch := make(chan core.StatusUpdate)
	return ch, func() { close(ch) }
}
func (m *mockStatusCollector) Snapshot() map[string]core.StatusUpdate {
	return nil
}

type mockConfigProvider struct {
	configs map[string]map[string]any
}

func (m *mockConfigProvider) GetPluginConfig(pluginName string) (map[string]any, bool) {
	if m.configs == nil {
		return nil, false
	}
	cfg, ok := m.configs[pluginName]
	return cfg, ok
}

// --- Tests ---

func startTestHostServer(t *testing.T, opts HostServerOptions) (*HostServer, siplyv1.SiplyHostServiceClient) {
	t.Helper()
	hs := NewHostServer(opts)
	require.NoError(t, hs.Start(context.Background()))
	t.Cleanup(func() { hs.Stop(context.Background()) })

	// Connect as gRPC client.
	//nolint:staticcheck // DialContext is deprecated but WithBlock isn't supported by NewClient
	conn, err := grpc.DialContext(context.Background(), hs.Addr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := siplyv1.NewSiplyHostServiceClient(conn)
	return hs, client
}

func TestHostServer_ExecuteTool_RoutesThrough(t *testing.T) {
	var captured core.ToolRequest
	executor := &mockToolExecutorForHost{
		execFn: func(_ context.Context, req core.ToolRequest) (core.ToolResponse, error) {
			captured = req
			return core.ToolResponse{Output: "tool result"}, nil
		},
	}

	_, client := startTestHostServer(t, HostServerOptions{
		ToolExecutor: executor,
	})

	params := json.RawMessage(`{"key":"value"}`)
	resp, err := client.ExecuteTool(context.Background(), &siplyv1.ExecuteToolRequest{
		ToolName:   "file_read",
		Parameters: params,
		Metadata:   map[string]string{"plugin_name": "test-plugin"},
	})
	require.NoError(t, err)
	assert.True(t, resp.GetSuccess())
	assert.Equal(t, []byte("tool result"), resp.GetOutput())

	assert.Equal(t, "file_read", captured.Name)
	assert.Equal(t, "plugin:test-plugin", captured.Source)
}

func TestHostServer_ExecuteTool_EmptyToolNameRejected(t *testing.T) {
	_, client := startTestHostServer(t, HostServerOptions{
		ToolExecutor: &mockToolExecutorForHost{},
	})

	_, err := client.ExecuteTool(context.Background(), &siplyv1.ExecuteToolRequest{
		ToolName: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool_name is required")
}

func TestHostServer_GetCredential_EmptyPluginNameRejected(t *testing.T) {
	_, client := startTestHostServer(t, HostServerOptions{
		CredentialStore: &mockCredentialStore{},
	})

	_, err := client.GetCredential(context.Background(), &siplyv1.GetCredentialRequest{
		PluginName:    "",
		CredentialKey: "api-key",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin_name is required")
}

func TestHostServer_GetCredential_EmptyCredentialKeyRejected(t *testing.T) {
	_, client := startTestHostServer(t, HostServerOptions{
		CredentialStore: &mockCredentialStore{},
	})

	_, err := client.GetCredential(context.Background(), &siplyv1.GetCredentialRequest{
		PluginName:    "test-plugin",
		CredentialKey: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credential_key is required")
}

func TestHostServer_GetCredential_NamespacedToPlugin(t *testing.T) {
	store := &mockCredentialStore{
		creds: map[string]map[string]core.Credential{
			"plugin-a": {"api-key": {Value: "secret-a"}},
			"plugin-b": {"api-key": {Value: "secret-b"}},
		},
	}

	_, client := startTestHostServer(t, HostServerOptions{
		CredentialStore: store,
	})

	// Plugin A gets its own credential.
	resp, err := client.GetCredential(context.Background(), &siplyv1.GetCredentialRequest{
		PluginName:    "plugin-a",
		CredentialKey: "api-key",
	})
	require.NoError(t, err)
	assert.True(t, resp.GetFound())
	assert.Equal(t, []byte("secret-a"), resp.GetValue())

	// Plugin B gets its own credential (not A's).
	resp, err = client.GetCredential(context.Background(), &siplyv1.GetCredentialRequest{
		PluginName:    "plugin-b",
		CredentialKey: "api-key",
	})
	require.NoError(t, err)
	assert.True(t, resp.GetFound())
	assert.Equal(t, []byte("secret-b"), resp.GetValue())

	// Plugin A can't see a nonexistent key.
	resp, err = client.GetCredential(context.Background(), &siplyv1.GetCredentialRequest{
		PluginName:    "plugin-a",
		CredentialKey: "nonexistent",
	})
	require.NoError(t, err)
	assert.False(t, resp.GetFound())
}

func TestHostServer_PublishStatus_MapProtoToCore(t *testing.T) {
	collector := &mockStatusCollector{}

	_, client := startTestHostServer(t, HostServerOptions{
		StatusCollector: collector,
	})

	_, err := client.PublishStatus(context.Background(), &siplyv1.PublishStatusRequest{
		PluginName: "test-plugin",
		Message:    "processing",
		Level:      "info",
		Timestamp:  1700000000,
	})
	require.NoError(t, err)

	require.Len(t, collector.published, 1)
	update := collector.published[0]
	assert.Equal(t, "test-plugin", update.Source)
	assert.Equal(t, "processing", update.Metrics["message"])
	assert.Equal(t, "info", update.Metrics["level"])
}

func TestHostServer_GetConfig_ReturnsPluginSpecificConfig(t *testing.T) {
	provider := &mockConfigProvider{
		configs: map[string]map[string]any{
			"test-plugin": {
				"api_url":  "https://example.com",
				"max_size": 42,
			},
		},
	}

	_, client := startTestHostServer(t, HostServerOptions{
		ConfigProvider: provider,
	})

	// Get specific key.
	resp, err := client.GetConfig(context.Background(), &siplyv1.GetConfigRequest{
		PluginName: "test-plugin",
		Key:        "api_url",
	})
	require.NoError(t, err)
	assert.True(t, resp.GetFound())

	var val string
	require.NoError(t, json.Unmarshal(resp.GetValue(), &val))
	assert.Equal(t, "https://example.com", val)

	// Get nonexistent plugin.
	resp, err = client.GetConfig(context.Background(), &siplyv1.GetConfigRequest{
		PluginName: "nonexistent",
		Key:        "api_url",
	})
	require.NoError(t, err)
	assert.False(t, resp.GetFound())
}

func TestHostServer_StartStop_Clean(t *testing.T) {
	hs := NewHostServer(HostServerOptions{})

	ctx := context.Background()
	require.NoError(t, hs.Start(ctx))
	assert.NotEmpty(t, hs.Addr())

	// Second start is no-op.
	require.NoError(t, hs.Start(ctx))

	require.NoError(t, hs.Stop(ctx))

	// Second stop is no-op.
	require.NoError(t, hs.Stop(ctx))
}
