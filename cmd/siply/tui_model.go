// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	appstartup "siply.dev/siply/internal/app"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/fileutil"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/tui"
	"siply.dev/siply/internal/tui/panels"
)

const defaultOllamaURL = "http://localhost:11434"

type startupAgent struct {
	startup *appstartup.Startup
}

func (a *startupAgent) Run(ctx context.Context, userMessage string) error {
	if a == nil || a.startup == nil || a.startup.Agent == nil {
		return fmt.Errorf("tui: agent unavailable")
	}
	return a.startup.Agent.Run(ctx, userMessage)
}

func (a *startupAgent) Stop(ctx context.Context) error {
	if a == nil || a.startup == nil || a.startup.Agent == nil {
		return nil
	}
	return a.startup.Agent.Stop(ctx)
}

func (a *startupAgent) Close(ctx context.Context) error {
	if a == nil || a.startup == nil {
		return nil
	}
	return a.startup.Stop(ctx)
}

type tuiModelControllerOptions struct {
	Flags      tui.CLIFlags
	Startup    *appstartup.Startup
	EventBus   core.EventBus
	Hooks      core.AgentHooks
	Checkpoint core.CheckpointManager
	REPL       *panels.REPLPanel
}

type tuiModelController struct {
	mu sync.Mutex
	tuiModelControllerOptions
}

func newTUIModelController(opts tuiModelControllerOptions) *tuiModelController {
	return &tuiModelController{tuiModelControllerOptions: opts}
}

func (c *tuiModelController) ListModels(ctx context.Context) tui.ModelListResultMsg {
	c.mu.Lock()
	flags := c.Flags
	startup := c.Startup
	c.mu.Unlock()

	cfg := currentProviderConfig(startup)
	activeProvider, activeModel := activeTUISelection(flags, startup, cfg)
	options := cloudModelOptions(activeProvider, activeModel, providerKeyChecker(ctx, startup))
	localOptions, err := c.localModelOptions(ctx, cfg, activeProvider, activeModel)
	options = append(options, localOptions...)
	if len(options) == 0 && err != nil {
		return tui.ModelListResultMsg{Err: err}
	}
	return tui.ModelListResultMsg{Options: options}
}

func activeTUISelection(flags tui.CLIFlags, startup *appstartup.Startup, cfg core.ProviderConfig) (string, string) {
	activeProvider := resolveTUIProviderName(flags, startup, cfg)
	activeModel := strings.TrimSpace(cfg.Model)
	if startup != nil && strings.TrimSpace(startup.Model.Model) != "" {
		activeModel = strings.TrimSpace(startup.Model.Model)
	}
	if flags.Local || activeProvider == "ollama" {
		activeProvider = "ollama"
		activeModel = strings.TrimSpace(cfg.LocalModel)
		if startup != nil && startup.Model.Model != "" {
			activeModel = strings.TrimSpace(startup.Model.Model)
		}
	}
	return activeProvider, activeModel
}

func (c *tuiModelController) SwitchModel(ctx context.Context, option tui.ModelOption) tui.ModelSwitchResultMsg {
	if option.Disabled {
		return tui.ModelSwitchResultMsg{Option: option, Err: fmt.Errorf("model %q is not selectable", option.Model)}
	}

	c.mu.Lock()
	flags := c.Flags
	c.mu.Unlock()

	providerName := option.Provider
	preferLocal := option.Kind == "local" || option.Provider == "ollama"
	if preferLocal {
		providerName = "ollama"
		flags.Local = true
	} else {
		flags.Local = false
	}

	next, err := appstartup.Start(ctx, appstartup.Options{
		Surface:         appstartup.SurfaceTUI,
		HomeDir:         homeDir(),
		SessionProvider: providerName,
		SessionModel:    option.Model,
		PreferLocal:     preferLocal,
		EventBus:        c.EventBus,
		Hooks:           c.Hooks,
		Checkpoint:      c.Checkpoint,
		PermissionMode:  permission.ModeAutoAccept,
		OnPermissionEvaluator: func(perm core.PermissionEvaluator) {
			if ctrl, ok := perm.(interface {
				SetMode(permission.Mode) error
				Mode() permission.Mode
			}); ok && c.REPL != nil {
				c.REPL.SetPermissionController(ctrl)
			}
		},
	})
	if err != nil {
		return tui.ModelSwitchResultMsg{Option: option, Err: err}
	}

	if err := c.persistSelection(option); err != nil {
		_ = next.Stop(context.Background())
		return tui.ModelSwitchResultMsg{Option: option, Err: err}
	}

	c.mu.Lock()
	c.Flags = flags
	c.Startup = next
	c.mu.Unlock()

	return tui.ModelSwitchResultMsg{Option: option, Agent: &startupAgent{startup: next}}
}

func (c *tuiModelController) persistSelection(option tui.ModelOption) error {
	c.mu.Lock()
	startup := c.Startup
	c.mu.Unlock()
	path := selectionConfigPath(startup)
	if path == "" {
		return nil
	}
	switch option.Kind {
	case "local":
		return saveProjectProviderConfig(path, "ollama", "", option.Model)
	case "cloud":
		return saveProjectProviderConfig(path, option.Provider, option.Model, "")
	default:
		return nil
	}
}

// selectionConfigPath resolves where a /model selection is persisted (story
// 12.13 D1). Inside a workspace/project the project config.yaml takes
// precedence (unchanged behavior). Outside any workspace — no project config
// dir — it falls back to the global ~/.siply/config.yaml, which the precedence
// chain reads back at the global level on the next start.
func selectionConfigPath(startup *appstartup.Startup) string {
	configDir := ""
	if startup != nil && startup.WorkspaceManager != nil {
		configDir = startup.WorkspaceManager.ConfigDir()
	}
	return resolveSelectionConfigPath(configDir, homeDir())
}

