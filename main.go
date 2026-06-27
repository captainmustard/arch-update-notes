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
	zone "github.com/lrstanley/bubblezone"

	"github.com/captainmustard/arch-update-notes/internal/data"
	"github.com/captainmustard/arch-update-notes/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logPath := flag.String("log", data.DefaultPacmanLog, "path to the pacman log")
	noNews := flag.Bool("no-news", false, "skip fetching online news feeds")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("arch-update-notes %s\n", version)
		return
	}

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
	snaps := data.LoadSnapshots("root")

	feeds := data.DefaultFeeds
	if *noNews {
		feeds = nil
	}

	zone.NewGlobal()
	m := tui.New(sessions, pacnew, feeds, snaps, !*noNews)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "arch-update-notes: %v\n", err)
		os.Exit(1)
	}
}
