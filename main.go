// Command arch-update-notes is a terminal UI that gathers the notes for your
// most recent system update on Arch-based distributions (CachyOS, Arch, etc.):
// which packages changed, relevant Arch/CachyOS news, pending .pacnew config
// files, and per-package changelogs.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ianataylor42/arch-update-notes/internal/data"
	"github.com/ianataylor42/arch-update-notes/internal/tui"
)

func main() {
	logPath := flag.String("log", data.DefaultPacmanLog, "path to the pacman log")
	noNews := flag.Bool("no-news", false, "skip fetching online news feeds")
	flag.Parse()

	sessions, err := data.ParseSessions(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "arch-update-notes: cannot read pacman log %q: %v\n", *logPath, err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "arch-update-notes: no update sessions found in the pacman log.")
		os.Exit(1)
	}

	pacnew, _ := data.PacnewFiles()

	feeds := data.DefaultFeeds
	if *noNews {
		feeds = nil
	}

	m := tui.New(sessions, pacnew, feeds, !*noNews)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "arch-update-notes: %v\n", err)
		os.Exit(1)
	}
}
