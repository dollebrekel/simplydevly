// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// session-intelligence is a Tier 3 plugin that persists session knowledge
// across sessions using local Ollama LLM distillation.
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

type sessionIntelligencePlugin struct {
	siplyv1.UnimplementedSiplyPluginServiceServer

	mu          sync.RWMutex
	name        string
	hostAddr    string
	hostClient  siplyv1.SiplyHostServiceClient
	hostConn    *grpc.ClientConn
	initialized bool

	store     *DistillateStore
	distiller *SessionDistiller
	consolid  *Consolidator
	config    Config
}

func main() {
	hostAddr := os.Getenv("SIPLY_HOST_ADDR")
	pluginName := os.Getenv("SIPLY_PLUGIN_NAME")

	if hostAddr == "" || pluginName == "" {
		slog.Error("missing required env vars", "SIPLY_HOST_ADDR", hostAddr, "SIPLY_PLUGIN_NAME", pluginName)
		os.Exit(1)
	}

	plugin := &sessionIntelligencePlugin{
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
		// Delay shutdown to allow in-flight session-end distillation RPCs to
		// start. GracefulStop then waits for any in-flight RPCs to complete.
		time.Sleep(2 * time.Second)
		srv.GracefulStop()
	}()

	if err := srv.Serve(lis); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func (p *sessionIntelligencePlugin) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
	p.mu.Lock()

	if p.initialized {
		p.mu.Unlock()
		return &siplyv1.InitializeResponse{Success: true}, nil
	}

	conn, err := grpc.NewClient(p.hostAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		p.mu.Unlock()
		return &siplyv1.InitializeResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("connect to host: %v", err)),
		}, nil
	}
	p.hostConn = conn
	p.hostClient = siplyv1.NewSiplyHostServiceClient(conn)

	cfg := DefaultConfig()
	configResp, configErr := p.hostClient.GetConfig(context.Background(), &siplyv1.GetConfigRequest{
		PluginName: p.name,
	})
	if configErr == nil && configResp.GetFound() {
		var userCfg map[string]any
		if json.Unmarshal(configResp.GetValue(), &userCfg) == nil {
			if v, ok := userCfg["enabled"].(bool); ok {
				cfg.Enabled = v
			}
			if v, ok := userCfg["model"].(string); ok {
				cfg.Model = v
			}
			if v, ok := userCfg["ollama_url"].(string); ok {
				cfg.OllamaURL = v
			}
			if v, ok := userCfg["min_turns"].(float64); ok {
				cfg.MinTurns = int(v)
			}
			if v, ok := userCfg["max_distillates"].(float64); ok {
				cfg.MaxDistillates = int(v)
			}
			if v, ok := userCfg["consolidation_tokens"].(float64); ok {
				cfg.ConsolidationTokens = int(v)
			}
		}
	}
	if err := ValidateConfig(cfg); err != nil {
		slog.Warn("session-intelligence: invalid config, using defaults", "err", err)
		cfg = DefaultConfig()
	}
	p.config = cfg

	client := NewOllamaClient(p.config.OllamaURL, p.config.Model)
	p.store = NewDistillateStore("")
	p.distiller = NewSessionDistiller(client, p.config.MinTurns)
	p.consolid = NewConsolidator(client, p.config.MaxDistillates, p.config.ConsolidationTokens)

	p.initialized = true
	p.mu.Unlock()

	p.publishStatus("ready")

	healthCtx, healthCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer healthCancel()
	if err := client.HealthCheck(healthCtx); err != nil {
		slog.Warn("session-intelligence: ollama not reachable, will degrade gracefully", "err", err)
		p.publishStatus("⚠ Session intelligence offline")
	}

	return &siplyv1.InitializeResponse{
		Success:      true,
		Capabilities: []string{"session-intelligence"},
	}, nil
}

