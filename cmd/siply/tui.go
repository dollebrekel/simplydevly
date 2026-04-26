// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
	"siply.dev/siply/internal/agent"
	"siply.dev/siply/internal/checkpoint"
	"siply.dev/siply/internal/commands"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/credential"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/extensions"
	"siply.dev/siply/internal/gate"
	"siply.dev/siply/internal/hooks"
	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/providers/ollama"
	"siply.dev/siply/internal/sandbox"
	"siply.dev/siply/internal/skills"
	"siply.dev/siply/internal/tools"
	"siply.dev/siply/internal/tui"
	"siply.dev/siply/internal/tui/components"
	"siply.dev/siply/internal/tui/menu"
	"siply.dev/siply/internal/tui/panels"
	"siply.dev/siply/internal/tui/statusline"
)

func newTUICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the full-screen TUI interface",
		RunE: func(cmd *cobra.Command, args []string) error {
			start := time.Now()

			// Detect terminal capabilities.
			caps := tui.DetectCapabilities()

			// Parse CLI flags into TUI flags struct.
			flags, err := parseTUIFlags(cmd)
			if err != nil {
				return fmt.Errorf("tui: parse flags: %w", err)
			}

			// Resolve profile: CLI flag > config file > first-run prompt.
			if !flags.Minimal && !flags.Standard {
				profile, err := loadProfileFromConfig()
				if err != nil {
					slog.Debug("tui: loading profile from config", "error", err)
				}
				if profile != "" {
					flags.ConfigProfile = profile
				} else {
					// First-run prompt — no profile in flags or config.
					// Skip prompt when stdin is not a TTY (pipes, CI, cron).
					if !term.IsTerminal(int(os.Stdin.Fd())) {
						flags.ConfigProfile = builtinStandard
					} else {
						chosen, err := promptProfile(os.Stdin, os.Stdout)
						if err != nil {
							return fmt.Errorf("tui: profile prompt: %w", err)
						}
						flags.ConfigProfile = chosen
						if err := saveProfileToConfig(chosen); err != nil {
							slog.Warn("tui: could not save profile to config", "error", err)
						}
					}
				}
			}

			// Local mode: probe Ollama but don't block startup if unavailable.
			if flags.Local {
				probe := ollama.New(nil)
				if err := probe.Init(cmd.Context()); err != nil {
					slog.Warn("tui: Ollama not reachable, local LLM features unavailable", "error", err)
					flags.OllamaAvailable = false
				} else if err := probe.Health(); err != nil {
					slog.Warn("tui: Ollama not reachable, local LLM features unavailable", "error", err)
					flags.OllamaAvailable = false
				} else {
					flags.OllamaAvailable = true
				}
			}

			elapsed := time.Since(start)
			if elapsed > 400*time.Millisecond {
				slog.Warn("TUI startup exceeded 400ms target", "elapsed", elapsed)
			}

			return runTUI(caps, flags)
		},
	}
	return cmd
}

// parseTUIFlags extracts TUI-related persistent flags from the cobra command.
func parseTUIFlags(cmd *cobra.Command) (tui.CLIFlags, error) {
	var flags tui.CLIFlags
	var err error

	flags.NoColor, err = cmd.Flags().GetBool("no-color")
	if err != nil {
		return flags, err
	}
	flags.NoEmoji, err = cmd.Flags().GetBool("no-emoji")
	if err != nil {
		return flags, err
	}
	flags.NoBorders, err = cmd.Flags().GetBool("no-borders")
	if err != nil {
		return flags, err
	}
	flags.NoMotion, err = cmd.Flags().GetBool("no-motion")
	if err != nil {
		return flags, err
	}
	flags.Accessible, err = cmd.Flags().GetBool("accessible")
	if err != nil {
		return flags, err
	}
	flags.LowBandwidth, err = cmd.Flags().GetBool("low-bandwidth")
	if err != nil {
		return flags, err
	}
	flags.Minimal, err = cmd.Flags().GetBool("minimal")
	if err != nil {
		return flags, err
	}
	flags.Standard, err = cmd.Flags().GetBool("standard")
	if err != nil {
		return flags, err
	}

	flags.ModelOverride, err = cmd.Flags().GetString("model")
	if err != nil {
		return flags, err
	}

	// Check --local flag and SIPLY_LOCAL env var.
	flags.Local, err = cmd.Flags().GetBool("local")
	if err != nil {
		return flags, err
	}
	if !flags.Local && providers.IsLocalEnv() {
		flags.Local = true
	}

	// Mutual exclusivity: --minimal and --standard cannot be used together.
	if flags.Minimal && flags.Standard {
		return flags, fmt.Errorf("cannot use --minimal and --standard together")
	}

	return flags, nil
}

