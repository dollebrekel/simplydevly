// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package plugins_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/plugins"
)

type stubRegistry struct {
	loadCount atomic.Int32
	devCalled atomic.Bool
}

func (s *stubRegistry) Init(_ context.Context) error                      { return nil }
func (s *stubRegistry) Start(_ context.Context) error                     { return nil }
func (s *stubRegistry) Stop(_ context.Context) error                      { return nil }
func (s *stubRegistry) Health() error                                     { return nil }
func (s *stubRegistry) Install(_ context.Context, _ string) error         { return nil }
func (s *stubRegistry) Load(_ context.Context, _ string) error            { s.loadCount.Add(1); return nil }
func (s *stubRegistry) List(_ context.Context) ([]core.PluginMeta, error) { return nil, nil }
func (s *stubRegistry) Remove(_ context.Context, _ string) error          { return nil }
func (s *stubRegistry) DevMode(_ context.Context, _ string) error {
	s.devCalled.Store(true)
	return nil
}

func TestDevWatcher_DetectsFileChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file watcher test in short mode")
	}

	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main\n"), 0o644)

	reg := &stubRegistry{}
	bus := events.NewBus()
	ctx := context.Background()
	bus.Init(ctx)
	bus.Start(ctx)
	defer bus.Stop(ctx)

	ch, unsub := bus.SubscribeChan(events.EventPluginReloaded)
	defer unsub()

	watcher := plugins.NewDevWatcher(dir, "test-plugin", reg, bus, nil)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)
	os.WriteFile(goFile, []byte("package main\n// changed\n"), 0o644)

	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for reload event")
	}

	if reg.loadCount.Load() == 0 {
		t.Error("expected registry.Load to be called")
	}
}

func TestDevWatcher_IgnoresNonWatchedFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file watcher test in short mode")
	}

	dir := t.TempDir()
	txtFile := filepath.Join(dir, "notes.txt")
	os.WriteFile(txtFile, []byte("hello"), 0o644)

	reg := &stubRegistry{}
	bus := events.NewBus()
	ctx := context.Background()
	bus.Init(ctx)
	bus.Start(ctx)
	defer bus.Stop(ctx)

	watcher := plugins.NewDevWatcher(dir, "test-plugin", reg, bus, nil)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)
	os.WriteFile(txtFile, []byte("changed"), 0o644)

	time.Sleep(700 * time.Millisecond)

	if reg.loadCount.Load() != 0 {
		t.Errorf("expected no reload for .txt file, got %d loads", reg.loadCount.Load())
	}
}

func TestDevWatcher_Debounce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping file watcher test in short mode")
	}

	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	os.WriteFile(goFile, []byte("package main\n"), 0o644)

	reg := &stubRegistry{}
	bus := events.NewBus()
	ctx := context.Background()
	bus.Init(ctx)
	bus.Start(ctx)
	defer bus.Stop(ctx)

	watcher := plugins.NewDevWatcher(dir, "test-plugin", reg, bus, nil)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer watcher.Stop()

	time.Sleep(100 * time.Millisecond)

	for i := range 5 {
		os.WriteFile(goFile, []byte("package main\n// "+string(rune('a'+i))+"\n"), 0o644)
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(800 * time.Millisecond)

	if reg.loadCount.Load() > 1 {
		t.Errorf("expected at most 1 reload due to debounce, got %d", reg.loadCount.Load())
	}
}
