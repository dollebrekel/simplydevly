// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
	"siply.dev/siply/internal/commands"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/extensions"
	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/providers"
	"siply.dev/siply/internal/skills"
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

			// Offline mode: verify Ollama is reachable before launching TUI.
			if flags.Offline {
				if err := checkOllamaReachable(); err != nil {
					return err
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

	// Check --offline flag and SIPLY_OFFLINE env var.
	flags.Offline, err = cmd.Flags().GetBool("offline")
	if err != nil {
		return flags, err
	}
	if !flags.Offline && providers.IsOfflineEnv() {
		flags.Offline = true
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
	if flags.Offline {
		sb.SetOffline(flags.ModelOverride)
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
	hostServer := plugins.NewHostServer(plugins.HostServerOptions{
		EventBus: bus,
	})
	if err := hostServer.Start(context.Background()); err != nil {
		slog.Warn("tui: host server start failed, Tier 3 plugins unavailable", "error", err)
	} else {
		defer func() { _ = hostServer.Stop(context.Background()) }()

		tier3Loader := plugins.NewTier3Loader(registry, hostServer)

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
		}
	}

	return tui.RunApp(app, caps, func(prog *tea.Program) {
		bridgeEventBus(bus, prog)
	})
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

// checkOllamaReachable does a quick HTTP health check against the local Ollama instance.
func checkOllamaReachable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "http://localhost:11434/api/tags", nil)
	if err != nil {
		return fmt.Errorf("Offline mode requires a running Ollama instance. Start with: ollama serve")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("Offline mode requires a running Ollama instance. Start with: ollama serve")
	}
	resp.Body.Close()
	return nil
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
