package data

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Snapshot is a single snapper snapshot.
type Snapshot struct {
	Number      int
	Type        string // "single" | "pre" | "post"
	Date        time.Time
	Description string
	Cleanup     string
	User        string
	Userdata    map[string]string
}

// SnapPair is a snap-pac pre/post snapshot pair wrapping one pacman
// transaction. Post is zero-valued if the post snapshot is missing.
type SnapPair struct {
	Pre  Snapshot
	Post Snapshot
}

// HasPost reports whether the pair has a completed post snapshot.
func (p SnapPair) HasPost() bool { return p.Post.Number != 0 }

// When is the time the pre snapshot was taken (≈ start of the transaction).
func (p SnapPair) When() time.Time { return p.Pre.Date }

// Command is the pacman command that triggered the snapshot (snap-pac stores it
// on the pre snapshot).
func (p SnapPair) Command() string { return p.Pre.Description }

// Summary is the affected-package list (snap-pac stores it on the post
// snapshot), falling back to the command.
func (p SnapPair) Summary() string {
	if p.Post.Description != "" {
		return p.Post.Description
	}
	return p.Pre.Description
}

// SnapshotInfo is the result of querying snapper.
type SnapshotInfo struct {
	Available bool       // true if snapshots could be read
	Reason    string     // why unavailable (permissions, not installed, …)
	Config    string     // snapper config queried
	Pairs     []SnapPair // pre/post pairs, newest first
}

const snapTimeLayout = "2006-01-02 15:04:05"

type snapRaw struct {
	Number      int               `json:"number"`
	Type        string            `json:"type"`
	Date        string            `json:"date"`
	Description string            `json:"description"`
	Cleanup     string            `json:"cleanup"`
	User        string            `json:"user"`
	PreNumber   *int              `json:"pre-number"`
	Userdata    map[string]string `json:"userdata"`
}

// LoadSnapshots queries snapper for the given config (default "root") and
// returns its pre/post pairs. It never errors hard: when snapper is missing or
// unreadable (needs root), Available is false with a Reason.
func LoadSnapshots(config string) SnapshotInfo {
	if config == "" {
		config = "root"
	}
	info := SnapshotInfo{Config: config}

	if _, err := exec.LookPath("snapper"); err != nil {
		info.Reason = "snapper is not installed"
		return info
	}

	out, err := exec.Command("snapper", "--jsonout", "-c", config, "list").Output()
	if err != nil {
		msg := strings.TrimSpace(string(exitStderr(err)))
		switch {
		case strings.Contains(msg, "permission") || strings.Contains(msg, "permissions"):
			info.Reason = "needs root to read snapshots (run with sudo, or set snapper ALLOW_GROUPS)"
		case msg != "":
			info.Reason = msg
		default:
			info.Reason = "could not read snapshots"
		}
		return info
	}

	var doc map[string][]snapRaw
	if err := json.Unmarshal(out, &doc); err != nil {
		info.Reason = "could not parse snapper output"
		return info
	}
	rows, ok := doc[config]
	if !ok {
		for _, v := range doc { // fall back to whatever config came back
			rows = v
			break
		}
	}

	info.Available = true
	info.Pairs = pairSnapshots(rows)
	return info
}

func pairSnapshots(rows []snapRaw) []SnapPair {
	byNum := make(map[int]Snapshot, len(rows))
	for _, r := range rows {
		byNum[r.Number] = toSnapshot(r)
	}
	var pairs []SnapPair
	for _, r := range rows {
		if r.Type == "post" && r.PreNumber != nil {
			pre := byNum[*r.PreNumber]
			pairs = append(pairs, SnapPair{Pre: pre, Post: byNum[r.Number]})
		}
	}
	// Include pre snapshots that have no matching post (interrupted updates).
	paired := map[int]bool{}
	for _, p := range pairs {
		paired[p.Pre.Number] = true
	}
	for _, r := range rows {
		if r.Type == "pre" && !paired[r.Number] {
			pairs = append(pairs, SnapPair{Pre: byNum[r.Number]})
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].Pre.Date.After(pairs[j].Pre.Date) })
	return pairs
}

func toSnapshot(r snapRaw) Snapshot {
	s := Snapshot{
		Number:      r.Number,
		Type:        r.Type,
		Description: r.Description,
		Cleanup:     r.Cleanup,
		User:        r.User,
		Userdata:    r.Userdata,
	}
	if r.Date != "" {
		if t, err := time.ParseInLocation(snapTimeLayout, r.Date, time.Local); err == nil {
			s.Date = t
		}
	}
	return s
}

// PairsForSession returns the snapshot pairs that wrap a given update session,
// matched by time (a snap-pac pre snapshot is taken just before the
// transaction and the post just after).
func PairsForSession(s Session, pairs []SnapPair) []SnapPair {
	const slack = 10 * time.Minute
	lo := s.Started.Add(-slack)
	hi := s.Completed.Add(slack)
	var out []SnapPair
	for _, p := range pairs {
		if p.Pre.Date.IsZero() {
			continue
		}
		if !p.Pre.Date.Before(lo) && !p.Pre.Date.After(hi) {
			out = append(out, p)
		}
	}
	return out
}

// exitStderr extracts stderr from an *exec.ExitError, if present.
func exitStderr(err error) []byte {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Stderr
	}
	return nil
}
