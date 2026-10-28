// Package app is sortdo's command line: argument handling, the built-in commands, and one command per server tool.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sortdo/cli/internal/client"
	"golang.org/x/term"
)

// App holds what a run needs, so tests can replace the streams and the version.
type App struct {
	Version string
	Skill   []byte // the embedded SKILL.md
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	// Interactive is true when a person is at a terminal: login then opens the browser and waits.
	Interactive bool

	origin string
	json   bool
	failed bool // a tool answered with isError: exit code 1
}

// New returns an App on the process's own streams.
func New(version string, skill []byte) *App {
	return &App{
		Version: version, Skill: skill, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Interactive: term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd())),
	}
}

// Run runs one command and returns the exit code.
func (a *App) Run(argv []string) int {
	client.Version = a.Version
	notice := startUpdateNotice(a)
	err := a.run(argv)
	if err != nil {
		var known *client.Error
		if errors.As(err, &known) {
			fmt.Fprintln(a.Stderr, known.Message)
		} else {
			fmt.Fprintf(a.Stderr, "Something went wrong: %v\n", err)
		}
	}
	notice()
	if err != nil || a.failed {
		return 1
	}
	return 0
}

func (a *App) run(argv []string) error {
	g, err := splitGlobals(argv)
	if err != nil {
		return err
	}
	if g.version && len(g.rest) == 0 {
		fmt.Fprintf(a.Stdout, "sortdo %s\n", a.Version)
		return nil
	}
	server := g.server
	if server == "" {
		server = os.Getenv("SORTDO_SERVER")
	}
	if server == "" {
		server = client.DefaultServer
	}
	if a.origin, err = client.NormalizeServer(server); err != nil {
		return err
	}
	a.json = g.json
	if len(g.rest) == 0 {
		fmt.Fprint(a.Stdout, a.rootHelp())
		return nil
	}
	name, rest := g.rest[0], g.rest[1:]
	if name == "help" {
		return a.help(rest)
	}
	if g.help {
		return a.help(g.rest)
	}
	if builtin, ok := builtins[name]; ok {
		return builtin.run(a, rest)
	}
	return a.runTool(name, rest)
}

// runTool runs a server tool as a command. An unknown name refreshes the tool list once, for tools added lately.
func (a *App) runTool(name string, rest []string) error {
	command, err := a.findToolCommand(name)
	if err != nil {
		return err
	}
	action := ""
	if len(command.actions) > 0 {
		if len(rest) == 0 || strings.HasPrefix(rest[0], "--") {
			return client.Errorf("Choose an action: sortdo %s <action>. See: sortdo %s --help", name, name)
		}
		var ok bool
		if action, ok = command.resolveAction(rest[0]); !ok {
			return client.Errorf("Unknown action %q. See: sortdo %s --help", rest[0], name)
		}
		rest = rest[1:]
	}
	arguments, err := command.parseToolArguments(rest, action, a.Stdin)
	if err != nil {
		return err
	}
	result, err := client.CallTool(a.origin, command.tool.Name, arguments)
	if err != nil {
		return err
	}
	return a.printResult(result)
}

func (a *App) findToolCommand(name string) (*toolCommand, error) {
	tools, err := loadTools(a.origin, false)
	if err != nil {
		return nil, client.Errorf("Unknown command %q. Run sortdo help. (%s)", name, err.Error())
	}
	if command, ok := toolCommands(tools)[name]; ok {
		return command, nil
	}
	if tools, err = loadTools(a.origin, true); err == nil {
		if command, ok := toolCommands(tools)[name]; ok {
			return command, nil
		}
	}
	return nil, client.Errorf("Unknown command %q. Run sortdo help.", name)
}

func (a *App) printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, string(data))
	return nil
}

// printResult prints a tool's text, or its data with --json. A tool error makes the exit code 1.
func (a *App) printResult(result *client.Result) error {
	if result.IsError {
		a.failed = true
	}
	if a.json {
		if result.Structured != nil {
			return a.printJSON(result.Structured)
		}
		return a.printJSON(result.Data())
	}
	fmt.Fprintln(a.Stdout, result.Text)
	return nil
}
