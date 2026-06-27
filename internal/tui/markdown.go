package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// mdRender lazily builds a glamour renderer for the given width and renders
// markdown to ANSI. The renderer is rebuilt only when the width changes.
func (m *Model) mdRender(md string) string {
	if md == "" {
		return ""
	}
	w := m.detail.Width
	if w < 10 {
		w = 10
	}
	if m.md == nil || m.mdWidth != w {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(w-1),
			glamour.WithEmoji(),
		)
		if err != nil {
			return md
		}
		m.md = r
		m.mdWidth = w
	}
	out, err := m.md.Render(md)
	if err != nil {
		return md
	}
	return strings.Trim(out, "\n")
}
