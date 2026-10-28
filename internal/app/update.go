package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/selfupdate"
	"github.com/sortdo/cli/internal/store"
	"golang.org/x/term"
)

const updateCheckName = "update-check"

type updateCheck struct {
	Latest string `json:"latest"`
}

func executablePath() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		return resolved
	}
	return executable
}

func updateCommand() string { return selfupdate.DetectMethod(executablePath()).Command() }

// latestKnownVersion is the newest release from the last daily check, when it is newer than this program.
func latestKnownVersion(current string) string {
	var check updateCheck
	if _, ok := store.ReadCache(updateCheckName, "github", &check); ok && check.Latest != "" && current != "dev" && selfupdate.Newer(check.Latest, current) {
		return check.Latest
	}
	return ""
}

// startUpdateNotice checks for a new release at most once a day, while the command runs, and only for a person at a
// terminal. The returned function prints the notice on stderr when there is one.
func startUpdateNotice(a *App) func() {
	quiet := func() {}
	if a.Version == "dev" || os.Getenv("SORTDO_NO_UPDATE_CHECK") != "" || os.Getenv("CI") != "" || !term.IsTerminal(int(os.Stderr.Fd())) {
		return quiet
	}
	print := func() {
		if latest := latestKnownVersion(a.Version); latest != "" {
			fmt.Fprintf(a.Stderr, "\nsortdo %s is available (you have %s). Run: %s\n", latest, a.Version, updateCommand())
		}
	}
	var check updateCheck
	if savedAt, ok := store.ReadCache(updateCheckName, "github", &check); ok && time.Since(savedAt) < 24*time.Hour {
		return print
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if latest, err := selfupdate.Latest(3 * time.Second); err == nil {
			store.WriteCache(updateCheckName, "github", updateCheck{Latest: latest})
		}
	}()
	return func() {
		select {
		case <-done:
			print()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (a *App) update(args []string) error {
	f, err := parseFlags(args, nil, []string{"check"}, "update")
	if err != nil {
		return err
	}
	latest, err := selfupdate.Latest(30 * time.Second)
	if err != nil {
		return client.Errorf("Could not find the latest release: %v", err)
	}
	store.WriteCache(updateCheckName, "github", updateCheck{Latest: latest})
	if a.Version != "dev" && !selfupdate.Newer(latest, a.Version) {
		fmt.Fprintf(a.Stdout, "sortdo %s is the latest version.\n", a.Version)
		return nil
	}
	if f.bool("check") {
		fmt.Fprintf(a.Stdout, "sortdo %s is available (you have %s). Run: %s\n", latest, a.Version, updateCommand())
		return nil
	}
	executable := executablePath()
	if method := selfupdate.DetectMethod(executable); method != selfupdate.Script {
		return client.Errorf("%s installed sortdo, so update it there: %s", method, method.Command())
	}
	if a.Version == "dev" {
		return client.Errorf("This is a development build. Install a release: curl -fsSL https://sort.do/install | bash")
	}
	fmt.Fprintf(a.Stdout, "Downloading sortdo %s…\n", latest)
	program, err := selfupdate.Fetch(latest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return client.Errorf("Could not download sortdo %s: %v", latest, err)
	}
	if err := selfupdate.Replace(executable, program); err != nil {
		return client.Errorf("Could not replace %s: %v", executable, err)
	}
	fmt.Fprintf(a.Stdout, "Updated sortdo %s to %s.\n", a.Version, latest)
	return nil
}
