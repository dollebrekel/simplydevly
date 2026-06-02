// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package app contains shared application startup wiring for CLI and TUI
// surfaces. Surface packages keep presentation and event adaptation here.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"siply.dev/siply/internal/agent"
	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/credential"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/hooks"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/providers/anthropic"
	"siply.dev/siply/internal/providers/kimi"
	"siply.dev/siply/internal/providers/ollama"
	"siply.dev/siply/internal/providers/openai"
	"siply.dev/siply/internal/providers/openrouter"
	"siply.dev/siply/internal/routing"
	"siply.dev/siply/internal/telemetry"
	"siply.dev/siply/internal/tools"
	"siply.dev/siply/internal/workspace"
)

const defaultProviderName = "anthropic"

const (
	SurfaceRun = "run"
	SurfaceTUI = "tui"
)

type ModelSource string

const (
	ModelSourceSession         ModelSource = "session"
	ModelSourceCLI             ModelSource = "cli"
	ModelSourceWorkspace       ModelSource = "workspace"
	ModelSourceProject         ModelSource = "project"
	ModelSourceGlobal          ModelSource = "global"
	ModelSourceEnv             ModelSource = "environment"
	ModelSourceLocalPreference ModelSource = "local-preference"
	ModelSourceProviderDefault ModelSource = "provider-default"
)

// ModelSelectionInput captures the model precedence contract. Startup normally
// receives already-merged config, but the expanded fields keep the precedence
// testable for future workspace-level model persistence.
type ModelSelectionInput struct {
	SessionModel    string
	CLIModel        string
	WorkspaceConfig core.ProviderConfig
	ProjectConfig   core.ProviderConfig
	GlobalConfig    core.ProviderConfig
	EnvModel        string
	PreferLocal     bool
	MergedConfig    core.ProviderConfig
}

type ModelSelection struct {
	Provider string
	Model    string
	Source   ModelSource
}

type ProviderSelectionInput struct {
	SessionProvider string
	CLIProvider     string
	ConfigProvider  string
	EnvProvider     string
	PreferLocal     bool
}

type ProviderFactory func(name string, credStore core.CredentialStore) (core.Provider, error)

type Options struct {
	Surface          string
	HomeDir          string
	WorkspaceName    string
	ModelOverride    string
	SessionModel     string
	SessionProvider  string
	ProviderOverride string
	PreferLocal      bool
	RoutingEnabled   bool
	TelemetryEnabled bool
	PermissionMode   permission.Mode
	EventBus         core.EventBus
	Hooks            core.AgentHooks
	Checkpoint       core.CheckpointManager
	ProviderFactory  ProviderFactory
	// AllowedTools, when non-empty, restricts the agent's tool registry to the
	// named built-in tools (e.g. a read-only/research center tab). Empty grants
	// the full built-in tool set.
	AllowedTools []string

	OnPermissionEvaluator func(core.PermissionEvaluator)
}

type Startup struct {
	Agent               *agent.Agent
	Provider            core.Provider
	EventBus            core.EventBus
	CredentialStore     core.CredentialStore
	WorkspaceManager    *workspace.Manager
	ToolRegistry        *tools.Registry
	PermissionEvaluator core.PermissionEvaluator
	ConfigLoader        *config.Loader
	Hooks               core.AgentHooks
	Telemetry           core.TelemetryCollector
	Model               ModelSelection
	ProviderName        string
	RoutingActive       bool

	owned   []lifecycleEntry
	started []lifecycleEntry
	stopMu  sync.Mutex
	stopped bool
}

type lifecycleEntry struct {
	name string
	lc   core.Lifecycle
}

