package ui

import (
	"bytes"
	"strings"
	"testing"
)

func terminal(keys string) (*Terminal, *bytes.Buffer) {
	var out bytes.Buffer
	return &Terminal{In: strings.NewReader(keys), Out: &out, Width: 80, Height: 24}, &out
}

func TestParseKeys(t *testing.T) {
	keys := parseKeys([]byte("\x1b[A\x1b[Bab\x7f\r\n\x1b\x03é\x1bOA"))
	want := []keyKind{keyUp, keyDown, keyText, keyText, keyBackspace, keyEnter, keyCancel, keyCancel, keyText, keyUp}
	if len(keys) != len(want) {
		t.Fatalf("keys: %+v", keys)
	}
	for i, k := range keys {
		if k.kind != want[i] {
			t.Fatalf("key %d: %+v, want %d", i, k, want[i])
		}
	}
	if keys[8].text != "é" {
		t.Fatalf("text: %q", keys[8].text)
	}
}

func TestPick(t *testing.T) {
	choices := []Choice{{"search", "Find pages."}, {"teams", "View and manage teams."}, {"members", "People in the workspace."}}
	// Down twice and up once lands on teams.
	term, out := terminal("\x1b[B\x1b[B\x1b[A\r")
	if index, err := Colored().Pick(term, "Command", choices); err != nil || index != 1 {
		t.Fatalf("arrows: %d %v", index, err)
	}
	if !strings.Contains(out.String(), "Command") || !strings.HasSuffix(out.String(), "\x1b[?25h") {
		t.Fatalf("drawing: %q", out.String())
	}
	// Typing narrows the list; the hint counts too.
	term, _ = terminal("people\r")
	if index, err := (Style{}).Pick(term, "Command", choices); err != nil || index != 2 {
		t.Fatalf("filter: %d %v", index, err)
	}
	// Up from the first wraps to the last.
	term, _ = terminal("\x1b[A\r")
	if index, _ := (Style{}).Pick(term, "Command", choices); index != 2 {
		t.Fatalf("wrap: %d", index)
	}
	// Esc, Ctrl+C and the end of input cancel.
	for _, keys := range []string{"\x1b", "\x03", ""} {
		term, _ = terminal(keys)
		if _, err := (Style{}).Pick(term, "Command", choices); err != ErrCancelled {
			t.Fatalf("%q: %v", keys, err)
		}
	}
	// Enter with no match does nothing; Backspace brings the list back.
	term, _ = terminal("zz\r\x7f\x7f\r")
	if index, err := (Style{}).Pick(term, "Command", choices); err != nil || index != 0 {
		t.Fatalf("no match: %d %v", index, err)
	}
}

func TestAsk(t *testing.T) {
	// Keys left after one prompt go to the next.
	term, _ := terminal("\rroadmapx\x7f\r")
	if _, err := (Style{}).Pick(term, "Command", []Choice{{"search", ""}}); err != nil {
		t.Fatal(err)
	}
	if value, err := (Style{}).Ask(term, "Search for"); err != nil || value != "roadmap" {
		t.Fatalf("ask: %q %v", value, err)
	}
	term, _ = terminal("abc\x1b")
	if _, err := (Style{}).Ask(term, "Search for"); err != ErrCancelled {
		t.Fatalf("cancel: %v", err)
	}
}
