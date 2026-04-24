// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// PanelContentUpdate represents a content update for a panel.
// This is the Go-native equivalent of the protobuf PanelContentUpdate message;
// the gRPC-to-native translation happens at the plugin host boundary.
type PanelContentUpdate struct {
	PanelName    string
	Lines        []string
	ChangedLines []int
	FrameID      int64
}

// ContentStream is the interface for receiving panel content updates.
// Implemented by gRPC stream clients and test doubles.
type ContentStream interface {
	Recv() (*PanelContentUpdate, error)
}

// ViewportRegistry is the interface for looking up panel viewports by name.
// Implemented by PanelManager.
type ViewportRegistry interface {
	PanelViewport(name string) *panelViewport
}

// ContentReceiver receives panel content streams and routes updates
// to the correct panel viewport. Thread-safe.
type ContentReceiver struct {
	mu       sync.Mutex
	registry ViewportRegistry
	cancels  map[string]context.CancelFunc
	frameBufs map[string]*frameBuf
}

type frameBuf struct {
	lines   []string
	frameID int64
}

// NewContentReceiver creates a ContentReceiver connected to the given registry.
func NewContentReceiver(registry ViewportRegistry) *ContentReceiver {
	return &ContentReceiver{
		registry:  registry,
		cancels:   make(map[string]context.CancelFunc),
		frameBufs: make(map[string]*frameBuf),
	}
}

func streamKey(pluginName, panelName string) string {
	return pluginName + ":" + panelName
}

// Subscribe starts consuming a content stream in a goroutine.
// If an existing stream is active for the same plugin+panel, it is cancelled first.
func (cr *ContentReceiver) Subscribe(
	ctx context.Context,
	pluginName string,
	panelName string,
	stream ContentStream,
) {
	key := streamKey(pluginName, panelName)

	cr.mu.Lock()
	if cancel, ok := cr.cancels[key]; ok {
		cancel()
	}
	streamCtx, cancel := context.WithCancel(ctx)
	cr.cancels[key] = cancel
	cr.mu.Unlock()

	go cr.consumeStream(streamCtx, pluginName, panelName, key, stream)
}

func (cr *ContentReceiver) consumeStream(
	ctx context.Context,
	pluginName string,
	panelName string,
	key string,
	stream ContentStream,
) {
	defer func() {
		cr.mu.Lock()
		delete(cr.cancels, key)
		delete(cr.frameBufs, panelName)
		cr.mu.Unlock()
	}()

	defer func() {
		if r := recover(); r != nil {
			slog.Error("content receiver panic", "plugin", pluginName, "panel", panelName, "error", r)
			cr.setPluginError(panelName, fmt.Errorf("internal error: %v", r))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		update, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				slog.Debug("panel content stream ended", "plugin", pluginName, "panel", panelName)
				return
			}
			slog.Warn("panel content stream error", "plugin", pluginName, "panel", panelName, "error", err)
			cr.setPluginError(panelName, fmt.Errorf("%s crashed", pluginName))
			return
		}

		cr.handleUpdate(panelName, update)
	}
}

func (cr *ContentReceiver) handleUpdate(panelName string, update *PanelContentUpdate) {
	target := update.PanelName
	if target == "" {
		target = panelName
	}

	if update.FrameID > 0 {
		cr.bufferFrame(target, update.Lines, update.FrameID)
		return
	}

	cr.applyContent(target, update.Lines)
}

func (cr *ContentReceiver) bufferFrame(panelName string, lines []string, frameID int64) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	buf, exists := cr.frameBufs[panelName]
	if !exists {
		cr.frameBufs[panelName] = &frameBuf{lines: lines, frameID: frameID}
		return
	}

	if buf.frameID != frameID {
		cr.applyContentLocked(panelName, buf.lines)
		cr.frameBufs[panelName] = &frameBuf{lines: lines, frameID: frameID}
		return
	}

	buf.lines = append(buf.lines, lines...)
}

// FlushFrame forces the current frame buffer to be applied to the viewport.
func (cr *ContentReceiver) FlushFrame(panelName string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	buf, exists := cr.frameBufs[panelName]
	if !exists || len(buf.lines) == 0 {
		return
	}
	cr.applyContentLocked(panelName, buf.lines)
	delete(cr.frameBufs, panelName)
}

func (cr *ContentReceiver) applyContent(panelName string, lines []string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.applyContentLocked(panelName, lines)
}

func (cr *ContentReceiver) applyContentLocked(panelName string, lines []string) {
	vp := cr.registry.PanelViewport(panelName)
	if vp == nil {
		slog.Warn("content update for unknown panel", "panel", panelName)
		return
	}
	vp.SetContent(strings.Join(lines, "\n"))
}

func (cr *ContentReceiver) setPluginError(panelName string, err error) {
	vp := cr.registry.PanelViewport(panelName)
	if vp == nil {
		return
	}
	vp.SetError(err)
}

// Unsubscribe stops the content stream for the given plugin+panel.
func (cr *ContentReceiver) Unsubscribe(pluginName, panelName string) {
	key := streamKey(pluginName, panelName)
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if cancel, ok := cr.cancels[key]; ok {
		cancel()
		delete(cr.cancels, key)
	}
	delete(cr.frameBufs, panelName)
}

// Stop cancels all active streams.
func (cr *ContentReceiver) Stop() {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	for key, cancel := range cr.cancels {
		cancel()
		delete(cr.cancels, key)
	}
	cr.frameBufs = make(map[string]*frameBuf)
}

// ActiveStreams returns the number of active stream subscriptions.
func (cr *ContentReceiver) ActiveStreams() int {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return len(cr.cancels)
}
