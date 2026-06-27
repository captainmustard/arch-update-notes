// Package data collects information about the most recent system update(s)
// from the local machine: the pacman log, pending .pacnew files, package
// changelogs, and distribution news feeds.
package data

import "time"

// Action describes what pacman did to a package in a transaction.
type Action string

const (
	Upgraded    Action = "upgraded"
	Downgraded  Action = "downgraded"
	Installed   Action = "installed"
	Removed     Action = "removed"
	Reinstalled Action = "reinstalled"
)

// PackageChange is a single package operation within an update session.
type PackageChange struct {
	Name       string
	Action     Action
	OldVersion string // empty for installed/removed
	NewVersion string // empty for removed
	When       time.Time
}

// Version renders the version transition for display.
func (p PackageChange) Version() string {
	switch p.Action {
	case Upgraded, Downgraded:
		return p.OldVersion + " → " + p.NewVersion
	case Removed:
		return p.OldVersion
	default:
		return p.NewVersion
	}
}

// Session is one logical update run: one or more pacman transactions that
// happened close together in time (e.g. repo packages then AUR packages from
// a single `cachy-update` invocation).
type Session struct {
	Started   time.Time
	Completed time.Time
	Changes   []PackageChange
}

// Counts summarises the actions in a session.
func (s Session) Counts() (upgraded, installed, removed, other int) {
	for _, c := range s.Changes {
		switch c.Action {
		case Upgraded:
			upgraded++
		case Installed:
			installed++
		case Removed:
			removed++
		default:
			other++
		}
	}
	return
}

// NewsItem is a single news/announcement entry from a feed.
type NewsItem struct {
	Title   string
	Link    string
	Date    time.Time
	Summary string // plain-text, HTML stripped
	Source  string // e.g. "Arch Linux"
}
