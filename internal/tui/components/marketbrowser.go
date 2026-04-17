// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"siply.dev/siply/internal/marketplace"
	"siply.dev/siply/internal/plugins"
	"siply.dev/siply/internal/tui"
)

// Compile-time interface check.
var _ tui.MarketplaceBrowser = (*MarketBrowser)(nil)

type browserState int

const (
	stateList browserState = iota
	stateInfo
	stateRate
)

// MarketBrowser is the TUI marketplace browser component.
type MarketBrowser struct {
	index        *marketplace.Index
	filtered     []marketplace.Item
	cursor       int
	searchInput  textinput.Model
	viewport     viewport.Model
	markdownView *MarkdownView
	theme        tui.Theme
	renderConfig tui.RenderConfig
	state        browserState
	installer    marketplace.InstallerFunc
	installMsg   string
	installing   bool
	ratingInput  textinput.Model
	infoContent  string
	width        int
	height       int
	open         bool
}

// NewMarketBrowser creates a marketplace browser component.
func NewMarketBrowser(theme tui.Theme, config tui.RenderConfig, loader func() (*marketplace.Index, error), installer marketplace.InstallerFunc) *MarketBrowser {
	ti := textinput.New()
	ti.Prompt = "🔍 "
	ti.Placeholder = "Search marketplace..."
	ti.Focus()

	vp := viewport.New()

	mv := NewMarkdownView(theme, config)

	ri := textinput.New()
	ri.Prompt = "Rating (1-5): "
	ri.CharLimit = 1

	var idx *marketplace.Index
	if loader != nil {
		idx, _ = loader()
	}

	var filtered []marketplace.Item
	if idx != nil {
		filtered = marketplace.Search(idx, "")
	}

	return &MarketBrowser{
		index:        idx,
		filtered:     filtered,
		searchInput:  ti,
		viewport:     vp,
		markdownView: mv,
		ratingInput:  ri,
		theme:        theme,
		renderConfig: config,
		installer:    installer,
	}
}

// Init returns the initial command (cursor blink for search input).
func (mb *MarketBrowser) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles incoming messages and updates state.
func (mb *MarketBrowser) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tui.MarketplaceInstallResultMsg:
		mb.installing = false
		if msg.Err != nil {
			mb.installMsg = fmt.Sprintf("❌ Install failed: %s", msg.Err)
		} else {
			mb.installMsg = fmt.Sprintf("✅ Installed %s v%s", msg.Name, msg.Version)
		}
		return nil

	case tui.MarketplaceRateResultMsg:
		if msg.Err != nil {
			mb.installMsg = fmt.Sprintf("❌ Rating failed: %s", msg.Err)
		} else {
			mb.installMsg = fmt.Sprintf("⭐ Rated %s %d/5. Average: %.1f (%d ratings)", msg.Name, msg.Score, msg.AverageRating, msg.TotalRatings)
		}
		mb.state = stateList
		return nil

	case tui.MarketplaceReviewsResultMsg:
		if item := mb.selectedItem(); item != nil && item.Name == msg.ItemName && msg.Content != "" {
			updated := mb.infoContent + "\n" + mb.markdownView.Render(msg.Content, max(mb.width-4, 0))
			mb.viewport.SetContent(updated)
		}
		return nil

	case tea.WindowSizeMsg:
		mb.width = msg.Width
		mb.height = msg.Height
		return nil

	case tea.KeyPressMsg:
		return mb.handleKey(msg)

	case tea.MouseMsg:
		return mb.handleMouse(msg)
	}

	return nil
}

func (mb *MarketBrowser) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()

	if mb.state == stateRate {
		return mb.handleRateKey(key, msg)
	}
	if mb.state == stateInfo {
		return mb.handleInfoKey(key, msg)
	}
	return mb.handleListKey(key, msg)
}

