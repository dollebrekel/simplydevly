// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	appstartup "siply.dev/siply/internal/app"
	"siply.dev/siply/internal/checkpoint"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/gate"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/sandbox"
	"siply.dev/siply/internal/skills"
)

// ansiPattern matches ANSI escape sequences for stripping in non-TTY mode.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func newRunCmd() *cobra.Command {
	var taskFlag, workspaceFlag, modelFlag string
	var yoloFlag, autoAcceptFlag, routingFlag, telemetryFlag, localFlag bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a one-shot task non-interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			if taskFlag == "" {
				return fmt.Errorf("run: --task flag is required")
			}
			if !localFlag && providers.IsLocalEnv() {
				localFlag = true
			}
			return executeRun(cmd.Context(), taskFlag, workspaceFlag, modelFlag, yoloFlag, autoAcceptFlag, routingFlag, telemetryFlag, localFlag)
		},
	}
	cmd.Flags().StringVar(&taskFlag, "task", "", "Task description to execute")
	cmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Workspace name to open")
	cmd.Flags().BoolVar(&yoloFlag, "yolo", false, "Skip all permission confirmations")
	cmd.Flags().BoolVar(&autoAcceptFlag, "auto-accept", false, "Auto-accept non-destructive actions")
	cmd.Flags().BoolVar(&routingFlag, "routing", false, "Enable smart model routing")
	cmd.Flags().BoolVar(&telemetryFlag, "telemetry", false, "Enable telemetry collection (opt-in)")
	cmd.Flags().BoolVar(&localFlag, "local", false, "Prefer local Ollama models when no provider/model is configured")
	cmd.Flags().StringVar(&modelFlag, "model", "", "Override the AI model to use")
	_ = cmd.MarkFlagRequired("task")
	return cmd
}

func executeRun(ctx context.Context, task, workspaceName, modelOverride string, yolo, autoAccept, routingEnabled, telemetryEnabled, local bool) error {
	// Bootstrap credential store.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("run: cannot determine home directory: %w", err)
	}
	siplyDir := filepath.Join(home, ".siply")

	permMode := permission.ModeDefault
	if yolo {
		permMode = permission.ModeYolo
	} else if autoAccept {
		permMode = permission.ModeAutoAccept
	}

	// Wire checkpoint manager (Pro feature).
	featureGate := gate.NewFeatureGate(nil)
	_ = featureGate.Init(ctx)
	_ = featureGate.Register(core.Feature{
		ID:          "checkpoint-rewind",
		Name:        "Checkpoint & Rewind",
		Description: "Deterministic session replay with conversation rewind",
		Tier:        core.TierPro,
	})
	var cpManager core.CheckpointManager
	if featureGate.Guard(ctx, "checkpoint-rewind") == nil {
		cpBaseDir := filepath.Join(siplyDir, "checkpoints")
		cpSessionID := fmt.Sprintf("sess-%s-%x", time.Now().Format("20060102-150405"), time.Now().UnixNano()%0xFFFF)
		cpMgr := checkpoint.NewManager(cpBaseDir, cpSessionID)
		cpManager = cpMgr
		defer func() { _ = cpMgr.Close() }()
		pruneLimit := int64(defaultMaxStorageMB) * 1024 * 1024
		if cfg := loadCheckpointConfig(); cfg.MaxStorageMB != nil {
			pruneLimit = int64(*cfg.MaxStorageMB) * 1024 * 1024
		}
		_ = cpMgr.Prune(pruneLimit)
	}

	startup, err := appstartup.Start(ctx, appstartup.Options{
		Surface:          appstartup.SurfaceRun,
		HomeDir:          home,
		WorkspaceName:    workspaceName,
		ModelOverride:    modelOverride,
		PreferLocal:      local,
		RoutingEnabled:   routingEnabled,
		TelemetryEnabled: telemetryEnabled,
		PermissionMode:   permMode,
		Checkpoint:       cpManager,
	})
	if err != nil {
		return fmt.Errorf("run: startup: %w", err)
	}
	defer func() { _ = startup.Stop(context.Background()) }()

	projectDir := startup.WorkspaceManager.ConfigDir()
	cfgLoader := startup.ConfigLoader
	registry := startup.ToolRegistry
	eventBus := startup.EventBus
	wireSkillActivationHook(ctx, startup.Hooks, newRuntimeSkillLoader(home, projectDir))

	// Wire execution sandbox (Pro feature — graceful degradation for Free users).
	// Must happen after config loading so user settings from ~/.siply/config.yaml apply.
	var sandboxCfg sandbox.Config
	if cfg := cfgLoader.Config(); cfg != nil {
		sandboxCfg = sandbox.ConfigFromCore(
			cfg.Sandbox.Enabled, cfg.Sandbox.FailIfUnavailable,
			cfg.Sandbox.ExtraReadPaths, cfg.Sandbox.ExtraWritePaths,
			cfg.Sandbox.AllowNetwork, cfg.Sandbox.MemoryLimitMB, cfg.Sandbox.MaxProcesses,
		)
	} else {
		sandboxCfg = sandbox.DefaultConfig()
	}
	if err := sandbox.ValidateConfig(sandboxCfg); err != nil {
		slog.Warn("sandbox: invalid config, using defaults", "error", err)
		sandboxCfg = sandbox.DefaultConfig()
	}
	if sandboxCfg.Enabled {
		sandboxProvider := sandbox.NewProvider(sandboxCfg)
		wsRoot := ""
		if ws := startup.WorkspaceManager.Active(); ws != nil {
			wsRoot = ws.RootDir
		}
		if sandboxProvider.Available() {
			registry.SetBashSandbox(sandboxProvider, sandboxCfg, nil, wsRoot)
			slog.Info("sandbox: active", "platform", sandboxProvider.Capabilities().Platform)
		} else if sandboxCfg.FailIfUnavailable {
			return fmt.Errorf("sandbox: required but not available (sandbox.fail_if_unavailable=true)")
		} else {
			slog.Debug("sandbox: not available, bash commands run unsandboxed")
		}
	}

	// Detect TTY for output formatting.
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	// Subscribe to stream.text events to collect agent output.
	// Mutex protects output because async EventBus delivery runs in a
	// dedicated goroutine, concurrent with the main goroutine that reads it.
	var outputMu sync.Mutex
	var output strings.Builder
	eventBus.Subscribe("stream.text", func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			outputMu.Lock()
			output.WriteString(te.Text())
			outputMu.Unlock()
		}
	})

	// Expand slash commands to skill prompt templates for CLI one-shot mode (AC#2, AC#3).
	task, slashErr := expandSlashCommand(ctx, task, home, projectDir)
	if slashErr != nil {
		return slashErr
	}

	// Execute the task.
	runErr := startup.Agent.Run(ctx, task)

	// Write collected output to stdout.
	outputMu.Lock()
	text := output.String()
	outputMu.Unlock()
	if text != "" {
		if !isTTY {
			text = stripANSI(text)
		}
		fmt.Fprint(os.Stdout, text)
	}

	if runErr != nil {
		return fmt.Errorf("run: agent execution: %w", runErr)
	}
	return nil
}