// promptProfile displays the first-run profile chooser and reads user input.
// Accepts io.Reader/io.Writer for testability.
func promptProfile(r io.Reader, w io.Writer) (string, error) {
	fmt.Fprintln(w, "Choose default layout:")
	fmt.Fprintln(w, "  [1] Minimal — bare REPL, no borders, single-line status")
	fmt.Fprintln(w, "  [2] Standard — borders, full status bar, emoji")
	fmt.Fprintln(w)
	fmt.Fprint(w, "Your choice (1/2): ")

	scanner := bufio.NewScanner(r)
	if scanner.Scan() {
		switch scanner.Text() {
		case "1":
			return builtinMinimal, nil
		case "2":
			return builtinStandard, nil
		default:
			return builtinStandard, nil // safe default
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return builtinStandard, nil // EOF → safe default
}

// siplyConfigData is the minimal struct for reading/writing ~/.siply/config.yaml.
type siplyConfigData struct {
	TUI struct {
		Profile string `yaml:"profile,omitempty"`
	} `yaml:"tui,omitempty"`

	// Preserve unknown fields during round-trip.
	Extra map[string]any `yaml:",inline"`
}

// loadProfileFromConfig reads the tui.profile field from ~/.siply/config.yaml.
func loadProfileFromConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".siply", "config.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var cfg siplyConfigData
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", err
	}

	// Validate profile value against allowlist.
	switch cfg.TUI.Profile {
	case builtinMinimal, builtinStandard, "":
		return cfg.TUI.Profile, nil
	default:
		slog.Warn("tui: ignoring unknown profile in config", "profile", cfg.TUI.Profile)
		return "", nil
	}
}

