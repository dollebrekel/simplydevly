// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// context-distillation is a Tier 3 plugin that compresses conversation context
// using a local Ollama LLM for 60%+ token savings on cloud API calls.
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

type distillationPlugin struct {
	siplyv1.UnimplementedSiplyPluginServiceServer

	mu          sync.RWMutex
	name        string
	hostAddr    string
	hostClient  siplyv1.SiplyHostServiceClient
	hostConn    *grpc.ClientConn
	initialized bool

	distiller *Distiller
	cache     *Cache
	config    Config
}

func main() {
	hostAddr := os.Getenv("SIPLY_HOST_ADDR")
	pluginName := os.Getenv("SIPLY_PLUGIN_NAME")

	if hostAddr == "" || pluginName == "" {
		slog.Error("missing required env vars", "SIPLY_HOST_ADDR", hostAddr, "SIPLY_PLUGIN_NAME", pluginName)
		os.Exit(1)
	}

	plugin := &distillationPlugin{
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

func (p *distillationPlugin) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
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
			if v, ok := userCfg["keep_turns"].(float64); ok {
				cfg.KeepTurns = int(v)
			}
		}
	}
	if err := ValidateConfig(cfg); err != nil {
		slog.Warn("distillation: invalid config, using defaults", "err", err)
		cfg = DefaultConfig()
	}
	p.config = cfg

	client := NewOllamaClient(p.config.OllamaURL, p.config.Model)
	p.distiller = NewDistiller(client, p.config.KeepTurns)
	p.cache = NewCache(100)

	p.initialized = true
	p.mu.Unlock()

	p.publishStatus("ready")

	healthCtx, healthCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer healthCancel()
	if err := client.HealthCheck(healthCtx); err != nil {
		slog.Warn("distillation: ollama not reachable, will degrade gracefully", "err", err)
		p.publishStatus("⚠ Distillation unavailable — full context mode ($$$)")
	}

	return &siplyv1.InitializeResponse{
		Success:      true,
		Capabilities: []string{"context-distillation"},
	}, nil
}

func (p *distillationPlugin) Execute(ctx context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	p.mu.RLock()
	if !p.initialized {
		p.mu.RUnlock()
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("not initialized")}, nil
	}
	p.mu.RUnlock()

	switch req.GetAction() {
	case "prequery":
		return p.handlePreQuery(ctx, req.GetPayload())
	default:
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("unknown action: %s", req.GetAction())),
		}, nil
	}
}

