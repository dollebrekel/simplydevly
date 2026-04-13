// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// memory-default is a Tier 3 plugin that implements MemoryBackend via gRPC.
// It provides a three-layer memory architecture: session memory (short-term),
// durable extraction (key facts), and consolidation (long-term patterns).
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

// strPtr returns a pointer to the given string (for proto optional fields).
func strPtr(s string) *string { return &s }

// memoryPlugin implements the SiplyPluginService gRPC server.
type memoryPlugin struct {
	siplyv1.UnimplementedSiplyPluginServiceServer

	mu          sync.RWMutex
	name        string
	hostAddr    string
	hostClient  siplyv1.SiplyHostServiceClient
	hostConn    *grpc.ClientConn
	initialized bool

	// Three-layer memory architecture:
	session      map[string][]byte // Short-term: current session facts
	durable      map[string][]byte // Key facts extracted across sessions
	consolidated map[string][]byte // Long-term patterns and summaries
}

func main() {
	hostAddr := os.Getenv("SIPLY_HOST_ADDR")
	pluginName := os.Getenv("SIPLY_PLUGIN_NAME")

	if hostAddr == "" || pluginName == "" {
		slog.Error("missing required env vars", "SIPLY_HOST_ADDR", hostAddr, "SIPLY_PLUGIN_NAME", pluginName)
		os.Exit(1)
	}

	plugin := &memoryPlugin{
		name:         pluginName,
		hostAddr:     hostAddr,
		session:      make(map[string][]byte),
		durable:      make(map[string][]byte),
		consolidated: make(map[string][]byte),
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		slog.Error("failed to listen", "err", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	siplyv1.RegisterSiplyPluginServiceServer(srv, plugin)

	// Print the address for the host to connect — protocol requirement.
	fmt.Printf("PLUGIN_ADDR=%s\n", lis.Addr().String())

	// Handle graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigCh
		slog.Info("received shutdown signal")
		srv.GracefulStop()
	}()

	if err := srv.Serve(lis); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// Initialize sets up the plugin, connects to the host, and loads persisted state.
func (p *memoryPlugin) Initialize(_ context.Context, req *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return &siplyv1.InitializeResponse{Success: true}, nil
	}

	// Connect to host server for callbacks.
	conn, err := grpc.NewClient(p.hostAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return &siplyv1.InitializeResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("failed to connect to host: %v", err)),
		}, nil
	}
	p.hostConn = conn
	p.hostClient = siplyv1.NewSiplyHostServiceClient(conn)

	// Load persisted durable state from host storage.
	// Release write lock before blocking network I/O, re-acquire after.
	hostClient := p.hostClient
	pluginName := p.name
	p.mu.Unlock()
	durableState, consolidatedState := loadPersistedStateFromHost(hostClient, pluginName)
	p.mu.Lock()

	if durableState != nil {
		p.durable = durableState
	}
	if consolidatedState != nil {
		p.consolidated = consolidatedState
	}

	p.initialized = true
	p.publishStatus("initialized, 0 items")

	return &siplyv1.InitializeResponse{
		Success:      true,
		Capabilities: []string{"remember", "recall", "forget", "search"},
	}, nil
}

// Execute dispatches memory operations based on the action field.
func (p *memoryPlugin) Execute(_ context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	p.mu.RLock()
	if !p.initialized {
		p.mu.RUnlock()
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr("plugin not initialized"),
		}, nil
	}
	p.mu.RUnlock()

	switch req.GetAction() {
	case "remember":
		return p.handleRemember(req.GetPayload())
	case "recall":
		return p.handleRecall(req.GetPayload())
	case "forget":
		return p.handleForget(req.GetPayload())
	case "search":
		return p.handleSearch(req.GetPayload())
	default:
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("unknown action: %s", req.GetAction())),
		}, nil
	}
}

// Shutdown persists state and cleans up resources.
func (p *memoryPlugin) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.persistState()

	if p.hostConn != nil {
		p.hostConn.Close()
	}
	p.initialized = false

	return &siplyv1.ShutdownResponse{}, nil
}

// memoryRequest is the JSON payload for remember/recall/forget operations.
type memoryRequest struct {
	Key   string `json:"key"`
	Value []byte `json:"value,omitempty"`
	Layer string `json:"layer,omitempty"` // "session", "durable", "consolidated" — defaults to "session"
}

// searchRequest is the JSON payload for search operations.
type searchRequest struct {
	Query string `json:"query"`
	Layer string `json:"layer,omitempty"` // empty searches all layers
}

func (p *memoryPlugin) handleRemember(payload []byte) (*siplyv1.ExecuteResponse, error) {
	var req memoryRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("invalid payload: " + err.Error())}, nil
	}
	if req.Key == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("empty key")}, nil
	}

	p.mu.Lock()
	layer := p.getLayer(req.Layer)
	layer[req.Key] = req.Value
	count := len(p.session) + len(p.durable) + len(p.consolidated)
	p.mu.Unlock()

	p.publishStatus(fmt.Sprintf("%d items indexed", count))

	return &siplyv1.ExecuteResponse{Success: true}, nil
}

