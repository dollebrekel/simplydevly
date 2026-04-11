// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

// Default timeouts for Tier 3 plugin operations.
const (
	defaultOperationTimeout = 30 * time.Second
	defaultStartupTimeout   = 5 * time.Second
	defaultShutdownTimeout  = 5 * time.Second
)

// Sentinel errors for Tier3Loader operations.
var (
	ErrNotTier3       = errors.New("plugins: plugin is not Tier 3")
	ErrBinaryNotFound = errors.New("plugins: plugin binary not found or not executable")
	ErrPluginCrashed  = errors.New("plugins: plugin process crashed")
	ErrPluginTimeout  = errors.New("plugins: plugin operation timed out")
)

// PluginInfo is a snapshot DTO for a loaded Tier 3 plugin's status.
type PluginInfo struct {
	Name    string
	Version string
	Tier    int
	Running bool
	Crashed bool
}

// Tier3Option is a functional option for configuring a Tier3Loader.
type Tier3Option func(*Tier3Loader)

// WithOperationTimeout sets the default operation timeout for Tier 3 plugin RPCs.
func WithOperationTimeout(d time.Duration) Tier3Option {
	return func(l *Tier3Loader) {
		l.operationTimeout = d
	}
}

// Tier3Plugin represents a loaded Tier 3 Go native plugin.
type Tier3Plugin struct {
	Manifest *Manifest
	cmd      *exec.Cmd
	conn     *grpc.ClientConn
	client   siplyv1.SiplyPluginServiceClient
	cancel   context.CancelFunc
	exited   chan struct{}
	crashed  bool
	spawnMu  sync.Mutex // serializes spawn attempts for this plugin (P1)
}

// Tier3Loader loads and manages Tier 3 Go native plugins running as isolated processes via gRPC.
type Tier3Loader struct {
	registry         *LocalRegistry
	hostServer       *HostServer
	mu               sync.RWMutex
	loaded           map[string]*Tier3Plugin
	operationTimeout time.Duration
}

