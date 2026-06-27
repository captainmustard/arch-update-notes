// Package tui implements the Bubble Tea terminal UI for browsing the notes of
// the most recent system update.
package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianataylor42/arch-update-notes/internal/data"
)

type tab int

const (
	tabPackages tab = iota
	tabNews
	tabPacnew
	numTabs
)

func (t tab) String() string {
	switch t {
	case tabPackages:
		return "Packages"
	case tabNews:
		return "News"
	case tabPacnew:
		return "Config files"
	}
	return ""
}

// Model is the root Bubble Tea model.
type Model struct {
	width, height int
	ready         bool

	sessions []data.Session
	cur      int // index into sessions; latest is len-1
	feeds    []data.Feed

	active tab

	pkgList    list.Model
	newsList   list.Model
	pacnewList list.Model
	detail     viewport.Model

	news        []data.NewsItem
	newsLoading bool
	newsErr     string

	pacnew []string

	clog        map[string]clog // changelog cache keyed by package name
	lastPkg     string
}

type clog struct {
	text    string
	ok      bool
	loading bool
}

// New builds a Model from already-collected local data. News is fetched
// asynchronously after start.
func New(sessions []data.Session, pacnew []string, feeds []data.Feed) Model {
	del := compactDelegate{}
	mk := func() list.Model {
		l := list.New(nil, del, 0, 0)
		l.SetShowTitle(false)
		l.SetShowStatusBar(false)
		l.SetShowHelp(false)
		l.SetFilteringEnabled(true)
		l.DisableQuitKeybindings()
		return l
	}

	m := Model{
		sessions:    sessions,
		cur:         len(sessions) - 1,
		feeds:       feeds,
		active:      tabPackages,
		pkgList:     mk(),
		newsList:    mk(),
		pacnewList:  mk(),
		pacnew:      pacnew,
		newsLoading: true,
		clog:        map[string]clog{},
	}
	m.populatePacnew()
	m.populatePackages()
	return m
}

func (m Model) Init() tea.Cmd {
	return fetchNewsCmd(m.feeds)
}

// --- messages ---

type newsMsg struct {
	items []data.NewsItem
	errs  []error
}

type clogMsg struct {
	pkg  string
	text string
	ok   bool
}

func fetchNewsCmd(feeds []data.Feed) tea.Cmd {
	return func() tea.Msg {
		items, errs := data.FetchNews(feeds, 10*time.Second, 12)
		return newsMsg{items: items, errs: errs}
	}
}

func fetchClogCmd(pkg string) tea.Cmd {
	return func() tea.Msg {
		text, ok := data.Changelog(pkg)
		return clogMsg{pkg: pkg, text: text, ok: ok}
	}
}

