// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

// Package commands provides CLI command constructors that wire together domain
// packages with Cobra. These constructors are imported by cmd/siply/main.go.
package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/tui"
	"siply.dev/siply/internal/tui/components"
)

// NewLocalIndexLoader returns a loader function that reads the marketplace index
// from cacheDir/marketplace-index.json. Pass to NewMarketplaceCmdWithLoader in
// tests to inject a loader backed by a temp directory.
func NewLocalIndexLoader(cacheDir string) func() (*marketplace.Index, error) {
	return func() (*marketplace.Index, error) {
		return marketplace.LoadIndex(filepath.Join(cacheDir, "marketplace-index.json"))
	}
}

// NewMarketplaceCmd creates the `siply marketplace` command group using the
// default cache directory (~/.siply/cache/) and wires the LocalRegistry installer.
func NewMarketplaceCmd() *cobra.Command {
	home, err := os.UserHomeDir()
	if err != nil {
		capturedErr := err
		return newMarketplaceCmdWithLoaderAndInstaller(func() (*marketplace.Index, error) {
			return nil, fmt.Errorf("marketplace: cannot determine home directory: %w", capturedErr)
		}, nil)
	}

	loader := NewLocalIndexLoader(filepath.Join(home, ".siply", "cache"))

	var installer marketplace.InstallerFunc
	registryDir := filepath.Join(home, ".siply", "plugins")
	registry := plugins.NewLocalRegistry(registryDir)
	// P1/F6: surface init errors at execute-time (not parse-time) so users only
	// see the message when they actually attempt to install something.
	if initErr := registry.Init(context.Background()); initErr == nil {
		installer = registry.Install
	} else {
		capturedInitErr := initErr
		installer = func(_ context.Context, _ string) error {
			return fmt.Errorf("Install functionality unavailable — plugins directory could not be initialized: %w", capturedInitErr)
		}
	}

	return newMarketplaceCmdWithLoaderAndInstaller(loader, installer)
}

// NewMarketplaceCmdWithLoader creates the marketplace command tree with a custom
// index loader. Intended for use in tests to inject a loader backed by a temp dir.
// The install subcommand is registered but uses a nil installer (prints advisory if invoked).
// This signature must NOT change — used by Story 9.1 integration tests.
func NewMarketplaceCmdWithLoader(loader func() (*marketplace.Index, error)) *cobra.Command {
	return newMarketplaceCmdWithLoaderAndInstaller(loader, nil)
}

// NewMarketplaceCmdWithLoaderAndInstaller creates the marketplace command tree with
// a custom index loader and installer. Used in tests to inject both dependencies.
func NewMarketplaceCmdWithLoaderAndInstaller(loader func() (*marketplace.Index, error), installer marketplace.InstallerFunc) *cobra.Command {
	return newMarketplaceCmdWithLoaderAndInstaller(loader, installer)
}

func newMarketplaceCmdWithLoaderAndInstaller(loader func() (*marketplace.Index, error), installer marketplace.InstallerFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Browse and search the Simply Devly marketplace",
	}
	cmd.AddCommand(newMarketplaceSearchCmd(loader))
	cmd.AddCommand(newMarketplaceListCmd(loader))
	cmd.AddCommand(newMarketplaceInfoCmd(loader))
	// P9: inject version getter so tests can exercise the incompatibility path.
	cmd.AddCommand(newMarketplaceInstallCmd(loader, installer, plugins.GetSiplyVersion))
	return cmd
}

// loadIndexOrAdvise loads the marketplace index using the provided loader.
// If the index is not found (file does not exist), it prints an advisory
// message and returns nil, nil — the caller treats nil as "no marketplace"
// and exits 0 (NFR27). All other errors (JSON parse errors, permission
// denied, etc.) are returned as real errors so the user sees actionable detail.
func loadIndexOrAdvise(cmd *cobra.Command, loader func() (*marketplace.Index, error)) (*marketplace.Index, error) {
	idx, err := loader()
	if err != nil {
		// P3: only suppress "file not found" — not all os.PathErrors.
		// Permission denied, corrupt files, etc. are surfaced as real errors.
		if errors.Is(err, marketplace.ErrIndexNotFound) || errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "Marketplace index unavailable. Run `siply marketplace sync` to fetch.")
			return nil, nil
		}
		return nil, fmt.Errorf("marketplace: load index: %w", err)
	}
	return idx, nil
}

// formatRating delegates to marketplace.FormatRating.
func formatRating(r float64) string {
	return marketplace.FormatRating(r)
}