// runTUI creates the App with all components wired and starts the Bubble Tea program.
func runTUI(caps tui.Capabilities, flags tui.CLIFlags) error {
	app := tui.NewApp(caps, flags)

	theme := tui.DefaultTheme()
	rc := tui.NewRenderConfig(caps, flags)

	// Wire REPL panel.
	repl := panels.NewREPLPanel(theme, rc)
	app.SetREPLPanel(repl)

	// Wire SlashDispatcher for skill slash-command expansion (Story 10.6 Task 1).
	globalSkillsDir := skills.GlobalDir(homeDir())
	projectSkillsDir := detectProjectSkillsDir()
	skillLoader := skills.NewSkillLoader(globalSkillsDir, projectSkillsDir)
	if err := skillLoader.LoadAll(context.Background()); err != nil {
		slog.Warn("tui: skills load failed, slash commands may be unavailable", "error", err)
	}
	slashDispatcher := skills.NewSlashDispatcher(skillLoader)
	repl.SetSlashDispatcher(slashDispatcher, skillLoader)

	// Wire activity feed.
	feed := components.NewActivityFeed(theme, rc)
	app.SetActivityFeed(feed)

	// Wire diff view.
	dv := components.NewDiffView(theme, rc)
	app.SetDiffView(dv)

	// Wire markdown renderer.
	md := components.NewMarkdownView(theme, rc)
	app.SetMarkdownView(md)

	// Wire menu overlay (with markdown renderer for Learn view).
	overlay := menu.NewOverlay(theme, rc, md)
	app.SetMenuOverlay(overlay)

	// Wire marketplace browser.
	cacheDir := filepath.Join(homeDir(), ".siply", "cache")
	mbLoader := commands.NewLocalIndexLoader(cacheDir)
	var mbInstaller marketplace.InstallerFunc
	pluginsDir := filepath.Join(homeDir(), ".siply", "plugins")
	registry := plugins.NewLocalRegistry(pluginsDir)
	if initErr := registry.Init(context.Background()); initErr == nil {
		mbInstaller = registry.Install
	}
	mb := components.NewMarketBrowser(theme, rc, mbLoader, mbInstaller, cacheDir, "")
	app.SetMarketplaceBrowser(mb)

	// Wire status bar.
	sb := statusline.NewStatusBar(theme, rc, rc.Profile)
	if flags.Local {
		if flags.OllamaAvailable {
			provCfg := loadProviderConfig()
			localModel := providers.ResolveLocalModel(flags.ModelOverride, provCfg)
			sb.SetLocal(localModel)
		} else {
			sb.SetLocalNoLLM()
		}
	}
	app.SetStatusBar(sb)

	// Wire EventBus and ExtensionManager.
	bus := events.NewBus()
	if err := bus.Init(context.Background()); err != nil {
		slog.Warn("tui: eventbus init failed", "error", err)
	}
	if err := bus.Start(context.Background()); err != nil {
		slog.Warn("tui: eventbus start failed", "error", err)
	}
	defer func() { _ = bus.Stop(context.Background()) }()

	// Wire FeatureGate and AgentHooks for PreQuery hook support.
	featureGate := gate.NewFeatureGate(nil)
	if err := featureGate.Init(context.Background()); err != nil {
		slog.Warn("tui: feature gate init failed", "error", err)
	}
	if err := featureGate.Register(core.Feature{
		ID:          "code-intelligence",
		Name:        "Code Intelligence",
		Description: "Tree-sitter powered code context injection",
		Tier:        core.TierPro,
		PluginName:  "tree-sitter",
	}); err != nil {
		slog.Warn("tui: feature register failed", "error", err)
	}
	if err := featureGate.Register(core.Feature{
		ID:          "context-distillation",
		Name:        "Context Distillation",
		Description: "Local LLM context compression for 60%+ token savings",
		Tier:        core.TierPro,
		PluginName:  "context-distillation",
	}); err != nil {
		slog.Warn("tui: feature register failed", "error", err)
	}
	if err := featureGate.Register(core.Feature{
		ID:          "session-intelligence",
		Name:        "Session Intelligence",
		Description: "Persistent cross-session context via local LLM distillation",
		Tier:        core.TierPro,
		PluginName:  "session-intelligence",
	}); err != nil {
		slog.Warn("tui: feature register failed", "error", err)
	}
	if err := featureGate.Register(core.Feature{
		ID:          "execution-sandbox",
		Name:        "Execution Sandbox",
		Description: "OS-level process isolation for bash commands (Linux namespaces, macOS Seatbelt)",
		Tier:        core.TierPro,
	}); err != nil {
		slog.Warn("tui: feature register failed", "error", err)
	}

	if err := featureGate.Register(core.Feature{
		ID:          "checkpoint-rewind",
		Name:        "Checkpoint & Rewind",
		Description: "Deterministic session replay with conversation rewind to any tool execution boundary",
		Tier:        core.TierPro,
	}); err != nil {
		slog.Warn("tui: feature register failed", "error", err)
	}

	// Wire checkpoint manager if feature is available (Pro).
	var cpManager core.CheckpointManager
	if featureGate.Guard(context.Background(), "checkpoint-rewind") == nil {
		cpBaseDir := filepath.Join(homeDir(), ".siply", "checkpoints")
		sessionID := fmt.Sprintf("sess-%s-%x", time.Now().Format("20060102-150405"), time.Now().UnixNano()%0xFFFF)
		cpManager = checkpoint.NewManager(cpBaseDir, sessionID)
		defer func() {
			if cpManager != nil {
				_ = cpManager.Close()
			}
		}()

		// Prune on start (default: true, 100 MB limit).
		cpMgr := cpManager.(*checkpoint.Manager)
		pruneLimit := int64(defaultMaxStorageMB) * 1024 * 1024
		if cfg := loadCheckpointConfig(); cfg.MaxStorageMB != nil {
			pruneLimit = int64(*cfg.MaxStorageMB) * 1024 * 1024
		}
		if err := cpMgr.Prune(pruneLimit); err != nil {
			slog.Warn("tui: checkpoint prune failed", "error", err)
		}
	}

	// Probe sandbox availability for status bar indicator (Pro feature).
	sandboxProvider := sandbox.NewProvider(sandbox.DefaultConfig())
	if featureGate.Guard(context.Background(), "execution-sandbox") == nil {
		if sandboxProvider.Available() {
			sb.SetSandboxStatus("active")
		} else {
			sb.SetSandboxStatus("unavailable")
		}
	}

	agentHooks := hooks.NewAgentHooks(bus)
	if err := agentHooks.Init(context.Background()); err != nil {
		slog.Warn("tui: agent hooks init failed", "error", err)
	}

	panelMgr := panels.NewPanelManager(theme, rc)
	em := extensions.NewManager(panelMgr, bus, pluginsDir)
	if err := em.Init(context.Background()); err != nil {
		slog.Warn("tui: extension manager init failed", "error", err)
	}
	if err := em.Start(context.Background()); err != nil {
		slog.Warn("tui: extension manager start failed", "error", err)
	}
	defer func() { _ = em.Stop(context.Background()) }()

	app.SetPanelManager(panelMgr)
	app.SetExtensionManager(em)

	// Wire Tier2Loader for Lua plugin support.
	tier2Loader := plugins.NewTier2Loader(registry, bus, em)
	registry.SetTier2Loader(tier2Loader)

	// Wire HostServer and Tier3Loader for native Go plugin support.
	var tier3Loader *plugins.Tier3Loader
	hostServer := plugins.NewHostServer(plugins.HostServerOptions{
		EventBus: bus,
	})
	if err := hostServer.Start(context.Background()); err != nil {
		slog.Warn("tui: host server start failed, Tier 3 plugins unavailable", "error", err)
	} else {
		defer func() { _ = hostServer.Stop(context.Background()) }()

		tier3Loader = plugins.NewTier3Loader(registry, hostServer)

		em.SetContentProvider(func(pluginName string) func(width, height int) string {
			return func(width, height int) string {
				payload := []byte{byte(width >> 8), byte(width), byte(height >> 8), byte(height)}
				result, err := tier3Loader.Execute(context.Background(), pluginName, "render", payload)
				if err != nil {
					return fmt.Sprintf("[%s: %v]", pluginName, err)
				}
				return string(result)
			}
		})

		em.SetActionProvider(func(pluginName, action string, payload []byte) {
			_, err := tier3Loader.Execute(context.Background(), pluginName, action, payload)
			if err != nil {
				slog.Debug("tui: plugin action failed", "plugin", pluginName, "action", action, "error", err)
			}
		})
		panelMgr.SetActionSender(em.SendAction)

		// Load and spawn all installed Tier 3 plugins.
		pluginList, listErr := registry.List(context.Background())
		if listErr != nil {
			slog.Warn("tui: registry list failed", "error", listErr)
		}
		for _, meta := range pluginList {
			if meta.Tier != 3 {
				continue
			}
			if err := tier3Loader.Load(context.Background(), meta.Name); err != nil {
				slog.Warn("tui: tier3 plugin load failed", "plugin", meta.Name, "error", err)
				continue
			}
			if err := tier3Loader.Spawn(context.Background(), meta.Name); err != nil {
				slog.Warn("tui: tier3 plugin spawn failed", "plugin", meta.Name, "error", err)
				_ = tier3Loader.Unload(context.Background(), meta.Name)
				continue
			}
			defer func(name string) {
				_ = tier3Loader.Unload(context.Background(), name)
			}(meta.Name)

			// Publish PluginLoadedEvent so ExtensionManager auto-registers extensions.
			_ = bus.Publish(context.Background(), events.NewPluginLoadedEvent(meta.Name, meta.Version, meta.Tier))

			if meta.Name == "tree-sitter" {
				wireTreeSitterHook(agentHooks, tier3Loader, featureGate)
			}
			if meta.Name == "context-distillation" {
				wireDistillationHook(agentHooks, tier3Loader, featureGate)
			}
			if meta.Name == "session-intelligence" {
				wireSessionIntelligenceHook(agentHooks, tier3Loader, featureGate)
			}
		}
	}

	// Message collector for session-end distillation.
	var latestMsgs []core.Message
	var msgsMu sync.Mutex
	agentHooks.OnPreQuery(func(_ context.Context, msgs []core.Message) ([]core.Message, error) {
		msgsMu.Lock()
		latestMsgs = make([]core.Message, len(msgs))
		copy(latestMsgs, msgs)
		msgsMu.Unlock()
		return msgs, nil
	}, core.HookConfig{
		Priority:  99,
		OnFailure: core.HookSkipOnFailure,
		Timeout:   1 * time.Second,
	})

	// Bootstrap AI agent for REPL interaction (Story 12.9).
	ag := bootstrapTUIAgent(flags, bus, agentHooks, cpManager)
	if ag != nil {
		app.SetAgent(ag)
		defer func() { _ = ag.Stop(context.Background()) }()
	}

	appErr := tui.RunApp(app, caps, func(prog *tea.Program) {
		bridgeEventBus(bus, prog)
	})

	// Session-end: trigger distillation for session-intelligence plugin.
	if tier3Loader != nil {
		if err := featureGate.Guard(context.Background(), "session-intelligence"); err == nil {
			msgsMu.Lock()
			collected := latestMsgs
			msgsMu.Unlock()

			if len(collected) > 0 {
				cwd, _ := os.Getwd()
				sessionID := fmt.Sprintf("sess-%s-%x", time.Now().Format("20060102-150405"), time.Now().UnixNano()%0xFFFF)
				_ = bus.Publish(context.Background(), events.NewSessionEndedEvent(sessionID, len(collected), countTurns(collected)))
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				payload, jsonErr := json.Marshal(map[string]any{
					"session_id": sessionID,
					"workspace":  cwd,
					"messages":   collected,
				})
				if jsonErr == nil {
					_, _ = tier3Loader.Execute(ctx, "session-intelligence", "distill-session", payload)
				}
			}
		}
	}

	return appErr
}

