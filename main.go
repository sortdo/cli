// sortdo is Sort from the command line. Every command runs the same tools an AI app gets through Sort's MCP server,
// as the signed-in member, with the same permission checks. https://docs.trysort.com/cli
package main

import (
	_ "embed"
	"os"

	"github.com/sortdo/cli/internal/app"
	"github.com/sortdo/cli/internal/selfupdate"
)

// version is set at release time: go build -ldflags "-X main.version=1.2.3".
var version = "dev"

//go:embed SKILL.md
var skill []byte

func main() {
	if executable, err := os.Executable(); err == nil {
		selfupdate.RemoveOld(executable)
	}
	os.Exit(app.New(version, skill).Run(os.Args[1:]))
}