func (mb *MarketBrowser) handleListKey(key string, msg tea.KeyPressMsg) tea.Cmd {
	switch key {
	case "esc":
		return func() tea.Msg { return tui.MarketplaceCloseMsg{} }
	case "up", "k":
		if mb.cursor > 0 {
			mb.cursor--
		}
		mb.installMsg = ""
		return nil
	case "down", "j":
		if mb.cursor < len(mb.filtered)-1 {
			mb.cursor++
		}
		mb.installMsg = ""
		return nil
	case "enter":
		return mb.installItem()
	case "i":
		if item := mb.selectedItem(); item != nil {
			mb.state = stateInfo
			mb.installMsg = ""
			return mb.populateInfoViewport(item)
		}
		return nil
	case "r":
		if mb.selectedItem() != nil {
			mb.state = stateRate
			mb.ratingInput.SetValue("")
			mb.ratingInput.Focus()
			mb.installMsg = ""
		}
		return nil
	case "w":
		if item := mb.selectedItem(); item != nil {
			if item.Homepage == "" {
				mb.installMsg = "No web URL available for this item"
			} else {
				_ = openBrowser(item.Homepage)
			}
		}
		return nil
	default:
		prevValue := mb.searchInput.Value()
		mb.searchInput, _ = mb.searchInput.Update(msg)
		if mb.searchInput.Value() != prevValue {
			mb.refilter()
		}
		return nil
	}
}

func (mb *MarketBrowser) handleRateKey(key string, _ tea.KeyPressMsg) tea.Cmd {
	switch key {
	case "esc":
		mb.state = stateList
		mb.installMsg = ""
		return nil
	case "enter":
		val := mb.ratingInput.Value()
		if len(val) != 1 || val[0] < '1' || val[0] > '5' {
			mb.installMsg = "Rating must be between 1 and 5"
			return nil
		}
		score := int(val[0] - '0')
		item := mb.selectedItem()
		if item == nil {
			return nil
		}
		mb.installMsg = fmt.Sprintf("Use CLI to rate: siply marketplace rate %s %d", item.Name, score)
		mb.state = stateList
		return nil
	default:
		mb.ratingInput, _ = mb.ratingInput.Update(tea.KeyPressMsg{})
		return nil
	}
}

func (mb *MarketBrowser) handleInfoKey(key string, msg tea.KeyPressMsg) tea.Cmd {
	switch key {
	case "esc":
		mb.state = stateList
		mb.installMsg = ""
		return nil
	case "enter":
		return mb.installItem()
	case "r":
		if mb.selectedItem() != nil {
			mb.state = stateRate
			mb.ratingInput.SetValue("")
			mb.ratingInput.Focus()
			mb.installMsg = ""
		}
		return nil
	case "w":
		if item := mb.selectedItem(); item != nil {
			if item.Homepage == "" {
				mb.installMsg = "No web URL available for this item"
			} else {
				_ = openBrowser(item.Homepage)
			}
		}
		return nil
	case "up", "k", "down", "j", "pgup", "pgdown":
		mb.viewport, _ = mb.viewport.Update(msg)
		return nil
	}
	return nil
}

func (mb *MarketBrowser) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if mb.state != stateList || len(mb.filtered) == 0 {
		return nil
	}
	if press, ok := msg.(tea.MouseClickMsg); ok {
		// Row 0 is search box; visible items start at row 1.
		// Account for scroll offset: the first visible item is at startIdx.
		visibleRow := press.Y - 1
		startIdx := mb.scrollStartIdx()
		idx := startIdx + visibleRow
		if idx >= 0 && idx < len(mb.filtered) {
			mb.cursor = idx
			mb.installMsg = ""
		}
	}
	return nil
}

// View renders the marketplace browser.
func (mb *MarketBrowser) View() string {
	if mb.index == nil {
		return mb.renderEmptyState()
	}

	switch mb.state {
	case stateInfo:
		return mb.renderInfoPanel()
	case stateRate:
		return mb.renderRateView()
	default:
		return mb.renderListView()
	}
}

