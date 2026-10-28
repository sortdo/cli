package ui

import (
	"strings"
	"testing"
)

const teams = `{
  "teams": [
    {"id": "4f0c2a8e-1111-4c3d-9e2f-000000000001", "name": "Everyone", "visibility": "open", "isEveryone": true, "joined": true, "memberCount": 12, "yourAccess": {"read": true, "write": true, "manage": false}},
    {"id": "4f0c2a8e-1111-4c3d-9e2f-000000000002", "name": "Research", "visibility": "private", "description": "Experiments, results and the notes that explain them, for the whole research group.", "isEveryone": false, "joined": false, "memberCount": 3, "yourAccess": {"read": true, "write": false, "manage": false}}
  ],
  "canManageTeams": true
}`

func TestRenderTable(t *testing.T) {
	out, ok := (Style{}).Render(teams, 200, "")
	if !ok {
		t.Fatal("not rendered")
	}
	for _, want := range []string{
		"Teams\n  Name      Visibility  Description",
		"Your access  ID",
		"0000002\n\nCan manage teams: yes",
		"Everyone  open",
		"read, write",
		"4f0c2a8e-1111-4c3d-9e2f-000000000002",
		"Can manage teams: yes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("a plain style printed colors:\n%s", out)
	}
	// A narrow terminal shortens the description but never an ID.
	narrow, _ := (Style{}).Render(teams, 150, "")
	if !strings.Contains(narrow, "…") || !strings.Contains(narrow, "4f0c2a8e-1111-4c3d-9e2f-000000000002") {
		t.Errorf("narrow table:\n%s", narrow)
	}
	for _, line := range strings.Split(narrow, "\n") {
		if len([]rune(line)) > 150 {
			t.Errorf("line wider than 150: %q", line)
		}
	}
	// When even shortened columns cannot fit, each item is listed on its own, under its name.
	tiny, _ := (Style{}).Render(teams, 80, "")
	if !strings.Contains(tiny, "  Research\n    ID:") || !strings.Contains(tiny, "    Member count: 3") || strings.Contains(tiny, "Name:") {
		t.Errorf("tiny listing:\n%s", tiny)
	}
}

func TestRenderTitle(t *testing.T) {
	// The frame shows the tool's name, so the section with the same name loses its heading.
	out, _ := (Style{}).Render(teams, 200, "Teams")
	if strings.HasPrefix(out, "Teams") || !strings.HasPrefix(out, "Name") {
		t.Errorf("titled:\n%s", out)
	}
	boxed := Colored().Box("Teams", out, 220)
	if !strings.Contains(boxed, "╭") || !strings.Contains(boxed, "\x1b[7m Teams ") {
		t.Errorf("box:\n%s", boxed)
	}
	if narrow := Colored().Box("Teams", out, 60); strings.Contains(narrow, "╭") {
		t.Errorf("a box wider than the terminal was drawn:\n%s", narrow)
	}
}

func TestRenderOther(t *testing.T) {
	if _, ok := (Style{}).Render("Created page Plans with id: 1", 80, ""); ok {
		t.Error("plain text was rendered")
	}
	if _, ok := (Style{}).Render(`{"a": 1} trailing`, 80, ""); ok {
		t.Error("JSON with trailing text was rendered")
	}
	out, _ := (Style{}).Render(`{"title":"Plans","content":"# Plans\nLine two","tags":["a","b"],"owner":null,"empty":[]}`, 80, "")
	for _, want := range []string{"Title: Plans", "Tags:  a, b", "Owner: —", "Empty: None", "Content\n  # Plans\n  Line two"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if label("pageId") != "Page ID" || label("canManageTeams") != "Can manage teams" || label("mcp_url") != "MCP URL" {
		t.Errorf("labels: %q %q %q", label("pageId"), label("canManageTeams"), label("mcp_url"))
	}
}

func TestPlainStyle(t *testing.T) {
	if (Style{}).Logo("x") != "" || (Style{}).Bold("a") != "a" {
		t.Error("a plain style added output")
	}
	if !strings.Contains(Colored().Bold("a"), "\x1b[1m") {
		t.Error("a colored style printed no colors")
	}
	frames := sortingFrames()
	last := frames[len(frames)-1]
	for i := 1; i < len(last); i++ {
		if last[i-1] > last[i] {
			t.Errorf("the spinner does not end sorted: %v", last)
		}
	}
}
