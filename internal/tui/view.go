package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

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
	return zone.Scan(lipgloss.JoinVertical(lipgloss.Left, m.headerView(), body, m.footerView()))
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
		id := fmt.Sprintf("tab-%d", int(t))
		if t == m.active {
			tabs = append(tabs, zone.Mark(id, tabActiveStyle.Render(label)))
		} else {
			tabs = append(tabs, zone.Mark(id, tabStyle.Render(label)))
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
	prev := zone.Mark("sess-prev", sessionStyle.Render("‹prev"))
	next := zone.Mark("sess-next", sessionStyle.Render("next›"))
	line := sessionStyle.Render(fmt.Sprintf("  %s · %s · %s  ", when, counts, pos))
	return line + prev + sessionStyle.Render(" ") + next
}

func (m Model) listPane() string {
	lw := listWidth
	if lw > m.width/2 {
		lw = m.width / 2
	}
	style := paneStyle
	style = style.Width(lw - 2).Height(m.detail.Height)
	return zone.Mark("listpane", style.Render(m.activeList().View()))
}

func (m Model) detailPane() string {
	style := paneActiveStyle
	style = style.Width(m.detail.Width).Height(m.detail.Height)
	return zone.Mark("detailpane", style.Render(m.detail.View()))
}

func (m Model) footerView() string {
	help := "↑/↓ move · tab/click sections · [ ]/np updates · / filter · PgUp/PgDn·g/G scroll · q quit"
	left := footerStyle.Render(" " + help)
	if !m.loadingActive() {
		return left
	}
	ind := m.indicatorView()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(ind) - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + ind
}

// --- detail content ---

// refreshDetail re-renders the detail pane only when its content signature
// changes, keeping the YOffset (and any in-flight scroll animation) intact.
func (m *Model) refreshDetail() {
	sig := m.detailSignature()
	if sig == m.detailSig {
		return
	}
	m.detailSig = sig
	m.detail.SetContent(m.detailContent())
	m.snapScroll()
}

func (m Model) detailSignature() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%d|%v|%v", m.active, m.cur, m.detail.Width, m.online, m.newsLoading)
	switch m.active {
	case tabPackages:
		if c, ok := m.selectedPkg(); ok {
			cl := m.clog[c.Name]
			r := m.refs[c.Name]
			fmt.Fprintf(&b, "|pkg:%s|cl:%v,%v|rf:%v,%v", c.Name, cl.loading, cl.ok, r.loading, r.done)
		}
	case tabNews:
		if it, ok := m.newsList.SelectedItem().(newsItem); ok {
			fmt.Fprintf(&b, "|news:%s", it.n.Link)
		}
	case tabPacnew:
		if it, ok := m.pacnewList.SelectedItem().(pacnewItem); ok {
			fmt.Fprintf(&b, "|pn:%s", it.path)
		}
	}
	return b.String()
}

type linkItem struct{ label, url string }

func (m *Model) detailContent() string {
	m.curLinks = nil
	switch m.active {
	case tabPackages:
		c, ok := m.selectedPkg()
		if !ok {
			return m.plain("No package changes in this update session.")
		}
		return m.compose(m.packageMarkdown(c), m.packageLinks(c))
	case tabNews:
		if m.newsLoading {
			return m.plain("Fetching news…")
		}
		it, ok := m.newsList.SelectedItem().(newsItem)
		if !ok {
			if m.newsErr != "" {
				return m.plain("Could not load news: " + m.newsErr)
			}
			return m.plain("No news items.")
		}
		var links []linkItem
		if it.n.Link != "" {
			links = append(links, linkItem{"Article", it.n.Link})
		}
		return m.compose(newsMarkdown(it.n), links)
	case tabPacnew:
		if len(m.pacnew) == 0 {
			return m.plain("No .pacnew or .pacsave files pending. Nothing to merge.")
		}
		it, ok := m.pacnewList.SelectedItem().(pacnewItem)
		if !ok {
			return m.plain("Select a config file.")
		}
		return m.mdRender(pacnewMarkdown(it.path))
	}
	return ""
}

