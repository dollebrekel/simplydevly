// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/spf13/cobra"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/extensions"
	"siply.dev/siply/internal/plugins"
)

func newDevCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Extension development tools",
	}

	cmd.AddCommand(newDevInitCmd())
	cmd.AddCommand(newDevWatchCmd())

	return cmd
}

func newDevInitCmd() *cobra.Command {
	var extensionName string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new extension project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if extensionName == "" {
				return fmt.Errorf("--extension flag is required")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("dev init: get working directory: %w", err)
			}

			dir, err := extensions.ScaffoldExtension(cwd, extensionName)
			if err != nil {
				return err
			}

			fmt.Printf("Extension scaffolded at %s\n", dir)
			fmt.Println("Next steps:")
			fmt.Printf("  cd %s\n", extensionName)
			fmt.Println("  siply dev watch --plugin .")
			return nil
		},
	}

	cmd.Flags().StringVar(&extensionName, "extension", "", "Extension name (required)")

	return cmd
}

func newDevWatchCmd() *cobra.Command {
	var pluginPath string

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch a plugin directory for changes and hot-reload",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pluginPath == "" {
				return fmt.Errorf("--plugin flag is required")
			}

			absPath, err := filepath.Abs(pluginPath)
			if err != nil {
				return fmt.Errorf("dev watch: resolve path: %w", err)
			}

			info, err := os.Stat(absPath)
			if err != nil {
				return fmt.Errorf("dev watch: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("dev watch: %s is not a directory", absPath)
			}

			pluginName := filepath.Base(absPath)

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer cancel()

			pluginsDir := filepath.Join(homeDir(), ".siply", "plugins")
			registry := plugins.NewLocalRegistry(pluginsDir)
			if err := registry.Init(ctx); err != nil {
				return fmt.Errorf("dev watch: init registry: %w", err)
			}

			bus := events.NewBus()
			if err := bus.Init(ctx); err != nil {
				return fmt.Errorf("dev watch: init eventbus: %w", err)
			}
			if err := bus.Start(ctx); err != nil {
				return fmt.Errorf("dev watch: start eventbus: %w", err)
			}
			defer func() { _ = bus.Stop(ctx) }()

			if err := registry.DevMode(ctx, absPath); err != nil {
				slog.Warn("dev watch: dev mode setup", "error", err)
			}

			watcher := plugins.NewDevWatcher(absPath, pluginName, registry, bus, nil)
			if err := watcher.Start(ctx); err != nil {
				return fmt.Errorf("dev watch: start watcher: %w", err)
			}
			defer func() { _ = watcher.Stop() }()

			fmt.Printf("Watching %s for changes (Ctrl+C to stop)\n", absPath)
			<-ctx.Done()
			fmt.Println("\nStopping watcher...")
			return nil
		},
	}

	cmd.Flags().StringVar(&pluginPath, "plugin", "", "Path to plugin directory (required)")

	return cmd
}
