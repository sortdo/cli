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
	"github.com/sortdo/cli/internal/ui"
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
	// Readable is true when stdout is a terminal: tool results are then tables and "Label: value" lines, not JSON.
	Readable bool
	// Width and Height are the terminal's size, for tables and menus.
	Width, Height int
	// Raw puts the terminal in raw mode for the arrow-key menus and returns the function that restores it.
	Raw func() (restore func(), err error)
	// Out and Err color stdout and stderr. They are plain for pipes, files and agents, and with NO_COLOR.
	Out, Err ui.Style

	origin string
	json   bool
	failed bool // a tool answered with isError: exit code 1
	term   *ui.Terminal
	title  string // the tool's name, on the frame around its result
}

// New returns an App on the process's own streams.
func New(version string, skill []byte) *App {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width, height = 100, 24
	}
	readable := term.IsTerminal(int(os.Stdout.Fd()))
	input := int(os.Stdin.Fd())
	return &App{
		Version: version, Skill: skill, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Interactive: readable && term.IsTerminal(input),
		Readable:    readable, Width: width, Height: height, Out: ui.For(os.Stdout), Err: ui.For(os.Stderr),
		Raw: func() (func(), error) {
			state, err := term.MakeRaw(input)
			if err != nil {
				return nil, err
			}
			return func() { _ = term.Restore(input, state) }, nil
		},
	}
}

// wait runs work with the spinner on stderr, when a person watches it.
func (a *App) wait(label string, work func() error) error {
	spinner := a.Err.Spin(a.Stderr, label)
	defer spinner.Stop()
	return work()
}

// callTool calls a server tool while the spinner runs.
func (a *App) callTool(name string, arguments map[string]any) (result *client.Result, err error) {
	err = a.wait("Sorting…", func() error {
		result, err = client.CallTool(a.origin, name, arguments)
		return err
	})
	return result, err
}

// Run runs one command and returns the exit code.
func (a *App) Run(argv []string) int {
	client.Version = a.Version
	notice := startUpdateNotice(a)
	err := a.run(argv)
	if err != nil {
		var known *client.Error
		if errors.As(err, &known) {
			fmt.Fprintln(a.Stderr, a.Err.Red(known.Message))
		} else {
			fmt.Fprintln(a.Stderr, a.Err.Red(fmt.Sprintf("Something went wrong: %v", err)))
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
		if a.promptsAllowed() && !g.help {
			return a.menu()
		}
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
	return a.dispatch(name, rest)
}

func (a *App) dispatch(name string, rest []string) error {
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
		if len(rest) == 0 && a.promptsAllowed() {
			return a.chooseAction(name, command)
		}
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
	result, err := a.callTool(command.tool.Name, arguments)
	if err != nil {
		return err
	}
	a.title = command.tool.Title
	return a.printResult(result)
}

func (a *App) findToolCommand(name string) (*toolCommand, error) {
	var tools []client.Tool
	err := a.wait("Loading commands…", func() (err error) {
		tools, err = loadTools(a.origin, false)
		return err
	})
	if err != nil {
		if client.TokenSource(a.origin) == "" {
			return nil, client.Errorf("Sign in to use the workspace commands, such as %s: sortdo login%s", name, client.ServerFlag(a.origin))
		}
		return nil, client.Errorf("Could not load the commands for %q: %s", name, err.Error())
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

// printResult prints a tool's text, or its data with --json. At a terminal, JSON text is shown readably. A tool error
// makes the exit code 1.
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
	if result.IsError {
		fmt.Fprintln(a.Stdout, a.Out.Red(result.Text))
		return nil
	}
	if a.Readable {
		if text, ok := a.Out.Render(result.Text, a.Width-6, a.title); ok {
			// A frame helps a short result; a long one stays bare, so it copies cleanly.
			if a.title != "" && strings.Count(text, "\n") <= 40 {
				text = a.Out.Box(a.title, text, a.Width)
			}
			fmt.Fprint(a.Stdout, text)
			return nil
		}
	}
	fmt.Fprintln(a.Stdout, result.Text)
	return nil
}
