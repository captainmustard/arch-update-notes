package tui

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
)

// tickMsg drives spring animations.
type tickMsg time.Time

const animFPS = 60

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/animFPS, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) initSpring() {
	if !m.springReady {
		m.spring = harmonica.NewSpring(harmonica.FPS(animFPS), 7.0, 0.65)
		m.springReady = true
	}
}

// loadingActive reports whether anything the indicator should reflect is still
// being fetched.
func (m Model) loadingActive() bool {
	if m.newsLoading {
		return true
	}
	if c, ok := m.selectedPkg(); ok {
		if cl, seen := m.clog[c.Name]; seen && cl.loading {
			return true
		}
		if r, seen := m.refs[c.Name]; seen && r.loading {
			return true
		}
	}
	return false
}

// ensureTick starts the animation loop if needed and not already running.
func (m *Model) ensureTick() tea.Cmd {
	if m.tickInFlight {
		return nil
	}
	if !m.scrolling && !m.loadingActive() {
		return nil
	}
	m.tickInFlight = true
	return tickCmd()
}

// maxScroll is the largest valid YOffset for the detail viewport.
func (m Model) maxScroll() float64 {
	max := m.detail.TotalLineCount() - m.detail.Height
	if max < 0 {
		max = 0
	}
	return float64(max)
}

// scrollBy animates the detail pane by delta lines (negative = up).
func (m *Model) scrollBy(delta float64) tea.Cmd {
	m.initSpring()
	base := m.sTarget
	if !m.scrolling {
		base = float64(m.detail.YOffset)
		m.sPos = base
		m.sVel = 0
	}
	m.sTarget = math.Max(0, math.Min(base+delta, m.maxScroll()))
	if m.sTarget == float64(m.detail.YOffset) && !m.scrolling {
		return nil
	}
	m.scrolling = true
	return m.ensureTick()
}

// snapScroll jumps the detail pane to the top, cancelling any animation. Called
// when the detail content changes (new selection / tab / session).
func (m *Model) snapScroll() {
	m.scrolling = false
	m.sPos, m.sVel, m.sTarget = 0, 0, 0
	m.detail.GotoTop()
}

// handleTick advances the spring animations and returns a follow-up tick if any
// animation is still running.
func (m *Model) handleTick() tea.Cmd {
	m.tickInFlight = false
	m.initSpring()

	if m.scrolling {
		m.sPos, m.sVel = m.spring.Update(m.sPos, m.sVel, m.sTarget)
		if math.Abs(m.sPos-m.sTarget) < 0.4 && math.Abs(m.sVel) < 0.4 {
			m.sPos = m.sTarget
			m.scrolling = false
		}
		m.detail.SetYOffset(int(math.Round(m.sPos)))
	}

	if m.loadingActive() {
		// Ping-pong the indicator phase between 0 and 1.
		if math.Abs(m.indPhase-m.indTarget) < 0.06 {
			if m.indTarget == 0 {
				m.indTarget = 1
			} else {
				m.indTarget = 0
			}
		}
		m.indPhase, m.indVel = m.spring.Update(m.indPhase, m.indVel, m.indTarget)
	}

	return m.ensureTick()
}

// indicatorView renders the spring-driven fetch indicator.
func (m Model) indicatorView() string {
	const width = 12
	pos := int(math.Round(m.indPhase * float64(width-1)))
	if pos < 0 {
		pos = 0
	}
	if pos >= width {
		pos = width - 1
	}
	var track []rune
	for i := 0; i < width; i++ {
		if i == pos {
			track = append(track, '█')
		} else {
			track = append(track, '░')
		}
	}
	return indicatorStyle.Render("fetching "+string(track))
}
