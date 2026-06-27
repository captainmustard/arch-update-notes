package data

import (
	"os/exec"
	"strings"
)

// Changelog returns the changelog for an installed package via `pacman -Qc`.
// Many packages ship no changelog; in that case ok is false.
func Changelog(pkg string) (text string, ok bool) {
	out, err := exec.Command("pacman", "-Qc", pkg).Output()
	if err != nil {
		return "", false
	}
	text = strings.TrimSpace(string(out))
	if text == "" {
		return "", false
	}
	return text, true
}