func countTurns(msgs []core.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Role == "user" || m.Role == "assistant" {
			n++
		}
	}
	return n
}

func wireTreeSitterHook(agentHooks core.AgentHooks, tl *plugins.Tier3Loader, fg core.FeatureGate) {
	agentHooks.OnPreQuery(func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		if err := fg.Guard(ctx, "code-intelligence"); err != nil {
			return msgs, nil
		}
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return nil, fmt.Errorf("tree-sitter prequery: getwd: %w", cwdErr)
		}
		result, err := tl.Execute(ctx, "tree-sitter", "prequery", []byte(cwd))
		if err != nil {
			return nil, fmt.Errorf("tree-sitter prequery: %w", err)
		}
		if len(result) == 0 {
			return msgs, nil
		}
		contextMsg := core.Message{
			Role:    "system",
			Content: string(result),
		}
		return append([]core.Message{contextMsg}, msgs...), nil
	}, core.HookConfig{
		Priority:  10,
		OnFailure: core.HookSkipOnFailure,
		Timeout:   5 * time.Second,
	})
}

func wireDistillationHook(agentHooks core.AgentHooks, tl *plugins.Tier3Loader, fg core.FeatureGate) {
	agentHooks.OnPreQuery(func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		if err := fg.Guard(ctx, "context-distillation"); err != nil {
			return msgs, nil
		}
		payload, jsonErr := json.Marshal(msgs)
		if jsonErr != nil {
			slog.Warn("distillation prequery: marshal failed", "err", jsonErr)
			return msgs, nil
		}
		result, err := tl.Execute(ctx, "context-distillation", "prequery", payload)
		if err != nil {
			slog.Warn("distillation prequery: execute failed", "err", err)
			return msgs, nil
		}
		if len(result) == 0 {
			return msgs, nil
		}
		var modified []core.Message
		if jsonErr := json.Unmarshal(result, &modified); jsonErr != nil {
			slog.Warn("distillation prequery: unmarshal failed", "err", jsonErr)
			return msgs, nil
		}
		// Preserve original core.Message objects for recent messages to avoid
		// losing ToolCalls/ToolResults during JSON round-trip through plugin.
		if len(modified) > 1 && strings.HasPrefix(modified[0].Content, "[Context Distillate]") {
			recentCount := len(modified) - 1
			if recentCount <= len(msgs) {
				preserved := make([]core.Message, 0, recentCount+1)
				preserved = append(preserved, modified[0])
				preserved = append(preserved, msgs[len(msgs)-recentCount:]...)
				return preserved, nil
			}
		}
		return modified, nil
	}, core.HookConfig{
		Priority:  20,
		OnFailure: core.HookSkipOnFailure,
		Timeout:   15 * time.Second,
	})
}

