// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
	"siply.dev/siply/internal/core"
)

// HostServerOptions holds dependencies for HostServer construction.
type HostServerOptions struct {
	ToolExecutor    core.ToolExecutor
	CredentialStore core.CredentialStore
	ConfigProvider  ConfigProvider
	StatusCollector core.StatusCollector
}

// ConfigProvider provides plugin-specific configuration.
// Defined here to avoid importing the config package (dependency inversion).
type ConfigProvider interface {
	GetPluginConfig(pluginName string) (map[string]any, bool)
}

// HostServer implements the SiplyHostService gRPC service that plugins call back into.
type HostServer struct {
	siplyv1.UnimplementedSiplyHostServiceServer
	toolExecutor    core.ToolExecutor
	credentialStore core.CredentialStore
	configProvider  ConfigProvider
	statusCollector core.StatusCollector
	listener        net.Listener
	grpcServer      *grpc.Server
	addr            string
	mu              sync.Mutex
	started         bool
}

// NewHostServer creates a new HostServer with the given dependencies.
func NewHostServer(opts HostServerOptions) *HostServer {
	return &HostServer{
		toolExecutor:    opts.ToolExecutor,
		credentialStore: opts.CredentialStore,
		configProvider:  opts.ConfigProvider,
		statusCollector: opts.StatusCollector,
	}
}

// Start begins listening on a random localhost port and serving gRPC.
// Safe to call multiple times — subsequent calls are no-ops.
func (s *HostServer) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return nil
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("plugins: host_server: listen: %w", err)
	}

	srv := grpc.NewServer()
	siplyv1.RegisterSiplyHostServiceServer(srv, s)

	s.listener = lis
	s.grpcServer = srv
	s.addr = lis.Addr().String()
	s.started = true

	go func() {
		if err := srv.Serve(lis); err != nil {
			slog.Error("host server serve error", "err", err)
		}
	}()

	slog.Info("host server started", "addr", s.addr)
	return nil
}

// Stop gracefully shuts down the gRPC server.
func (s *HostServer) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	// Try graceful stop with timeout, fallback to hard stop.
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(defaultShutdownTimeout):
		s.grpcServer.Stop()
	}

	s.started = false
	s.addr = ""
	s.listener = nil
	s.grpcServer = nil
	slog.Info("host server stopped")
	return nil
}

// Addr returns the host:port address the server is listening on.
func (s *HostServer) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// ExecuteTool implements SiplyHostServiceServer.ExecuteTool.
// Routes tool requests through the ToolExecutor pipeline (includes PermissionEvaluator).
func (s *HostServer) ExecuteTool(ctx context.Context, req *siplyv1.ExecuteToolRequest) (*siplyv1.ExecuteToolResponse, error) {
	if req.GetToolName() == "" {
		return nil, status.Error(codes.InvalidArgument, "tool_name is required")
	}

	if s.toolExecutor == nil {
		return nil, status.Error(codes.Internal, "tool executor not configured")
	}

	toolReq := core.ToolRequest{
		Name:   req.GetToolName(),
		Input:  json.RawMessage(req.GetParameters()),
		Source: fmt.Sprintf("plugin:%s", req.GetMetadata()["plugin_name"]),
	}

	resp, err := s.toolExecutor.Execute(ctx, toolReq)
	if err != nil {
		errStr := err.Error()
		return &siplyv1.ExecuteToolResponse{
			Success: false,
			Error:   &errStr,
		}, nil
	}

	if resp.IsError {
		errStr := resp.Output
		return &siplyv1.ExecuteToolResponse{
			Success: false,
			Error:   &errStr,
			Output:  []byte(resp.Output),
		}, nil
	}

	return &siplyv1.ExecuteToolResponse{
		Success: true,
		Output:  []byte(resp.Output),
	}, nil
}

// GetCredential implements SiplyHostServiceServer.GetCredential.
// Credentials are namespaced — a plugin can only access its own credentials.
func (s *HostServer) GetCredential(ctx context.Context, req *siplyv1.GetCredentialRequest) (*siplyv1.GetCredentialResponse, error) {
	if req.GetPluginName() == "" {
		return nil, status.Error(codes.InvalidArgument, "plugin_name is required")
	}
	if req.GetCredentialKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "credential_key is required")
	}

	if s.credentialStore == nil {
		return &siplyv1.GetCredentialResponse{Found: false}, nil
	}

	cred, err := s.credentialStore.GetPluginCredential(ctx, req.GetPluginName(), req.GetCredentialKey())
	if err != nil {
		// Log the error to distinguish store failures from missing credentials (P6).
		slog.Debug("credential lookup failed", "plugin", req.GetPluginName(), "key", req.GetCredentialKey(), "err", err)
		return &siplyv1.GetCredentialResponse{Found: false}, nil
	}

	return &siplyv1.GetCredentialResponse{
		Value: []byte(cred.Value),
		Found: true,
	}, nil
}

// PublishStatus implements SiplyHostServiceServer.PublishStatus.
// Maps proto fields to core StatusUpdate (bridging the proto/core divergence).
func (s *HostServer) PublishStatus(_ context.Context, req *siplyv1.PublishStatusRequest) (*siplyv1.PublishStatusResponse, error) {
	if s.statusCollector == nil {
		return &siplyv1.PublishStatusResponse{}, nil
	}

	ts := time.Now()
	if req.GetTimestamp() > 0 {
		ts = time.Unix(req.GetTimestamp(), 0)
	}

	// Map proto fields to core StatusUpdate:
	// proto Message/Level → core Metrics["message"]/Metrics["level"]
	// proto PluginName → core Source
	update := core.StatusUpdate{
		Source: req.GetPluginName(),
		Metrics: map[string]any{
			"message": req.GetMessage(),
			"level":   req.GetLevel(),
		},
		Timestamp: ts,
	}

	s.statusCollector.Publish(update)
	return &siplyv1.PublishStatusResponse{}, nil
}

// GetConfig implements SiplyHostServiceServer.GetConfig.
// Returns plugin-specific config values serialized as JSON bytes.
func (s *HostServer) GetConfig(_ context.Context, req *siplyv1.GetConfigRequest) (*siplyv1.GetConfigResponse, error) {
	if s.configProvider == nil {
		return &siplyv1.GetConfigResponse{Found: false}, nil
	}

	pluginConfig, found := s.configProvider.GetPluginConfig(req.GetPluginName())
	if !found {
		return &siplyv1.GetConfigResponse{Found: false}, nil
	}

	// If a specific key is requested, look it up in the plugin config map.
	key := req.GetKey()
	if key != "" {
		val, ok := pluginConfig[key]
		if !ok {
			return &siplyv1.GetConfigResponse{Found: false}, nil
		}
		data, err := json.Marshal(val)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal config value: %v", err)
		}
		return &siplyv1.GetConfigResponse{
			Value: data,
			Found: true,
		}, nil
	}

	// Return entire plugin config.
	data, err := json.Marshal(pluginConfig)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal config: %v", err)
	}
	return &siplyv1.GetConfigResponse{
		Value: data,
		Found: true,
	}, nil
}