func Start(ctx context.Context, opts Options) (*Startup, error) {
	if ctx == nil {
		return nil, fmt.Errorf("app startup: context is required")
	}
	homeDir := opts.HomeDir
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("app startup: determine home directory: %w", err)
		}
	}
	siplyDir := filepath.Join(homeDir, ".siply")

	st := &Startup{}
	st.CredentialStore = credential.NewFileStore(siplyDir)
	st.WorkspaceManager = workspace.NewManager(siplyDir)
	st.addOwned("credential-store", st.CredentialStore)
	st.addOwned("workspace", st.WorkspaceManager)

	if opts.EventBus != nil {
		st.EventBus = opts.EventBus
	} else {
		st.EventBus = events.NewBus()
		st.addOwned("events", st.EventBus)
	}

	if err := st.startPending(ctx); err != nil {
		return nil, err
	}

	if err := activateWorkspace(ctx, st.WorkspaceManager, opts.WorkspaceName); err != nil {
		_ = st.Stop(context.Background())
		return nil, err
	}

	st.ConfigLoader = config.NewLoader(config.LoaderOptions{
		GlobalDir:  siplyDir,
		ProjectDir: st.WorkspaceManager.ConfigDir(),
	})
	st.addOwned("config", st.ConfigLoader)
	if err := st.startPending(ctx); err != nil {
		return nil, err
	}

	cfg := st.ConfigLoader.Config()
	var providerCfg core.ProviderConfig
	if cfg != nil {
		providerCfg = cfg.Provider
	}

	st.Model = ResolveModelSelection(ModelSelectionInput{
		SessionModel: opts.SessionModel,
		CLIModel:     opts.ModelOverride,
		EnvModel:     os.Getenv("SIPLY_MODEL"),
		PreferLocal:  opts.PreferLocal,
		MergedConfig: providerCfg,
	})
	if st.Model.Source == ModelSourceProviderDefault && strings.TrimSpace(providerCfg.Model) != "" {
		st.Model.Model = strings.TrimSpace(providerCfg.Model)
		st.Model.Source = ModelSourceProject
	}

	st.ProviderName = ResolveProviderName(ProviderSelectionInput{
		SessionProvider: opts.SessionProvider,
		CLIProvider:     opts.ProviderOverride,
		ConfigProvider:  providerCfg.Default,
		EnvProvider:     os.Getenv("SIPLY_PROVIDER"),
		PreferLocal:     opts.PreferLocal,
	})
	if st.Model.Provider != "" && strings.TrimSpace(opts.SessionProvider) == "" &&
		strings.TrimSpace(opts.ProviderOverride) == "" && strings.TrimSpace(providerCfg.Default) == "" &&
		strings.TrimSpace(os.Getenv("SIPLY_PROVIDER")) == "" {
		st.ProviderName = st.Model.Provider
	}

	factory := opts.ProviderFactory
	if factory == nil {
		factory = BuildProvider
	}
	provider, err := factory(st.ProviderName, st.CredentialStore)
	if err != nil {
		_ = st.Stop(context.Background())
		return nil, fmt.Errorf("app startup: bootstrap provider: %w", err)
	}

	envRoutingEnabled := strings.EqualFold(os.Getenv("SIPLY_ROUTING_ENABLED"), "true")
	routingEnabled := opts.RoutingEnabled || envRoutingEnabled
	if cfg != nil && cfg.Routing.Enabled != nil && !opts.RoutingEnabled && !envRoutingEnabled {
		routingEnabled = *cfg.Routing.Enabled
	}
	if routingEnabled {
		routed, active, routeErr := BootstrapRouting(BootstrapRoutingOptions{
			CredentialStore: st.CredentialStore,
			Primary:         provider,
			PrimaryName:     st.ProviderName,
			EventBus:        st.EventBus,
			Factory:         factory,
		})
		if routeErr != nil {
			_ = st.Stop(context.Background())
			return nil, fmt.Errorf("app startup: bootstrap routing: %w", routeErr)
		}
		provider = routed
		st.RoutingActive = active
	}
	st.Provider = provider
	st.addOwned("provider", st.Provider)

	permCfg := permission.DefaultConfig()
	if opts.PermissionMode != "" {
		permCfg.Mode = opts.PermissionMode
	}
	perm, err := permission.NewEvaluator(permCfg)
	if err != nil {
		_ = st.Stop(context.Background())
		return nil, fmt.Errorf("app startup: bootstrap permission: %w", err)
	}
	st.PermissionEvaluator = perm
	if opts.OnPermissionEvaluator != nil {
		opts.OnPermissionEvaluator(perm)
	}
	st.ToolRegistry = tools.NewRegistry(perm)
	statusCollector := &agent.NoopStatusCollector{}
	contextMgr := agent.NewTruncationCompactor()
	if opts.TelemetryEnabled || strings.EqualFold(os.Getenv("SIPLY_TELEMETRY"), "true") {
		st.Telemetry = telemetry.NewTelemetryCollector()
	} else {
		st.Telemetry = telemetry.NewNoopCollector()
	}
	if opts.Hooks != nil {
		st.Hooks = opts.Hooks
	} else {
		st.Hooks = hooks.NewAgentHooks(st.EventBus)
		st.addOwned("hooks", st.Hooks)
	}

	st.addOwned("permission", st.PermissionEvaluator)
	st.addOwned("tools", st.ToolRegistry)
	st.addOwned("status", statusCollector)
	st.addOwned("context", contextMgr)
	st.addOwned("telemetry", st.Telemetry)
	if err := st.startPending(ctx); err != nil {
		return nil, err
	}

	// Restrict the toolset before the agent captures the registry (AC 6:
	// read-only/research tabs).
	if len(opts.AllowedTools) > 0 {
		st.ToolRegistry.Retain(opts.AllowedTools)
	}

	deps := agent.AgentDeps{
		Provider:   st.Provider,
		Tools:      st.ToolRegistry,
		Events:     st.EventBus,
		Tokens:     &agent.NoopTokenCounter{},
		Context:    contextMgr,
		Status:     statusCollector,
		Perm:       st.PermissionEvaluator,
		Hooks:      st.Hooks,
		Telemetry:  st.Telemetry,
		Checkpoint: opts.Checkpoint,
	}

	projectDir := ""
	if ws := st.WorkspaceManager.Active(); ws != nil {
		projectDir = ws.RootDir
	} else if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		projectDir = cwd
	}
	st.Agent = agent.NewAgent(deps, agent.AgentConfig{
		ProjectDir:    projectDir,
		HomeDir:       homeDir,
		ModelOverride: st.Model.Model,
	})
	st.addOwned("agent", st.Agent)
	if err := st.startPending(ctx); err != nil {
		return nil, err
	}

	return st, nil
}