func wireSessionIntelligenceHook(agentHooks core.AgentHooks, tl *plugins.Tier3Loader, fg core.FeatureGate) {
	var injected atomic.Bool
	agentHooks.OnPreQuery(func(ctx context.Context, msgs []core.Message) ([]core.Message, error) {
		if injected.Load() {
			return msgs, nil
		}
		if err := fg.Guard(ctx, "session-intelligence"); err != nil {
			return msgs, nil
		}
		cwd, _ := os.Getwd()
		payload, jsonErr := json.Marshal(map[string]any{
			"workspace": cwd,
			"messages":  msgs,
		})
		if jsonErr != nil {
			slog.Warn("session-intelligence prequery: marshal failed", "err", jsonErr)
			return msgs, nil
		}
		result, err := tl.Execute(ctx, "session-intelligence", "prequery", payload)
		if err != nil {
			slog.Warn("session-intelligence prequery: execute failed", "err", err)
			return msgs, nil
		}
		injected.Store(true)
		if len(result) == 0 {
			return msgs, nil
		}
		var modified []core.Message
		if jsonErr := json.Unmarshal(result, &modified); jsonErr != nil {
			slog.Warn("session-intelligence prequery: unmarshal failed", "err", jsonErr)
			return msgs, nil
		}
		if len(modified) > 1 && strings.HasPrefix(modified[0].Content, "[Session Intelligence]") {
			preserved := make([]core.Message, 0, len(modified))
			preserved = append(preserved, modified[0])
			preserved = append(preserved, msgs...)
			return preserved, nil
		}
		return modified, nil
	}, core.HookConfig{
		Priority:  5,
		OnFailure: core.HookSkipOnFailure,
		Timeout:   10 * time.Second,
	})
}

