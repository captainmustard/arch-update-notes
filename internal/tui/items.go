package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/reflow/truncate"

	"github.com/ianataylor42/arch-update-notes/internal/data"
)

// --- package items ---

type pkgItem struct{ c data.PackageChange }

func (i pkgItem) FilterValue() string { return i.c.Name }

// --- news items ---

type newsItem struct {
	n     data.NewsItem
	isNew bool
}

func (i newsItem) FilterValue() string { return i.n.Title }

// --- pacnew items ---

type pacnewItem struct{ path string }

func (i pacnewItem) FilterValue() string { return i.path }

// compactDelegate renders single-line items for all three lists.
type compactDelegate struct{}

func (d compactDelegate) Height() int                              { return 1 }
func (d compactDelegate) Spacing() int                             { return 0 }
func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd  { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	selected := index == m.Index()
	width := m.Width()
	cursor := "  "
	if selected {
		cursor = lipgloss.NewStyle().Foreground(colAccent).Render("▌ ")
	}

	var line, prefix string
	switch it := item.(type) {
	case pkgItem:
		glyph := lipgloss.NewStyle().Foreground(actionColor(string(it.c.Action))).Render(actionGlyph(string(it.c.Action)))
		name := it.c.Name
		ver := lipgloss.NewStyle().Foreground(colMuted).Render(it.c.Version())
		line = fmt.Sprintf("%s %s  %s", glyph, name, ver)
		prefix = rowPkg
	case newsItem:
		tag := ""
		if it.isNew {
			tag = newTagStyle.Render(" [NEW]")
		}
		src := lipgloss.NewStyle().Foreground(colMuted).Render("(" + it.n.Source + ")")
		line = fmt.Sprintf("%s %s%s", src, it.n.Title, tag)
		prefix = rowNews
	case pacnewItem:
		line = lipgloss.NewStyle().Foreground(colWarn).Render("⚠ ") + it.path
		prefix = rowPacnew
	}

	full := cursor + line
	if selected {
		full = lipgloss.NewStyle().Bold(true).Render(full)
	}
	// Truncate to width (rune- and ANSI-aware) to keep each item on one line.
	if width > 0 {
		full = truncate.StringWithTail(full, uint(width), "…")
	}
	io.WriteString(w, zone.Mark(fmt.Sprintf("%s%d", prefix, index), full))
}

// row zone id prefixes for each list.
const (
	rowPkg    = "row-pkg-"
	rowNews   = "row-news-"
	rowPacnew = "row-pacnew-"
)
