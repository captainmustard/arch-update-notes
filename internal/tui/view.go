package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ianataylor42/arch-update-notes/internal/data"
)

const (
	listWidth   = 40
	footerLines = 1
)

// layout recomputes child component sizes for the current terminal size.
func (m *Model) layout() {
	header := lipgloss.Height(m.headerView())
	bodyH := m.height - header - footerLines
	if bodyH < 3 {
		bodyH = 3
	}

	lw := listWidth
	if lw > m.width/2 {
		lw = m.width / 2
	}
	// account for borders (2) on each pane
	innerListW := lw - 2
	if innerListW < 10 {
		innerListW = 10
	}
	innerH := bodyH - 2
	if innerH < 1 {
		innerH = 1
	}

	m.pkgList.SetSize(innerListW, innerH)
	m.newsList.SetSize(innerListW, innerH)
	m.pacnewList.SetSize(innerListW, innerH)

	detailW := m.width - lw - 2
	if detailW < 10 {
		detailW = 10
	}
	m.detail.Width = detailW
	m.detail.Height = innerH
	m.refreshDetail()
}

func (m Model) View() string {
	if !m.ready {
		return "Loading…"
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.listPane(), m.detailPane())
	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), body, m.footerView())
}

func (m Model) headerView() string {
	title := titleStyle.Render("  Arch Update Notes  ")

	tabs := make([]string, 0, numTabs)
	for t := tab(0); t < numTabs; t++ {
		label := t.String()
		switch t {
		case tabNews:
			if m.newsLoading {
				label += " …"
			} else if n := len(m.news); n > 0 {
				label += fmt.Sprintf(" %d", n)
			}
		case tabPacnew:
			if n := len(m.pacnew); n > 0 {
				label += fmt.Sprintf(" %d", n)
			}
		case tabPackages:
			if s, ok := m.curSession(); ok {
				label += fmt.Sprintf(" %d", len(s.Changes))
			}
		}
		if t == m.active {
			tabs = append(tabs, tabActiveStyle.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	tabBar := strings.Join(tabs, "")

	session := m.sessionLine()
	top := lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", tabBar)
	return lipgloss.JoinVertical(lipgloss.Left, top, session)
}

func (m Model) sessionLine() string {
	s, ok := m.curSession()
	if !ok {
		return sessionStyle.Render("  No update sessions found in the pacman log.")
	}
	u, i, r, o := s.Counts()
	when := s.Started.Format("Mon 2 Jan 2006 15:04")
	pos := fmt.Sprintf("update %d/%d", m.cur+1, len(m.sessions))
	parts := []string{fmt.Sprintf("↑%d", u)}
	if i > 0 {
		parts = append(parts, fmt.Sprintf("+%d", i))
	}
	if r > 0 {
		parts = append(parts, fmt.Sprintf("−%d", r))
	}
	if o > 0 {
		parts = append(parts, fmt.Sprintf("~%d", o))
	}
	counts := strings.Join(parts, " ")
	return sessionStyle.Render(fmt.Sprintf("  %s · %s · %s  [ ]/np to switch", when, counts, pos))
}

func (m Model) listPane() string {
	lw := listWidth
	if lw > m.width/2 {
		lw = m.width / 2
	}
	style := paneStyle
	style = style.Width(lw - 2).Height(m.detail.Height)
	return style.Render(m.activeList().View())
}

func (m Model) detailPane() string {
	style := paneActiveStyle
	style = style.Width(m.detail.Width).Height(m.detail.Height)
	return style.Render(m.detail.View())
}

func (m Model) footerView() string {
	help := "↑/↓ move · tab sections · [ ]/np updates · / filter · PgUp/PgDn scroll · q quit"
	return footerStyle.Render(" " + help)
}

// --- detail content ---

func (m *Model) refreshDetail() {
	m.detail.SetContent(m.detailContent(m.detail.Width))
}

func (m Model) detailContent(w int) string {
	if w < 4 {
		w = 4
	}
	wrap := lipgloss.NewStyle().Width(w)

	switch m.active {
	case tabPackages:
		it, ok := m.pkgList.SelectedItem().(pkgItem)
		if !ok {
			return wrap.Render("No package changes in this update session.")
		}
		c := it.c
		var b strings.Builder
		b.WriteString(detailTitleStyle.Render(c.Name) + "\n")
		b.WriteString(labelStyle.Render("Action:  ") + lipgloss.NewStyle().Foreground(actionColor(string(c.Action))).Render(string(c.Action)) + "\n")
		b.WriteString(labelStyle.Render("Version: ") + c.Version() + "\n")
		if !c.When.IsZero() {
			b.WriteString(labelStyle.Render("When:    ") + c.When.Format("2006-01-02 15:04:05") + "\n")
		}
		cl, seen := m.clog[c.Name]
		hasClog := seen && cl.ok
		if hasClog {
			b.WriteString("\n" + labelStyle.Render("Changelog") + "\n")
			b.WriteString(cl.text + "\n")
		}
		b.WriteString(m.referencesSection(c, hasClog))
		return wrap.Render(b.String())

	case tabNews:
		if m.newsLoading {
			return wrap.Render(lipgloss.NewStyle().Foreground(colMuted).Render("Fetching news…"))
		}
		it, ok := m.newsList.SelectedItem().(newsItem)
		if !ok {
			msg := "No news items."
			if m.newsErr != "" {
				msg = "Could not load news: " + m.newsErr
			}
			return wrap.Render(lipgloss.NewStyle().Foreground(colMuted).Render(msg))
		}
		n := it.n
		var b strings.Builder
		b.WriteString(detailTitleStyle.Render(n.Title) + "\n")
		b.WriteString(labelStyle.Render("Source: ") + n.Source + "\n")
		if !n.Date.IsZero() {
			b.WriteString(labelStyle.Render("Date:   ") + n.Date.Format("2006-01-02") + "\n")
		}
		if n.Link != "" {
			b.WriteString(labelStyle.Render("Link:   ") + lipgloss.NewStyle().Foreground(colAccent).Render(n.Link) + "\n")
		}
		b.WriteString("\n" + n.Summary)
		return wrap.Render(b.String())

	case tabPacnew:
		if len(m.pacnew) == 0 {
			return wrap.Render(lipgloss.NewStyle().Foreground(colNew).Render("No .pacnew or .pacsave files pending. Nothing to merge."))
		}
		it, ok := m.pacnewList.SelectedItem().(pacnewItem)
		if !ok {
			return wrap.Render("Select a config file.")
		}
		var b strings.Builder
		b.WriteString(detailTitleStyle.Render(it.path) + "\n\n")
		b.WriteString("This update shipped a new default for a config file you've modified. " +
			"The package manager saved the new version alongside yours so nothing was overwritten.\n\n")
		b.WriteString(labelStyle.Render("Merge it with:") + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colAccent).Render("  sudo pacdiff") + "\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colMuted).Render(
			"pacdiff walks each file interactively (view diff, merge, keep, or remove). " +
				"All pending files are listed on the left."))
		return wrap.Render(b.String())
	}
	return ""
}

// referencesSection renders the "what changed" fallback for a package: version
// interpretation, upstream release notes, and packaging source links.
func (m Model) referencesSection(c data.PackageChange, hasClog bool) string {
	muted := lipgloss.NewStyle().Foreground(colMuted)
	link := lipgloss.NewStyle().Foreground(colAccent)

	var b strings.Builder
	heading := "What changed"
	if hasClog {
		heading = "References"
	}
	b.WriteString("\n" + labelStyle.Render(heading) + "\n")

	st, seen := m.refs[c.Name]
	if !seen || st.loading {
		b.WriteString(muted.Render("loading…"))
		return b.String()
	}
	r := st.ref

	if r.VersionNote != "" {
		note := r.VersionNote
		if r.IsRebuild {
			note = lipgloss.NewStyle().Foreground(colWarn).Render(note)
		}
		b.WriteString(note + "\n")
	}

	if !hasClog && !m.online {
		b.WriteString(muted.Render("Offline (--no-news): showing links only.\n"))
	}

	if r.Release != nil {
		b.WriteString("\n" + detailTitleStyle.Render("Upstream release: "+r.Release.Title) + "\n")
		if r.Release.URL != "" {
			b.WriteString(link.Render(r.Release.URL) + "\n")
		}
		if r.Release.Body != "" {
			b.WriteString("\n" + r.Release.Body + "\n")
		}
	} else if m.online && !r.IsRebuild {
		b.WriteString(muted.Render("No upstream release notes found for this version.\n"))
	}

	b.WriteString("\n" + labelStyle.Render("Sources") + "\n")
	if r.UpstreamURL != "" {
		b.WriteString(muted.Render("Upstream:   ") + link.Render(r.UpstreamURL) + "\n")
	}
	if r.PackagingURL != "" {
		b.WriteString(muted.Render(pad(r.PackagingLabel)) + link.Render(r.PackagingURL) + "\n")
	}
	if r.CachyOSURL != "" {
		b.WriteString(muted.Render("CachyOS:    ") + link.Render(r.CachyOSURL) + "\n")
	}

	if len(r.PackagingCommits) > 0 {
		b.WriteString("\n" + labelStyle.Render("Recent packaging commits") + "\n")
		for _, msg := range r.PackagingCommits {
			b.WriteString(muted.Render("• ") + msg + "\n")
		}
	}

	return b.String()
}

// pad right-pads a sources label to align the links.
func pad(label string) string {
	label += ":"
	for lipgloss.Width(label) < 12 {
		label += " "
	}
	if !strings.HasSuffix(label, " ") {
		label += " "
	}
	return label
}
