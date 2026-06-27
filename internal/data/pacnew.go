package data

import (
	"os/exec"
	"sort"
	"strings"
)

// PacnewFiles returns the list of pending .pacnew / .pacsave files on the
// system using `pacdiff -o` (from the pacman-contrib package). If pacdiff is
// unavailable it returns an empty list rather than an error.
func PacnewFiles() ([]string, error) {
	path, err := exec.LookPath("pacdiff")
	if err != nil {
		return nil, nil
	}
	out, err := exec.Command(path, "-o").Output()
	if err != nil {
		// pacdiff exits non-zero in some edge cases; treat output as truth.
		if len(out) == 0 {
			return nil, nil
		}
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	sort.Strings(files)
	return files, nil
}
