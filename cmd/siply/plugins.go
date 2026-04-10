// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"siply.dev/siply/internal/config"
	"siply.dev/siply/internal/plugins"
)

// pluginsRegistryDir returns the path to the local plugin registry directory.
func pluginsRegistryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("plugins: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".siply", "plugins"), nil
}

// newPluginsCmd creates the root `siply plugins` command group.
func newPluginsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "Manage plugins",
	}
	cmd.AddCommand(newPluginsInstallCmd())
	cmd.AddCommand(newPluginsListCmd())
	cmd.AddCommand(newPluginsRemoveCmd())
	return cmd
}

// newPluginsInstallCmd creates `siply plugins install <source>`.
func newPluginsInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <source>",
		Short: "Install a plugin from a local directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executePluginsInstall(cmd, args[0])
		},
	}
}

func executePluginsInstall(cmd *cobra.Command, source string) error {
	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("plugins: registry init: %w", err)
	}

	// Install the plugin from the local source directory.
	if err := registry.Install(ctx, source); err != nil {
		return fmt.Errorf("plugins: install: %w", err)
	}

	// Determine the plugin name from the source manifest (same logic as Install).
	m, err := plugins.LoadManifestFromDir(source)
	if err != nil {
		return fmt.Errorf("plugins: read source manifest: %w", err)
	}
	installedName := m.Metadata.Name
	installedVersion := m.Metadata.Version
	installedTier := m.Spec.Tier

	if installedTier == 1 {
		// Auto-load Tier 1 plugin to validate and merge its config.
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("plugins: cannot determine home directory: %w", err)
		}
		siplyDir := filepath.Join(home, ".siply")

		loader := config.NewLoader(config.LoaderOptions{
			GlobalDir: siplyDir,
		})
		if err := loader.Init(ctx); err != nil {
			return fmt.Errorf("plugins: config init: %w", err)
		}
		defer loader.Stop(ctx) //nolint:errcheck

		merger := config.NewPluginConfigMerger(loader)
		tier1Loader := plugins.NewTier1Loader(registry, merger)

		if err := tier1Loader.Load(ctx, installedName); err != nil {
			return fmt.Errorf("plugins: auto-load after install: %w", err)
		}
		fmt.Printf("✓ Installed and loaded: %s v%s (tier %d)\n", installedName, installedVersion, installedTier)
	} else if installedTier == 3 {
		// Tier 3: validate binary exists and is executable but do NOT start — lazy loading applies (P7).
		pluginDir := filepath.Join(registryDir, installedName)
		binName := installedName
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		binPath := filepath.Join(pluginDir, binName)
		info, statErr := os.Stat(binPath)
		if statErr != nil {
			fmt.Printf("✓ Installed: %s v%s (tier %d) — warning: binary not found at %s\n", installedName, installedVersion, installedTier, binPath)
		} else if runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
			fmt.Printf("✓ Installed: %s v%s (tier %d) — warning: binary not executable at %s\n", installedName, installedVersion, installedTier, binPath)
		} else {
			fmt.Printf("✓ Installed: %s v%s (tier %d, lazy-loaded)\n", installedName, installedVersion, installedTier)
		}
	} else {
		fmt.Printf("✓ Installed: %s v%s (tier %d)\n", installedName, installedVersion, installedTier)
	}

	return nil
}

// newPluginsListCmd creates `siply plugins list`.
func newPluginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed plugins",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executePluginsList(cmd)
		},
	}
}

func executePluginsList(cmd *cobra.Command) error {
	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("plugins: registry init: %w", err)
	}

	metas, err := registry.List(ctx)
	if err != nil {
		return fmt.Errorf("plugins: list: %w", err)
	}

	if len(metas) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tTIER\tSTATUS")
	for _, meta := range metas {
		status := "installed"
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", meta.Name, meta.Version, meta.Tier, status)
	}
	return w.Flush()
}

// newPluginsRemoveCmd creates `siply plugins remove <name>`.
func newPluginsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executePluginsRemove(cmd, args[0])
		},
	}
}

func executePluginsRemove(cmd *cobra.Command, name string) error {
	// Reject path traversal attempts in plugin name.
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("plugins: invalid plugin name %q: path separators and '..' not allowed", name)
	}

	ctx := cmd.Context()

	registryDir, err := pluginsRegistryDir()
	if err != nil {
		return err
	}

	registry := plugins.NewLocalRegistry(registryDir)
	if err := registry.Init(ctx); err != nil {
		return fmt.Errorf("plugins: registry init: %w", err)
	}

	// Check if the plugin is Tier 1 and currently installed.
	metas, err := registry.List(ctx)
	if err != nil {
		return fmt.Errorf("plugins: list before remove: %w", err)
	}

	var tier int
	for _, meta := range metas {
		if meta.Name == name {
			tier = meta.Tier
			break
		}
	}

	// For Tier 1 plugins: unload config contribution before removing.
	if tier == 1 {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("plugins: cannot determine home directory: %w", err)
		}
		siplyDir := filepath.Join(home, ".siply")

		loader := config.NewLoader(config.LoaderOptions{
			GlobalDir: siplyDir,
		})
		if err := loader.Init(ctx); err != nil {
			return fmt.Errorf("plugins: config init: %w", err)
		}
		defer loader.Stop(ctx) //nolint:errcheck

		merger := config.NewPluginConfigMerger(loader)

		// Remove plugin config contribution from in-memory config.
		// In a CLI context this is ephemeral — config is not persisted to disk.
		// A subsequent `siply lock` regenerates the lockfile without this plugin.
		if err := merger.RemovePluginConfig(name); err != nil {
			return fmt.Errorf("plugins: remove: clear config %s: %w", name, err)
		}
	}

	if err := registry.Remove(ctx, name); err != nil {
		return fmt.Errorf("plugins: remove: %w", err)
	}

	fmt.Printf("✓ Removed: %s\n", name)
	return nil
}
