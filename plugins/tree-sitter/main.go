// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// tree-sitter is a Tier 3 plugin that provides code intelligence via
// tree-sitter parsing — Go & Python symbol extraction for AI context injection.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

func strPtr(s string) *string { return &s }

type treeSitterPlugin struct {
	siplyv1.UnimplementedSiplyPluginServiceServer

	mu          sync.RWMutex
	name        string
	hostAddr    string
	hostClient  siplyv1.SiplyHostServiceClient
	hostConn    *grpc.ClientConn
	initialized bool

	rootDir string
	parser  *Parser
	cache   *FileCache
	gated   bool
}

func main() {
	hostAddr := os.Getenv("SIPLY_HOST_ADDR")
	pluginName := os.Getenv("SIPLY_PLUGIN_NAME")

	if hostAddr == "" || pluginName == "" {
		slog.Error("missing required env vars", "SIPLY_HOST_ADDR", hostAddr, "SIPLY_PLUGIN_NAME", pluginName)
		os.Exit(1)
	}

	plugin := &treeSitterPlugin{
		name:     pluginName,
		hostAddr: hostAddr,
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		slog.Error("failed to listen", "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	siplyv1.RegisterSiplyPluginServiceServer(srv, plugin)

	fmt.Printf("PLUGIN_ADDR=%s\n", lis.Addr().String())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		srv.GracefulStop()
	}()

	if err := srv.Serve(lis); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func (p *treeSitterPlugin) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return &siplyv1.InitializeResponse{Success: true}, nil
	}

	conn, err := grpc.NewClient(p.hostAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return &siplyv1.InitializeResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("connect to host: %v", err)),
		}, nil
	}
	p.hostConn = conn
	p.hostClient = siplyv1.NewSiplyHostServiceClient(conn)

	cwd := os.Getenv("SIPLY_WORKING_DIR")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if cwd == "" {
		cwd = "."
	}
	p.rootDir = cwd

	p.parser = NewParser()
	p.cache = NewFileCache(p.parser, 10000)

	// Start file watcher for cache invalidation.
	if err := p.cache.StartWatcher(p.rootDir); err != nil {
		slog.Warn("tree-sitter: file watcher failed, incremental parsing disabled", "err", err)
	}

	p.initialized = true
	p.publishStatus("ready")

	return &siplyv1.InitializeResponse{
		Success:      true,
		Capabilities: []string{"code-intelligence", "symbols"},
	}, nil
}

func (p *treeSitterPlugin) Execute(_ context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	p.mu.RLock()
	if !p.initialized {
		p.mu.RUnlock()
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("not initialized")}, nil
	}
	p.mu.RUnlock()

	switch req.GetAction() {
	case "symbols":
		return p.handleSymbols(req.GetPayload())
	case "prequery":
		return p.handlePreQuery(req.GetPayload())
	default:
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("unknown action: %s", req.GetAction())),
		}, nil
	}
}

func (p *treeSitterPlugin) handleSymbols(payload []byte) (*siplyv1.ExecuteResponse, error) {
	path := strings.TrimSpace(string(payload))
	if path == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("empty path")}, nil
	}

	symbols, err := p.cache.GetOrParse(path)
	if err != nil {
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("parse failed: %v", err)),
		}, nil
	}

	result := FormatSymbols(symbols)
	return &siplyv1.ExecuteResponse{Success: true, Result: []byte(result)}, nil
}

func (p *treeSitterPlugin) handlePreQuery(payload []byte) (*siplyv1.ExecuteResponse, error) {
	workspacePath := strings.TrimSpace(string(payload))
	if workspacePath == "" {
		p.mu.RLock()
		workspacePath = p.rootDir
		p.mu.RUnlock()
	}

	codeCtx, stats := GenerateContext(p.cache, workspacePath)
	if codeCtx == "" {
		return &siplyv1.ExecuteResponse{Success: true, Result: []byte("")}, nil
	}

	p.publishCodeIntelEvent(stats)

	return &siplyv1.ExecuteResponse{Success: true, Result: []byte(codeCtx)}, nil
}

func (p *treeSitterPlugin) HandleEvent(_ context.Context, _ *siplyv1.HandleEventRequest) (*siplyv1.HandleEventResponse, error) {
	return &siplyv1.HandleEventResponse{}, nil
}

func (p *treeSitterPlugin) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cache != nil {
		p.cache.StopWatcher()
	}
	if p.hostConn != nil {
		p.hostConn.Close()
	}
	p.initialized = false
	return &siplyv1.ShutdownResponse{}, nil
}

func (p *treeSitterPlugin) publishStatus(message string) {
	p.mu.RLock()
	client := p.hostClient
	name := p.name
	p.mu.RUnlock()

	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.PublishStatus(ctx, &siplyv1.PublishStatusRequest{
		PluginName: name,
		Message:    message,
	})
	if err != nil {
		slog.Debug("tree-sitter: publish status failed", "err", err)
	}
}

func (p *treeSitterPlugin) publishCodeIntelEvent(stats ContextStats) {
	p.mu.RLock()
	client := p.hostClient
	name := p.name
	p.mu.RUnlock()

	if client == nil {
		return
	}

	payload, jsonErr := json.Marshal(map[string]any{
		"fileCount":    stats.FileCount,
		"symbolCount":  stats.SymbolCount,
		"parseTimeMS":  stats.ParseTimeMS,
		"cacheHitRate": stats.CacheHitRate,
	})
	if jsonErr != nil {
		slog.Debug("tree-sitter: marshal code-intel event failed", "err", jsonErr)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.PublishEvent(ctx, &siplyv1.PublishEventRequest{
		EventType:  "code-intel.stats",
		PluginName: name,
		Payload:    payload,
	})
	if err != nil {
		slog.Debug("tree-sitter: publish code-intel event failed", "err", err)
	}
}
