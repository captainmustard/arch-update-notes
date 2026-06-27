package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/reflow/truncate"

	"github.com/captainmustard/arch-update-notes/internal/data"
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

// --- snapshot items ---

type snapItem struct{ p data.SnapPair }

func (i snapItem) FilterValue() string { return i.p.Summary() }

// compactDelegate renders single-line items for all three lists.
type compactDelegate struct{}

func (d compactDelegate) Height() int                              { return 1 }
func (d compactDelegate) Spacing() int                             { return 0 }
func (d compactDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd  { return nil }

func (d compactDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	selected := index == m.Index()
	width := m.Width()
	if width < 1 {
		width = 1
	}

	// Each item produces a plain-text form (for the selected highlight bar) and
	// a colored form (for normal rows).
	var plain, colored, prefix string
	muted := lipgloss.NewStyle().Foreground(colMuted)
	switch it := item.(type) {
	case pkgItem:
		a := string(it.c.Action)
		plain = fmt.Sprintf("%s %s  %s", actionGlyph(a), it.c.Name, it.c.Version())
		colored = fmt.Sprintf("%s %s  %s",
			lipgloss.NewStyle().Foreground(actionColor(a)).Render(actionGlyph(a)),
			it.c.Name, muted.Render(it.c.Version()))
		prefix = rowPkg
	case newsItem:
		tag := ""
		if it.isNew {
			tag = " [NEW]"
		}
		plain = fmt.Sprintf("(%s) %s%s", it.n.Source, it.n.Title, tag)
		colTag := ""
		if it.isNew {
			colTag = newTagStyle.Render(" [NEW]")
		}
		colored = fmt.Sprintf("%s %s%s", muted.Render("("+it.n.Source+")"), it.n.Title, colTag)
		prefix = rowNews
	case pacnewItem:
		plain = "⚠ " + it.path
		colored = lipgloss.NewStyle().Foreground(colWarn).Render("⚠ ") + it.path
		prefix = rowPacnew
	case snapItem:
		when := it.p.When().Format("01-02 15:04")
		plain = fmt.Sprintf("❄ %s  %s", when, it.p.Summary())
		colored = fmt.Sprintf("%s %s  %s",
			lipgloss.NewStyle().Foreground(colAccent).Render("❄"),
			muted.Render(when), it.p.Summary())
		prefix = rowSnap
	}

	var line string
	if selected {
		// Full-width accent bar (PaddingLeft 2 → text starts at column 2).
		txt := truncate.StringWithTail(plain, uint(width-2), "…")
		line = selStyle.Width(width).Render(txt)
	} else {
		line = truncate.StringWithTail("  "+colored, uint(width), "…")
	}
	io.WriteString(w, zone.Mark(fmt.Sprintf("%s%d", prefix, index), line))
}

// row zone id prefixes for each list.
const (
	rowPkg    = "row-pkg-"
	rowNews   = "row-news-"
	rowPacnew = "row-pacnew-"
	rowSnap   = "row-snap-"
)