func (p *memoryPlugin) handleRecall(payload []byte) (*siplyv1.ExecuteResponse, error) {
	var req memoryRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("invalid payload: " + err.Error())}, nil
	}
	if req.Key == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("empty key")}, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	// Search across layers: session → durable → consolidated.
	if req.Layer != "" {
		layer := p.getLayerRLock(req.Layer)
		val, ok := layer[req.Key]
		if !ok {
			return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("key not found")}, nil
		}
		return &siplyv1.ExecuteResponse{Success: true, Result: val}, nil
	}

	for _, layer := range []map[string][]byte{p.session, p.durable, p.consolidated} {
		if val, ok := layer[req.Key]; ok {
			return &siplyv1.ExecuteResponse{Success: true, Result: val}, nil
		}
	}

	return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("key not found")}, nil
}

func (p *memoryPlugin) handleForget(payload []byte) (*siplyv1.ExecuteResponse, error) {
	var req memoryRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("invalid payload: " + err.Error())}, nil
	}
	if req.Key == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("empty key")}, nil
	}

	p.mu.Lock()
	if req.Layer != "" {
		layer := p.getLayer(req.Layer)
		delete(layer, req.Key)
	} else {
		delete(p.session, req.Key)
		delete(p.durable, req.Key)
		delete(p.consolidated, req.Key)
	}
	count := len(p.session) + len(p.durable) + len(p.consolidated)
	p.mu.Unlock()

	p.publishStatus(fmt.Sprintf("%d items indexed", count))

	return &siplyv1.ExecuteResponse{Success: true}, nil
}

func (p *memoryPlugin) handleSearch(payload []byte) (*siplyv1.ExecuteResponse, error) {
	var req searchRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("invalid payload: " + err.Error())}, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var results []json.RawMessage

	searchLayer := func(layerName string, data map[string][]byte) {
		for key, val := range data {
			if req.Query == "" || strings.Contains(key, req.Query) {
				entry, _ := json.Marshal(map[string]any{
					"key":   key,
					"layer": layerName,
					"value": val,
				})
				results = append(results, entry)
			}
		}
	}

	if req.Layer != "" {
		searchLayer(req.Layer, p.getLayerRLock(req.Layer))
	} else {
		searchLayer("session", p.session)
		searchLayer("durable", p.durable)
		searchLayer("consolidated", p.consolidated)
	}

	if results == nil {
		results = []json.RawMessage{}
	}

	encoded, err := json.Marshal(results)
	if err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("failed to encode results")}, nil
	}

	return &siplyv1.ExecuteResponse{Success: true, Result: encoded}, nil
}

// getLayer returns the map for the given layer name. Caller must hold write lock.
func (p *memoryPlugin) getLayer(name string) map[string][]byte {
	switch name {
	case "durable":
		return p.durable
	case "consolidated":
		return p.consolidated
	default:
		return p.session
	}
}

// getLayerRLock returns the map for the given layer name. Caller must hold read lock.
func (p *memoryPlugin) getLayerRLock(name string) map[string][]byte {
	switch name {
	case "durable":
		return p.durable
	case "consolidated":
		return p.consolidated
	default:
		return p.session
	}
}

// publishStatus sends a status update to the host's StatusCollector.
func (p *memoryPlugin) publishStatus(message string) {
	if p.hostClient == nil {
		return
	}
	_, err := p.hostClient.PublishStatus(context.Background(), &siplyv1.PublishStatusRequest{
		PluginName: p.name,
		Message:    message,
	})
	if err != nil {
		slog.Debug("failed to publish status", "err", err)
	}
}

// persistState serializes durable and consolidated layers for storage.
func (p *memoryPlugin) persistState() {
	if p.hostClient == nil {
		return
	}

	state := map[string]map[string][]byte{
		"durable":      p.durable,
		"consolidated": p.consolidated,
	}
	data, err := json.Marshal(state)
	if err != nil {
		slog.Warn("failed to marshal state for persistence", "err", err)
		return
	}

	// Use ExecuteTool to persist via the host's storage system.
	payload, _ := json.Marshal(map[string]any{
		"tool":  "storage_put",
		"path":  fmt.Sprintf("plugins/%s/state/memory.json", p.name),
		"value": string(data),
	})
	_, err = p.hostClient.ExecuteTool(context.Background(), &siplyv1.ExecuteToolRequest{
		ToolName:   "storage_put",
		Parameters: payload,
	})
	if err != nil {
		slog.Debug("failed to persist state", "err", err)
	}
}

// loadPersistedStateFromHost loads durable and consolidated layers from host storage.
// This is a free function (no receiver) so it can be called without holding the plugin lock.
func loadPersistedStateFromHost(hostClient siplyv1.SiplyHostServiceClient, pluginName string) (durable, consolidated map[string][]byte) {
	if hostClient == nil {
		return nil, nil
	}

	payload, _ := json.Marshal(map[string]any{
		"tool": "storage_get",
		"path": fmt.Sprintf("plugins/%s/state/memory.json", pluginName),
	})
	resp, err := hostClient.ExecuteTool(context.Background(), &siplyv1.ExecuteToolRequest{
		ToolName:   "storage_get",
		Parameters: payload,
	})
	if err != nil {
		slog.Debug("no persisted state found", "err", err)
		return nil, nil
	}
	if !resp.GetSuccess() {
		return nil, nil
	}

	var state map[string]map[string][]byte
	if err := json.Unmarshal(resp.GetOutput(), &state); err != nil {
		slog.Warn("failed to unmarshal persisted state", "err", err)
		return nil, nil
	}

	if d, ok := state["durable"]; ok && d != nil {
		durable = d
	}
	if c, ok := state["consolidated"]; ok && c != nil {
		consolidated = c
	}
	return durable, consolidated
}
