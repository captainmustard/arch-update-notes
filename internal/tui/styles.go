package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.Color("39")  // blue
	colNew    = lipgloss.Color("42")  // green
	colMuted  = lipgloss.Color("245") // grey
	colWarn   = lipgloss.Color("214") // orange
	colRemove = lipgloss.Color("203") // red
	colBg     = lipgloss.Color("236")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(colAccent).
			Padding(0, 1)

	tabStyle = lipgloss.NewStyle().
			Foreground(colMuted).
			Padding(0, 2)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(colBg).
			Padding(0, 2)

	sessionStyle = lipgloss.NewStyle().Foreground(colMuted)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colMuted)

	paneActiveStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent)

	detailTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	labelStyle       = lipgloss.NewStyle().Bold(true).Foreground(colMuted)
	newTagStyle      = lipgloss.NewStyle().Bold(true).Foreground(colNew)
	footerStyle      = lipgloss.NewStyle().Foreground(colMuted)
	indicatorStyle   = lipgloss.NewStyle().Foreground(colAccent)
	linkStyle        = lipgloss.NewStyle().Foreground(colAccent).Underline(true)
)

// actionColor maps a pacman action to a display colour.
func actionColor(a string) lipgloss.Color {
	switch a {
	case "upgraded":
		return colAccent
	case "installed":
		return colNew
	case "removed":
		return colRemove
	case "downgraded":
		return colWarn
	default:
		return colMuted
	}
}

// actionGlyph returns a short symbol for an action.
func actionGlyph(a string) string {
	switch a {
	case "upgraded":
		return "↑"
	case "installed":
		return "+"
	case "removed":
		return "−"
	case "downgraded":
		return "↓"
	case "reinstalled":
		return "↻"
	default:
		return "•"
	}
}