// bootstrapTUIAgent creates and initializes the AI agent for TUI REPL interaction.
// Returns nil if the agent could not be created (provider unavailable, etc.).
func bootstrapTUIAgent(flags tui.CLIFlags, bus *events.Bus, agentHooks core.AgentHooks, cpManager core.CheckpointManager) *agent.Agent {
	siplyDir := filepath.Join(homeDir(), ".siply")
	credStore := credential.NewFileStore(siplyDir)

	ctx := context.Background()
	if err := credStore.Init(ctx); err != nil {
		slog.Warn("tui: credential store init failed, agent unavailable", "error", err)
		return nil
	}
	if err := credStore.Start(ctx); err != nil {
		slog.Warn("tui: credential store start failed, agent unavailable", "error", err)
		return nil
	}

	success := false
	defer func() {
		if !success {
			_ = credStore.Stop(ctx)
		}
	}()

	var provider core.Provider
	if flags.Local {
		provider = ollama.New(credStore)
	} else {
		var bErr error
		provider, bErr = bootstrapProvider(credStore)
		if bErr != nil {
			slog.Warn("tui: provider bootstrap failed, agent unavailable", "error", bErr)
			return nil
		}
	}

	if err := provider.Init(ctx); err != nil {
		slog.Warn("tui: provider init failed, agent unavailable", "error", err)
		return nil
	}
	if err := provider.Start(ctx); err != nil {
		slog.Warn("tui: provider start failed, agent unavailable", "error", err)
		return nil
	}
	defer func() {
		if !success {
			_ = provider.Stop(ctx)
		}
	}()

	permCfg := permission.DefaultConfig()
	permCfg.Mode = permission.ModeAutoAccept
	perm, err := permission.NewEvaluator(permCfg)
	if err != nil {
		slog.Warn("tui: permission bootstrap failed, agent unavailable", "error", err)
		return nil
	}
	if err := perm.Init(ctx); err != nil {
		slog.Warn("tui: permission init failed", "error", err)
		return nil
	}

	toolRegistry := tools.NewRegistry(perm)
	if err := toolRegistry.Init(ctx); err != nil {
		slog.Warn("tui: tool registry init failed, agent unavailable", "error", err)
		return nil
	}

	tokenCounter := &agent.NoopTokenCounter{}
	statusCollector := &agent.NoopStatusCollector{}
	contextMgr := agent.NewTruncationCompactor()

	deps := agent.AgentDeps{
		Provider:   provider,
		Tools:      toolRegistry,
		Events:     bus,
		Tokens:     tokenCounter,
		Context:    contextMgr,
		Status:     statusCollector,
		Perm:       perm,
		Hooks:      agentHooks,
		Checkpoint: cpManager,
	}

	var localModel string
	if flags.Local {
		provCfg := loadProviderConfig()
		localModel = providers.ResolveLocalModel(flags.ModelOverride, provCfg)
	}

	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		slog.Warn("tui: getwd failed, using empty project dir", "error", cwdErr)
	}
	ag := agent.NewAgent(deps, agent.AgentConfig{
		ProjectDir:    cwd,
		HomeDir:       homeDir(),
		ModelOverride: localModel,
	})
	if err := ag.Init(ctx); err != nil {
		slog.Warn("tui: agent init failed", "error", err)
		return nil
	}
	if err := ag.Start(ctx); err != nil {
		slog.Warn("tui: agent start failed", "error", err)
		return nil
	}

	success = true
	return ag
}

