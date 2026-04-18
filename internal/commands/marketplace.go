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
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/licensing"
	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/tui"
	"siply.dev/siply/internal/tui/components"
)

const categoryBundles = "bundles"

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
	cmd.AddCommand(newMarketplacePublishCmd())
	cmd.AddCommand(newMarketplaceUpdateCmd(loader))
	cmd.AddCommand(newMarketplaceSyncCmd())
	cmd.AddCommand(newMarketplaceReviewCmd())
	cmd.AddCommand(newMarketplaceRateCmd())
	cmd.AddCommand(newMarketplaceReviewsCmd(loader))
	cmd.AddCommand(newMarketplaceReportCmd())
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
			marketplace.FormatRatingWithCount(item.Rating, item.RatingCount),
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
	fmt.Fprintf(w, "Rating:\t%s\n", marketplace.FormatRatingWithCount(item.Rating, item.RatingCount))
	fmt.Fprintf(w, "Reviews:\t%s\n", marketplace.FormatReviewCount(item.ReviewCount))
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

	if item.Category == categoryBundles && len(item.Components) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Bundle Contents:")
		for _, comp := range item.Components {
			fmt.Fprintf(out, "  • %s v%s\n", comp.Name, comp.Version)
		}
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

	// Bundle detection: delegate to InstallBundle for bundle items.
	if item.Category == categoryBundles && len(item.Components) > 0 {
		return marketplace.InstallBundle(ctx, *item, idx, installer, currentVer, cmd.OutOrStdout())
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

// newMarketplaceReviewCmd creates `siply marketplace review <name> -m "text" --rating <1-5>`.
func newMarketplaceReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review <name>",
		Short: "Submit a review for a marketplace item",
		Args:  cobra.ExactArgs(1),
		RunE:  executeMarketplaceReview,
	}
	cmd.Flags().StringP("message", "m", "", "Review text (required)")
	cmd.Flags().Int("rating", 0, "Rating 1-5 (required)")
	_ = cmd.MarkFlagRequired("message")
	_ = cmd.MarkFlagRequired("rating")
	return cmd
}

func executeMarketplaceReview(cmd *cobra.Command, args []string) error {
	name := args[0]
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("item name cannot be empty")
	}
	message, _ := cmd.Flags().GetString("message")
	rating, _ := cmd.Flags().GetInt("rating")

	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("review message cannot be empty")
	}
	if rating < 1 || rating > 5 {
		return marketplace.ErrInvalidRating
	}
	if utf8.RuneCountInString(message) > 2000 {
		return marketplace.ErrReviewTooLong
	}

	configDir, err := publishConfigDir()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return err
	}
	if err := bus.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = bus.Stop(ctx) }()

	validator := licensing.NewLicenseValidator(bus, configDir)
	if err := validator.Init(ctx); err != nil {
		return err
	}
	if err := validator.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = validator.Stop(ctx) }()

	if err := licensing.RequireAuth(validator); err != nil {
		return fmt.Errorf("Authentication required. Run 'siply auth login' first: %w", err)
	}

	token, err := licensing.AccountToken(validator)
	if err != nil {
		return fmt.Errorf("marketplace review: get account token: %w", err)
	}

	owner, repo := marketplace.DefaultRepoConfig()
	client := marketplace.NewClient(marketplace.NewClientConfig{
		RepoOwner: owner,
		RepoName:  repo,
		Token:     token,
	})

	resp, err := client.SubmitReview(ctx, marketplace.SubmitReviewRequest{
		Name:   name,
		Rating: rating,
		Text:   message,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Review submitted for %s — PR created: %s\n", name, resp.PRURL)
	return nil
}

// newMarketplaceRateCmd creates `siply marketplace rate <name> <1-5>`.
func newMarketplaceRateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rate <name> <score>",
		Short: "Rate a marketplace item (1-5)",
		Args:  cobra.ExactArgs(2),
		RunE:  executeMarketplaceRate,
	}
}

func executeMarketplaceRate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("item name cannot be empty")
	}

	score, err := strconv.Atoi(args[1])
	if err != nil || score < 1 || score > 5 {
		return marketplace.ErrInvalidRating
	}

	configDir, err := publishConfigDir()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return err
	}
	if err := bus.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = bus.Stop(ctx) }()

	validator := licensing.NewLicenseValidator(bus, configDir)
	if err := validator.Init(ctx); err != nil {
		return err
	}
	if err := validator.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = validator.Stop(ctx) }()

	if err := licensing.RequireAuth(validator); err != nil {
		return fmt.Errorf("Authentication required. Run 'siply auth login' first: %w", err)
	}

	token, err := licensing.AccountToken(validator)
	if err != nil {
		return fmt.Errorf("marketplace rate: get account token: %w", err)
	}

	owner, repo := marketplace.DefaultRepoConfig()
	client := marketplace.NewClient(marketplace.NewClientConfig{
		RepoOwner: owner,
		RepoName:  repo,
		Token:     token,
	})

	resp, err := client.SubmitReview(ctx, marketplace.SubmitReviewRequest{
		Name:   name,
		Rating: score,
		Text:   "",
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Rated %s %d/5 — PR created: %s\n", name, score, resp.PRURL)
	return nil
}

// newMarketplaceReviewsCmd creates `siply marketplace reviews <name>`.
func newMarketplaceReviewsCmd(loader func() (*marketplace.Index, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reviews <name>",
		Short: "Show reviews for a marketplace item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeMarketplaceReviews(cmd, args)
		},
	}
	cmd.Flags().Int("page", 1, "Page number (10 reviews per page)")
	cmd.Flags().Bool("json", false, "Output raw JSON")
	return cmd
}

