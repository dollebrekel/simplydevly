// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package checkpoint

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
)

const (
	channelCapacity   = 32
	gzipThreshold     = 10 * 1024 // 10 KB
	maxFileHashSize   = 10 * 1024 * 1024 // 10 MB
	defaultMaxStorage = 100 * 1024 * 1024 // 100 MB
)

// Manager implements core.CheckpointManager with async disk writes.
type Manager struct {
	baseDir   string
	sessionID string
	writeCh   chan core.StepCheckpoint
	done      chan struct{}
	mu        sync.Mutex
}

// NewManager creates a checkpoint manager. Starts a background writer goroutine.
func NewManager(baseDir, sessionID string) *Manager {
	m := &Manager{
		baseDir:   baseDir,
		sessionID: sessionID,
		writeCh:   make(chan core.StepCheckpoint, channelCapacity),
		done:      make(chan struct{}),
	}
	go m.writer()
	return m
}

// Checkpoint enqueues a checkpoint for async disk write. Non-blocking.
func (m *Manager) Checkpoint(_ context.Context, step core.StepCheckpoint) error {
	select {
	case m.writeCh <- step:
		return nil
	default:
		slog.Warn("checkpoint dropped, write channel full", "step", step.StepNumber)
		return nil
	}
}

// List returns metadata for all checkpoints in a session, sorted by step number.
func (m *Manager) List(sessionID string) ([]core.CheckpointMeta, error) {
	dir := filepath.Join(m.baseDir, sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("checkpoint: list: %w", err)
	}

	var metas []core.CheckpointMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "step-") {
			continue
		}
		step, parseErr := parseStepNumber(e.Name())
		if parseErr != nil {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil {
			continue
		}

		meta, metaErr := m.readMeta(filepath.Join(dir, e.Name()), step, info.Size())
		if metaErr != nil {
			slog.Debug("checkpoint: skip corrupt step file", "file", e.Name(), "error", metaErr)
			continue
		}
		metas = append(metas, meta)
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].StepNumber < metas[j].StepNumber
	})
	return metas, nil
}

// Load reads and deserializes a specific checkpoint from disk.
func (m *Manager) Load(sessionID string, step int) (*core.Checkpoint, error) {
	path := m.stepPath(sessionID, step)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: load step %d: %w", step, err)
	}

	data, err = maybeDecompress(data)
	if err != nil {
		return nil, fmt.Errorf("checkpoint: decompress step %d: %w", step, err)
	}

	var sc core.StepCheckpoint
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("checkpoint: unmarshal step %d: %w", step, err)
	}

	info, _ := os.Stat(path)
	var diskSize int64
	if info != nil {
		diskSize = info.Size()
	}

	return &core.Checkpoint{
		StepCheckpoint: sc,
		DiskSize:       diskSize,
	}, nil
}

// Prune removes oldest session checkpoint directories until total size is under maxBytes.
// Never prunes the current session.
func (m *Manager) Prune(maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxStorage
	}

	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checkpoint: prune: %w", err)
	}

	type sessionInfo struct {
		name    string
		size    int64
		modTime time.Time
	}

	var sessions []sessionInfo
	var totalSize int64

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirPath := filepath.Join(m.baseDir, e.Name())
		size := dirSize(dirPath)
		info, infoErr := e.Info()
		var mod time.Time
		if infoErr == nil {
			mod = info.ModTime()
		}
		sessions = append(sessions, sessionInfo{name: e.Name(), size: size, modTime: mod})
		totalSize += size
	}

	if totalSize <= maxBytes {
		return nil
	}

	// Sort oldest first.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].modTime.Before(sessions[j].modTime)
	})

	target := maxBytes * 80 / 100 // prune to 80%
	for _, s := range sessions {
		if totalSize <= target {
			break
		}
		if s.name == m.sessionID {
			continue
		}
		if err := os.RemoveAll(filepath.Join(m.baseDir, s.name)); err != nil {
			slog.Warn("checkpoint: prune failed", "session", s.name, "error", err)
			continue
		}
		totalSize -= s.size
	}

	return nil
}

// DeleteAfterStep removes all checkpoint files with step numbers > step.
func (m *Manager) DeleteAfterStep(sessionID string, step int) error {
	dir := filepath.Join(m.baseDir, sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checkpoint: delete after step %d: %w", step, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "step-") {
			continue
		}
		n, parseErr := parseStepNumber(e.Name())
		if parseErr != nil {
			continue
		}
		if n > step {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
				slog.Warn("checkpoint: delete step failed", "step", n, "error", err)
			}
		}
	}
	return nil
}

// SessionID returns the session ID this manager was created with.
func (m *Manager) SessionID() string { return m.sessionID }

// Close shuts down the background writer and waits for pending writes to complete.
// Safe to call multiple times.
func (m *Manager) Close() error {
	m.mu.Lock()
	select {
	case <-m.done:
		// Already closed.
		m.mu.Unlock()
		return nil
	default:
	}
	close(m.writeCh)
	m.mu.Unlock()
	<-m.done
	return nil
}

// HashWorkspaceFiles computes SHA256 hashes for the given file paths.
// Skips files >10 MB and binary files.
func HashWorkspaceFiles(paths []string) map[string]string {
	hashes := make(map[string]string, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Size() > maxFileHashSize {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if isBinary(data) {
			continue
		}
		h := sha256.Sum256(data)
		hashes[p] = hex.EncodeToString(h[:])
	}
	return hashes
}

// writer is the background goroutine that serializes checkpoints to disk.
func (m *Manager) writer() {
	defer close(m.done)
	for step := range m.writeCh {
		if err := m.writeStep(step); err != nil {
			slog.Warn("checkpoint: write failed", "step", step.StepNumber, "error", err)
		}
	}
}

func (m *Manager) writeStep(step core.StepCheckpoint) error {
	dir := filepath.Join(m.baseDir, step.SessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	data, err := json.Marshal(step)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if len(data) > gzipThreshold {
		compressed, cErr := compressGzip(data)
		if cErr == nil {
			data = compressed
		}
	}

	path := m.stepPath(step.SessionID, step.StepNumber)
	return fileutil.AtomicWriteFile(path, data, 0o644)
}

func (m *Manager) stepPath(sessionID string, step int) string {
	return filepath.Join(m.baseDir, sessionID, fmt.Sprintf("step-%03d.json", step))
}

func (m *Manager) readMeta(path string, step int, diskSize int64) (core.CheckpointMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.CheckpointMeta{}, err
	}
	data, err = maybeDecompress(data)
	if err != nil {
		return core.CheckpointMeta{}, err
	}

	var partial struct {
		Timestamp  time.Time         `json:"timestamp"`
		ToolName   string            `json:"tool_name"`
		Messages   []json.RawMessage `json:"messages"`
		FileHashes map[string]string `json:"file_hashes"`
	}
	if err := json.Unmarshal(data, &partial); err != nil {
		return core.CheckpointMeta{}, err
	}

	return core.CheckpointMeta{
		StepNumber:   step,
		Timestamp:    partial.Timestamp,
		ToolName:     partial.ToolName,
		MessageCount: len(partial.Messages),
		FileCount:    len(partial.FileHashes),
		DiskSize:     diskSize,
	}, nil
}

func parseStepNumber(filename string) (int, error) {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	parts := strings.SplitN(name, "-", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid step filename: %s", filename)
	}
	return strconv.Atoi(parts[1])
}

func compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func maybeDecompress(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func isBinary(data []byte) bool {
	checkLen := 512
	if len(data) < checkLen {
		checkLen = len(data)
	}
	for i := 0; i < checkLen; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func dirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}