// compose renders the markdown body via glamour and appends a clickable Links
// section, recording the URLs in m.curLinks (indexed by zone id).
func (m *Model) compose(md string, links []linkItem) string {
	out := m.mdRender(md)
	if len(links) == 0 {
		return out
	}
	var b strings.Builder
	b.WriteString(out)
	b.WriteString("\n\n  " + labelStyle.Render("Links") + "\n")
	for i, l := range links {
		id := fmt.Sprintf("link-%d", i)
		m.curLinks = append(m.curLinks, l.url)
		label := lipgloss.NewStyle().Foreground(colMuted).Render(pad(l.label))
		b.WriteString("  " + label + zone.Mark(id, linkStyle.Render(l.url)) + "\n")
	}
	b.WriteString("\n  " + lipgloss.NewStyle().Foreground(colMuted).Render("click a link to open it in your browser"))
	return b.String()
}

// pad right-pads a links label for alignment.
func pad(label string) string {
	label += ":"
	for lipgloss.Width(label) < 9 {
		label += " "
	}
	if !strings.HasSuffix(label, " ") {
		label += " "
	}
	return label
}

func (m Model) packageLinks(c data.PackageChange) []linkItem {
	st, seen := m.refs[c.Name]
	if !seen || st.loading {
		return nil
	}
	r := st.ref
	var links []linkItem
	if r.Release != nil && r.Release.URL != "" {
		links = append(links, linkItem{"Release", r.Release.URL})
	}
	if r.UpstreamURL != "" {
		links = append(links, linkItem{"Upstream", r.UpstreamURL})
	}
	if r.PackagingURL != "" {
		links = append(links, linkItem{r.PackagingLabel, r.PackagingURL})
	}
	if r.CachyOSURL != "" {
		links = append(links, linkItem{"CachyOS", r.CachyOSURL})
	}
	return links
}

// plain renders a short status message (no markdown) wrapped to the pane width.
func (m Model) plain(s string) string {
	return lipgloss.NewStyle().Width(m.detail.Width).Foreground(colMuted).Render(s)
}

func (m Model) packageMarkdown(c data.PackageChange) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", c.Name)
	fmt.Fprintf(&b, "**%s** · `%s`", c.Action, c.Version())
	if !c.When.IsZero() {
		fmt.Fprintf(&b, " · %s", c.When.Format("2006-01-02 15:04"))
	}
	b.WriteString("\n\n")

	cl, seen := m.clog[c.Name]
	hasClog := seen && cl.ok
	if hasClog {
		b.WriteString("**Changelog**\n\n```\n" + cl.text + "\n```\n\n")
	}
	b.WriteString(m.referencesMarkdown(c, hasClog))
	return b.String()
}

// referencesMarkdown is the "what changed" fallback as a markdown fragment.
func (m Model) referencesMarkdown(c data.PackageChange, hasClog bool) string {
	var b strings.Builder
	if hasClog {
		b.WriteString("**References**\n\n")
	} else {
		b.WriteString("**What changed**\n\n")
	}

	st, seen := m.refs[c.Name]
	if !seen || st.loading {
		b.WriteString("_loading…_\n")
		return b.String()
	}
	r := st.ref

	if r.VersionNote != "" {
		if r.IsRebuild {
			b.WriteString("> " + r.VersionNote + "\n\n")
		} else {
			b.WriteString(r.VersionNote + "\n\n")
		}
	}
	if !hasClog && !m.online {
		b.WriteString("_Offline (--no-news): showing links only._\n\n")
	}

	if r.Release != nil {
		fmt.Fprintf(&b, "**Upstream release: %s**\n\n", r.Release.Title)
		if r.Release.Body != "" {
			b.WriteString(r.Release.Body + "\n\n")
		}
	} else if m.online && !r.IsRebuild {
		b.WriteString("_No upstream release notes found for this version._\n\n")
	}

	if len(r.PackagingCommits) > 0 {
		b.WriteString("\n**Recent packaging commits**\n\n")
		for _, msg := range r.PackagingCommits {
			b.WriteString("- " + msg + "\n")
		}
	}
	return b.String()
}

func newsMarkdown(n data.NewsItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", n.Title)
	fmt.Fprintf(&b, "**%s**", n.Source)
	if !n.Date.IsZero() {
		fmt.Fprintf(&b, " · %s", n.Date.Format("2006-01-02"))
	}
	b.WriteString("\n\n")
	b.WriteString(n.Summary + "\n")
	return b.String()
}

func pacnewMarkdown(path string) string {
	return fmt.Sprintf("# %s\n\n"+
		"This update shipped a new default for a config file you've modified. "+
		"The package manager saved the new version alongside yours so nothing was overwritten.\n\n"+
		"**Merge it with:**\n\n"+
		"```\nsudo pacdiff\n```\n\n"+
		"_pacdiff walks each file interactively (view diff, merge, keep, or remove). "+
		"All pending files are listed on the left._\n", path)
}