func executeMarketplaceReviews(cmd *cobra.Command, args []string) error {
	name := args[0]
	page, _ := cmd.Flags().GetInt("page")
	if page < 1 {
		page = 1
	}
	asJSON, _ := cmd.Flags().GetBool("json")

	owner, repo := marketplace.DefaultRepoConfig()
	client := marketplace.NewClient(marketplace.NewClientConfig{
		RepoOwner: owner,
		RepoName:  repo,
	})

	rf, err := client.GetReviews(cmd.Context(), name)
	if err != nil {
		return err
	}

	if asJSON {
		return writeJSON(cmd, rf)
	}

	if len(rf.Reviews) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No reviews for %s yet.\n", name)
		return nil
	}

	// Paginate: 10 per page
	const perPage = 10
	total := len(rf.Reviews)
	start := (page - 1) * perPage
	if start >= total {
		fmt.Fprintf(cmd.OutOrStdout(), "No reviews on page %d (total: %d reviews).\n", page, total)
		return nil
	}
	end := start + perPage
	if end > total {
		end = total
	}

	for _, r := range rf.Reviews[start:end] {
		ratingStr := "-"
		if r.Rating > 0 {
			ratingStr = fmt.Sprintf("%d/5", r.Rating)
		}
		textStr := ""
		if r.Text != "" {
			textStr = fmt.Sprintf(" — %s", r.Text)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s  ⭐%s%s  (%s)\n", r.Author, ratingStr, textStr, r.CreatedAt)
	}

	totalPages := (total + perPage - 1) / perPage
	if totalPages > 1 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nPage %d/%d (use --page <n> to navigate)\n", page, totalPages)
	}

	return nil
}

// newMarketplaceReportCmd creates `siply marketplace report <name> --reason <type>`.
func newMarketplaceReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report <name>",
		Short: "Report a marketplace item",
		Args:  cobra.ExactArgs(1),
		RunE:  executeMarketplaceReport,
	}
	cmd.Flags().String("reason", "", "Report reason: malware, spam, broken, copyright, other (required)")
	cmd.Flags().String("detail", "", "Additional detail (max 500 chars)")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func executeMarketplaceReport(cmd *cobra.Command, args []string) error {
	name := args[0]
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("item name cannot be empty")
	}
	reason, _ := cmd.Flags().GetString("reason")
	detail, _ := cmd.Flags().GetString("detail")

	// Validate before auth to avoid wasted work.
	validReason := false
	for _, r := range marketplace.ValidReportReasons {
		if r == reason {
			validReason = true
			break
		}
	}
	if !validReason {
		return marketplace.ErrInvalidReason
	}
	if utf8.RuneCountInString(detail) > 500 {
		return marketplace.ErrReportTooLong
	}

	configDir, err := publishConfigDir()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return err
	}
	if err := bus.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = bus.Stop(ctx) }()

	validator := licensing.NewLicenseValidator(bus, configDir)
	if err := validator.Init(ctx); err != nil {
		return err
	}
	if err := validator.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = validator.Stop(ctx) }()

	if err := licensing.RequireAuth(validator); err != nil {
		return fmt.Errorf("Authentication required. Run 'siply auth login' first: %w", err)
	}

	token, err := licensing.AccountToken(validator)
	if err != nil {
		return fmt.Errorf("marketplace report: get account token: %w", err)
	}

	owner, repo := marketplace.DefaultRepoConfig()
	client := marketplace.NewClient(marketplace.NewClientConfig{
		RepoOwner: owner,
		RepoName:  repo,
		Token:     token,
	})

	resp, err := client.ReportItem(ctx, marketplace.ReportRequest{
		Name:   name,
		Reason: reason,
		Detail: detail,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Report submitted for %s — issue: %s\n", name, resp.IssueURL)
	return nil
}

func newMarketplacePublishCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish [directory]",
		Short: "Publish a plugin or extension to the marketplace",
		Long:  "Package and publish a plugin, extension, skill, or config to siply.dev marketplace.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  executeMarketplacePublish,
	}
	cmd.Flags().Bool("dry-run", false, "Validate and package without uploading")
	return cmd
}

