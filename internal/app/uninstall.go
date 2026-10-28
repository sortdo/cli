package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/selfupdate"
	"github.com/sortdo/cli/internal/skills"
	"github.com/sortdo/cli/internal/store"
)

// uninstall signs out everywhere and removes the agent skill, the settings and, for the install script's copy, the
// program. npm, Homebrew and winget remove their own copies, so for those it prints the command.
func (a *App) uninstall(args []string) error {
	f, err := parseFlags(args, nil, []string{"yes"}, "uninstall")
	if err != nil {
		return err
	}
	if len(f.words) > 0 {
		return client.Errorf("Unexpected %q. See: sortdo uninstall --help", f.words[0])
	}
	if !f.bool("yes") {
		if !a.Interactive {
			return client.Errorf("This signs out and removes sortdo, its settings and its agent skill from this computer. To go ahead, run: sortdo uninstall --yes")
		}
		fmt.Fprint(a.Stdout, "Sign out and remove sortdo, its settings and its agent skill from this computer? [y/N] ")
		answer, _ := bufio.NewReader(a.Stdin).ReadString('\n')
		if answer = strings.ToLower(strings.TrimSpace(answer)); answer != "y" && answer != "yes" {
			fmt.Fprintln(a.Stdout, "Nothing was removed.")
			return nil
		}
	}
	for _, origin := range store.SignedInServers() {
		if client.Revoke(origin) {
			fmt.Fprintln(a.Stdout, a.Out.Done("Signed out of "+origin))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		removed, _ := skills.Uninstall(skills.DefaultDirs(home))
		for _, folder := range removed {
			fmt.Fprintln(a.Stdout, a.Out.Done("Removed the agent skill in "+folder))
		}
	}
	if err := store.RemoveAll(); err != nil {
		return client.Errorf("Could not remove the settings in %s: %v", store.Dir(), err)
	}
	fmt.Fprintln(a.Stdout, a.Out.Done("Removed the settings in "+store.Dir()))
	executable := executablePath()
	others := otherCopies(executable)
	switch method := selfupdate.DetectMethod(executable); {
	case method != selfupdate.Script:
		fmt.Fprintf(a.Stdout, "%s installed this sortdo. To remove the program, run: %s\n", method, a.Out.Bold(method.UninstallCommand()))
	case a.Version == "dev":
		fmt.Fprintf(a.Stdout, "This is a development build. Delete it yourself: %s\n", executable)
	default:
		if err := selfupdate.Remove(executable); err != nil {
			return client.Errorf("Could not remove %s: %v", executable, err)
		}
		fmt.Fprintln(a.Stdout, a.Out.Done("Removed "+executable))
	}
	if len(others) > 0 {
		fmt.Fprintf(a.Stdout, "%s\n", a.Out.Yellow("Another sortdo is still installed: "+strings.Join(others, ", ")))
	}
	return nil
}

// otherCopies are the sortdo programs on PATH other than the one that runs now.
func otherCopies(executable string) []string {
	var others []string
	for _, copy := range selfupdate.Copies(os.Getenv("PATH"), executable) {
		if !copy.Self {
			others = append(others, copy.Path)
		}
	}
	return others
}

// copiesNote warns when PATH has more than one sortdo: only the first one runs, and an update changes only one.
func (a *App) copiesNote() string {
	copies := selfupdate.Copies(os.Getenv("PATH"), executablePath())
	if len(copies) < 2 {
		return ""
	}
	var out strings.Builder
	fmt.Fprintln(&out, a.Out.Yellow("More than one sortdo is installed. The first one on PATH runs:"))
	for _, copy := range copies {
		line := "  " + copy.Path
		if copy.Self {
			line += a.Out.Dim(" (this one)")
		}
		fmt.Fprintln(&out, line)
	}
	fmt.Fprintln(&out, "Keep one. Remove the others with the tool that installed them: https://docs.trysort.com/cli/install/#uninstall")
	return out.String()
}
