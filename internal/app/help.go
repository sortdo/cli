package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sortdo/cli/internal/client"
)

func jsonObject(raw string, target *map[string]any) error {
	if json.Unmarshal([]byte(raw), target) != nil || *target == nil {
		return client.Errorf(`The arguments must be a JSON object, for example '{"action":"list"}'.`)
	}
	return nil
}

func jsonValue(raw string, target any) error { return json.Unmarshal([]byte(raw), target) }

var helpGroups = []struct {
	title    string
	commands []string
}{
	{"Start", []string{"login", "status", "logout"}},
	{"Read and write", []string{"search", "read", "pages", "approval"}},
	{"More", []string{"tools", "call", "token", "skill", "update", "uninstall", "version"}},
}

// brief is a command with its arguments, when they are short enough for the overview.
func brief(name string) string {
	usage, _, _ := strings.Cut(builtins[name].usage, "\n")
	if len(usage) <= 26 {
		return usage
	}
	return name
}

// rootHelp lists the built-in commands, and the workspace commands when the tool list is cached.
func (a *App) rootHelp() string {
	var out strings.Builder
	style := a.Out
	if style.On() {
		fmt.Fprintf(&out, "\n%s\n%s sortdo <command> [options]\n", style.Logo(a.Version, "Sort from the command line"), style.Bold("Usage:"))
	} else {
		fmt.Fprintln(&out, "sortdo — Sort from the command line\n\nUsage: sortdo <command> [options]")
	}
	for _, group := range helpGroups {
		fmt.Fprintf(&out, "\n%s\n", style.Bold(group.title+":"))
		rows := make([][2]string, 0, len(group.commands))
		for _, name := range group.commands {
			rows = append(rows, [2]string{brief(name), builtins[name].summary})
		}
		writeRows(&out, style, rows)
	}
	fmt.Fprintf(&out, "\n%s\n", style.Bold("Workspace commands (one for each tool on the server):"))
	if tools := cachedTools(a.origin); len(tools) > 0 {
		var names []string
		for name := range toolCommands(tools) {
			if _, taken := builtins[name]; !taken {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		fmt.Fprintln(&out, style.Accent(wrap(strings.Join(names, "  "), 2)))
		fmt.Fprintln(&out, "  sortdo <command> --help shows the actions and options.")
	} else {
		fmt.Fprintln(&out, "  teams, members, comments, inbox, email, schedules and more. Run sortdo tools to list them.")
	}
	fmt.Fprintf(&out, `
%s --server <origin> (default %s, or SORTDO_SERVER), --json, --help, --version
%s SORTDO_TOKEN (an access token to use), SORTDO_KEYRING=0 (keep the sign-in in a file), SORTDO_CONFIG_DIR, SORTDO_NO_INPUT=1 (no menus), NO_COLOR
%s https://docs.trysort.com/cli
`, style.Bold("Options:"), client.DefaultServer, style.Bold("Environment:"), style.Bold("Docs:"))
	return out.String()
}

// help prints help for a built-in command, a tool command, or one of its actions.
func (a *App) help(words []string) error {
	if len(words) == 0 {
		fmt.Fprint(a.Stdout, a.rootHelp())
		return nil
	}
	name := words[0]
	if builtin, ok := builtins[name]; ok && !(name == "pages" && len(words) > 1 && words[1] != "create" && words[1] != "edit") {
		fmt.Fprintf(a.Stdout, "%s\n\n%s\n", a.Out.Bold("sortdo "+builtin.usage), builtin.summary)
		if builtin.details != "" {
			fmt.Fprintf(a.Stdout, "\n%s\n", builtin.details)
		}
		return nil
	}
	command, err := a.findToolCommand(name)
	if err != nil {
		return err
	}
	action := ""
	if len(words) > 1 && !strings.HasPrefix(words[1], "--") {
		var ok bool
		if action, ok = command.resolveAction(words[1]); !ok {
			return client.Errorf("Unknown action %q. See: sortdo %s --help", words[1], name)
		}
	}
	fmt.Fprint(a.Stdout, command.help(action, a.Out))
	return nil
}