// bridgeEventBus subscribes to EventBus events and forwards them as BubbleTea messages.
// The bridge is one-way: EventBus → tea.Program.Send(). Never the reverse.
func bridgeEventBus(bus *events.Bus, prog *tea.Program) {
	bus.Subscribe(events.EventPluginLoaded, func(_ context.Context, ev core.Event) {
		if ple, ok := ev.(*events.PluginLoadedEvent); ok {
			prog.Send(tui.PluginLoadedMsg{Name: ple.Name, Version: ple.Version, Tier: ple.Tier})
		}
	})
	bus.Subscribe(events.EventMenuChanged, func(_ context.Context, _ core.Event) {
		prog.Send(tui.MenuChangedMsg{})
	})
	bus.Subscribe(events.EventKeybindChanged, func(_ context.Context, _ core.Event) {
		prog.Send(tui.KeybindChangedMsg{})
	})
	bus.Subscribe(events.EventPanelActivated, func(_ context.Context, ev core.Event) {
		if pae, ok := ev.(*events.PanelActivatedEvent); ok {
			prog.Send(tui.PanelActivatedMsg{Name: pae.PanelName})
		}
	})

	// Stream agent text chunks to REPL panel.
	bus.Subscribe(events.EventStreamText, func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			prog.Send(tui.AgentOutputMsg{Text: te.Text()})
		}
	})

	// Show tool calls in activity feed.
	bus.Subscribe(events.EventStreamToolCall, func(_ context.Context, ev core.Event) {
		if tc, ok := ev.(interface{ ToolName() string }); ok {
			prog.Send(tui.FeedEntryMsg{
				Type:  "tool",
				Label: tc.ToolName(),
			})
		}
	})

	// Show tool execution results in activity feed.
	bus.Subscribe(events.EventToolExecuted, func(_ context.Context, ev core.Event) {
		if te, ok := ev.(*agent.ToolExecutedEvent); ok {
			prog.Send(tui.FeedEntryMsg{
				Type:     "tool-done",
				Label:    te.ToolName,
				Duration: te.Duration,
				IsError:  te.IsError,
			})
		}
	})
}

func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("could not determine home directory, marketplace features may be unavailable", "error", err)
		return os.TempDir()
	}
	return home
}

// detectProjectSkillsDir returns the project-level skills directory based on
// the current working directory. Returns empty string if detection fails.
func detectProjectSkillsDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	skillsDir := filepath.Join(cwd, ".siply", "skills")
	if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
		return skillsDir
	}
	return ""
}

// loadProviderConfig reads the provider section from ~/.siply/config.yaml.
func loadProviderConfig() core.ProviderConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return core.ProviderConfig{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".siply", "config.yaml"))
	if err != nil {
		return core.ProviderConfig{}
	}
	var cfg core.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return core.ProviderConfig{}
	}
	return cfg.Provider
}

const defaultMaxStorageMB = 100

// loadCheckpointConfig reads checkpoint config from ~/.siply/config.yaml.
func loadCheckpointConfig() core.CheckpointConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return core.CheckpointConfig{}
	}
	data, err := os.ReadFile(filepath.Join(home, ".siply", "config.yaml"))
	if err != nil {
		return core.CheckpointConfig{}
	}
	var cfg core.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return core.CheckpointConfig{}
	}
	return cfg.Checkpoint
}

// saveProfileToConfig writes the tui.profile field to ~/.siply/config.yaml.
// If the file exists, it preserves other fields.
func saveProfileToConfig(profile string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".siply")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	path := filepath.Join(dir, "config.yaml")

	// Try to read existing config to preserve other fields.
	var cfg siplyConfigData
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &cfg) // ignore error, start fresh on parse failure
	}

	cfg.TUI.Profile = profile

	// Remove known keys from Extra to prevent duplicate YAML keys on marshal.
	delete(cfg.Extra, "tui")

	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}