// NewTier3Loader creates a new Tier3Loader backed by the given registry and host server.
func NewTier3Loader(registry *LocalRegistry, hostServer *HostServer, opts ...Tier3Option) *Tier3Loader {
	l := &Tier3Loader{
		registry:         registry,
		hostServer:       hostServer,
		loaded:           make(map[string]*Tier3Plugin),
		operationTimeout: defaultOperationTimeout,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Load registers a Tier 3 plugin for lazy loading. The plugin binary is validated
// but the process is NOT started — it will be spawned on first Execute call.
func (l *Tier3Loader) Load(ctx context.Context, name string) error {
	if l.registry == nil {
		return fmt.Errorf("plugins: tier3: registry is nil")
	}

	pluginDir, err := l.pluginDir(name)
	if err != nil {
		return err
	}

	manifest, err := LoadManifestFromDir(pluginDir)
	if err != nil {
		return fmt.Errorf("plugins: tier3: load manifest %s: %w", name, err)
	}

	if manifest.Spec.Tier != 3 {
		return fmt.Errorf("%w: %s has tier %d", ErrNotTier3, name, manifest.Spec.Tier)
	}

	// Validate binary exists and is executable.
	binPath, err := findPluginBinary(pluginDir, name)
	if err != nil {
		return err
	}
	_ = binPath // validated, not used until Spawn

	plugin := &Tier3Plugin{
		Manifest: manifest,
		exited:   make(chan struct{}),
	}

	// Check if already loaded — unload first to prevent orphaned process (P8).
	l.mu.RLock()
	_, alreadyLoaded := l.loaded[name]
	l.mu.RUnlock()
	if alreadyLoaded {
		if err := l.Unload(ctx, name); err != nil {
			slog.Warn("tier3 plugin unload before reload failed", "name", name, "err", err)
		}
	}

	l.mu.Lock()
	l.loaded[name] = plugin
	l.mu.Unlock()

	slog.Info("tier3 plugin registered", "name", name, "version", manifest.Metadata.Version)
	return nil
}

// Spawn starts the plugin process and establishes gRPC communication.
// This is called internally on first use (lazy loading).
func (l *Tier3Loader) Spawn(ctx context.Context, name string) error {
	if l.hostServer == nil {
		return fmt.Errorf("plugins: tier3: hostServer is nil")
	}

	// Start host server if not already running.
	if err := l.hostServer.Start(ctx); err != nil {
		return fmt.Errorf("plugins: tier3: start host server: %w", err)
	}

	// Assert host server address is available (P10).
	if l.hostServer.Addr() == "" {
		return fmt.Errorf("plugins: tier3: host server started but address is empty")
	}

	l.mu.RLock()
	plugin, ok := l.loaded[name]
	l.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, name)
	}

	// Serialize spawn attempts per plugin to prevent double-spawn race (P1).
	plugin.spawnMu.Lock()
	defer plugin.spawnMu.Unlock()

	// Re-check under spawn lock — another goroutine may have spawned already.
	if plugin.cmd != nil && plugin.conn != nil {
		select {
		case <-plugin.exited:
			// Process exited, need to respawn.
		default:
			return nil // still running
		}
	}

	pluginDir, err := l.pluginDir(name)
	if err != nil {
		return err
	}

	binPath, err := findPluginBinary(pluginDir, name)
	if err != nil {
		return err
	}

	// Create a process-scoped context so we can kill the plugin on Unload.
	procCtx, procCancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(procCtx, binPath)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SIPLY_HOST_ADDR=%s", l.hostServer.Addr()),
		fmt.Sprintf("SIPLY_PLUGIN_NAME=%s", name),
	)

	// Capture stdout to read the plugin's listen address.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		procCancel()
		return fmt.Errorf("plugins: tier3: stdout pipe %s: %w", name, err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		procCancel()
		return fmt.Errorf("plugins: tier3: start %s: %w", name, err)
	}

	slog.Info("tier3 plugin spawned", "name", name, "pid", cmd.Process.Pid)

	// Read plugin address from stdout (first line: PLUGIN_ADDR=host:port).
	// Safety (P3): channels are buffered(1) so the goroutine never blocks on send.
	// Process kill closes stdout, causing scanner.Scan() to return false and the goroutine to exit.
	addrCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PLUGIN_ADDR=") {
				addrCh <- strings.TrimPrefix(line, "PLUGIN_ADDR=")
				return
			}
			errCh <- fmt.Errorf("plugins: tier3: unexpected first line from %s: %q", name, line)
			return
		}
		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("plugins: tier3: read stdout %s: %w", name, err)
			return
		}
		errCh <- fmt.Errorf("plugins: tier3: %s closed stdout without sending address", name)
	}()

	// Wait for plugin address with startup timeout.
	startupCtx, startupCancel := context.WithTimeout(ctx, defaultStartupTimeout)
	defer startupCancel()

	var pluginAddr string
	select {
	case pluginAddr = <-addrCh:
	case err := <-errCh:
		procCancel()
		_ = cmd.Wait()
		return err
	case <-startupCtx.Done():
		procCancel()
		_ = cmd.Wait()
		return fmt.Errorf("plugins: tier3: %s startup timeout (%v)", name, defaultStartupTimeout)
	}

	// Validate plugin address is loopback only (P4: reject non-local addresses).
	host, _, err := net.SplitHostPort(pluginAddr)
	if err != nil {
		procCancel()
		_ = cmd.Wait()
		return fmt.Errorf("plugins: tier3: invalid plugin address from %s: %w", name, err)
	}
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		procCancel()
		_ = cmd.Wait()
		return fmt.Errorf("plugins: tier3: plugin %s returned non-loopback address %q", name, pluginAddr)
	}

	// Connect to plugin gRPC server.
	conn, err := waitForPluginReady(startupCtx, pluginAddr, defaultStartupTimeout)
	if err != nil {
		procCancel()
		_ = cmd.Wait()
		return fmt.Errorf("plugins: tier3: connect to %s: %w", name, err)
	}

	client := siplyv1.NewSiplyPluginServiceClient(conn)

	// Initialize the plugin.
	initResp, err := client.Initialize(startupCtx, &siplyv1.InitializeRequest{
		PluginName: name,
		ApiVersion: "siply/v1",
	})
	if err != nil {
		conn.Close()
		procCancel()
		_ = cmd.Wait()
		return fmt.Errorf("plugins: tier3: initialize %s: %w", name, err)
	}
	if !initResp.GetSuccess() {
		conn.Close()
		procCancel()
		_ = cmd.Wait()
		return fmt.Errorf("plugins: tier3: initialize %s failed: %s", name, initResp.GetError())
	}

	// Update plugin state under write lock.
	exited := make(chan struct{})
	l.mu.Lock()
	plugin.cmd = cmd
	plugin.conn = conn
	plugin.client = client
	plugin.cancel = procCancel
	plugin.exited = exited
	plugin.crashed = false
	l.mu.Unlock()

	// Monitor process exit.
	go func() {
		err := cmd.Wait()
		if err != nil {
			slog.Warn("tier3 plugin exited", "name", name, "err", err)
		} else {
			slog.Info("tier3 plugin exited", "name", name)
		}
		l.mu.Lock()
		plugin.crashed = true
		l.mu.Unlock()
		close(exited)
	}()

	slog.Info("tier3 plugin initialized", "name", name, "addr", pluginAddr)
	return nil
}