func (mb *MarketBrowser) renderRateView() string {
	item := mb.selectedItem()
	if item == nil {
		return ""
	}

	cs := mb.renderConfig.Color
	var b strings.Builder
	title := fmt.Sprintf("Rate %s", item.Name)
	b.WriteString(mb.theme.Heading.Resolve(cs).Render(title))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(mb.ratingInput.View())
	b.WriteByte('\n')
	if mb.installMsg != "" {
		b.WriteString(mb.installMsg)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	keyStyle := mb.theme.Keybind.Resolve(cs)
	descStyle := mb.theme.TextMuted.Resolve(cs)
	b.WriteString(keyStyle.Render("Enter") + descStyle.Render(" Submit  ") +
		keyStyle.Render("Esc") + descStyle.Render(" Cancel"))
	return b.String()
}

func (mb *MarketBrowser) renderEmptyState() string {
	msg := tui.EmptyStateMsg{
		Reason:     "No marketplace data available.",
		Suggestion: "siply marketplace sync",
	}
	return RenderEmptyState(msg, &mb.theme, &mb.renderConfig, mb.width)
}

func (mb *MarketBrowser) renderListView() string {
	cs := mb.renderConfig.Color
	var b strings.Builder

	// Search box
	searchBox := mb.searchInput.View()
	b.WriteString(searchBox)
	b.WriteByte('\n')

	// Available height for list + summary + action bar
	listHeight := max(mb.height-4, 1) // search box + summary area + action bar

	if len(mb.filtered) == 0 {
		noResults := mb.theme.TextMuted.Resolve(cs).Render("No items match your search")
		b.WriteString(noResults)
		b.WriteByte('\n')
	} else {
		// Calculate visible window
		summaryHeight := 6
		actionBarHeight := 2
		visibleItems := min(max(listHeight-summaryHeight-actionBarHeight, 3), len(mb.filtered))

		// Scroll offset
		startIdx := 0
		if mb.cursor >= visibleItems {
			startIdx = mb.cursor - visibleItems + 1
		}
		endIdx := startIdx + visibleItems
		if endIdx > len(mb.filtered) {
			endIdx = len(mb.filtered)
			startIdx = max(endIdx-visibleItems, 0)
		}

		// Render list items
		for i := startIdx; i < endIdx; i++ {
			item := mb.filtered[i]
			row := mb.renderItemRow(item, i == mb.cursor)
			b.WriteString(row)
			b.WriteByte('\n')
		}

		// Summary card for selected item
		if item := mb.selectedItem(); item != nil {
			b.WriteByte('\n')
			b.WriteString(mb.renderSummaryCard(item))
		}
	}

	// Install feedback
	if mb.installMsg != "" {
		b.WriteByte('\n')
		b.WriteString(mb.installMsg)
	}

	// Action bar
	b.WriteByte('\n')
	b.WriteString(mb.renderActionBar(false))

	return b.String()
}

func (mb *MarketBrowser) renderItemRow(item marketplace.Item, selected bool) string {
	cs := mb.renderConfig.Color

	name := item.Name
	rating := marketplace.FormatRatingWithCount(item.Rating, item.RatingCount)
	installs := marketplace.FormatInstalls(item.InstallCount)
	verified := ""
	if item.Verified {
		verified = mb.theme.Success.Resolve(cs).Render(" ✓")
	}

	stats := fmt.Sprintf("  %s  %s", rating, installs)

	if selected {
		nameStyle := mb.theme.Primary.Resolve(cs).Bold(true)
		row := nameStyle.Render(name) + mb.theme.Text.Resolve(cs).Render(stats) + verified
		return ansi.Truncate(row, mb.width, "…")
	}

	nameStyle := mb.theme.Text.Resolve(cs)
	statsStyle := mb.theme.TextMuted.Resolve(cs)
	row := nameStyle.Render(name) + statsStyle.Render(stats) + verified
	return ansi.Truncate(row, mb.width, "…")
}

func (mb *MarketBrowser) renderSummaryCard(item *marketplace.Item) string {
	cs := mb.renderConfig.Color
	mutedStyle := mb.theme.TextMuted.Resolve(cs)

	desc := item.Description
	if utf8.RuneCountInString(desc) > 100 {
		desc = string([]rune(desc)[:97]) + "..."
	}

	lines := []string{
		desc,
		mutedStyle.Render(fmt.Sprintf("Author: %s  License: %s  v%s  Updated: %s",
			item.Author, item.License, item.Version, item.UpdatedAt)),
	}

	content := strings.Join(lines, "\n")
	return tui.RenderBorder(item.Name, content, mb.renderConfig, mb.theme, mb.width)
}

func (mb *MarketBrowser) renderInfoPanel() string {
	cs := mb.renderConfig.Color
	item := mb.selectedItem()
	if item == nil {
		return ""
	}

	var b strings.Builder

	// Viewport content is set once in populateInfoViewport; only adjust size here.
	vpHeight := max(mb.height-6, 3)
	mb.viewport.SetHeight(vpHeight)
	mb.viewport.SetWidth(mb.width)

	title := fmt.Sprintf("%s — Info", item.Name)
	b.WriteString(mb.theme.Heading.Resolve(cs).Render(title))
	b.WriteByte('\n')
	b.WriteString(mb.viewport.View())
	b.WriteByte('\n')

	// Trust signals
	trustLine := mb.theme.TextMuted.Resolve(cs).Render(
		fmt.Sprintf("%s  %s  Installs: %s  Author: %s  License: %s",
			marketplace.FormatRatingWithCount(item.Rating, item.RatingCount),
			marketplace.FormatReviewCount(item.ReviewCount),
			marketplace.FormatInstalls(item.InstallCount),
			item.Author,
			item.License))
	b.WriteString(trustLine)
	b.WriteByte('\n')

	// Install feedback
	if mb.installMsg != "" {
		b.WriteString(mb.installMsg)
		b.WriteByte('\n')
	}

	// Action bar
	b.WriteString(mb.renderActionBar(true))

	return b.String()
}

func (mb *MarketBrowser) renderActionBar(infoMode bool) string {
	cs := mb.renderConfig.Color
	keyStyle := mb.theme.Keybind.Resolve(cs)
	descStyle := mb.theme.TextMuted.Resolve(cs)

	if infoMode {
		return keyStyle.Render("Enter") + descStyle.Render(" Install  ") +
			keyStyle.Render("r") + descStyle.Render(" Rate  ") +
			keyStyle.Render("w") + descStyle.Render(" Web  ") +
			keyStyle.Render("Esc") + descStyle.Render(" Back")
	}

	return keyStyle.Render("Enter") + descStyle.Render(" Install  ") +
		keyStyle.Render("i") + descStyle.Render(" Info  ") +
		keyStyle.Render("r") + descStyle.Render(" Rate  ") +
		keyStyle.Render("w") + descStyle.Render(" Web  ") +
		keyStyle.Render("Esc") + descStyle.Render(" Close")
}

// installItem starts async install of the selected item.
// TODO(9.5): Check RequiresAuth on item before install
func (mb *MarketBrowser) installItem() tea.Cmd {
	item := mb.selectedItem()
	if item == nil {
		return nil
	}

	if mb.installing {
		return nil
	}

	if mb.installer == nil {
		mb.installMsg = "❌ Install functionality unavailable"
		return nil
	}

	currentVer := plugins.GetSiplyVersion()
	if !plugins.IsCompatible(item.SiplyMin, currentVer) {
		mb.installMsg = "❌ " + plugins.FormatIncompatibleMessage(item.Name, item.Version, currentVer, item.SiplyMin)
		return nil
	}

	if item.Category == "bundles" && len(item.Components) > 0 {
		mb.installMsg = fmt.Sprintf("Use CLI to install bundles: siply marketplace install %s", item.Name)
		return nil
	}

	mb.installing = true
	mb.installMsg = fmt.Sprintf("⏳ Installing %s...", item.Name)
	capturedItem := *item
	installer := mb.installer
	return func() tea.Msg {
		err := marketplace.Install(context.Background(), capturedItem, installer)
		return tui.MarketplaceInstallResultMsg{
			Name:    capturedItem.Name,
			Version: capturedItem.Version,
			Err:     err,
		}
	}
}

// openBrowser opens a URL in the default browser.
func openBrowser(url string) error {
	if url == "" {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (mb *MarketBrowser) selectedItem() *marketplace.Item {
	if len(mb.filtered) == 0 || mb.cursor < 0 || mb.cursor >= len(mb.filtered) {
		return nil
	}
	item := mb.filtered[mb.cursor]
	return &item
}

func (mb *MarketBrowser) scrollStartIdx() int {
	listHeight := max(mb.height-4, 1)
	summaryHeight := 6
	actionBarHeight := 2
	visibleItems := min(max(listHeight-summaryHeight-actionBarHeight, 3), len(mb.filtered))
	startIdx := 0
	if mb.cursor >= visibleItems {
		startIdx = mb.cursor - visibleItems + 1
	}
	if startIdx+visibleItems > len(mb.filtered) {
		startIdx = max(len(mb.filtered)-visibleItems, 0)
	}
	return startIdx
}

func (mb *MarketBrowser) refilter() {
	query := mb.searchInput.Value()
	mb.filtered = marketplace.Search(mb.index, query)
	if mb.cursor >= len(mb.filtered) {
		mb.cursor = len(mb.filtered) - 1
	}
	if mb.cursor < 0 {
		mb.cursor = 0
	}
}

func (mb *MarketBrowser) populateInfoViewport(item *marketplace.Item) tea.Cmd {
	var contentBuilder strings.Builder
	if item.Category == "bundles" && len(item.Components) > 0 {
		contentBuilder.WriteString("## Bundle Contents\n\n")
		for _, comp := range item.Components {
			fmt.Fprintf(&contentBuilder, "- **%s** v%s\n", comp.Name, comp.Version)
		}
		contentBuilder.WriteString("\n")
	}
	readme := item.Readme
	if strings.TrimSpace(readme) == "" {
		readme = item.Description
	}
	contentBuilder.WriteString(readme)
	content := contentBuilder.String()
	rendered := mb.markdownView.Render(content, max(mb.width-4, 0))
	mb.infoContent = rendered
	mb.viewport.SetContent(rendered)
	mb.viewport.GotoTop()

	capturedName := item.Name
	return func() tea.Msg {
		client := marketplace.NewClient(mb.marketBaseURL())
		reviews, err := client.GetReviews(context.Background(), capturedName, 1, 3)
		if err != nil || len(reviews.Reviews) == 0 {
			return tui.MarketplaceReviewsResultMsg{ItemName: capturedName}
		}
		var b strings.Builder
		b.WriteString("## Recent Reviews\n\n")
		for _, rev := range reviews.Reviews {
			ratingStr := ""
			if rev.Rating > 0 {
				ratingStr = fmt.Sprintf(" ⭐ %d", rev.Rating)
			}
			text := rev.Text
			if utf8.RuneCountInString(text) > 120 {
				text = string([]rune(text)[:117]) + "..."
			}
			fmt.Fprintf(&b, "**%s**%s (%s): %s\n\n", rev.Author, ratingStr, rev.CreatedAt, text)
		}
		return tui.MarketplaceReviewsResultMsg{ItemName: capturedName, Content: b.String()}
	}
}

func (mb *MarketBrowser) marketBaseURL() string {
	return marketplace.DefaultBaseURL()
}

// IsOpen returns whether the marketplace browser is currently open.
func (mb *MarketBrowser) IsOpen() bool {
	return mb.open
}

// Open makes the marketplace browser visible.
func (mb *MarketBrowser) Open() {
	mb.open = true
	mb.searchInput.Focus()
}

// Close hides the marketplace browser.
func (mb *MarketBrowser) Close() {
	mb.open = false
	mb.state = stateList
	mb.installMsg = ""
	mb.installing = false
}

// SetSize updates the component dimensions.
func (mb *MarketBrowser) SetSize(width, height int) {
	mb.width = width
	mb.height = height
	mb.viewport.SetWidth(width)
	mb.viewport.SetHeight(max(height-6, 3))
}
