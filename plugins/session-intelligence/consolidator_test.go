// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConsolidator_ShouldConsolidate(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := "/tmp/test-project"
	consolid := NewConsolidator(nil, 3, 800)

	for i := range 3 {
		id := "sess-" + string(rune('a'+i))
		d := &Distillate{SessionID: id, Workspace: workspace, Timestamp: time.Now()}
		store.Save(id, workspace, d)
	}
	if consolid.ShouldConsolidate(workspace, store) {
		t.Error("should not consolidate with exactly max distillates")
	}

	store.Save("sess-d", workspace, &Distillate{SessionID: "sess-d", Workspace: workspace, Timestamp: time.Now()})
	if !consolid.ShouldConsolidate(workspace, store) {
		t.Error("should consolidate when exceeding max")
	}
}

func TestConsolidator_Consolidate(t *testing.T) {
	dir := t.TempDir()
	store := NewDistillateStore(dir)
	workspace := t.TempDir()

	// Create a real file in workspace for file existence check.
	testFile := filepath.Join(workspace, "exists.go")
	os.WriteFile(testFile, []byte("package main"), 0o644)

	response := `{
		"key_decisions": ["merged decision"],
		"active_files": ["exists.go"],
		"current_task": "merged task",
		"constraints": [],
		"patterns": [{"pattern": "guard clauses", "confidence": "medium"}]
	}`
	srv := newMockOllama(response)
	defer srv.Close()

	client := NewOllamaClient(srv.URL, "test-model")
	consolid := NewConsolidator(client, 3, 800)

	for i := range 4 {
		id := "sess-" + string(rune('a'+i))
		d := &Distillate{
			SessionID: id,
			Workspace: workspace,
			Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
			Content: DistillateContent{
				ActiveFiles: []string{"exists.go", "deleted.go"},
				Patterns: []Pattern{
					{Pattern: "guard clauses", Confidence: "medium"},
				},
			},
		}
		store.Save(id, workspace, d)
	}

	consolidated, err := consolid.Consolidate(t.Context(), workspace, store)
	if err != nil {
		t.Fatalf("consolidation failed: %v", err)
	}
	if consolidated == nil {
		t.Fatal("expected consolidated distillate")
	}
	if consolidated.SessionID != "consolidated" {
		t.Errorf("expected session ID 'consolidated', got %s", consolidated.SessionID)
	}
}

func TestConsolidator_StrengthenPatterns(t *testing.T) {
	consolid := NewConsolidator(nil, 10, 800)

	content := &DistillateContent{
		Patterns: []Pattern{
			{Pattern: "guard clauses", Confidence: "medium"},
			{Pattern: "rare pattern", Confidence: "low"},
		},
	}

	sources := []*Distillate{
		{Content: DistillateContent{Patterns: []Pattern{{Pattern: "guard clauses", Confidence: "medium"}}}},
		{Content: DistillateContent{Patterns: []Pattern{{Pattern: "guard clauses", Confidence: "medium"}}}},
		{Content: DistillateContent{Patterns: []Pattern{{Pattern: "guard clauses", Confidence: "medium"}}}},
		{Content: DistillateContent{Patterns: []Pattern{{Pattern: "rare pattern", Confidence: "low"}}}},
	}

	consolid.strengthenPatterns(content, sources)

	if content.Patterns[0].Confidence != "high" {
		t.Errorf("pattern mentioned 3+ times should be high confidence, got %s", content.Patterns[0].Confidence)
	}
	if content.Patterns[1].Confidence != "low" {
		t.Errorf("rare pattern should stay low, got %s", content.Patterns[1].Confidence)
	}
}

func TestConsolidator_RemoveStaleFiles(t *testing.T) {
	consolid := NewConsolidator(nil, 10, 800)
	workspace := t.TempDir()

	existingFile := filepath.Join(workspace, "exists.go")
	os.WriteFile(existingFile, []byte("package main"), 0o644)

	distillates := []*Distillate{
		{Content: DistillateContent{ActiveFiles: []string{"exists.go", "deleted.go"}}},
	}

	consolid.removeStaleFiles(distillates, workspace)

	if len(distillates[0].Content.ActiveFiles) != 1 {
		t.Errorf("expected 1 valid file, got %d", len(distillates[0].Content.ActiveFiles))
	}
	if distillates[0].Content.ActiveFiles[0] != "exists.go" {
		t.Errorf("expected exists.go, got %s", distillates[0].Content.ActiveFiles[0])
	}
}