// Execute runs an action on a Tier 3 plugin via gRPC. The plugin is lazily
// spawned on first call. Operations are subject to the configured timeout.
func (l *Tier3Loader) Execute(ctx context.Context, name string, action string, payload []byte) ([]byte, error) {
	if l.registry == nil {
		return nil, fmt.Errorf("plugins: tier3: registry is nil")
	}

	// Snapshot fields under read lock to prevent data race (P1).
	l.mu.RLock()
	plugin, ok := l.loaded[name]
	var client siplyv1.SiplyPluginServiceClient
	var exited chan struct{}
	if ok {
		client = plugin.client
		exited = plugin.exited
	}
	l.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, name)
	}

	// Lazy spawn on first use.
	needsSpawn := false
	if client == nil {
		needsSpawn = true
	} else {
		select {
		case <-exited:
			needsSpawn = true
		default:
		}
	}

	if needsSpawn {
		if err := l.Spawn(ctx, name); err != nil {
			return nil, err
		}
		// Re-read after spawn — check ok in case Unload ran concurrently (P1).
		l.mu.RLock()
		plugin, ok = l.loaded[name]
		if !ok {
			l.mu.RUnlock()
			return nil, fmt.Errorf("%w: %s", ErrPluginNotLoaded, name)
		}
		client = plugin.client
		l.mu.RUnlock()
	}

	// Apply operation timeout.
	opCtx, opCancel := context.WithTimeout(ctx, l.operationTimeout)
	defer opCancel()

	resp, err := client.Execute(opCtx, &siplyv1.ExecuteRequest{
		Action:  action,
		Payload: payload,
	})
	if err != nil {
		// Check if the plugin crashed during execution.
		select {
		case <-plugin.exited:
			return nil, fmt.Errorf("%w: %s", ErrPluginCrashed, name)
		default:
		}
		// Check for timeout.
		if opCtx.Err() == context.DeadlineExceeded {
			// Kill the plugin on timeout.
			if plugin.cancel != nil {
				plugin.cancel()
			}
			return nil, fmt.Errorf("%w: %s exceeded %v", ErrPluginTimeout, name, l.operationTimeout)
		}
		return nil, fmt.Errorf("plugins: tier3: execute %s: %w", name, err)
	}

	// Handle success+error field contradictions (deferred item from Story 1.4).
	if !resp.GetSuccess() {
		return nil, fmt.Errorf("plugins: tier3: execute %s failed: %s", name, resp.GetError())
	}

	return resp.GetResult(), nil
}

// Unload stops a Tier 3 plugin process and removes it from the loaded map.
func (l *Tier3Loader) Unload(ctx context.Context, name string) error {
	// Single write lock for check+delete to prevent TOCTOU.
	l.mu.Lock()
	plugin, ok := l.loaded[name]
	if !ok {
		l.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotLoaded, name)
	}
	delete(l.loaded, name)
	l.mu.Unlock()

	// 1. Try graceful shutdown via RPC (needs connection alive).
	// Use Background context since parent may be cancelled during app shutdown (P9).
	if plugin.client != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		_, err := plugin.client.Shutdown(shutdownCtx, &siplyv1.ShutdownRequest{})
		shutdownCancel()
		if err != nil {
			slog.Warn("tier3 plugin graceful shutdown failed", "name", name, "err", err)
		}
	}

	// 2. Cancel process context — sends kill signal via CommandContext (P2).
	if plugin.cancel != nil {
		plugin.cancel()
	}

	// 3. Wait for process to be reaped to prevent zombie (P2).
	if plugin.exited != nil {
		select {
		case <-plugin.exited:
		case <-time.After(defaultShutdownTimeout):
			slog.Warn("tier3 plugin process reap timeout, forcing kill", "name", name)
			if plugin.cmd != nil && plugin.cmd.Process != nil {
				_ = plugin.cmd.Process.Kill()
			}
		}
	}

	// 4. Close gRPC connection after process exit.
	if plugin.conn != nil {
		_ = plugin.conn.Close()
	}

	slog.Info("tier3 plugin unloaded", "name", name)
	return nil
}