// --- update ---

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		m.ready = true

	case newsMsg:
		m.newsLoading = false
		m.news = msg.items
		if len(msg.errs) > 0 {
			var parts []string
			for _, e := range msg.errs {
				parts = append(parts, e.Error())
			}
			m.newsErr = strings.Join(parts, "; ")
		}
		m.populateNews()

	case clogMsg:
		c := m.clog[msg.pkg]
		c.loading = false
		c.text, c.ok = msg.text, msg.ok
		m.clog[msg.pkg] = c
		if m.active == tabPackages && m.selectedPkgName() == msg.pkg {
			m.refreshDetail()
		}

	case tea.KeyMsg:
		// Let an active filter input consume keys first.
		if m.activeList().FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "l", "right":
			m.active = (m.active + 1) % numTabs
			m.afterNav(&cmds)
			return m, tea.Batch(cmds...)
		case "shift+tab", "h", "left":
			m.active = (m.active + numTabs - 1) % numTabs
			m.afterNav(&cmds)
			return m, tea.Batch(cmds...)
		case "1":
			m.active = tabPackages
			m.afterNav(&cmds)
			return m, tea.Batch(cmds...)
		case "2":
			m.active = tabNews
			m.afterNav(&cmds)
			return m, tea.Batch(cmds...)
		case "3":
			m.active = tabPacnew
			m.afterNav(&cmds)
			return m, tea.Batch(cmds...)
		case "]", "n":
			if m.cur < len(m.sessions)-1 {
				m.cur++
				m.onSessionChange(&cmds)
			}
			return m, tea.Batch(cmds...)
		case "[", "p":
			if m.cur > 0 {
				m.cur--
				m.onSessionChange(&cmds)
			}
			return m, tea.Batch(cmds...)
		case "pgup", "pgdown", "u", "d", "ctrl+u", "ctrl+d":
			var c tea.Cmd
			m.detail, c = m.detail.Update(message)
			return m, c
		}
	}

	// Route remaining messages to the active list, then sync the detail pane.
	switch m.active {
	case tabPackages:
		prev := m.selectedPkgName()
		var c tea.Cmd
		m.pkgList, c = m.pkgList.Update(message)
		cmds = append(cmds, c)
		if name := m.selectedPkgName(); name != prev {
			cmds = append(cmds, m.ensureClog(name)...)
		}
		m.refreshDetail()
	case tabNews:
		var c tea.Cmd
		m.newsList, c = m.newsList.Update(message)
		cmds = append(cmds, c)
		m.refreshDetail()
	case tabPacnew:
		var c tea.Cmd
		m.pacnewList, c = m.pacnewList.Update(message)
		cmds = append(cmds, c)
		m.refreshDetail()
	}

	return m, tea.Batch(cmds...)
}

// afterNav refreshes the detail pane and loads a changelog after a tab switch.
func (m *Model) afterNav(cmds *[]tea.Cmd) {
	if m.active == tabPackages {
		*cmds = append(*cmds, m.ensureClog(m.selectedPkgName())...)
	}
	m.refreshDetail()
}

func (m *Model) onSessionChange(cmds *[]tea.Cmd) {
	m.populatePackages()
	m.populateNews() // recompute [NEW] tags relative to the new session
	if m.active == tabPackages {
		*cmds = append(*cmds, m.ensureClog(m.selectedPkgName())...)
	}
	m.refreshDetail()
}

// ensureClog returns a command to load a package changelog if not cached.
func (m *Model) ensureClog(pkg string) []tea.Cmd {
	if pkg == "" {
		return nil
	}
	if _, seen := m.clog[pkg]; seen {
		return nil
	}
	m.clog[pkg] = clog{loading: true}
	return []tea.Cmd{fetchClogCmd(pkg)}
}

func (m Model) curSession() (data.Session, bool) {
	if m.cur < 0 || m.cur >= len(m.sessions) {
		return data.Session{}, false
	}
	return m.sessions[m.cur], true
}

func (m Model) selectedPkgName() string {
	if it, ok := m.pkgList.SelectedItem().(pkgItem); ok {
		return it.c.Name
	}
	return ""
}

func (m Model) activeList() list.Model {
	switch m.active {
	case tabNews:
		return m.newsList
	case tabPacnew:
		return m.pacnewList
	default:
		return m.pkgList
	}
}

// --- population ---

func (m *Model) populatePackages() {
	s, ok := m.curSession()
	if !ok {
		m.pkgList.SetItems(nil)
		return
	}
	items := make([]list.Item, 0, len(s.Changes))
	for _, c := range s.Changes {
		items = append(items, pkgItem{c: c})
	}
	m.pkgList.SetItems(items)
	m.pkgList.Select(0)
}

func (m *Model) populateNews() {
	var ref time.Time
	if s, ok := m.curSession(); ok {
		ref = s.Started
	}
	items := make([]list.Item, 0, len(m.news))
	for _, n := range m.news {
		isNew := !n.Date.IsZero() && !ref.IsZero() && !n.Date.Before(ref.Add(-7*24*time.Hour))
		items = append(items, newsItem{n: n, isNew: isNew})
	}
	m.newsList.SetItems(items)
}

func (m *Model) populatePacnew() {
	items := make([]list.Item, 0, len(m.pacnew))
	for _, p := range m.pacnew {
		items = append(items, pacnewItem{path: p})
	}
	m.pacnewList.SetItems(items)
}
