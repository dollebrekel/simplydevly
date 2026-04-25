// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	siplyv1 "siply.dev/siply/api/proto/gen/siply/v1"
)

func TestHandlePreQuery_NotInjected(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace, _ := os.Getwd()

	d := &Distillate{
		SessionID:  "sess-001",
		Workspace:  workspace,
		Timestamp:  time.Now(),
		TokenCount: 100,
		Content: DistillateContent{
			KeyDecisions: []string{"test decision"},
			CurrentTask:  "testing",
		},
	}
	store.Save("sess-001", workspace, d)

	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       store,
		config:      Config{Enabled: true},
	}

	req := prequeryRequest{
		Workspace: workspace,
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}
	payload, _ := json.Marshal(req)

	resp, err := plugin.handlePreQuery(t.Context(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}

	var result []Message
	json.Unmarshal(resp.Result, &result)
	if len(result) < 2 {
		t.Fatal("expected at least 2 messages (distillate + original)")
	}
	if result[0].Role != "system" {
		t.Errorf("expected system message first, got %s", result[0].Role)
	}
}

func TestHandlePreQuery_Disabled(t *testing.T) {
	plugin := &sessionIntelligencePlugin{
		initialized: true,
		config:      Config{Enabled: false},
	}

	msgs := []Message{{Role: "user", Content: "hello"}}
	payload, _ := json.Marshal(prequeryRequest{Workspace: "/tmp", Messages: msgs})

	resp, err := plugin.handlePreQuery(t.Context(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Disabled returns original payload unchanged.
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestHandlePreQuery_NoDistillate(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)

	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       store,
		config:      Config{Enabled: true},
	}

	req := prequeryRequest{
		Workspace: "/tmp/nonexistent",
		Messages:  []Message{{Role: "user", Content: "hello"}},
	}
	payload, _ := json.Marshal(req)

	resp, err := plugin.handlePreQuery(t.Context(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No distillate should return original payload.
	if !resp.Success {
		t.Error("expected success")
	}
}

func TestHandleDistillSession_Success(t *testing.T) {
	dir := t.TempDir()
	response := `{"key_decisions": ["test"], "active_files": [], "current_task": "testing", "constraints": [], "patterns": []}`
	srv := newMockOllama(response)
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-workspace"

	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       store,
		distiller:   NewSessionDistiller(client, 2),
		consolid:    NewConsolidator(client, 10, 800),
		config:      Config{Enabled: true},
	}

	req := distillSessionRequest{
		SessionID: "sess-test",
		Workspace: workspace,
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "do something"},
			{Role: "assistant", Content: "done"},
		},
	}
	payload, _ := json.Marshal(req)

	resp, err := plugin.handleDistillSession(t.Context(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.GetError())
	}

	latest, loadErr := store.LoadLatest(workspace)
	if loadErr != nil {
		t.Fatalf("load latest: %v", loadErr)
	}
	if latest == nil {
		t.Fatal("expected distillate to be saved")
	}
	if latest.SessionID != "sess-test" {
		t.Errorf("expected session ID sess-test, got %s", latest.SessionID)
	}
}

func TestHandleDistillSession_Disabled(t *testing.T) {
	plugin := &sessionIntelligencePlugin{
		initialized: true,
		config:      Config{Enabled: false},
	}

	req := distillSessionRequest{Messages: []Message{{Role: "user", Content: "hello"}}}
	payload, _ := json.Marshal(req)

	resp, err := plugin.handleDistillSession(t.Context(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("disabled should succeed silently")
	}
}

func TestHandleListDistillates(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-workspace"

	store.Save("sess-001", workspace, &Distillate{
		SessionID: "sess-001", Workspace: workspace, Timestamp: time.Now(), TokenCount: 100,
	})

	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       store,
		config:      Config{Enabled: true},
	}

	resp, err := plugin.handleListDistillates([]byte(workspace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.GetError())
	}

	var metas []*DistillateMeta
	json.Unmarshal(resp.Result, &metas)
	if len(metas) != 1 {
		t.Errorf("expected 1 meta, got %d", len(metas))
	}
}

func TestHandleListDistillates_EmptyWorkspace(t *testing.T) {
	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       NewDistillateStore(t.TempDir()),
		config:      Config{Enabled: true},
	}

	resp, err := plugin.handleListDistillates([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Error("expected failure for empty workspace")
	}
}

func TestHandleClearDistillates(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-workspace"

	store.Save("sess-001", workspace, &Distillate{SessionID: "sess-001", Workspace: workspace, Timestamp: time.Now()})

	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       store,
		config:      Config{Enabled: true},
	}

	resp, err := plugin.handleClearDistillates([]byte(workspace))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success")
	}

	if store.Count(workspace) != 0 {
		t.Error("expected 0 distillates after clear")
	}
}

func TestHandleClearDistillates_EmptyWorkspace(t *testing.T) {
	plugin := &sessionIntelligencePlugin{
		initialized: true,
		store:       NewDistillateStore(t.TempDir()),
		config:      Config{Enabled: true},
	}

	resp, err := plugin.handleClearDistillates([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Error("expected failure for empty workspace")
	}
}

func TestExecute_UnknownAction(t *testing.T) {
	plugin := &sessionIntelligencePlugin{initialized: true}

	resp, err := plugin.Execute(t.Context(), &siplyv1.ExecuteRequest{Action: "unknown-action"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Error("unknown action should fail")
	}
}
