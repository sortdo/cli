// changelog prepares release notes from CHANGELOG.md.
//
//	go run ./tools/changelog cut <version>     give the Unreleased notes this version's heading (fails when empty)
//	go run ./tools/changelog notes <version>   print the notes of a released version
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/sortdo/cli/internal/changelog"
)

const file = "CHANGELOG.md"

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "Usage: changelog cut <version> | changelog notes <version>")
		os.Exit(2)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fail(err)
	}
	version := os.Args[2]
	switch os.Args[1] {
	case "cut":
		updated, err := changelog.Cut(string(data), version, time.Now().UTC().Format("2006-01-02"))
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(file, []byte(updated), 0o644); err != nil {
			fail(err)
		}
	case "notes":
		notes, err := changelog.Section(string(data), version)
		if err != nil {
			fail(err)
		}
		fmt.Println(notes)
	default:
		fail(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "changelog:", err)
	os.Exit(1)
}
