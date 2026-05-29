// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/skills"
)

func newRuntimeSkillLoader(homeDir, projectDir string) *skills.SkillLoader {
	globalSkillsDir := skills.GlobalDir(homeDir)
	projectSkillsDir := ""
	if projectDir != "" {
		projectSkillsDir = filepath.Join(filepath.Dir(projectDir), ".siply", "skills")
	}
	return skills.NewSkillLoader(globalSkillsDir, projectSkillsDir)
}

func wireSkillActivationHook(ctx context.Context, agentHooks core.AgentHooks, loader *skills.SkillLoader) {
	if agentHooks == nil || loader == nil {
		return
	}
	if err := loader.LoadAll(ctx); err != nil {
		slog.Warn("skills: load failed, activation disabled", "error", err)
		return
	}
	activator := skills.NewActivator(loader)
	if activator == nil {
		return
	}
	agentHooks.OnPreQuery(activator.PreQueryHook, core.HookConfig{
		Priority:  5,
		OnFailure: core.HookSkipOnFailure,
		Timeout:   2 * time.Second,
	})
}
