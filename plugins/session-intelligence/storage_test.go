// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceHash(t *testing.T) {
	h1 := workspaceHash("/home/user/project-a")
	h2 := workspaceHash("/home/user/project-b")
	h3 := workspaceHash("/home/user/project-a")

	if h1 == h2 {
		t.Error("different paths should produce different hashes")
	}
	if h1 != h3 {
		t.Error("same path should produce same hash")
	}
	if len(h1) != 12 {
		t.Errorf("hash should be 12 hex chars, got %d: %s", len(h1), h1)
	}
}

func TestDistillateStore_SaveAndLoadLatest(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"

	d1 := &Distillate{
		SessionID:  "sess-001",
		Workspace:  workspace,
		Timestamp:  time.Now().Add(-1 * time.Hour),
		Model:      "test-model",
		TokenCount: 100,
		Content: DistillateContent{
			KeyDecisions: []string{"decision 1"},
			ActiveFiles:  []string{"file1.go"},
			CurrentTask:  "task 1",
		},
	}
	d2 := &Distillate{
		SessionID:  "sess-002",
		Workspace:  workspace,
		Timestamp:  time.Now(),
		Model:      "test-model",
		TokenCount: 200,
		Content: DistillateContent{
			KeyDecisions: []string{"decision 2"},
			ActiveFiles:  []string{"file2.go"},
			CurrentTask:  "task 2",
		},
	}

	if err := store.Save("sess-001", workspace, d1); err != nil {
		t.Fatalf("save d1: %v", err)
	}
	if err := store.Save("sess-002", workspace, d2); err != nil {
		t.Fatalf("save d2: %v", err)
	}

	latest, err := store.LoadLatest(workspace)
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest distillate, got nil")
	}
	if latest.SessionID != "sess-002" {
		t.Errorf("expected sess-002, got %s", latest.SessionID)
	}
}

func TestDistillateStore_LoadLatest_NoDistillates(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)

	latest, err := store.LoadLatest("/nonexistent/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latest != nil {
		t.Error("expected nil for nonexistent workspace")
	}
}

func TestDistillateStore_ListAll(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"

	for i, id := range []string{"sess-001", "sess-002", "sess-003"} {
		d := &Distillate{
			SessionID:  id,
			Workspace:  workspace,
			Timestamp:  time.Now().Add(time.Duration(i) * time.Hour),
			TokenCount: 100 + i*50,
		}
		if err := store.Save(id, workspace, d); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	metas, err := store.ListAll(workspace)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(metas) != 3 {
		t.Fatalf("expected 3 distillates, got %d", len(metas))
	}
	if metas[0].SessionID != "sess-001" {
		t.Errorf("expected oldest first, got %s", metas[0].SessionID)
	}
}

func TestDistillateStore_Clear(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"

	d := &Distillate{SessionID: "sess-001", Workspace: workspace, Timestamp: time.Now()}
	if err := store.Save("sess-001", workspace, d); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := store.Clear(workspace); err != nil {
		t.Fatalf("clear: %v", err)
	}

	metas, err := store.ListAll(workspace)
	if err != nil {
		t.Fatalf("list after clear: %v", err)
	}
	if len(metas) != 0 {
		t.Errorf("expected 0 distillates after clear, got %d", len(metas))
	}
}

func TestDistillateStore_SaveConsolidated(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"

	for _, id := range []string{"sess-001", "sess-002", "sess-003"} {
		d := &Distillate{SessionID: id, Workspace: workspace, Timestamp: time.Now()}
		if err := store.Save(id, workspace, d); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}

	consolidated := &Distillate{
		SessionID:  "consolidated",
		Workspace:  workspace,
		Timestamp:  time.Now(),
		TokenCount: 500,
		Content: DistillateContent{
			KeyDecisions: []string{"merged decision"},
		},
	}

	if err := store.SaveConsolidated(workspace, consolidated, []string{"sess-001", "sess-002"}); err != nil {
		t.Fatalf("save consolidated: %v", err)
	}

	metas, err := store.ListAll(workspace)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}

	// Should have: consolidated-distillate.json + sess-003-distillate.json
	if len(metas) != 2 {
		t.Errorf("expected 2 distillates after consolidation, got %d", len(metas))
	}

	// Verify replaced files are gone.
	wsDir := store.workspaceDir(workspace)
	for _, id := range []string{"sess-001", "sess-002"} {
		path := filepath.Join(wsDir, id+"-distillate.json")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("replaced distillate %s should be deleted", id)
		}
	}
}

func TestDistillateStore_Count(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"

	if count := store.Count(workspace); count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	for _, id := range []string{"sess-001", "sess-002"} {
		d := &Distillate{SessionID: id, Workspace: workspace, Timestamp: time.Now()}
		store.Save(id, workspace, d)
	}

	if count := store.Count(workspace); count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestDistillateStore_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"

	d := &Distillate{
		SessionID: "sess-001",
		Workspace: workspace,
		Timestamp: time.Now(),
		Content: DistillateContent{
			KeyDecisions: []string{"test atomic write"},
		},
	}

	if err := store.Save("sess-001", workspace, d); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify no .tmp files remain.
	wsDir := store.workspaceDir(workspace)
	entries, _ := os.ReadDir(wsDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file should not remain: %s", e.Name())
		}
	}
}