// formatInstalls delegates to marketplace.FormatInstalls.
func formatInstalls(n int64) string {
	return marketplace.FormatInstalls(n)
}

// formatVerified delegates to marketplace.FormatVerified.
func formatVerified(v bool) string {
	return marketplace.FormatVerified(v)
}

// renderItemsTable writes a tabwriter-formatted table of items to cmd's output.
// Columns: NAME, CATEGORY, RATING, INSTALLS, VERIFIED, DESCRIPTION
func renderItemsTable(cmd *cobra.Command, items []marketplace.Item) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCATEGORY\tRATING\tINSTALLS\tVERIFIED\tDESCRIPTION")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			item.Name,
			item.Category,
			formatRating(item.Rating),
			formatInstalls(item.InstallCount),
			formatVerified(item.Verified),
			item.Description,
		)
	}
	return w.Flush()
}

// newMarketplaceSearchCmd creates `siply marketplace search <query>`.
func newMarketplaceSearchCmd(loader func() (*marketplace.Index, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search marketplace items by name, description, author, or tags",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeMarketplaceSearch(cmd, loader, args[0])
		},
	}
	cmd.Flags().Bool("json", false, "Output results as JSON")
	return cmd
}

func executeMarketplaceSearch(cmd *cobra.Command, loader func() (*marketplace.Index, error), query string) error {
	idx, err := loadIndexOrAdvise(cmd, loader)
	if err != nil {
		return err
	}
	if idx == nil {
		return nil // advisory already printed, exit 0
	}

	results := marketplace.Search(idx, query)

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		if results == nil {
			results = []marketplace.Item{}
		}
		return writeJSON(cmd, results)
	}

	if len(results) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No marketplace items found matching %q.\n", query)
		return nil
	}
	return renderItemsTable(cmd, results)
}

// newMarketplaceListCmd creates `siply marketplace list [--category <cat>]`.
func newMarketplaceListCmd(loader func() (*marketplace.Index, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available marketplace items",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return executeMarketplaceList(cmd, loader)
		},
	}
	cmd.Flags().String("category", "", fmt.Sprintf("Filter by category (%s)", strings.Join(marketplace.ValidCategories, ", ")))
	cmd.Flags().Bool("json", false, "Output results as JSON")
	return cmd
}

func executeMarketplaceList(cmd *cobra.Command, loader func() (*marketplace.Index, error)) error {
	idx, err := loadIndexOrAdvise(cmd, loader)
	if err != nil {
		return err
	}
	if idx == nil {
		return nil // advisory already printed, exit 0
	}

	category, _ := cmd.Flags().GetString("category")
	asJSON, _ := cmd.Flags().GetBool("json")

	var items []marketplace.Item
	if category != "" {
		filtered, filterErr := marketplace.FilterByCategory(idx, category)
		if filterErr != nil {
			return filterErr
		}
		items = filtered
	} else {
		items = idx.Items
	}

	if asJSON {
		if items == nil {
			items = []marketplace.Item{}
		}
		return writeJSON(cmd, items)
	}

	if len(items) == 0 {
		if category != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "No marketplace items found in category %q.\n", category)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "No marketplace items available.")
		}
		return nil
	}
	return renderItemsTable(cmd, items)
}

// newMarketplaceInfoCmd creates `siply marketplace info <name>`.
func newMarketplaceInfoCmd(loader func() (*marketplace.Index, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show detailed information about a marketplace item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeMarketplaceInfo(cmd, loader, args[0])
		},
	}
	cmd.Flags().Bool("json", false, "Output item as JSON")
	return cmd
}

func executeMarketplaceInfo(cmd *cobra.Command, loader func() (*marketplace.Index, error), name string) error {
	idx, err := loadIndexOrAdvise(cmd, loader)
	if err != nil {
		return err
	}
	if idx == nil {
		return nil // advisory already printed, exit 0
	}

	item, err := marketplace.FindByName(idx, name)
	if err != nil {
		// F1: forward err directly — FindByName already wraps ErrItemNotFound
		// with the item name; re-wrapping it here would duplicate sentinel info.
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		// F8: use a local copy before mutating Capabilities to avoid side effects
		// on the pointer returned by FindByName (even though it's already a copy,
		// mutating via pointer is unexpected and confusing).
		out := *item
		if out.Capabilities == nil {
			out.Capabilities = []string{}
		}
		return writeJSON(cmd, out)
	}

	return renderItemCard(cmd, *item)
}