func (p *sessionIntelligencePlugin) Execute(ctx context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	p.mu.RLock()
	if !p.initialized {
		p.mu.RUnlock()
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("not initialized")}, nil
	}
	p.mu.RUnlock()

	switch req.GetAction() {
	case "prequery":
		return p.handlePreQuery(ctx, req.GetPayload())
	case "distill-session":
		return p.handleDistillSession(ctx, req.GetPayload())
	case "list-distillates":
		return p.handleListDistillates(req.GetPayload())
	case "show-distillate":
		return p.handleShowDistillate(req.GetPayload())
	case "clear-distillates":
		return p.handleClearDistillates(req.GetPayload())
	default:
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("unknown action: %s", req.GetAction())),
		}, nil
	}
}

func (p *sessionIntelligencePlugin) handlePreQuery(_ context.Context, payload []byte) (*siplyv1.ExecuteResponse, error) {
	if !p.config.Enabled {
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	var req prequeryRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		slog.Warn("session-intelligence: unmarshal prequery failed", "err", err)
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	workspace := req.Workspace
	if workspace == "" {
		workspace, _ = os.Getwd()
	}

	distillate, loadErr := p.store.LoadLatest(workspace)
	if loadErr != nil || distillate == nil {
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	contentBytes, jsonErr := json.Marshal(distillate.Content)
	if jsonErr != nil {
		slog.Warn("session-intelligence: marshal distillate content failed", "err", jsonErr)
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	result := make([]Message, 0, len(req.Messages)+1)
	result = append(result, Message{
		Role:    "system",
		Content: "[Session Intelligence]\n" + string(contentBytes),
	})
	result = append(result, req.Messages...)

	out, err := json.Marshal(result)
	if err != nil {
		slog.Warn("session-intelligence: marshal result failed", "err", err)
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}
	return &siplyv1.ExecuteResponse{Success: true, Result: out}, nil
}

func (p *sessionIntelligencePlugin) handleDistillSession(ctx context.Context, payload []byte) (*siplyv1.ExecuteResponse, error) {
	if !p.config.Enabled {
		return &siplyv1.ExecuteResponse{Success: true}, nil
	}

	var req distillSessionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		slog.Warn("session-intelligence: unmarshal distill request failed", "err", err)
		return &siplyv1.ExecuteResponse{Success: true}, nil
	}

	start := time.Now()
	originalTokens := 0
	for _, m := range req.Messages {
		originalTokens += estimateTokens(m.Content)
	}

	distillate, err := p.distiller.DistillSession(ctx, req.Messages)
	latencyMS := time.Since(start).Milliseconds()
	if err != nil {
		slog.Warn("session-intelligence: distillation failed", "err", err)
		if !strings.Contains(err.Error(), "session too short") {
			p.publishStatus("⚠ Session intelligence offline")
		}
		return &siplyv1.ExecuteResponse{Success: true}, nil
	}

	distillate.SessionID = req.SessionID
	distillate.Workspace = req.Workspace

	if saveErr := p.store.Save(req.SessionID, req.Workspace, distillate); saveErr != nil {
		slog.Warn("session-intelligence: save distillate failed", "err", saveErr)
		return &siplyv1.ExecuteResponse{Success: true}, nil
	}

	compressionRatio := 0.0
	if originalTokens > 0 {
		compressionRatio = float64(distillate.TokenCount) / float64(originalTokens)
	}

	p.publishSessionIntelligenceEvent(req.SessionID, distillate, false, sessionIntelligenceMetrics{
		originalTokens:   originalTokens,
		compressionRatio: compressionRatio,
		latencyMS:        latencyMS,
	})

	if p.consolid.ShouldConsolidate(req.Workspace, p.store) {
		consolStart := time.Now()
		consolidated, consolErr := p.consolid.Consolidate(ctx, req.Workspace, p.store)
		consolLatency := time.Since(consolStart).Milliseconds()
		if consolErr != nil {
			slog.Warn("session-intelligence: consolidation failed", "err", consolErr)
		} else {
			p.publishSessionIntelligenceEvent(req.SessionID, consolidated, true, sessionIntelligenceMetrics{
				originalTokens:   originalTokens,
				compressionRatio: compressionRatio,
				latencyMS:        consolLatency,
			})
		}
	}

	return &siplyv1.ExecuteResponse{Success: true}, nil
}

func (p *sessionIntelligencePlugin) handleListDistillates(payload []byte) (*siplyv1.ExecuteResponse, error) {
	workspace := strings.TrimSpace(string(payload))
	if workspace == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("workspace path is required")}, nil
	}
	metas, err := p.store.ListAll(workspace)
	if err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr(err.Error())}, nil
	}
	out, err := json.Marshal(metas)
	if err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr(err.Error())}, nil
	}
	return &siplyv1.ExecuteResponse{Success: true, Result: out}, nil
}

