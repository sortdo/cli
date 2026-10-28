package app

import (
	"fmt"
	"os"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/skills"
)

func (a *App) skill(args []string) error {
	f, err := parseFlags(args, []string{"dir"}, nil, "skill")
	if err != nil {
		return err
	}
	if len(f.words) != 1 {
		return client.Errorf("Use: sortdo skill install, sortdo skill uninstall or sortdo skill print")
	}
	dirs := f.values["dir"]
	if len(dirs) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return client.Errorf("Could not find your home folder. Give a skills folder: sortdo skill install --dir <folder>")
		}
		dirs = skills.DefaultDirs(home)
	}
	switch f.words[0] {
	case "print":
		_, err := a.Stdout.Write(a.Skill)
		return err
	case "install":
		written, err := skills.Install(a.Skill, dirs)
		for _, file := range written {
			fmt.Fprintf(a.Stdout, "Installed %s\n", file)
		}
		if err != nil {
			return client.Errorf("Could not install the skill: %v", err)
		}
		fmt.Fprintln(a.Stdout, "Coding agents now know how to use sortdo. Run sortdo skill install again after an update to refresh it.")
		return nil
	case "uninstall":
		removed, err := skills.Uninstall(dirs)
		for _, folder := range removed {
			fmt.Fprintf(a.Stdout, "Removed %s\n", folder)
		}
		if err != nil {
			return client.Errorf("Could not remove the skill: %v", err)
		}
		if len(removed) == 0 {
			fmt.Fprintln(a.Stdout, "The skill was not installed.")
		}
		return nil
	}
	return client.Errorf("Unknown skill command %q. Use install, uninstall or print.", f.words[0])
}
