package changelog

import (
	"strings"
	"testing"
)

const sample = `# Changelog

## Unreleased

- New: sortdo pages move.

## 0.1.0 (2026-10-28)

- First release.
`

func TestCutAndSection(t *testing.T) {
	cut, err := Cut(sample, "0.2.0", "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cut, "## Unreleased\n\n## 0.2.0 (2026-11-02)\n\n- New: sortdo pages move.\n\n## 0.1.0") {
		t.Fatalf("cut:\n%s", cut)
	}
	if notes, err := Section(cut, "0.2.0"); err != nil || notes != "- New: sortdo pages move." {
		t.Fatalf("Section 0.2.0 = %q, %v", notes, err)
	}
	if notes, err := Section(cut, "0.1.0"); err != nil || notes != "- First release." {
		t.Fatalf("Section 0.1.0 = %q, %v", notes, err)
	}
	// Nothing new: the next release must say what changed first.
	if _, err := Cut(cut, "0.2.1", "2026-11-03"); err == nil || !strings.Contains(err.Error(), "nothing under") {
		t.Fatalf("an empty Unreleased section stops the release: %v", err)
	}
	if _, err := Cut(sample, "0.1.0", "2026-11-03"); err == nil || !strings.Contains(err.Error(), "already has a section") {
		t.Fatalf("a version is released once: %v", err)
	}
	if _, err := Section(sample, "0.1"); err == nil {
		t.Fatal("0.1 is not 0.1.0")
	}
	if _, err := Cut("# Changelog\n", "1.0.0", "2026-11-03"); err == nil {
		t.Fatal("a changelog needs an Unreleased section")
	}
}