// bootstrapProvider creates a provider adapter based on SIPLY_PROVIDER env var.
func bootstrapProvider(credStore core.CredentialStore) (core.Provider, error) {
	return buildProvider(resolveProviderName(), credStore)
}

func resolveProviderName() string {
	return appstartup.ResolveProviderName(appstartup.ProviderSelectionInput{
		ConfigProvider: loadProviderConfig().Default,
		EnvProvider:    os.Getenv("SIPLY_PROVIDER"),
	})
}

// bootstrapRouting creates a RoutingProvider wrapping the primary provider
// and a preprocess provider configured via environment variables.
func bootstrapRouting(credStore core.CredentialStore, primary core.Provider, eventBus core.EventBus) (core.Provider, error) {
	provider, _, err := appstartup.BootstrapRouting(appstartup.BootstrapRoutingOptions{
		CredentialStore: credStore,
		Primary:         primary,
		PrimaryName:     resolveProviderName(),
		EventBus:        eventBus,
	})
	return provider, err
}

// buildProvider creates a provider adapter by name.
func buildProvider(name string, credStore core.CredentialStore) (core.Provider, error) {
	return appstartup.BuildProvider(name, credStore)
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// expandSlashCommand checks if task is a skill slash command and, if so, returns
// the rendered prompt template. Returns the original task unchanged for non-slash input.
// Returns an error if the input is a recognized slash command but dispatch fails.
func expandSlashCommand(ctx context.Context, task, homeDir, projectDir string) (string, error) {
	globalSkillsDir := skills.GlobalDir(homeDir)

	// Compute project-level skills dir.
	// projectDir is the .siply/ dir for the workspace — skills sit beside it.
	projectSkillsDir := ""
	if projectDir != "" {
		projectSkillsDir = filepath.Join(filepath.Dir(projectDir), ".siply", "skills")
	}

	loader := skills.NewSkillLoader(globalSkillsDir, projectSkillsDir)
	if err := loader.LoadAll(ctx); err != nil {
		slog.Warn("skills: load failed, falling through", "err", err)
		return task, nil
	}
	d := skills.NewSlashDispatcher(loader)
	if d == nil || !d.IsSlashCommand(task) {
		return task, nil
	}
	expanded, err := d.Dispatch(task)
	if err != nil {
		return "", fmt.Errorf("slash dispatch failed: %w", err)
	}
	return expanded, nil
}
