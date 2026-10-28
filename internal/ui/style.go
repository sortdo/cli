// Package ui is sortdo's look in a terminal: colors, the logo, the spinner and readable tool results. It is only for a
// person at a terminal. Pipes, files, coding agents, NO_COLOR and TERM=dumb get plain text, unchanged.
package ui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// Style colors text when it is on. The zero Style is plain.
type Style struct{ on bool }

// For returns the style for a stream: on when it is a terminal and colors are allowed. A terminal gets escape
// sequences turned on either way, because the menus move the cursor with them.
func For(file *os.File) Style {
	if os.Getenv("TERM") == "dumb" || !term.IsTerminal(int(file.Fd())) {
		return Style{}
	}
	escapes := enableColors(file)
	if os.Getenv("NO_COLOR") != "" {
		return Style{}
	}
	return Style{on: escapes}
}

// Colored returns a style that is on, for tests.
func Colored() Style { return Style{on: true} }

// On reports whether the style colors text.
func (s Style) On() bool { return s.on }

// Sort's blues, as 256-color codes so that every terminal shows them, from light to dark.
var blues = []int{117, 75, 33}

func (s Style) paint(code, text string) string {
	if !s.on || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(text string) string   { return s.paint("1", text) }
func (s Style) Dim(text string) string    { return s.paint("2", text) }
func (s Style) Accent(text string) string { return s.paint(fmt.Sprintf("38;5;%d", blues[1]), text) }
func (s Style) Green(text string) string  { return s.paint("32", text) }
func (s Style) Red(text string) string    { return s.paint("31", text) }
func (s Style) Yellow(text string) string { return s.paint("33", text) }

// Done marks a finished step: a green check, then the text.
func (s Style) Done(text string) string {
	if !s.on {
		return text
	}
	return s.Green("✓") + " " + text
}

// Tag is a label in reverse video, like a name plate on a panel.
func (s Style) Tag(text string) string { return s.paint("7", " "+text+" ") }

// Status colors a value that names a state: done states green, waiting states yellow, failed states red.
func (s Style) Status(text string) string {
	switch strings.ToLower(text) {
	case "yes", "active", "approved", "published", "done", "completed", "connected", "enabled", "allowed", "ok", "sent":
		return s.Green(text)
	case "pending", "waiting", "draft", "invited", "running", "queued", "scheduled", "paused":
		return s.Yellow(text)
	case "denied", "expired", "failed", "error", "rejected", "disabled", "cancelled", "canceled", "revoked":
		return s.Red(text)
	case "no", "none", "—":
		return s.Dim(text)
	}
	return text
}

var escapes = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

// Width is the number of columns text takes, without its colors.
func Width(text string) int { return utf8.RuneCountInString(escapes.ReplaceAllString(text, "")) }

// Box draws a thin frame around body, with an optional tag on top. When the frame does not fit in width columns,
// the body comes back as it is, after the tag.
func (s Style) Box(tag, body string, width int) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if tag != "" && s.on {
		lines = append([]string{s.Tag(tag), ""}, lines...)
	} else if tag != "" {
		lines = append([]string{tag, ""}, lines...)
	}
	inner := 0
	for _, line := range lines {
		inner = max(inner, Width(line))
	}
	if !s.on || inner+6 > width {
		return strings.Join(lines, "\n") + "\n"
	}
	var out strings.Builder
	out.WriteString(s.Dim("╭"+strings.Repeat("─", inner+4)+"╮") + "\n")
	for _, line := range lines {
		out.WriteString(s.Dim("│") + "  " + line + strings.Repeat(" ", inner-Width(line)) + "  " + s.Dim("│") + "\n")
	}
	out.WriteString(s.Dim("╰"+strings.Repeat("─", inner+4)+"╯") + "\n")
	return out.String()
}

// Logo is Sort's mark (three bars, the top one shorter) beside the name, with up to two lines of text.
func (s Style) Logo(lines ...string) string {
	if !s.on {
		return ""
	}
	bars := []string{"  ▄▄▄▄▄", "▄▄▄▄▄▄▄", "▄▄▄▄▄▄▄"}
	name := []string{"┌─┐┌─┐┬─┐┌┬┐", "└─┐│ │├┬┘ │ ", "└─┘└─┘┴└─ ┴ "}
	var out strings.Builder
	for i := range bars {
		fmt.Fprintf(&out, " %s  %s", s.paint(fmt.Sprintf("38;5;%d", blues[i]), bars[i]), s.Bold(name[i]))
		if i > 0 && i-1 < len(lines) {
			out.WriteString("   " + s.Dim(lines[i-1]))
		}
		out.WriteString("\n")
	}
	return out.String()
}