func (p *distillationPlugin) handlePreQuery(ctx context.Context, payload []byte) (*siplyv1.ExecuteResponse, error) {
	if !p.config.Enabled {
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	var msgs []Message
	if err := json.Unmarshal(payload, &msgs); err != nil {
		slog.Warn("distillation: unmarshal messages failed, returning original", "err", err)
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	turns := countConversationTurns(msgs)
	if turns <= p.config.KeepTurns {
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	olderMsgs, recentMsgs := splitMessages(msgs, p.config.KeepTurns)

	hash := hashTurns(olderMsgs)
	if cached, ok := p.cache.Get(hash); ok {
		result := assembleResult(cached, recentMsgs)
		out, _ := json.Marshal(result)
		p.publishDistillationEvent(len(msgs), estimateTokens(cached), estimateTokens(concatContent(msgs)), true, 0)
		return &siplyv1.ExecuteResponse{Success: true, Result: out}, nil
	}

	start := time.Now()
	prompt := p.distiller.BuildPrompt(olderMsgs)
	distillate, err := p.distiller.client.Distill(ctx, prompt)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		slog.Warn("distillation failed, returning original messages", "err", err)
		p.publishHookFailed(err)
		p.publishStatus("⚠ Distillation unavailable — full context mode ($$$)")
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	if strings.TrimSpace(distillate) == "" {
		slog.Warn("distillation returned empty result, returning original messages")
		return &siplyv1.ExecuteResponse{Success: true, Result: payload}, nil
	}

	const maxDistillateTokens = 500
	if estimateTokens(distillate) > maxDistillateTokens {
		maxChars := maxDistillateTokens * 4
		if len(distillate) > maxChars {
			distillate = distillate[:maxChars]
		}
	}

	p.cache.Put(hash, distillate)

	result := assembleResult(distillate, recentMsgs)
	out, _ := json.Marshal(result)

	originalTokens := estimateTokens(concatContent(msgs))
	distillateTokens := estimateTokens(distillate)
	p.publishDistillationEvent(turns, distillateTokens, originalTokens, false, latency)

	return &siplyv1.ExecuteResponse{Success: true, Result: out}, nil
}

func (p *distillationPlugin) HandleEvent(_ context.Context, _ *siplyv1.HandleEventRequest) (*siplyv1.HandleEventResponse, error) {
	return &siplyv1.HandleEventResponse{}, nil
}

func (p *distillationPlugin) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.hostConn != nil {
		p.hostConn.Close()
	}
	p.initialized = false
	return &siplyv1.ShutdownResponse{}, nil
}

func (p *distillationPlugin) publishStatus(message string) {
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
		slog.Debug("distillation: publish status failed", "err", err)
	}
}

func (p *distillationPlugin) publishDistillationEvent(turnCount, distillateTokens, originalTokens int, cacheHit bool, latencyMS int64) {
	p.mu.RLock()
	client := p.hostClient
	name := p.name
	p.mu.RUnlock()

	if client == nil {
		return
	}

	ratio := 0.0
	if originalTokens > 0 {
		ratio = 1.0 - float64(distillateTokens)/float64(originalTokens)
	}

	payload, jsonErr := json.Marshal(map[string]any{
		"turnCount":        turnCount,
		"distillateTokens": distillateTokens,
		"originalTokens":   originalTokens,
		"compressionRatio": ratio,
		"latencyMS":        latencyMS,
		"cacheHit":         cacheHit,
		"ts":               time.Now().UTC(),
	})
	if jsonErr != nil {
		slog.Debug("distillation: marshal event failed", "err", jsonErr)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.PublishEvent(ctx, &siplyv1.PublishEventRequest{
		EventType:  "distillation.stats",
		PluginName: name,
		Payload:    payload,
	})
	if err != nil {
		slog.Debug("distillation: publish event failed", "err", err)
	}
}

func (p *distillationPlugin) publishHookFailed(origErr error) {
	p.mu.RLock()
	client := p.hostClient
	name := p.name
	p.mu.RUnlock()

	if client == nil {
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"hookName":   "context-distillation",
		"point":      "PreQuery",
		"err":        origErr.Error(),
		"fallback":   "full-context",
		"costEffect": "higher",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.PublishEvent(ctx, &siplyv1.PublishEventRequest{
		EventType:  "hook.failed",
		PluginName: name,
		Payload:    payload,
	})
	if err != nil {
		slog.Debug("distillation: publish hook-failed event failed", "err", err)
	}
}

// Message mirrors core.Message for JSON serialization across the gRPC boundary.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	ToolID  string `json:"tool_id,omitempty"`
}

func assembleResult(distillate string, recentMsgs []Message) []Message {
	result := make([]Message, 0, len(recentMsgs)+1)
	result = append(result, Message{
		Role:    "system",
		Content: "[Context Distillate]\n" + distillate,
	})
	result = append(result, recentMsgs...)
	return result
}

func countConversationTurns(msgs []Message) int {
	count := 0
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			count++
		}
	}
	return count
}

func splitMessages(msgs []Message, keepTurns int) (older, recent []Message) {
	turns := countConversationTurns(msgs)
	if turns <= keepTurns {
		return nil, msgs
	}

	turnsSeen := 0
	splitIdx := len(msgs)
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" || msgs[i].Role == "assistant" {
			turnsSeen++
			if turnsSeen >= keepTurns {
				splitIdx = i
				break
			}
		}
	}

	// Preserve system messages from the older portion — they contain
	// instructions and config that must not be distilled away.
	var preserved []Message
	for i := 0; i < splitIdx; i++ {
		if msgs[i].Role == "system" {
			preserved = append(preserved, msgs[i])
		} else {
			older = append(older, msgs[i])
		}
	}
	recent = append(preserved, msgs[splitIdx:]...)
	return older, recent
}

func concatContent(msgs []Message) string {
	var total string
	for _, m := range msgs {
		total += m.Content
	}
	return total
}

func estimateTokens(s string) int {
	// ~4 chars per token is a reasonable approximation for English text.
	return len(s) / 4
}
