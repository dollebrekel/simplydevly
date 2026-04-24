// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package panels

import (
	"io"
	"testing"
	"time"

	"context"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test helpers ───────────────────────────────────────────────────────────

type stubViewportRegistry struct {
	viewports map[string]*panelViewport
}

func newStubRegistry() *stubViewportRegistry {
	return &stubViewportRegistry{viewports: make(map[string]*panelViewport)}
}

func (s *stubViewportRegistry) PanelViewport(name string) *panelViewport {
	return s.viewports[name]
}

type fakeStream struct {
	updates []*PanelContentUpdate
	idx     int
	errAt   int
	err     error
}

func newFakeStream(updates ...*PanelContentUpdate) *fakeStream {
	return &fakeStream{updates: updates, errAt: -1}
}

func newFakeStreamWithError(err error, errAt int, updates ...*PanelContentUpdate) *fakeStream {
	return &fakeStream{updates: updates, errAt: errAt, err: err}
}

func (fs *fakeStream) Recv() (*PanelContentUpdate, error) {
	if fs.errAt >= 0 && fs.idx == fs.errAt {
		return nil, fs.err
	}
	if fs.idx >= len(fs.updates) {
		return nil, io.EOF
	}
	u := fs.updates[fs.idx]
	fs.idx++
	return u, nil
}

type blockingStream struct {
	ch chan struct{}
}

func (bs *blockingStream) Recv() (*PanelContentUpdate, error) {
	<-bs.ch
	return nil, io.EOF
}

func waitForStreamDone(t *testing.T, cr *ContentReceiver, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for streams to finish (active: %d)", cr.ActiveStreams())
		default:
			if cr.ActiveStreams() == 0 {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// ─── Tests ─────────────────────────────────────────────────────────────────

func TestContentReceiver_New(t *testing.T) {
	reg := newStubRegistry()
	cr := NewContentReceiver(reg)
	require.NotNil(t, cr)
	assert.Equal(t, 0, cr.ActiveStreams())
}

func TestContentReceiver_Subscribe_DirectUpdate(t *testing.T) {
	reg := newStubRegistry()
	vp := newPanelViewport(40, 10, "test-plugin")
	reg.viewports["my-panel"] = vp

	cr := NewContentReceiver(reg)
	stream := newFakeStream(&PanelContentUpdate{
		PanelName: "my-panel",
		Lines:     []string{"hello", "world"},
	})

	cr.Subscribe(context.Background(), "test-plugin", "my-panel", stream)
	waitForStreamDone(t, cr, 2*time.Second)

	assert.Equal(t, "hello\nworld", vp.Content())
	assert.True(t, vp.IsDirty())
}

func TestContentReceiver_Subscribe_MultipleUpdates(t *testing.T) {
	reg := newStubRegistry()
	vp := newPanelViewport(40, 10, "test-plugin")
	reg.viewports["my-panel"] = vp

	cr := NewContentReceiver(reg)
	stream := newFakeStream(
		&PanelContentUpdate{PanelName: "my-panel", Lines: []string{"first"}},
		&PanelContentUpdate{PanelName: "my-panel", Lines: []string{"second", "update"}},
	)

	cr.Subscribe(context.Background(), "test-plugin", "my-panel", stream)
	waitForStreamDone(t, cr, 2*time.Second)

	assert.Equal(t, "second\nupdate", vp.Content())
}

func TestContentReceiver_FrameBoundary_FlushOnNewFrame(t *testing.T) {
	reg := newStubRegistry()
	vp := newPanelViewport(40, 10, "test-plugin")
	reg.viewports["my-panel"] = vp

	cr := NewContentReceiver(reg)
	stream := newFakeStream(
		&PanelContentUpdate{PanelName: "my-panel", Lines: []string{"frame1-line1"}, FrameID: 1},
		&PanelContentUpdate{PanelName: "my-panel", Lines: []string{"frame1-line2"}, FrameID: 1},
		&PanelContentUpdate{PanelName: "my-panel", Lines: []string{"frame2-line1"}, FrameID: 2},
	)

	cr.Subscribe(context.Background(), "test-plugin", "my-panel", stream)
	waitForStreamDone(t, cr, 2*time.Second)

	content := vp.Content()
	assert.Contains(t, content, "frame1-line1")
}

func TestContentReceiver_FlushFrame_Manual(t *testing.T) {
	reg := newStubRegistry()
	vp := newPanelViewport(40, 10, "test-plugin")
	reg.viewports["my-panel"] = vp

	cr := NewContentReceiver(reg)
	cr.mu.Lock()
	cr.frameBufs["my-panel"] = &frameBuf{lines: []string{"buffered1", "buffered2"}, frameID: 42}
	cr.mu.Unlock()

	cr.FlushFrame("my-panel")
	assert.Equal(t, "buffered1\nbuffered2", vp.Content())
}

func TestContentReceiver_FlushFrame_NoOp_WhenEmpty(t *testing.T) {
	reg := newStubRegistry()
	cr := NewContentReceiver(reg)
	cr.FlushFrame("nonexistent")
}

func TestContentReceiver_PluginCrash_SetsError(t *testing.T) {
	reg := newStubRegistry()
	vp := newPanelViewport(40, 10, "crashy-plugin")
	reg.viewports["my-panel"] = vp

	cr := NewContentReceiver(reg)
	stream := newFakeStreamWithError(io.ErrUnexpectedEOF, 0)

	cr.Subscribe(context.Background(), "crashy-plugin", "my-panel", stream)
	waitForStreamDone(t, cr, 2*time.Second)

	assert.True(t, vp.HasError())
	assert.Contains(t, vp.ErrorMsg(), "crashy-plugin crashed")
}

func TestContentReceiver_PluginCrash_AfterData(t *testing.T) {
	reg := newStubRegistry()
	vp := newPanelViewport(40, 10, "crashy-plugin")
	reg.viewports["my-panel"] = vp

	cr := NewContentReceiver(reg)
	stream := newFakeStreamWithError(
		io.ErrUnexpectedEOF, 1,
		&PanelContentUpdate{PanelName: "my-panel", Lines: []string{"good data"}},
	)

	cr.Subscribe(context.Background(), "crashy-plugin", "my-panel", stream)
	waitForStreamDone(t, cr, 2*time.Second)

	assert.True(t, vp.HasError())
}

func TestContentReceiver_Unsubscribe(t *testing.T) {
	reg := newStubRegistry()
	cr := NewContentReceiver(reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockCh := make(chan struct{})
	stream := &blockingStream{ch: blockCh}

	cr.Subscribe(ctx, "test-plugin", "my-panel", stream)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, cr.ActiveStreams())

	cr.Unsubscribe("test-plugin", "my-panel")
	close(blockCh)
	waitForStreamDone(t, cr, 2*time.Second)
}

func TestContentReceiver_Stop(t *testing.T) {
	reg := newStubRegistry()
	cr := NewContentReceiver(reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockCh := make(chan struct{})
	stream := &blockingStream{ch: blockCh}

	cr.Subscribe(ctx, "p1", "panel1", stream)
	time.Sleep(50 * time.Millisecond)

	cr.Stop()
	close(blockCh)
	waitForStreamDone(t, cr, 2*time.Second)
}

func TestContentReceiver_UnknownPanel_NoError(t *testing.T) {
	reg := newStubRegistry()
	cr := NewContentReceiver(reg)

	stream := newFakeStream(&PanelContentUpdate{
		PanelName: "nonexistent",
		Lines:     []string{"data"},
	})

	cr.Subscribe(context.Background(), "test-plugin", "nonexistent", stream)
	waitForStreamDone(t, cr, 2*time.Second)
}