// renderItemCard writes a tabwriter-formatted detail card for a single item.
func renderItemCard(cmd *cobra.Command, item marketplace.Item) error {
	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	// P8: always render ALL fields in spec-defined order (AC1/FR38).
	// Empty slice fields show as empty string; never omit a row.
	verifiedStr := ""
	if item.Verified {
		verifiedStr = "✓"
	}

	fmt.Fprintf(w, "Name:\t%s\n", item.Name)
	fmt.Fprintf(w, "Category:\t%s\n", item.Category)
	fmt.Fprintf(w, "Version:\t%s\n", item.Version)
	fmt.Fprintf(w, "Author:\t%s\n", item.Author)
	fmt.Fprintf(w, "License:\t%s\n", item.License)
	fmt.Fprintf(w, "Rating:\t%s\n", formatRating(item.Rating))
	fmt.Fprintf(w, "Installs:\t%s\n", formatInstalls(item.InstallCount))
	fmt.Fprintf(w, "Verified:\t%s\n", verifiedStr)
	fmt.Fprintf(w, "Tags:\t%s\n", strings.Join(item.Tags, ", "))
	fmt.Fprintf(w, "SiplyMin:\t%s\n", item.SiplyMin)
	fmt.Fprintf(w, "Homepage:\t%s\n", item.Homepage)
	fmt.Fprintf(w, "Capabilities:\t%s\n", strings.Join(item.Capabilities, ", "))
	fmt.Fprintf(w, "Updated:\t%s\n", item.UpdatedAt)
	if err := w.Flush(); err != nil {
		return err
	}

	// Render README section.
	readmeContent := item.Readme
	if strings.TrimSpace(readmeContent) == "" {
		readmeContent = item.Description
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "--- README ---")

	width := terminalWidth()
	mv := components.NewMarkdownView(tui.DefaultTheme(), tui.RenderConfig{Color: tui.ColorNone})
	rendered := mv.Render(readmeContent, width)
	fmt.Fprintln(out, rendered)

	return nil
}

// terminalWidth returns the current terminal width, falling back to 80.
func terminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w < 1 {
		return 80
	}
	return w
}

// newMarketplaceInstallCmd creates `siply marketplace install <name>`.
// versionGetter is called to obtain the running siply version for compatibility
// checks; inject a stub in tests to exercise the incompatibility path.
func newMarketplaceInstallCmd(loader func() (*marketplace.Index, error), installer marketplace.InstallerFunc, versionGetter func() string) *cobra.Command {
	return &cobra.Command{
		Use:   "install <name>",
		Short: "Install a marketplace item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeMarketplaceInstall(cmd, loader, installer, versionGetter, args[0])
		},
	}
}

func executeMarketplaceInstall(cmd *cobra.Command, loader func() (*marketplace.Index, error), installer marketplace.InstallerFunc, versionGetter func() string, name string) error {
	idx, err := loadIndexOrAdvise(cmd, loader)
	if err != nil {
		return err
	}
	if idx == nil {
		return nil // advisory already printed, exit 0
	}

	item, err := marketplace.FindByName(idx, name)
	if err != nil {
		// F1: forward err directly — FindByName already wraps ErrItemNotFound
		// with the item name; re-wrapping it here would duplicate sentinel info.
		return err
	}

	// Compatibility check (P9: versionGetter injected for testability).
	currentVer := versionGetter()
	if !plugins.IsCompatible(item.SiplyMin, currentVer) {
		return errors.New(plugins.FormatIncompatibleMessage(item.Name, item.Version, currentVer, item.SiplyMin))
	}

	if installer == nil {
		return errors.New("Install functionality unavailable — plugins directory could not be initialized.")
	}

	// P6: guard against nil context (e.g. when cmd has no context set).
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if err := marketplace.Install(ctx, *item, installer); err != nil {
		// P5: don't print advisory AND return error — Cobra would double-print.
		// Return a single well-formatted error wrapping the sentinel so callers
		// can still use errors.Is(err, marketplace.ErrNoDownloadURL).
		if errors.Is(err, marketplace.ErrNoDownloadURL) {
			return fmt.Errorf("Item %q cannot be installed — run 'siply marketplace sync' to fetch download metadata: %w", item.Name, marketplace.ErrNoDownloadURL)
		}
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✅ Installed %s v%s\n", item.Name, item.Version)
	return nil
}

// writeJSON encodes v as indented JSON to cmd's output writer.
func writeJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
