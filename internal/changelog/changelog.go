// Package changelog reads and updates CHANGELOG.md: each change adds a line under "## Unreleased", and a release
// turns that section into "## X.Y.Z (date)", whose text becomes the release notes.
package changelog

import (
	"errors"
	"fmt"
	"strings"
)

const unreleased = "## Unreleased"

func isHeading(line string) bool { return strings.HasPrefix(line, "## ") }

// isVersionHeading reports whether line is the heading of version, such as "## 1.2.3" or "## 1.2.3 (2026-10-28)".
func isVersionHeading(line, version string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "## "+version)
	return ok && (rest == "" || strings.HasPrefix(rest, " "))
}

// body returns the lines after heading index start, up to the next "## " heading, trimmed.
func body(lines []string, start int) string {
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		if isHeading(lines[index]) {
			end = index
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
}

// Section returns the notes of a released version.
func Section(text, version string) (string, error) {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if isVersionHeading(line, version) {
			notes := body(lines, index)
			if notes == "" {
				return "", fmt.Errorf("CHANGELOG.md has no notes under %s", version)
			}
			return notes, nil
		}
	}
	return "", fmt.Errorf("CHANGELOG.md has no section for %s", version)
}

// Cut gives the Unreleased notes the version's heading and puts an empty Unreleased section above them.
func Cut(text, version, date string) (string, error) {
	lines := strings.Split(text, "\n")
	start := -1
	for index, line := range lines {
		if isVersionHeading(line, version) {
			return "", fmt.Errorf("CHANGELOG.md already has a section for %s", version)
		}
		if strings.TrimSpace(line) == unreleased && start < 0 {
			start = index
		}
	}
	if start < 0 {
		return "", errors.New("CHANGELOG.md has no \"## Unreleased\" section")
	}
	if body(lines, start) == "" {
		return "", errors.New("CHANGELOG.md has nothing under \"## Unreleased\". Say what changed for users, then release")
	}
	out := append([]string{}, lines[:start]...)
	out = append(out, unreleased, "", fmt.Sprintf("## %s (%s)", version, date))
	out = append(out, lines[start+1:]...)
	return strings.Join(out, "\n"), nil
}