func ResolveModelSelection(in ModelSelectionInput) ModelSelection {
	if model := strings.TrimSpace(in.SessionModel); model != "" {
		return ModelSelection{Model: model, Source: ModelSourceSession}
	}
	if model := strings.TrimSpace(in.CLIModel); model != "" {
		return ModelSelection{Model: model, Source: ModelSourceCLI}
	}
	if model := strings.TrimSpace(in.WorkspaceConfig.Model); model != "" {
		return ModelSelection{Model: model, Source: ModelSourceWorkspace}
	}
	if model := strings.TrimSpace(in.ProjectConfig.Model); model != "" {
		return ModelSelection{Model: model, Source: ModelSourceProject}
	}
	if model := strings.TrimSpace(in.GlobalConfig.Model); model != "" {
		return ModelSelection{Model: model, Source: ModelSourceGlobal}
	}
	if model := strings.TrimSpace(in.EnvModel); model != "" {
		return ModelSelection{Model: model, Source: ModelSourceEnv}
	}
	if in.PreferLocal {
		if model := strings.TrimSpace(providers.ResolveLocalModel("", in.MergedConfig)); model != "" {
			return ModelSelection{Provider: "ollama", Model: model, Source: ModelSourceLocalPreference}
		}
	}
	return ModelSelection{Source: ModelSourceProviderDefault}
}

func ResolveProviderName(in ProviderSelectionInput) string {
	if providerName := strings.TrimSpace(in.SessionProvider); providerName != "" {
		return providerName
	}
	if providerName := strings.TrimSpace(in.CLIProvider); providerName != "" {
		return providerName
	}
	if providerName := strings.TrimSpace(in.ConfigProvider); providerName != "" {
		return providerName
	}
	if providerName := strings.TrimSpace(in.EnvProvider); providerName != "" {
		return providerName
	}
	if in.PreferLocal {
		return "ollama"
	}
	return defaultProviderName
}

func BuildProvider(name string, credStore core.CredentialStore) (core.Provider, error) {
	switch strings.TrimSpace(name) {
	case "anthropic":
		return anthropic.New(credStore), nil
	case "openai":
		return openai.New(credStore), nil
	case "ollama":
		return ollama.New(credStore), nil
	case "openrouter":
		return openrouter.New(credStore), nil
	case "kimi":
		return kimi.New(credStore), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: anthropic, openai, ollama, openrouter, kimi)", name)
	}
}

type BootstrapRoutingOptions struct {
	CredentialStore core.CredentialStore
	Primary         core.Provider
	PrimaryName     string
	EventBus        core.EventBus
	Factory         ProviderFactory
}

