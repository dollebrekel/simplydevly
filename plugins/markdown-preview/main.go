// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// markdown-preview is a Tier 3 plugin that renders markdown files in a siply panel.
// It subscribes to FileSelectedEvent and renders .md files via pkg/siplyui.MarkdownView.
// When no file is selected, a placeholder is shown.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
	"siply.dev/siply/pkg/siplyui"
)

const placeholder = "Select a .md file to preview"

func strPtr(s string) *string { return &s }

// markdownPlugin implements the SiplyPluginService gRPC server.
type markdownPlugin struct {
	siplyv1.UnimplementedSiplyPluginServiceServer

	mu          sync.RWMutex
	name        string
	hostAddr    string
	ownAddr     string
	hostClient  siplyv1.SiplyHostServiceClient
	hostConn    *grpc.ClientConn
	initialized bool

	selectedFile string
	scrollOffset int
	lastHeight   int
}

func main() {
	hostAddr := os.Getenv("SIPLY_HOST_ADDR")
	pluginName := os.Getenv("SIPLY_PLUGIN_NAME")

	if hostAddr == "" || pluginName == "" {
		slog.Error("missing required env vars", "SIPLY_HOST_ADDR", hostAddr, "SIPLY_PLUGIN_NAME", pluginName)
		os.Exit(1)
	}

	plugin := &markdownPlugin{
		name:     pluginName,
		hostAddr: hostAddr,
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		slog.Error("failed to listen", "err", err)
		os.Exit(1)
	}
	plugin.ownAddr = lis.Addr().String()

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

// Initialize connects to the host and subscribes to file.selected events.
func (p *markdownPlugin) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
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

	// Subscribe to file.selected events so the panel re-renders on file selection.
	_, subErr := p.hostClient.SubscribeEvent(context.Background(), &siplyv1.SubscribeEventRequest{
		EventType:  "file.selected",
		PluginName: p.name,
		PluginAddr: p.ownAddr,
	})
	if subErr != nil {
		slog.Debug("markdown-preview: subscribe to file.selected failed", "err", subErr)
	}

	p.initialized = true
	p.publishStatus("ready — no file selected")

	return &siplyv1.InitializeResponse{
		Success:      true,
		Capabilities: []string{"panel", "markdown"},
	}, nil
}

// Execute handles render actions from the panel system.
func (p *markdownPlugin) Execute(_ context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	p.mu.RLock()
	if !p.initialized {
		p.mu.RUnlock()
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("not initialized")}, nil
	}
	p.mu.RUnlock()

	switch req.GetAction() {
	case "render":
		return p.handleRender(req.GetPayload())
	case "key":
		return p.handleKey(req.GetPayload())
	default:
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("unknown action: %s", req.GetAction())),
		}, nil
	}
}

// handleRender reads the selected .md file (if any) and renders it via MarkdownView.
func (p *markdownPlugin) handleRender(payload []byte) (*siplyv1.ExecuteResponse, error) {
	width, height := 80, 24
	const maxWidth, maxHeight = 500, 200
	if len(payload) >= 4 {
		w := int(payload[0])<<8 | int(payload[1])
		h := int(payload[2])<<8 | int(payload[3])
		if w > 0 {
			width = min(w, maxWidth)
		}
		if h > 0 {
			height = min(h, maxHeight)
		}
	} else if len(payload) >= 2 {
		w := int(payload[0])<<8 | int(payload[1])
		if w > 0 {
			width = min(w, maxWidth)
		}
	}

	p.mu.Lock()
	selectedFile := p.selectedFile
	p.lastHeight = height
	p.mu.Unlock()

	var fullContent string
	if selectedFile == "" {
		fullContent = placeholder
	} else {
		data, err := os.ReadFile(selectedFile)
		if err != nil {
			fullContent = fmt.Sprintf("Error reading %s: %v", selectedFile, err)
		} else {
			mv := siplyui.NewMarkdownView(siplyui.DefaultTheme(), siplyui.DefaultRenderConfig())
			fullContent = mv.Render(string(data), width)
		}
	}

	// Apply scroll offset.
	lines := strings.Split(fullContent, "\n")
	p.mu.Lock()
	maxOff := len(lines) - height
	if maxOff < 0 {
		maxOff = 0
	}
	if p.scrollOffset > maxOff {
		p.scrollOffset = maxOff
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
	off := p.scrollOffset
	p.mu.Unlock()

	end := off + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[off:end]

	return &siplyv1.ExecuteResponse{
		Success: true,
		Result:  []byte(strings.Join(visible, "\n")),
	}, nil
}

// handleKey processes keyboard/scroll input for the markdown preview.
func (p *markdownPlugin) handleKey(payload []byte) (*siplyv1.ExecuteResponse, error) {
	key := strings.TrimSpace(string(payload))
	p.mu.Lock()
	switch key {
	case "up", "k":
		if p.scrollOffset > 0 {
			p.scrollOffset--
		}
	case "down", "j":
		p.scrollOffset++
	case "pgup":
		p.scrollOffset -= p.lastHeight / 2
	case "pgdown":
		p.scrollOffset += p.lastHeight / 2
	case "home":
		p.scrollOffset = 0
	case "end":
		p.scrollOffset = 99999
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
	p.mu.Unlock()
	return &siplyv1.ExecuteResponse{Success: true}, nil
}

// HandleEvent is called by the host when a subscribed event fires.
// Only file.selected events for .md files cause a panel re-render.
func (p *markdownPlugin) HandleEvent(_ context.Context, req *siplyv1.HandleEventRequest) (*siplyv1.HandleEventResponse, error) {
	if req.GetEventType() == "file.selected" {
		path := strings.TrimSpace(string(req.GetPayload()))
		if path != "" {
			if strings.HasSuffix(strings.ToLower(path), ".md") {
				p.mu.Lock()
				p.selectedFile = path
				p.scrollOffset = 0
				p.mu.Unlock()
				p.publishStatus("previewing " + filepath.Base(path))
			} else {
				p.mu.Lock()
				p.selectedFile = ""
				p.mu.Unlock()
				p.publishStatus("ready — no .md file selected")
			}
		}
	}
	return &siplyv1.HandleEventResponse{}, nil
}

// Shutdown closes the host connection.
func (p *markdownPlugin) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hostConn != nil {
		p.hostConn.Close()
	}
	p.initialized = false
	return &siplyv1.ShutdownResponse{}, nil
}

func (p *markdownPlugin) publishStatus(message string) {
	if p.hostClient == nil {
		return
	}
	_, err := p.hostClient.PublishStatus(context.Background(), &siplyv1.PublishStatusRequest{
		PluginName: p.name,
		Message:    message,
	})
	if err != nil {
		slog.Debug("markdown-preview: publish status failed", "err", err)
	}
}