func (p *sessionIntelligencePlugin) handleShowDistillate(payload []byte) (*siplyv1.ExecuteResponse, error) {
	var req showDistillateRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("invalid request")}, nil
	}
	distillate, err := p.store.Load(req.Workspace, req.SessionID)
	if err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr(err.Error())}, nil
	}
	out, err := json.Marshal(distillate)
	if err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr(err.Error())}, nil
	}
	return &siplyv1.ExecuteResponse{Success: true, Result: out}, nil
}

func (p *sessionIntelligencePlugin) handleClearDistillates(payload []byte) (*siplyv1.ExecuteResponse, error) {
	workspace := strings.TrimSpace(string(payload))
	if workspace == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("workspace path is required")}, nil
	}
	if err := p.store.Clear(workspace); err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr(err.Error())}, nil
	}
	return &siplyv1.ExecuteResponse{Success: true}, nil
}

func (p *sessionIntelligencePlugin) HandleEvent(_ context.Context, req *siplyv1.HandleEventRequest) (*siplyv1.HandleEventResponse, error) {
	if req.GetEventType() == "session.ended" {
		slog.Info("session-intelligence: received session.ended event")
	}
	return &siplyv1.HandleEventResponse{}, nil
}

func (p *sessionIntelligencePlugin) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.hostConn != nil {
		p.hostConn.Close()
	}
	p.initialized = false
	return &siplyv1.ShutdownResponse{}, nil
}

func (p *sessionIntelligencePlugin) publishStatus(message string) {
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
		slog.Debug("session-intelligence: publish status failed", "err", err)
	}
}

type sessionIntelligenceMetrics struct {
	originalTokens   int
	compressionRatio float64
	latencyMS        int64
}

func (p *sessionIntelligencePlugin) publishSessionIntelligenceEvent(sessionID string, d *Distillate, consolidation bool, metrics sessionIntelligenceMetrics) {
	p.mu.RLock()
	client := p.hostClient
	name := p.name
	p.mu.RUnlock()

	if client == nil {
		return
	}

	distillateCount := 0
	if d.Workspace != "" {
		metas, err := p.store.ListAll(d.Workspace)
		if err == nil {
			distillateCount = len(metas)
		}
	}

	payload, jsonErr := json.Marshal(map[string]any{
		"sessionID":        sessionID,
		"distillateTokens": d.TokenCount,
		"originalTokens":   metrics.originalTokens,
		"compressionRatio": metrics.compressionRatio,
		"latencyMS":        metrics.latencyMS,
		"consolidation":    consolidation,
		"distillateCount":  distillateCount,
		"ts":               time.Now().UTC(),
	})
	if jsonErr != nil {
		slog.Debug("session-intelligence: marshal event failed", "err", jsonErr)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.PublishEvent(ctx, &siplyv1.PublishEventRequest{
		EventType:  "session-intelligence.stats",
		PluginName: name,
		Payload:    payload,
	})
	if err != nil {
		slog.Debug("session-intelligence: publish event failed", "err", err)
	}
}

type prequeryRequest struct {
	Workspace string    `json:"workspace"`
	Messages  []Message `json:"messages"`
}

type distillSessionRequest struct {
	SessionID string    `json:"session_id"`
	Workspace string    `json:"workspace"`
	Messages  []Message `json:"messages"`
}

type showDistillateRequest struct {
	SessionID string `json:"session_id"`
	Workspace string `json:"workspace"`
}

// Message mirrors core.Message for JSON serialization across the gRPC boundary.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	ToolID  string `json:"tool_id,omitempty"`
}
