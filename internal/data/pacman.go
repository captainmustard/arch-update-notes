package data

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DefaultPacmanLog is the standard pacman log path on Arch-based systems.
const DefaultPacmanLog = "/var/log/pacman.log"

// sessionGap is the maximum time between the end of one pacman transaction and
// the start of the next for them to be considered part of the same update run.
const sessionGap = 15 * time.Minute

// pacman log timestamp layout, e.g. 2026-06-26T18:47:30-0500
const pacmanTimeLayout = "2006-01-02T15:04:05-0700"

var (
	lineRe   = regexp.MustCompile(`^\[([^\]]+)\] \[ALPM\] (upgraded|downgraded|reinstalled|installed|removed) (\S+) \((.+)\)$`)
	markerRe = regexp.MustCompile(`^\[([^\]]+)\] \[ALPM\] transaction (started|completed)$`)
)

type transaction struct {
	start   time.Time
	end     time.Time
	changes []PackageChange
}

// ParseSessions reads a pacman log and returns update sessions, oldest first.
// Transactions that occur within sessionGap of each other are merged into a
// single session.
func ParseSessions(logPath string) ([]Session, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var txns []transaction
	var cur *transaction

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()

		if m := markerRe.FindStringSubmatch(line); m != nil {
			ts, _ := time.Parse(pacmanTimeLayout, m[1])
			switch m[2] {
			case "started":
				cur = &transaction{start: ts}
			case "completed":
				if cur != nil {
					cur.end = ts
					txns = append(txns, *cur)
					cur = nil
				}
			}
			continue
		}

		if m := lineRe.FindStringSubmatch(line); m != nil {
			ts, _ := time.Parse(pacmanTimeLayout, m[1])
			pc := parseChange(m[2], m[3], m[4], ts)
			if cur != nil {
				cur.changes = append(cur.changes, pc)
			} else {
				// Defensive: a change outside an explicit transaction block.
				cur = &transaction{start: ts}
				cur.changes = append(cur.changes, pc)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// Flush an unterminated transaction (e.g. log written mid-update).
	if cur != nil && len(cur.changes) > 0 {
		cur.end = cur.changes[len(cur.changes)-1].When
		txns = append(txns, *cur)
	}

	return mergeSessions(txns), nil
}

func parseChange(action, name, detail string, ts time.Time) PackageChange {
	pc := PackageChange{Name: name, Action: Action(action), When: ts}
	if strings.Contains(detail, " -> ") {
		parts := strings.SplitN(detail, " -> ", 2)
		pc.OldVersion = strings.TrimSpace(parts[0])
		pc.NewVersion = strings.TrimSpace(parts[1])
	} else {
		switch pc.Action {
		case Removed:
			pc.OldVersion = strings.TrimSpace(detail)
		default:
			pc.NewVersion = strings.TrimSpace(detail)
		}
	}
	return pc
}

// mergeSessions groups transactions that happened close in time and only keeps
// sessions that actually changed packages. Result is oldest-first.
func mergeSessions(txns []transaction) []Session {
	// Keep only transactions with package changes.
	filtered := txns[:0]
	for _, t := range txns {
		if len(t.changes) > 0 {
			filtered = append(filtered, t)
		}
	}
	txns = filtered

	sort.SliceStable(txns, func(i, j int) bool { return txns[i].start.Before(txns[j].start) })

	var sessions []Session
	for _, t := range txns {
		if n := len(sessions); n > 0 && t.start.Sub(sessions[n-1].Completed) <= sessionGap {
			s := &sessions[n-1]
			s.Completed = t.end
			s.Changes = append(s.Changes, t.changes...)
			continue
		}
		sessions = append(sessions, Session{
			Started:   t.start,
			Completed: t.end,
			Changes:   append([]PackageChange(nil), t.changes...),
		})
	}

	// Within each session, sort changes: upgraded first, then installed,
	// removed, others; alphabetical within each group.
	order := map[Action]int{Upgraded: 0, Installed: 1, Reinstalled: 2, Downgraded: 3, Removed: 4}
	for i := range sessions {
		ch := sessions[i].Changes
		sort.SliceStable(ch, func(a, b int) bool {
			oa, ob := order[ch[a].Action], order[ch[b].Action]
			if oa != ob {
				return oa < ob
			}
			return ch[a].Name < ch[b].Name
		})
	}
	return sessions
}
