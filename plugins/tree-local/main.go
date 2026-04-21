// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// tree-local is a Tier 3 plugin that renders a local file tree in a siply panel.
// On first activation (lazy) it scans the current working directory, annotates
// nodes with git status, and renders via pkg/siplyui.Tree.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
	"siply.dev/siply/pkg/siplyui"
)

func strPtr(s string) *string { return &s }

// fileIcon returns an emoji icon for the given file/dir name.
func fileIcon(name string, isDir bool) string {
	if isDir {
		return "📁"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go":
		return "🐹"
	case ".js", ".ts", ".jsx", ".tsx":
		return "🟨"
	case ".json":
		return "📋"
	case ".yaml", ".yml":
		return "📄"
	case ".md":
		return "📝"
	case ".py":
		return "🐍"
	case ".rs":
		return "🦀"
	case ".proto":
		return "📡"
	case ".sh", ".bash":
		return "🔧"
	default:
		return "📄"
	}
}

// gitStatusIndicator returns an ANSI-colored suffix for a git status code.
func gitStatusIndicator(code byte) string {
	switch code {
	case 'M':
		return " \x1b[33m[M]\x1b[0m"
	case 'A':
		return " \x1b[32m[A]\x1b[0m"
	case '?':
		return " \x1b[90m[?]\x1b[0m"
	case 'D':
		return " \x1b[31m[D]\x1b[0m"
	default:
		return ""
	}
}

// parseGitStatus runs `git status --porcelain` and returns a map of relative path → status code.
func parseGitStatus(root string) map[string]byte {
	result := make(map[string]byte)
	out, err := exec.Command("git", "-C", root, "status", "--porcelain").Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		code := line[0]
		if code == ' ' {
			code = line[1]
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		result[filepath.FromSlash(path)] = code
	}
	return result
}

// skipDir returns true for directories that should not be walked.
func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", ".siply", "vendor", ".cache":
		return true
	}
	return false
}

// buildFileTree recursively walks root and returns siplyui.TreeNode slices.
// maxDepth prevents infinite recursion; dirs are sorted before files.
func buildFileTree(root, relBase string, gitStatus map[string]byte, depth int) []siplyui.TreeNode {
	const maxDepth = 10
	if depth > maxDepth {
		return nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var dirs, files []os.DirEntry
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(filepath.Join(root, e.Name()))
			if err != nil {
				continue
			}
			if !strings.HasPrefix(resolved, root) {
				continue
			}
		}
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	var nodes []siplyui.TreeNode
	for _, e := range dirs {
		if skipDir(e.Name()) {
			continue
		}
		children := buildFileTree(
			filepath.Join(root, e.Name()),
			filepath.Join(relBase, e.Name()),
			gitStatus,
			depth+1,
		)
		nodes = append(nodes, siplyui.TreeNode{
			Label:    e.Name(),
			Icon:     "📁",
			Children: children,
			Expanded: depth == 0,
			Data:     filepath.Join(root, e.Name()),
		})
	}

	for _, e := range files {
		relPath := filepath.Join(relBase, e.Name())
		label := e.Name()
		if code, ok := gitStatus[relPath]; ok {
			label += gitStatusIndicator(code)
		}
		nodes = append(nodes, siplyui.TreeNode{
			Label: label,
			Icon:  fileIcon(e.Name(), false),
			Data:  filepath.Join(root, e.Name()),
		})
	}

	return nodes
}

// treePlugin implements the SiplyPluginService gRPC server.
type treePlugin struct {
	siplyv1.UnimplementedSiplyPluginServiceServer

	mu          sync.RWMutex
	name        string
	hostAddr    string
	ownAddr     string
	hostClient  siplyv1.SiplyHostServiceClient
	hostConn    *grpc.ClientConn
	initialized bool

	rootDir     string
	cachedNodes []siplyui.TreeNode
}

