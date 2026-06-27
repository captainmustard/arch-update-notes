package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// openURL opens a http(s) URL in the user's default browser via xdg-open.
func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return nil
		}
		_ = exec.Command("xdg-open", url).Start()
		return nil
	}
}

// handleMouse routes mouse events to tabs, list rows, session controls, and
// pane scrolling via bubblezone hit-testing.
func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if zone.Get("detailpane").InBounds(msg) {
			return m.scrollBy(-3)
		}
		if zone.Get("listpane").InBounds(msg) {
			return m.moveCursor(-1)
		}
		return nil
	case tea.MouseButtonWheelDown:
		if zone.Get("detailpane").InBounds(msg) {
			return m.scrollBy(3)
		}
		if zone.Get("listpane").InBounds(msg) {
			return m.moveCursor(1)
		}
		return nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}

	// Section tabs.
	for t := 0; t < int(numTabs); t++ {
		if zone.Get(fmt.Sprintf("tab-%d", t)).InBounds(msg) {
			if tab(t) != m.active {
				m.active = tab(t)
				var cmds []tea.Cmd
				m.afterNav(&cmds)
				return tea.Batch(cmds...)
			}
			return nil
		}
	}

	// Clickable links in the detail pane.
	for i, url := range m.curLinks {
		if zone.Get(fmt.Sprintf("link-%d", i)).InBounds(msg) {
			return openURL(url)
		}
	}

	// Session navigation.
	if zone.Get("sess-prev").InBounds(msg) {
		return m.gotoSession(-1)
	}
	if zone.Get("sess-next").InBounds(msg) {
		return m.gotoSession(1)
	}

	// List rows.
	prefix := m.rowPrefix()
	n := len(m.activeList().Items())
	for i := 0; i < n; i++ {
		if zone.Get(fmt.Sprintf("%s%d", prefix, i)).InBounds(msg) {
			return m.selectIndex(i)
		}
	}
	return nil
}

// moveCursor shifts the active list selection by delta and reloads detail.
func (m *Model) moveCursor(delta int) tea.Cmd {
	lp := m.activeListPtr()
	idx := lp.Index() + delta
	if idx < 0 {
		idx = 0
	}
	if max := len(lp.Items()) - 1; idx > max {
		idx = max
	}
	return m.selectIndex(idx)
}

// selectIndex selects a row and lazily loads its detail.
func (m *Model) selectIndex(i int) tea.Cmd {
	lp := m.activeListPtr()
	lp.Select(i)
	var cmds []tea.Cmd
	if m.active == tabPackages {
		cmds = append(cmds, m.loadSelection()...)
		cmds = append(cmds, m.ensureTick())
	}
	m.refreshDetail()
	return tea.Batch(cmds...)
}

// gotoSession moves between update sessions (dir -1 prev, +1 next).
func (m *Model) gotoSession(dir int) tea.Cmd {
	next := m.cur + dir
	if next < 0 || next >= len(m.sessions) {
		return nil
	}
	m.cur = next
	var cmds []tea.Cmd
	m.onSessionChange(&cmds)
	return tea.Batch(cmds...)
}