func BootstrapRouting(opts BootstrapRoutingOptions) (core.Provider, bool, error) {
	preprocessProviderName := strings.TrimSpace(os.Getenv("SIPLY_PREPROCESS_PROVIDER"))
	if preprocessProviderName == "" {
		fmt.Fprintln(os.Stderr, "[routing] warning: routing enabled but SIPLY_PREPROCESS_PROVIDER not set - routing disabled")
		return opts.Primary, false, nil
	}

	factory := opts.Factory
	if factory == nil {
		factory = BuildProvider
	}
	preprocessModel := os.Getenv("SIPLY_PREPROCESS_MODEL")
	primaryName := strings.TrimSpace(opts.PrimaryName)
	if primaryName == "" {
		primaryName = defaultProviderName
	}

	providerMap := make(map[string]core.Provider)
	preprocessKey := preprocessProviderName
	primaryKey := primaryName
	if preprocessProviderName == primaryName {
		primaryKey = primaryName + "-primary"
		preprocessKey = preprocessProviderName + "-preprocess"
		providerMap[primaryKey] = opts.Primary
		providerMap[preprocessKey] = opts.Primary
	} else {
		providerMap[primaryKey] = opts.Primary
		preprocess, err := factory(preprocessProviderName, opts.CredentialStore)
		if err != nil {
			return nil, false, fmt.Errorf("routing: preprocess provider: %w", err)
		}
		providerMap[preprocessKey] = preprocess
	}

	cfg := routing.RoutingConfig{
		Rules: []routing.RoutingRule{
			{Category: routing.CategoryPreprocess, Provider: preprocessKey, Model: preprocessModel},
			{Category: routing.CategoryPrimary, Provider: primaryKey},
		},
		DefaultProvider: primaryKey,
		Enabled:         true,
	}
	if opts.EventBus != nil {
		opts.EventBus.Subscribe("routing.decision", func(_ context.Context, ev core.Event) {
			if re, ok := ev.(*routing.RoutingDecisionEvent); ok {
				model := re.SelectedModel
				if model == "" {
					model = "(default)"
				}
				fmt.Fprintf(os.Stderr, "[routing] -> provider=%s model=%s category=%s\n",
					re.SelectedProvider, model, re.Category)
			}
		})
	}

	return routing.NewRoutingProvider(routing.RoutingProviderConfig{
		Providers:       providerMap,
		Policy:          routing.NewConfigPolicy(cfg),
		DefaultProvider: primaryKey,
		EventBus:        opts.EventBus,
	}), true, nil
}

func (s *Startup) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.stopMu.Lock()
	if s.stopped {
		s.stopMu.Unlock()
		return nil
	}
	s.stopped = true
	started := append([]lifecycleEntry(nil), s.started...)
	s.stopMu.Unlock()

	if ctx == nil {
		ctx = context.Background()
	}
	if s.Telemetry != nil {
		if err := s.Telemetry.Flush(ctx); err != nil {
			slog.Warn("app startup: telemetry flush failed", "error", err)
		}
	}
	var errs []error
	for i := len(started) - 1; i >= 0; i-- {
		if err := started[i].lc.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("app startup: stop %s: %w", started[i].name, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Startup) addOwned(name string, lc core.Lifecycle) {
	if lc == nil {
		return
	}
	s.owned = append(s.owned, lifecycleEntry{name: name, lc: lc})
}

func (s *Startup) startOwned(ctx context.Context) error {
	return s.startRange(ctx, 0)
}

func (s *Startup) startPending(ctx context.Context) error {
	return s.startRange(ctx, len(s.started))
}

func (s *Startup) startRange(ctx context.Context, from int) error {
	initialized := make([]lifecycleEntry, 0, len(s.owned)-from)
	for i := from; i < len(s.owned); i++ {
		entry := s.owned[i]
		if err := entry.lc.Init(ctx); err != nil {
			s.rollback(ctx, initialized, nil)
			return fmt.Errorf("app startup: init %s: %w", entry.name, err)
		}
		initialized = append(initialized, entry)
	}
	for _, entry := range initialized {
		if err := entry.lc.Start(ctx); err != nil {
			s.rollback(ctx, nil, nil)
			return fmt.Errorf("app startup: start %s: %w", entry.name, err)
		}
		s.started = append(s.started, entry)
	}
	return nil
}

func (s *Startup) rollback(ctx context.Context, initialized, started []lifecycleEntry) {
	for i := len(initialized) - 1; i >= 0; i-- {
		_ = initialized[i].lc.Stop(ctx)
	}
	for i := len(started) - 1; i >= 0; i-- {
		_ = started[i].lc.Stop(ctx)
	}
	for i := len(s.started) - 1; i >= 0; i-- {
		_ = s.started[i].lc.Stop(ctx)
	}
	s.started = nil
	s.stopped = true
}

func activateWorkspace(ctx context.Context, manager *workspace.Manager, workspaceName string) error {
	if workspaceName != "" {
		if _, err := manager.Open(ctx, workspaceName); err != nil {
			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				return fmt.Errorf("app startup: determine working directory: %w", cwdErr)
			}
			if _, createErr := manager.Create(ctx, workspaceName, cwd); createErr != nil {
				return fmt.Errorf("app startup: create workspace %q: %w", workspaceName, createErr)
			}
		}
		return nil
	}
	ws, err := manager.Detect(ctx)
	if err != nil {
		return fmt.Errorf("app startup: detect workspace: %w", err)
	}
	if ws == nil {
		slog.Info("app startup: no git repository detected, running without workspace")
	}
	return nil
}
