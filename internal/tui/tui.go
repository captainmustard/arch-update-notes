// Package tui implements the Bubble Tea terminal UI for browsing the notes of
// the most recent system update.
package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/harmonica"

	"github.com/captainmustard/arch-update-notes/internal/data"
)

type tab int

const (
	tabPackages tab = iota
	tabNews
	tabPacnew
	tabSnapshots
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
	case tabSnapshots:
		return "Snapshots"
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
	online   bool

	active tab

	pkgList    list.Model
	newsList   list.Model
	pacnewList list.Model
	snapList   list.Model
	detail     viewport.Model

	news        []data.NewsItem
	newsLoading bool
	newsErr     string

	pacnew []string
	snaps  data.SnapshotInfo

	clog map[string]clog // changelog cache keyed by package name
	refs map[string]refState

	md        *glamour.TermRenderer
	mdWidth   int
	detailSig string   // signature of the currently rendered detail content
	curLinks  []string // URLs in the current detail's Links section, by zone index

	// animation (harmonica)
	spring      harmonica.Spring
	springReady bool
	sPos, sVel  float64 // detail scroll position/velocity (lines)
	sTarget     float64 // detail scroll target (lines)
	scrolling   bool
	indPhase    float64 // fetch indicator phase 0..1
	indVel      float64
	indTarget   float64
	tickInFlight bool
}

type clog struct {
	text    string
	ok      bool
	loading bool
}

type refState struct {
	ref     data.Reference
	done    bool
	loading bool
}

// New builds a Model from already-collected local data. News is fetched
// asynchronously after start.
func New(sessions []data.Session, pacnew []string, feeds []data.Feed, snaps data.SnapshotInfo, online bool) Model {
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
		online:      online,
		active:      tabPackages,
		pkgList:     mk(),
		newsList:    mk(),
		pacnewList:  mk(),
		snapList:    mk(),
		pacnew:      pacnew,
		snaps:       snaps,
		newsLoading: len(feeds) > 0,
		clog:        map[string]clog{},
		refs:        map[string]refState{},
	}
	m.populatePacnew()
	m.populateSnapshots()
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

type refsMsg struct {
	pkg string
	ref data.Reference
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

func fetchRefsCmd(c data.PackageChange, online bool) tea.Cmd {
	return func() tea.Msg {
		return refsMsg{pkg: c.Name, ref: data.GatherReferences(c, online)}
	}
}

// --- update ---

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		wasReady := m.ready
		m.width, m.height = msg.Width, msg.Height
		m.initSpring()
		m.layout()
		m.ready = true
		if !wasReady {
			cmds = append(cmds, m.loadSelection()...)
			cmds = append(cmds, m.ensureTick())
		}

	case tickMsg:
		return m, m.handleTick()

	case tea.MouseMsg:
		return m, m.handleMouse(msg)

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

	case refsMsg:
		m.refs[msg.pkg] = refState{ref: msg.ref, done: true}
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
		case "4":
			m.active = tabSnapshots
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
		case "pgdown", " ":
			return m, m.scrollBy(float64(m.detail.Height - 1))
		case "pgup":
			return m, m.scrollBy(-float64(m.detail.Height - 1))
		case "d", "ctrl+d":
			return m, m.scrollBy(float64(m.detail.Height / 2))
		case "u", "ctrl+u":
			return m, m.scrollBy(-float64(m.detail.Height / 2))
		case "g", "home":
			return m, m.scrollBy(-m.maxScroll() - 1)
		case "G", "end":
			return m, m.scrollBy(m.maxScroll() + 1)
		}
	}

	// Route remaining messages to the active list, then sync the detail pane.
	switch m.active {
	case tabPackages:
		prev := m.selectedPkgName()
		var c tea.Cmd
		m.pkgList, c = m.pkgList.Update(message)
		cmds = append(cmds, c)
		if m.selectedPkgName() != prev {
			cmds = append(cmds, m.loadSelection()...)
			cmds = append(cmds, m.ensureTick())
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
	case tabSnapshots:
		var c tea.Cmd
		m.snapList, c = m.snapList.Update(message)
		cmds = append(cmds, c)
		m.refreshDetail()
	}

	return m, tea.Batch(cmds...)
}

// afterNav refreshes the detail pane and loads data after a tab switch.
func (m *Model) afterNav(cmds *[]tea.Cmd) {
	if m.active == tabPackages {
		*cmds = append(*cmds, m.loadSelection()...)
		*cmds = append(*cmds, m.ensureTick())
	}
	m.refreshDetail()
}

func (m *Model) onSessionChange(cmds *[]tea.Cmd) {
	m.populatePackages()
	m.populateNews() // recompute [NEW] tags relative to the new session
	if m.active == tabPackages {
		*cmds = append(*cmds, m.loadSelection()...)
		*cmds = append(*cmds, m.ensureTick())
	}
	m.refreshDetail()
}

// loadSelection lazily loads the changelog and references for the currently
// selected package.
func (m *Model) loadSelection() []tea.Cmd {
	c, ok := m.selectedPkg()
	if !ok {
		return nil
	}
	var cmds []tea.Cmd
	if _, seen := m.clog[c.Name]; !seen {
		m.clog[c.Name] = clog{loading: true}
		cmds = append(cmds, fetchClogCmd(c.Name))
	}
	if _, seen := m.refs[c.Name]; !seen {
		m.refs[c.Name] = refState{loading: true}
		cmds = append(cmds, fetchRefsCmd(c, m.online))
	}
	return cmds
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

func (m Model) selectedPkg() (data.PackageChange, bool) {
	if it, ok := m.pkgList.SelectedItem().(pkgItem); ok {
		return it.c, true
	}
	return data.PackageChange{}, false
}

func (m Model) activeList() list.Model {
	switch m.active {
	case tabNews:
		return m.newsList
	case tabPacnew:
		return m.pacnewList
	case tabSnapshots:
		return m.snapList
	default:
		return m.pkgList
	}
}

func (m *Model) activeListPtr() *list.Model {
	switch m.active {
	case tabNews:
		return &m.newsList
	case tabPacnew:
		return &m.pacnewList
	case tabSnapshots:
		return &m.snapList
	default:
		return &m.pkgList
	}
}

func (m Model) rowPrefix() string {
	switch m.active {
	case tabNews:
		return rowNews
	case tabPacnew:
		return rowPacnew
	case tabSnapshots:
		return rowSnap
	default:
		return rowPkg
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

func (m *Model) populateSnapshots() {
	items := make([]list.Item, 0, len(m.snaps.Pairs))
	for _, p := range m.snaps.Pairs {
		items = append(items, snapItem{p: p})
	}
	m.snapList.SetItems(items)
}