// List returns a consistent snapshot of all loaded Tier 3 plugins as PluginInfo DTOs (thread-safe).
func (l *Tier3Loader) List(_ context.Context) []PluginInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	infos := make([]PluginInfo, 0, len(l.loaded))
	for name, p := range l.loaded {
		running := false
		if p.cmd != nil {
			select {
			case <-p.exited:
			default:
				running = true
			}
		}
		infos = append(infos, PluginInfo{
			Name:    name,
			Version: p.Manifest.Metadata.Version,
			Tier:    p.Manifest.Spec.Tier,
			Running: running,
			Crashed: p.crashed,
		})
	}
	return infos
}

// IsLoaded returns true if the named plugin is registered (thread-safe).
func (l *Tier3Loader) IsLoaded(name string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.loaded[name]
	return ok
}

// IsRunning returns true if the named plugin process is alive (thread-safe).
func (l *Tier3Loader) IsRunning(name string) bool {
	l.mu.RLock()
	plugin, ok := l.loaded[name]
	l.mu.RUnlock()
	if !ok {
		return false
	}
	if plugin.cmd == nil {
		return false
	}
	select {
	case <-plugin.exited:
		return false
	default:
		return true
	}
}

// pluginDir returns the effective directory for a plugin, respecting dev mode paths.
func (l *Tier3Loader) pluginDir(name string) (string, error) {
	if l.registry.registryDir == "" {
		return "", fmt.Errorf("plugins: tier3: registry not initialised (empty registryDir)")
	}

	// Reject path traversal attempts.
	if strings.ContainsAny(name, "/\\") || name == ".." || strings.Contains(name, "..") {
		return "", fmt.Errorf("plugins: tier3: invalid plugin name %q: path traversal not allowed", name)
	}

	l.registry.mu.RLock()
	devPath, isDev := l.registry.devPaths[name]
	l.registry.mu.RUnlock()

	if isDev {
		// Dev paths are user-configured via `siply plugins dev` — trusted by design (D1).
		return devPath, nil
	}

	dir := filepath.Join(l.registry.registryDir, name)

	// Path containment check.
	cleanDir := filepath.Clean(dir)
	cleanBase := filepath.Clean(l.registry.registryDir)
	if !strings.HasPrefix(cleanDir, cleanBase+string(filepath.Separator)) {
		return "", fmt.Errorf("plugins: tier3: invalid plugin name %q: path escapes registry", name)
	}

	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return "", fmt.Errorf("plugins: tier3: stat plugin dir %s: %w", name, err)
	}
	return dir, nil
}

// findPluginBinary locates the plugin binary in the plugin directory.
func findPluginBinary(pluginDir, pluginName string) (string, error) {
	binName := pluginName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	binPath := filepath.Join(pluginDir, binName)
	info, err := os.Stat(binPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s at %s", ErrBinaryNotFound, pluginName, binPath)
		}
		return "", fmt.Errorf("plugins: tier3: stat binary %s: %w", pluginName, err)
	}

	// Verify executable bit (Unix only — Windows executability is by extension).
	if runtime.GOOS != "windows" {
		if info.Mode()&0111 == 0 {
			return "", fmt.Errorf("%w: %s is not executable", ErrBinaryNotFound, binPath)
		}
	}

	return binPath, nil
}

// waitForPluginReady dials a gRPC server with retry until it becomes ready.
func waitForPluginReady(ctx context.Context, addr string, timeout time.Duration) (*grpc.ClientConn, error) {
	dialCtx, dialCancel := context.WithTimeout(ctx, timeout)
	defer dialCancel()

	conn, err := grpc.DialContext(dialCtx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("plugins: tier3: dial %s: %w", addr, err)
	}
	return conn, nil
}