func main() {
	hostAddr := os.Getenv("SIPLY_HOST_ADDR")
	pluginName := os.Getenv("SIPLY_PLUGIN_NAME")

	if hostAddr == "" || pluginName == "" {
		slog.Error("missing required env vars", "SIPLY_HOST_ADDR", hostAddr, "SIPLY_PLUGIN_NAME", pluginName)
		os.Exit(1)
	}

	plugin := &treePlugin{
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

// Initialize connects to the host and marks the plugin ready.
func (p *treePlugin) Initialize(_ context.Context, _ *siplyv1.InitializeRequest) (*siplyv1.InitializeResponse, error) {
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

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	p.rootDir = cwd
	p.initialized = true

	p.publishStatus("ready")
	return &siplyv1.InitializeResponse{
		Success:      true,
		Capabilities: []string{"panel", "tree"},
	}, nil
}

// Execute dispatches render, select, and refresh actions.
func (p *treePlugin) Execute(_ context.Context, req *siplyv1.ExecuteRequest) (*siplyv1.ExecuteResponse, error) {
	p.mu.RLock()
	if !p.initialized {
		p.mu.RUnlock()
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("not initialized")}, nil
	}
	p.mu.RUnlock()

	switch req.GetAction() {
	case "render":
		return p.handleRender(req.GetPayload())
	case "select":
		return p.handleSelect(req.GetPayload())
	case "refresh":
		p.mu.Lock()
		p.cachedNodes = nil
		p.mu.Unlock()
		return &siplyv1.ExecuteResponse{Success: true}, nil
	default:
		return &siplyv1.ExecuteResponse{
			Success: false,
			Error:   strPtr(fmt.Sprintf("unknown action: %s", req.GetAction())),
		}, nil
	}
}

// handleRender builds and renders the file tree. The tree is cached after first build
// (refresh action or panel re-activate to invalidate).
func (p *treePlugin) handleRender(payload []byte) (*siplyv1.ExecuteResponse, error) {
	width, height := 40, 24
	const maxWidth, maxHeight = 500, 200
	// payload may encode [widthHigh, widthLow, heightHigh, heightLow]
	if len(payload) >= 4 {
		w := int(payload[0])<<8 | int(payload[1])
		h := int(payload[2])<<8 | int(payload[3])
		if w > 0 {
			width = min(w, maxWidth)
		}
		if h > 0 {
			height = min(h, maxHeight)
		}
	}

	p.mu.Lock()
	if p.cachedNodes == nil {
		gitStatus := parseGitStatus(p.rootDir)
		p.cachedNodes = buildFileTree(p.rootDir, "", gitStatus, 0)
	}
	nodes := p.cachedNodes
	p.mu.Unlock()

	tree := siplyui.NewTree(nodes, siplyui.DefaultTheme(), siplyui.DefaultRenderConfig())
	return &siplyv1.ExecuteResponse{
		Success: true,
		Result:  []byte(tree.Render(width, height)),
	}, nil
}

// handleSelect publishes a FileSelectedEvent for the given file path.
func (p *treePlugin) handleSelect(payload []byte) (*siplyv1.ExecuteResponse, error) {
	path := strings.TrimSpace(string(payload))
	if path == "" {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("empty path")}, nil
	}

	// Prevent path traversal: resolve symlinks and verify path is under rootDir.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("invalid path")}, nil
	}
	p.mu.RLock()
	rootDir := p.rootDir
	p.mu.RUnlock()
	absRoot, _ := filepath.Abs(rootDir)
	if !strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) && absPath != absRoot {
		return &siplyv1.ExecuteResponse{Success: false, Error: strPtr("path outside project root")}, nil
	}

	p.mu.RLock()
	hostClient := p.hostClient
	pluginName := p.name
	p.mu.RUnlock()

	if hostClient != nil {
		_, err := hostClient.PublishEvent(context.Background(), &siplyv1.PublishEventRequest{
			EventType:  "file.selected",
			PluginName: pluginName,
			Payload:    []byte(path),
		})
		if err != nil {
			slog.Debug("tree-local: publish file.selected event failed", "err", err)
		}
	}

	return &siplyv1.ExecuteResponse{Success: true}, nil
}

// HandleEvent is a no-op for tree-local; it does not subscribe to any events.
func (p *treePlugin) HandleEvent(_ context.Context, _ *siplyv1.HandleEventRequest) (*siplyv1.HandleEventResponse, error) {
	return &siplyv1.HandleEventResponse{}, nil
}

// Shutdown closes the host connection.
func (p *treePlugin) Shutdown(_ context.Context, _ *siplyv1.ShutdownRequest) (*siplyv1.ShutdownResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hostConn != nil {
		p.hostConn.Close()
	}
	p.initialized = false
	return &siplyv1.ShutdownResponse{}, nil
}

func (p *treePlugin) publishStatus(message string) {
	if p.hostClient == nil {
		return
	}
	_, err := p.hostClient.PublishStatus(context.Background(), &siplyv1.PublishStatusRequest{
		PluginName: p.name,
		Message:    message,
	})
	if err != nil {
		slog.Debug("tree-local: publish status failed", "err", err)
	}
}
