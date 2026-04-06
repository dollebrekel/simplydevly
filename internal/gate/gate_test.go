// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package gate

import (
	"context"
	"testing"

	"siply.dev/siply/internal/core"
)

func TestRegisterFeature(t *testing.T) {
	g := NewFeatureGate(nil)

	feature := core.Feature{
		ID:          "context-distillation",
		Name:        "Context Distillation",
		Description: "Distill context for long conversations",
		Tier:        core.TierPro,
		PluginName:  "distiller",
	}

	if err := g.Register(feature); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}

	// Verify via List
	statuses := g.List()
	if len(statuses) != 1 {
		t.Fatalf("List() returned %d features, want 1", len(statuses))
	}
	if statuses[0].ID != "context-distillation" {
		t.Errorf("List()[0].ID = %q, want %q", statuses[0].ID, "context-distillation")
	}
	if statuses[0].Available != true {
		t.Errorf("List()[0].Available = %v, want true", statuses[0].Available)
	}
	if statuses[0].Tier != core.TierPro {
		t.Errorf("List()[0].Tier = %v, want TierPro", statuses[0].Tier)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	g := NewFeatureGate(nil)

	feature := core.Feature{ID: "my-feature", Name: "My Feature"}
	if err := g.Register(feature); err != nil {
		t.Fatalf("first Register() returned error: %v", err)
	}

	if err := g.Register(feature); err == nil {
		t.Fatal("second Register() should return error for duplicate feature")
	}
}

func TestRegisterEmptyID(t *testing.T) {
	g := NewFeatureGate(nil)

	if err := g.Register(core.Feature{ID: ""}); err == nil {
		t.Fatal("Register() with empty ID should return error")
	}
}

func TestGuardAlwaysAllows(t *testing.T) {
	g := NewFeatureGate(nil)
	ctx := context.Background()

	// Guard returns nil even for unregistered features (stub mode)
	if err := g.Guard(ctx, "nonexistent-feature"); err != nil {
		t.Errorf("Guard() returned error: %v, want nil", err)
	}

	// Guard returns nil for registered features too
	if err := g.Register(core.Feature{ID: "some-feature", Tier: core.TierPro}); err != nil {
		t.Fatalf("Register() returned error: %v", err)
	}
	if err := g.Guard(ctx, "some-feature"); err != nil {
		t.Errorf("Guard() returned error: %v, want nil", err)
	}
}

func TestGuardWithFallbackAlwaysAllowed(t *testing.T) {
	g := NewFeatureGate(nil)
	ctx := context.Background()

	result, err := g.GuardWithFallback(ctx, "any-feature")
	if err != nil {
		t.Fatalf("GuardWithFallback() returned error: %v", err)
	}
	if !result.Allowed {
		t.Error("GuardWithFallback().Allowed = false, want true")
	}
	if result.FeatureID != "any-feature" {
		t.Errorf("GuardWithFallback().FeatureID = %q, want %q", result.FeatureID, "any-feature")
	}
}

func TestListEmpty(t *testing.T) {
	g := NewFeatureGate(nil)

	statuses := g.List()
	if len(statuses) != 0 {
		t.Errorf("List() returned %d features, want 0", len(statuses))
	}
}

func TestLifecycleMethods(t *testing.T) {
	g := NewFeatureGate(nil)
	ctx := context.Background()

	if err := g.Init(ctx); err != nil {
		t.Errorf("Init() returned error: %v", err)
	}
	if err := g.Start(ctx); err != nil {
		t.Errorf("Start() returned error: %v", err)
	}
	if err := g.Health(); err != nil {
		t.Errorf("Health() returned error: %v", err)
	}
	if err := g.Stop(ctx); err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}
