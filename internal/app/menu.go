package app

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/ui"
)

// noPromptVariables are set by CI services and by coding agents that run commands in a terminal. Where one is set,
// sortdo never waits for a key: it prints help or an error as it does for scripts. SORTDO_NO_INPUT=1 does the same.
var noPromptVariables = []string{"SORTDO_NO_INPUT", "CI", "CLAUDECODE", "CODEX_SANDBOX", "CODEX_CI", "CURSOR_AGENT", "GEMINI_CLI", "AGENT"}

// promptsAllowed is true when a person is at a terminal, so a menu can wait for their keys.
func (a *App) promptsAllowed() bool {
	if !a.Interactive || os.Getenv("TERM") == "dumb" {
		return false
	}
	for _, name := range noPromptVariables {
		if os.Getenv(name) != "" {
			return false
		}
	}
	return true
}

func (a *App) terminal() *ui.Terminal {
	if a.term == nil {
		a.term = &ui.Terminal{In: a.Stdin, Out: a.Stdout, Raw: a.Raw, Width: max(a.Width, 40), Height: max(a.Height, 10)}
	}
	return a.term
}

// The built-in commands in the menu, in order. call needs JSON and version adds nothing, so they are left out.
var menuBuiltins = []string{"search", "read", "pages", "approval", "status", "tools", "skill", "token", "update", "uninstall", "logout"}

// menu lets a person choose a command with the arrow keys, then its action and what it needs, and runs it.
func (a *App) menu() error {
	fmt.Fprint(a.Stdout, "\n"+a.Out.Logo(a.Version, "Sort from the command line")+"\n")
	signedIn := client.TokenSource(a.origin) != ""
	names := []string{}
	if !signedIn {
		names = append(names, "login")
	}
	names = append(names, menuBuiltins...)
	hints := map[string]string{}
	for _, name := range names {
		hints[name] = builtins[name].summary
	}
	if signedIn {
		var tools []client.Tool
		_ = a.wait("Loading commands…", func() (err error) {
			tools, err = loadTools(a.origin, false)
			return err
		})
		var workspace []string
		for name, command := range toolCommands(tools) {
			if _, taken := builtins[name]; !taken {
				workspace = append(workspace, name)
				hints[name] = firstSentence(command.tool.Description)
			}
		}
		sort.Strings(workspace)
		names = append(names, workspace...)
	}
	choices := make([]ui.Choice, len(names))
	for i, name := range names {
		choices[i] = ui.Choice{Label: name, Hint: hints[name]}
	}
	index, err := a.Out.Pick(a.terminal(), "Command", choices)
	if err != nil {
		return cancelled(err)
	}
	name := names[index]
	switch name {
	case "search", "read", "approval":
		question := map[string]string{"search": "Search for", "read": "Page ID or URL", "approval": "Approval ID"}[name]
		value, err := a.Out.Ask(a.terminal(), question)
		if err != nil || strings.TrimSpace(value) == "" {
			return cancelled(err)
		}
		return a.runChosen(name, value)
	case "skill":
		actions := []string{"install", "uninstall", "print"}
		index, err := a.Out.Pick(a.terminal(), "Action", []ui.Choice{
			{Label: "install", Hint: "Teach your coding agents to use sortdo."}, {Label: "uninstall", Hint: "Remove the skill."}, {Label: "print", Hint: "Show the skill."},
		})
		if err != nil {
			return cancelled(err)
		}
		return a.runChosen("skill", actions[index])
	}
	if _, ok := builtins[name]; ok && name != "pages" {
		return a.runChosen(name)
	}
	command, err := a.findToolCommand(name)
	if err != nil {
		return err
	}
	return a.chooseAction(name, command)
}

// chooseAction lets a person choose the action of a tool command, asks for its required options, and runs it.
func (a *App) chooseAction(name string, command *toolCommand) error {
	var choices []ui.Choice
	if name == "pages" {
		choices = append(choices, ui.Choice{Label: "create", Hint: "Create a page."})
	}
	for _, action := range command.actions {
		choices = append(choices, ui.Choice{Label: actionName(action)})
	}
	index, err := a.Out.Pick(a.terminal(), "Action", choices)
	if err != nil {
		return cancelled(err)
	}
	args := []string{choices[index].Label}
	if name == "pages" && args[0] == "create" {
		title, err := a.Out.Ask(a.terminal(), "Title")
		if err != nil || strings.TrimSpace(title) == "" {
			return cancelled(err)
		}
		return a.runChosen("pages", "create", "--title", title)
	}
	for _, o := range command.options() {
		if !o.required {
			continue
		}
		var value string
		if len(o.enum) > 0 {
			options := make([]ui.Choice, len(o.enum))
			for i, item := range o.enum {
				options[i] = ui.Choice{Label: item}
			}
			index, err := a.Out.Pick(a.terminal(), "--"+o.flag, options)
			if err != nil {
				return cancelled(err)
			}
			value = o.enum[index]
		} else if value, err = a.Out.Ask(a.terminal(), "--"+o.flag); err != nil || value == "" {
			return cancelled(err)
		}
		args = append(args, "--"+o.flag, value)
	}
	return a.runChosen(name, args...)
}

// runChosen shows the command that the menus built, so the person learns it, and runs it.
func (a *App) runChosen(name string, args ...string) error {
	words := append([]string{"sortdo", name}, args...)
	for i, word := range words {
		if word == "" || strings.ContainsAny(word, " \t\"'$`\\|&;<>()*?") {
			words[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
		}
	}
	fmt.Fprintln(a.Stdout, a.Out.Dim("› "+strings.Join(words, " ")+client.ServerFlag(a.origin)))
	return a.dispatch(name, args)
}

// cancelled ends a menu quietly when the person left it, or with nothing given.
func cancelled(err error) error {
	if err == nil || errors.Is(err, ui.ErrCancelled) {
		return nil
	}
	return err
}