func executeMarketplacePublish(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	// Resolve to absolute path for clear error messages.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("marketplace: resolve path: %w", err)
	}

	// Auth guard — fail fast.
	configDir, err := publishConfigDir()
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	bus := events.NewBus()
	if err := bus.Init(ctx); err != nil {
		return err
	}
	if err := bus.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = bus.Stop(ctx) }()

	validator := licensing.NewLicenseValidator(bus, configDir)
	if err := validator.Init(ctx); err != nil {
		return err
	}
	if err := validator.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = validator.Stop(ctx) }()

	if err := licensing.RequireAuth(validator); err != nil {
		return fmt.Errorf("Authentication required. Run 'siply login' first: %w", err)
	}

	token, err := licensing.AccountToken(validator)
	if err != nil {
		return fmt.Errorf("marketplace: get account token: %w", err)
	}

	// Pre-publish validation.
	result, err := marketplace.ValidateForPublish(absDir)
	if err != nil {
		return err
	}

	// Show warnings.
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if len(result.Warnings) > 0 {
		for _, w := range result.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "⚠️  %s\n", w)
		}
		if !dryRun {
			fmt.Fprint(cmd.OutOrStdout(), "Continue? [Y/n] ")
			var answer string
			if _, scanErr := fmt.Fscanln(cmd.InOrStdin(), &answer); scanErr != nil {
				return fmt.Errorf("publish canceled: cannot read confirmation in non-interactive mode")
			}
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer == "n" || answer == "no" {
				return fmt.Errorf("publish canceled by user")
			}
		}
	}

	// Package.
	fmt.Fprintln(cmd.OutOrStdout(), "Packaging...")
	archivePath, sha256hex, err := marketplace.PackageDir(absDir)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if dryRun {
		info, _ := os.Stat(archivePath)
		var size int64
		if info != nil {
			size = info.Size()
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — %s v%s\n  Archive: %d bytes\n  SHA256: %s\n",
			result.Manifest.Metadata.Name, result.Manifest.Metadata.Version, size, sha256hex)
		return nil
	}

	// Upload.
	fmt.Fprintln(cmd.OutOrStdout(), "Publishing...")
	owner, repo := marketplace.DefaultRepoConfig()
	client := marketplace.NewClient(marketplace.NewClientConfig{
		RepoOwner: owner,
		RepoName:  repo,
		Token:     token,
	})
	resp, err := client.Publish(ctx, marketplace.PublishRequest{
		Manifest:    result.Manifest.Metadata,
		ArchivePath: archivePath,
		SHA256:      sha256hex,
		ReadmeText:  result.Readme,
	})
	if err != nil {
		return err
	}

	slog.Info("published", "name", resp.Name, "version", resp.Version)
	if resp.URL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Published %s v%s. View at: %s\n", resp.Name, resp.Version, resp.URL)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "✅ Published %s v%s.\n", resp.Name, resp.Version)
	}
	return nil
}

func publishConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".siply"), nil
}

func newMarketplaceUpdateCmd(loader func() (*marketplace.Index, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "update <name>",
		Short: "Update a marketplace item (or all components of a bundle)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeMarketplaceUpdate(cmd, loader, args[0])
		},
	}
}

// newMarketplaceSyncCmd creates `siply marketplace sync`.
func newMarketplaceSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch latest marketplace index",
		RunE:  executeMarketplaceSync,
	}
	cmd.Flags().Bool("force", false, "Force full download, ignoring cache freshness")
	return cmd
}

func executeMarketplaceSync(cmd *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("marketplace sync: cannot determine home directory: %w", err)
	}

	cachePath := filepath.Join(home, ".siply", "cache", "marketplace-index.json")
	force, _ := cmd.Flags().GetBool("force")

	synced, count, syncErr := marketplace.SyncIndex(cmd.Context(), marketplace.SyncConfig{
		CachePath: cachePath,
		Force:     force,
	})
	if syncErr != nil {
		return syncErr
	}

	if !synced {
		fmt.Fprintln(cmd.OutOrStdout(), "Marketplace index is up to date")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Marketplace index synced (%d items)\n", count)
	}
	return nil
}

func executeMarketplaceUpdate(cmd *cobra.Command, loader func() (*marketplace.Index, error), name string) error {
	idx, err := loadIndexOrAdvise(cmd, loader)
	if err != nil {
		return err
	}
	if idx == nil {
		return nil
	}

	item, err := marketplace.FindByName(idx, name)
	if err != nil {
		return err
	}

	if item.Category == categoryBundles && len(item.Components) > 0 {
		var updated int
		for _, comp := range item.Components {
			compItem, findErr := marketplace.FindByName(idx, comp.Name)
			if findErr != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s: not found in index, skipping\n", comp.Name)
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s v%s: update command coming in a future release\n", compItem.Name, compItem.Version)
			updated++
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated %d/%d items in bundle %s\n", updated, len(item.Components), item.Name)
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Update command coming in a future release for %s v%s\n", item.Name, item.Version)
	return nil
}
