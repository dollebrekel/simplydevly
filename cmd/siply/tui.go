// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
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
						flags.ConfigProfile = "standard"
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
			return "minimal", nil
		case "2":
			return "standard", nil
		default:
			return "standard", nil // safe default
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return "standard", nil // EOF → safe default
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
	case "minimal", "standard", "":
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

	// Wire status bar.
	sb := statusline.NewStatusBar(theme, rc, rc.Profile)
	app.SetStatusBar(sb)

	return tui.RunApp(app, caps)
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
