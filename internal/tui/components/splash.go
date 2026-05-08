// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Simply Devly contributors

package components

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"siply.dev/siply/internal/tui"
)

const (
	splashCyan   = "#06b6d4"
	splashViolet = "#7c3aed"
	defaultGap   = 3
	pixelRows    = 12
)

// Half-block characters for 2x vertical resolution.
const (
	blockFull  = "█"
	blockUpper = "▀"
	blockLower = "▄"
)

// letters maps each character to a 12-row pixel grid (10 cols max).
// Ported from simply-tui-demo/simply-devly-splash.py.
var letters = map[byte][][]byte{
	'S': {
		{0, 0, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 1, 1, 1, 1, 1, 1, 1, 1, 0},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{0, 1, 1, 1, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 0, 0, 0, 1, 1, 1, 1, 0},
		{0, 0, 0, 0, 0, 0, 0, 1, 1, 1},
		{0, 0, 0, 0, 0, 0, 0, 1, 1, 1},
		{0, 1, 1, 0, 0, 0, 1, 1, 1, 0},
		{0, 1, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 1, 1, 1, 1, 1, 0, 0, 0},
	},
	'I': {
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
	},
	'M': {
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 1, 0, 0, 1, 1, 1, 1},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		{1, 1, 1, 0, 1, 1, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
	},
	'P': {
		{1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 1, 0, 0},
		{1, 1, 1, 0, 0, 0, 1, 1, 1, 0},
		{1, 1, 1, 0, 0, 0, 1, 1, 1, 0},
		{1, 1, 1, 0, 0, 1, 1, 1, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
	},
	'L': {
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
	},
	'Y': {
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{0, 1, 1, 1, 0, 0, 1, 1, 1, 0},
		{0, 1, 1, 1, 0, 0, 1, 1, 1, 0},
		{0, 0, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 0, 1, 1, 1, 1, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
	},
	'D': {
		{1, 1, 1, 1, 1, 1, 0, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
		{1, 1, 1, 0, 0, 1, 1, 1, 0, 0},
		{1, 1, 1, 0, 0, 0, 1, 1, 1, 0},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 1, 1, 1, 0},
		{1, 1, 1, 0, 0, 1, 1, 1, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 0, 0, 0, 0},
	},
	'E': {
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
		{1, 1, 1, 1, 1, 1, 1, 1, 1, 0},
	},
	'V': {
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{1, 1, 1, 0, 0, 0, 0, 1, 1, 1},
		{0, 1, 1, 1, 0, 0, 1, 1, 1, 0},
		{0, 1, 1, 1, 0, 0, 1, 1, 1, 0},
		{0, 0, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 0, 0, 1, 1, 1, 1, 0, 0, 0},
		{0, 0, 0, 1, 1, 1, 1, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
		{0, 0, 0, 0, 1, 1, 0, 0, 0, 0},
	},
	' ': {
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	},
}

var kerning = map[[2]byte]int{
	{'L', 'Y'}: -2,
}

func trimLetter(grid [][]byte) [][]byte {
	maxCol := 0
	for _, row := range grid {
		for c := len(row) - 1; c >= 0; c-- {
			if row[c] != 0 {
				if c+1 > maxCol {
					maxCol = c + 1
				}
				break
			}
		}
	}
	if maxCol == 0 {
		return grid
	}
	out := make([][]byte, len(grid))
	for i, row := range grid {
		if maxCol <= len(row) {
			out[i] = row[:maxCol]
		} else {
			out[i] = row
		}
	}
	return out
}

type wordGrid struct {
	pixels [][]byte
	owners [][]byte
}

func getWordPixels(word string) wordGrid {
	result := make([][]byte, pixelRows)
	owner := make([][]byte, pixelRows)
	for i := range pixelRows {
		result[i] = []byte{}
		owner[i] = []byte{}
	}

	for i := 0; i < len(word); i++ {
		ch := word[i]
		raw, ok := letters[ch]
		if !ok {
			raw = letters[' ']
		}
		trimmed := trimLetter(raw)
		w := len(trimmed[0])

		gap := defaultGap
		if i > 0 {
			prev := word[i-1]
			if k, ok := kerning[[2]byte{prev, ch}]; ok {
				gap = k
			}
		} else {
			gap = 0
		}

		if gap >= 0 {
			for r := range pixelRows {
				for range gap {
					result[r] = append(result[r], 0)
					owner[r] = append(owner[r], 0)
				}
				for c := 0; c < w; c++ {
					result[r] = append(result[r], trimmed[r][c])
					if trimmed[r][c] != 0 {
						owner[r] = append(owner[r], ch)
					} else {
						owner[r] = append(owner[r], 0)
					}
				}
			}
		} else {
			overlap := -gap
			for r := range pixelRows {
				for k := 0; k < overlap && k < w; k++ {
					idx := len(result[r]) - overlap + k
					if idx >= 0 && idx < len(result[r]) && trimmed[r][k] != 0 {
						result[r][idx] = 1
						owner[r][idx] = ch
					}
				}
				for c := overlap; c < w; c++ {
					result[r] = append(result[r], trimmed[r][c])
					if trimmed[r][c] != 0 {
						owner[r] = append(owner[r], ch)
					} else {
						owner[r] = append(owner[r], 0)
					}
				}
			}
		}
	}

	return wordGrid{pixels: result, owners: owner}
}

func splashStyles(cs tui.ColorSetting) (cyan, violet lipgloss.Style) {
	switch cs {
	case tui.ColorTrueColor, tui.Color256Color:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(splashCyan)),
			lipgloss.NewStyle().Foreground(lipgloss.Color(splashViolet))
	case tui.Color16Color:
		return lipgloss.NewStyle().Foreground(lipgloss.Cyan),
			lipgloss.NewStyle().Foreground(lipgloss.Blue)
	default:
		return lipgloss.NewStyle(), lipgloss.NewStyle()
	}
}

const (
	colorIDNone   byte = 0
	colorIDCyan   byte = 1
	colorIDViolet byte = 2
)

func colorIDForChar(ch byte, lineNum int) byte {
	if ch == 'Y' {
		if lineNum == 1 {
			return colorIDViolet
		}
		return colorIDCyan
	}
	if lineNum == 1 {
		return colorIDCyan
	}
	return colorIDViolet
}

func renderColoredGrid(grid wordGrid, lineNum int, cs tui.ColorSetting) []string {
	cyanStyle, violetStyle := splashStyles(cs)
	noColor := cs == tui.ColorNone

	styleFor := func(id byte) lipgloss.Style {
		switch id {
		case colorIDCyan:
			return cyanStyle
		case colorIDViolet:
			return violetStyle
		default:
			return lipgloss.NewStyle()
		}
	}

	var lines []string
	for r := 0; r < len(grid.pixels); r += 2 {
		top := grid.pixels[r]
		var bot []byte
		if r+1 < len(grid.pixels) {
			bot = grid.pixels[r+1]
		} else {
			bot = make([]byte, len(top))
		}
		otop := grid.owners[r]
		var obot []byte
		if r+1 < len(grid.owners) {
			obot = grid.owners[r+1]
		} else {
			obot = make([]byte, len(top))
		}

		cols := len(top)
		if len(bot) > cols {
			cols = len(bot)
		}

		type run struct {
			colorID byte
			text    strings.Builder
		}
		var runs []run
		var currentID byte = colorIDNone
		var currentText strings.Builder
		hasRun := false

		flush := func() {
			if hasRun && currentText.Len() > 0 {
				runs = append(runs, run{colorID: currentID, text: currentText})
				currentText = strings.Builder{}
			}
			hasRun = false
		}

		for c := range cols {
			var t, bv byte
			if c < len(top) {
				t = top[c]
			}
			if c < len(bot) {
				bv = bot[c]
			}

			var ch string
			switch {
			case t != 0 && bv != 0:
				ch = blockFull
			case t != 0:
				ch = blockUpper
			case bv != 0:
				ch = blockLower
			default:
				ch = " "
			}

			if noColor || (t == 0 && bv == 0) {
				flush()
				currentID = colorIDNone
				currentText.WriteString(ch)
				hasRun = true
				continue
			}

			var ownerCh byte
			if t != 0 && c < len(otop) {
				ownerCh = otop[c]
			} else if bv != 0 && c < len(obot) {
				ownerCh = obot[c]
			}

			cid := colorIDForChar(ownerCh, lineNum)

			if !hasRun {
				currentID = cid
				currentText.WriteString(ch)
				hasRun = true
			} else if cid == currentID {
				currentText.WriteString(ch)
			} else {
				flush()
				currentID = cid
				currentText.WriteString(ch)
				hasRun = true
			}
		}
		flush()

		var line strings.Builder
		for _, rn := range runs {
			s := styleFor(rn.colorID)
			line.WriteString(s.Render(rn.text.String()))
		}
		lines = append(lines, line.String())
	}
	return lines
}

const (
	// SplashHeight is the number of terminal lines the full pixel-art splash occupies
	// (6 lines per word + 1 blank + 6 lines + 1 blank + tagline = 15).
	SplashHeight = 15

	// SplashFallbackHeight is the number of lines for the text-only fallback
	// (SIMPLY + blank + DEVLY + blank + tagline = 5).
	SplashFallbackHeight = 5

	splashMinArtWidth = 62
)

// RenderSplash returns the SIMPLY DEVLY splash, horizontally centered.
// For terminals >= 62 cols wide: full half-block pixel art.
// For narrower terminals: styled text fallback with the same color scheme.
func RenderSplash(width int, cs tui.ColorSetting) string {
	if width < 1 {
		width = 1
	}
	if width < splashMinArtWidth {
		return renderSplashText(width, cs)
	}
	return renderSplashArt(width, cs)
}

// SplashLines returns how many terminal lines the splash occupies at the given width.
func SplashLines(width int) int {
	if width < splashMinArtWidth {
		return SplashFallbackHeight
	}
	return SplashHeight
}

func renderTagline(cs tui.ColorSetting) string {
	cyanStyle, violetStyle := splashStyles(cs)
	return violetStyle.Render("Build smarter. Ship faster. ") + cyanStyle.Render("Simply Devly.")
}

func renderSplashArt(width int, cs tui.ColorSetting) string {
	grid1 := getWordPixels("SIMPLY")
	grid2 := getWordPixels("DEVLY")

	lines1 := renderColoredGrid(grid1, 1, cs)
	lines2 := renderColoredGrid(grid2, 2, cs)

	all := make([]string, 0, SplashHeight)
	all = append(all, lines1...)
	all = append(all, "")
	all = append(all, lines2...)
	all = append(all, "")
	all = append(all, renderTagline(cs))

	return centerLines(all, width)
}

func renderSplashText(width int, cs tui.ColorSetting) string {
	cyanStyle, violetStyle := splashStyles(cs)

	simply := cyanStyle.Render("SIMPL") + violetStyle.Render("Y")
	devl := violetStyle.Render("DEVL")
	devly := devl + cyanStyle.Render("Y")

	lines := []string{simply, "", devly, "", renderTagline(cs)}
	return centerLines(lines, width)
}

func centerLines(lines []string, width int) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		lineW := ansi.StringWidth(line)
		pad := (width - lineW) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(line)
	}
	return b.String()
}