// resolveSelectionConfigPath is the pure path-resolution rule behind
// selectionConfigPath: a non-empty project configDir wins (AC2 — no regression);
// otherwise fall back to the global ~/.siply/config.yaml (AC1). Returns "" only
// when neither a project dir nor a home dir is available.
func resolveSelectionConfigPath(configDir, home string) string {
	if configDir != "" {
		return filepath.Join(configDir, "config.yaml")
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".siply", "config.yaml")
}

func (c *tuiModelController) localModelOptions(ctx context.Context, cfg core.ProviderConfig, activeProvider, activeModel string) ([]tui.ModelOption, error) {
	baseURL := strings.TrimSpace(cfg.LocalURL)
	if baseURL == "" {
		baseURL = defaultOllamaURL
	}
	models, err := listOllamaModels(ctx, baseURL)
	if err != nil {
		return []tui.ModelOption{{
			Kind:     "local",
			Provider: "ollama",
			Model:    "(Ollama not running)",
			Disabled: true,
		}}, err
	}
	options := make([]tui.ModelOption, 0, len(models))
	for _, model := range models {
		options = append(options, tui.ModelOption{
			Kind:     "local",
			Provider: "ollama",
			Model:    model,
			Active:   activeProvider == "ollama" && model == activeModel,
		})
	}
	return options, nil
}

// noAPIKeyHint marks catalog entries for providers without a usable credential.
// Such entries are shown (for browsing/testing) but cannot be selected.
const noAPIKeyHint = "(no API key)"

// cloudModelOptions enumerates every curated cloud model across all known
// providers (story 12.13 D2). Providers are listed in deterministic order and
// each provider's models are shown even when no API key is configured — those
// entries are marked Disabled with a "(no API key)" hint so they remain visible
// but cannot be activated (the picker's move()/enter handling skips Disabled).
//
// hasKey reports whether a usable credential exists for a provider. A nil hasKey
// treats every provider as usable (used in contexts without a credential store).
func cloudModelOptions(activeProvider, activeModel string, hasKey func(provider string) bool) []tui.ModelOption {
	var options []tui.ModelOption
	for _, provider := range providers.CloudProviders() {
		models := providers.CloudModels(provider)
		// Keep the active model visible/markable even when it is not part of the
		// curated catalog (e.g. a date-suffixed variant or a custom override).
		if provider == activeProvider && activeModel != "" && !containsString(models, activeModel) {
			models = append(models, activeModel)
			sort.Strings(models)
		}
		keyed := hasKey == nil || hasKey(provider)
		for _, model := range models {
			opt := tui.ModelOption{
				Kind:     "cloud",
				Provider: provider,
				Model:    model,
				Active:   provider == activeProvider && model == activeModel,
			}
			if !keyed {
				opt.Disabled = true
				opt.Description = noAPIKeyHint
			}
			options = append(options, opt)
		}
	}
	return options
}

// providerKeyChecker returns a predicate reporting whether a usable credential
// exists for a cloud provider, backed by the active startup's CredentialStore.
// When no store is available (e.g. tests) it reports every provider as usable so
// models are never falsely dimmed.
func providerKeyChecker(ctx context.Context, startup *appstartup.Startup) func(provider string) bool {
	if startup == nil || startup.CredentialStore == nil {
		return func(string) bool { return true }
	}
	store := startup.CredentialStore
	return func(provider string) bool {
		cred, err := store.GetProvider(ctx, provider)
		if err != nil {
			return false
		}
		return strings.TrimSpace(cred.Value) != ""
	}
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func currentProviderConfig(startup *appstartup.Startup) core.ProviderConfig {
	if startup != nil && startup.ConfigLoader != nil {
		if cfg := startup.ConfigLoader.Config(); cfg != nil {
			return cfg.Provider
		}
	}
	return loadProviderConfig()
}

func resolveTUIProviderName(flags tui.CLIFlags, startup *appstartup.Startup, cfg core.ProviderConfig) string {
	if flags.Local {
		return "ollama"
	}
	if startup != nil && strings.TrimSpace(startup.ProviderName) != "" {
		return startup.ProviderName
	}
	if providerName := strings.TrimSpace(cfg.Default); providerName != "" {
		return providerName
	}
	return "anthropic"
}

func listOllamaModels(ctx context.Context, baseURL string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama models: create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama models: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("ollama models: decode tags: %w", err)
	}
	models := make([]string, 0, len(payload.Models))
	for _, model := range payload.Models {
		if name := strings.TrimSpace(model.Name); name != "" {
			models = append(models, name)
		}
	}
	sort.Strings(models)
	return models, nil
}

func saveProjectProviderConfig(path, provider, model, localModel string) error {
	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("model config: parse project config: %w", err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("model config: read project config: %w", err)
	}
	providerMap, _ := doc["provider"].(map[string]any)
	if providerMap == nil {
		providerMap = map[string]any{}
	}
	if provider != "" {
		providerMap["default"] = provider
	}
	if model != "" {
		providerMap["model"] = model
		delete(providerMap, "local_model")
	}
	if localModel != "" {
		providerMap["local_model"] = localModel
		delete(providerMap, "model")
	}
	doc["provider"] = providerMap
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("model config: marshal project config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("model config: create project config dir: %w", err)
	}
	if err := fileutil.AtomicWriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("model config: write project config: %w", err)
	}
	return nil
}
