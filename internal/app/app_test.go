package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/store"
	"github.com/sortdo/cli/internal/testserver"
	"github.com/sortdo/cli/internal/ui"
)

type run struct {
	code           int
	stdout, stderr string
}

func setup(t *testing.T) *testserver.Server {
	t.Helper()
	t.Setenv("SORTDO_CONFIG_DIR", t.TempDir())
	t.Setenv("SORTDO_KEYRING", "0")
	t.Setenv("SORTDO_TOKEN", "")
	t.Setenv("SORTDO_NO_UPDATE_CHECK", "1")
	server := testserver.New(t)
	t.Setenv("SORTDO_SERVER", server.URL)
	client.Sleep = func(time.Duration) {}
	return server
}

func runApp(stdin string, args ...string) run {
	var stdout, stderr bytes.Buffer
	app := &App{Version: "1.2.3", Skill: []byte("---\nname: sortdo\n---\n"), Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &stderr}
	code := app.Run(args)
	return run{code, stdout.String(), stderr.String()}
}

func signIn(t *testing.T, server *testserver.Server) {
	t.Helper()
	server.Approved = true
	if result := runApp("", "login", "--no-wait"); result.code != 0 {
		t.Fatalf("login: %+v", result)
	}
	if result := runApp("", "login", "poll"); result.code != 0 {
		t.Fatalf("login poll: %+v", result)
	}
}

func TestAgentLoginIsTwoSteps(t *testing.T) {
	server := setup(t)
	started := runApp("", "login", "--email", "ada@example.test", "--workspace-name", "Alpha")
	if started.code != 0 || !strings.Contains(started.stdout, "/app/connect?code=BCDF-GHJK") || !strings.Contains(started.stdout, "For agents:") {
		t.Fatalf("without a terminal, login prints the link and exits: %+v", started)
	}
	if got := server.DeviceRequests[0]; got["client_id"] != "sortdo" || got["login_hint"] != "ada@example.test" || got["workspace_name"] != "Alpha" {
		t.Fatalf("device request: %v", got)
	}
	if status := runApp("", "status"); !strings.Contains(status.stdout, "Waiting sign-in: code BCDF-GHJK") {
		t.Fatalf("status shows the waiting code: %s", status.stdout)
	}
	server.Approved = true
	polled := runApp("", "login", "poll")
	if polled.code != 0 || !strings.Contains(polled.stdout, "Signed in to Alpha") {
		t.Fatalf("poll: %+v", polled)
	}
	if _, ok := store.LoadPending(server.URL); ok {
		t.Fatal("the waiting sign-in is removed")
	}
	// Windows has no Unix modes; the file sits in the user's own profile folder there.
	info, err := os.Stat(store.CredentialsPath())
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials are readable only by the user: %v %v", info, err)
	}
	if again := runApp("", "login", "poll"); again.code != 1 || !strings.Contains(again.stderr, "No sign-in is waiting") {
		t.Fatalf("a second poll has nothing to finish: %+v", again)
	}
}

func TestDeniedLogin(t *testing.T) {
	server := setup(t)
	server.Denied = true
	if result := runApp("", "login", "--wait", "--no-browser"); result.code != 1 || !strings.Contains(result.stderr, "cancelled") {
		t.Fatalf("a cancelled sign-in fails: %+v", result)
	}
}

func TestToolCommands(t *testing.T) {
	server := setup(t)
	signIn(t, server)

	result := runApp("", "teams", "add-members", "--team-id", "t1", "--member-ids", "m1", "--member-ids", `["m2","m3"]`, "--permissions", `{"edit":true}`, "--limit", "5")
	name, arguments := server.LastCall()
	if result.code != 0 || name != "manage_teams" {
		t.Fatalf("teams add-members: %+v %s", result, name)
	}
	want := `{"action":"add_members","limit":5,"memberIds":["m1","m2","m3"],"permissions":{"edit":true},"teamId":"t1"}`
	if got, _ := json.Marshal(arguments); string(got) != want {
		t.Fatalf("arguments\n got %s\nwant %s", got, want)
	}

	// --args from stdin, with options winning; a text option from a file; underscores work for actions.
	file := filepath.Join(t.TempDir(), "description.md")
	_ = os.WriteFile(file, []byte("From a file"), 0o600)
	runApp(`{"teamId":"from-stdin","visibility":"public"}`, "teams", "update", "--args", "-", "--visibility", "private", "--description-file", file)
	_, arguments = server.LastCall()
	if arguments["teamId"] != "from-stdin" || arguments["visibility"] != "private" || arguments["description"] != "From a file" {
		t.Fatalf("--args and -file: %v", arguments)
	}
	runApp("", "preferences", "update", "--ai-memory", "false")
	if _, arguments = server.LastCall(); arguments["aiMemory"] != false {
		t.Fatalf("boolean false: %v", arguments)
	}
	runApp("", "preferences", "update", "--ai-memory")
	if _, arguments = server.LastCall(); arguments["aiMemory"] != true {
		t.Fatalf("boolean flag: %v", arguments)
	}

	for _, bad := range []struct {
		args []string
		want string
	}{
		{[]string{"teams"}, "Choose an action"},
		{[]string{"teams", "rename"}, `Unknown action "rename"`},
		{[]string{"teams", "list", "--limit", "many"}, "--limit needs a whole number"},
		{[]string{"teams", "list", "--visibility", "secret"}, "--visibility must be one of: public, private"},
		{[]string{"teams", "list", "--colour", "red"}, "Unknown option --colour"},
		{[]string{"teams", "list", "stray"}, `Unexpected "stray"`},
		{[]string{"do-search", "--query", "x"}, "Missing --type"},
		{[]string{"no-such-command"}, `Unknown command "no-such-command"`},
	} {
		calls := len(server.Calls)
		result := runApp("", bad.args...)
		if result.code != 1 || !strings.Contains(result.stderr, bad.want) || len(server.Calls) != calls {
			t.Errorf("%v: want %q and no call, got %+v", bad.args, bad.want, result)
		}
	}

	help := runApp("", "teams", "add-members", "--help")
	if !strings.Contains(help.stdout, "Options for add-members:\n  --member-ids") || !strings.Contains(help.stdout, "--visibility <public|private>") {
		t.Fatalf("action help: %s", help.stdout)
	}
	if root := runApp("", "--help"); !strings.Contains(root.stdout, "do-search preferences teams") {
		t.Fatalf("root help lists the cached workspace commands: %s", root.stdout)
	}
}

func TestBuiltins(t *testing.T) {
	server := setup(t)
	server.Stream = true
	signIn(t, server)

	if result := runApp("", "search", "release", "notes"); result.stdout != "Notes\n  https://trysort.com/alpha/1\n" {
		t.Fatalf("search: %q", result.stdout)
	}
	if result := runApp("", "call", "no_such_tool"); result.code != 1 || result.stdout != "Unknown tool\n" {
		t.Fatalf("a tool error exits 1: %+v", result)
	}
	if result := runApp("", "call", "manage_teams", "{bad"); result.code != 1 || !strings.Contains(result.stderr, "JSON object") {
		t.Fatalf("bad JSON: %+v", result)
	}
	created := runApp("# Notes", "pages", "create", "--title", "Notes", "--file", "-", "--team", "t1", "--publish")
	name, arguments := server.LastCall()
	if created.code != 0 || name != "manage_pages" || arguments["pageId"] != "01a18253-6129-77df-964c-7a4c6a4f2235" || arguments["action"] != "publish" {
		t.Fatalf("pages create --publish: %+v %s %v", created, name, arguments)
	}
	if call := server.Calls[len(server.Calls)-2]["arguments"].(map[string]any); call["content"] != "# Notes" || call["teamId"] != "t1" {
		t.Fatalf("create_page arguments: %v", call)
	}
	runApp("", "pages", "edit", "p1", "--replace", "old", "--with", "new")
	if name, arguments = server.LastCall(); name != "update_page" || arguments["replace"].(map[string]any)["newText"] != "new" {
		t.Fatalf("pages edit: %s %v", name, arguments)
	}
	if result := runApp("", "pages", "create", "--draft", "d1"); result.code != 1 || !strings.Contains(result.stderr, "Say where the draft goes") {
		t.Fatalf("a draft needs a place: %+v", result)
	}
	if result := runApp("", "token"); result.stdout != "access-1\n" {
		t.Fatalf("token: %q", result.stdout)
	}
	status := runApp("", "status", "--json")
	var report map[string]any
	if json.Unmarshal([]byte(status.stdout), &report) != nil || report["signedIn"] != true || report["workspace"] != "Alpha" {
		t.Fatalf("status --json: %s", status.stdout)
	}
	if result := runApp("", "logout"); result.stdout != "Signed out.\n" || len(server.Revoked) != 1 || server.Revoked[0] != "refresh-1" {
		t.Fatalf("logout revokes the refresh token: %+v %v", result, server.Revoked)
	}
	if result := runApp("", "search", "x"); result.code != 1 || !strings.Contains(result.stderr, "Not signed in") {
		t.Fatalf("after logout: %+v", result)
	}
}

func TestRefreshAndToken(t *testing.T) {
	server := setup(t)
	signIn(t, server)
	tokens, _ := store.LoadTokens(server.URL)
	tokens.ExpiresAt = time.Now().Add(-time.Minute)
	if _, _, err := store.SaveTokens(server.URL, *tokens); err != nil {
		t.Fatal(err)
	}
	if result := runApp("", "token"); result.stdout != "access-2\n" || server.Refreshes != 1 {
		t.Fatalf("an expired token is refreshed: %+v", result)
	}
	// A refresh the server refuses ends the sign-in.
	tokens, _ = store.LoadTokens(server.URL)
	tokens.ExpiresAt, tokens.RefreshToken = time.Now().Add(-time.Minute), "revoked"
	_, _, _ = store.SaveTokens(server.URL, *tokens)
	if result := runApp("", "token"); result.code != 1 || !strings.Contains(result.stderr, "The sign-in has ended") {
		t.Fatalf("refused refresh: %+v", result)
	}
	if current, _ := store.LoadTokens(server.URL); current != nil {
		t.Fatal("the ended sign-in is removed")
	}
	// SORTDO_TOKEN wins over the stored sign-in.
	t.Setenv("SORTDO_TOKEN", "access-99")
	if result := runApp("", "status"); !strings.Contains(result.stdout, "token from SORTDO_TOKEN") {
		t.Fatalf("SORTDO_TOKEN: %s", result.stdout)
	}
}

func TestSkillCommand(t *testing.T) {
	setup(t)
	dir := t.TempDir()
	if result := runApp("", "skill", "install", "--dir", dir); result.code != 0 {
		t.Fatalf("skill install: %+v", result)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "sortdo", "SKILL.md")); err != nil || !strings.Contains(string(data), "name: sortdo") {
		t.Fatalf("skill file: %v", err)
	}
	if result := runApp("", "skill", "uninstall", "--dir", dir); !strings.Contains(result.stdout, "Removed") {
		t.Fatalf("skill uninstall: %+v", result)
	}
}

func TestParsing(t *testing.T) {
	g, err := splitGlobals([]string{"teams", "--server", "x.test", "list", "--json", "-h"})
	if err != nil || g.server != "x.test" || !g.json || !g.help || strings.Join(g.rest, " ") != "teams list" {
		t.Fatalf("globals: %+v %v", g, err)
	}
	f, err := parseFlags([]string{"--title=A=B", "word", "--publish", "--", "--not-an-option"}, []string{"title"}, []string{"publish"}, "x")
	if err != nil || f.get("title") != "A=B" || !f.bool("publish") || strings.Join(f.words, " ") != "word --not-an-option" {
		t.Fatalf("flags: %+v %v", f, err)
	}
	for in, want := range map[string]string{"memberIds": "member-ids", "teamId": "team-id", "aiMemory": "ai-memory", "url": "url"} {
		if got := kebab(in); got != want {
			t.Errorf("kebab(%s) = %s", in, got)
		}
	}
	if commandName("manage_search_sources") != "search-sources" || commandName("do_search") != "do-search" {
		t.Error("command names")
	}
	draft := &client.Result{Text: `Nothing is created yet: "Notes" waits for a location (draftId 01a1824e-21fb-7e8e-8320-6aacaca55e56).`}
	if createdPageID(draft) != "" {
		t.Error("a draft that waits for a location has no page id")
	}
}

// A person at a terminal gets readable results, colors and the logo; pipes and agents keep the plain text.
func TestTerminalOutput(t *testing.T) {
	server := setup(t)
	signIn(t, server)
	at := func(style ui.Style, args ...string) run {
		var stdout, stderr bytes.Buffer
		app := &App{Version: "1.2.3", Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, Readable: true, Width: 100, Out: style, Err: style}
		code := app.Run(args)
		return run{code, stdout.String(), stderr.String()}
	}
	if piped := runApp("", "teams", "list"); !strings.Contains(piped.stdout, `"action":"list"`) {
		t.Fatalf("piped output changed: %+v", piped)
	}
	if readable := at(ui.Style{}, "teams", "list"); readable.stdout != "Teams\n\nAction: list\n" {
		t.Fatalf("readable output: %+v", readable)
	}
	if asJSON := at(ui.Style{}, "teams", "list", "--json"); !strings.Contains(asJSON.stdout, `"action": "list"`) {
		t.Fatalf("--json at a terminal: %+v", asJSON)
	}
	if help := at(ui.Colored()); !strings.Contains(help.stdout, "┌─┐") || !strings.Contains(help.stdout, "\x1b[") {
		t.Fatalf("colored help has no logo: %q", help.stdout)
	}
	if help := at(ui.Style{}); strings.Contains(help.stdout, "┌─┐") || strings.Contains(help.stdout, "\x1b[") {
		t.Fatalf("plain help has colors or the logo: %q", help.stdout)
	}
	if failed := at(ui.Colored(), "nope"); failed.code != 1 || !strings.Contains(failed.stderr, "\x1b[31mUnknown command") {
		t.Fatalf("colored error: %+v", failed)
	}
}

func TestUninstall(t *testing.T) {
	server := setup(t)
	signIn(t, server)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	program := filepath.Join(dir, "sortdo")
	if err := os.WriteFile(program, []byte("program"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	saved := executablePath
	executablePath = func() string { return program }
	t.Cleanup(func() { executablePath = saved })
	if result := runApp("", "skill", "install"); result.code != 0 {
		t.Fatalf("skill install: %+v", result)
	}

	// Without a terminal, nothing is removed until --yes.
	if result := runApp("", "uninstall"); result.code != 1 || !strings.Contains(result.stderr, "sortdo uninstall --yes") {
		t.Fatalf("uninstall without --yes: %+v", result)
	}
	if _, err := os.Stat(program); err != nil {
		t.Fatalf("the program went without --yes: %v", err)
	}
	// In a terminal it asks, and "no" keeps everything.
	var stdout, stderr bytes.Buffer
	asked := &App{Version: "1.2.3", Stdin: strings.NewReader("n\n"), Stdout: &stdout, Stderr: &stderr, Interactive: true}
	if code := asked.Run([]string{"uninstall"}); code != 0 || !strings.Contains(stdout.String(), "[y/N]") || !strings.Contains(stdout.String(), "Nothing was removed.") {
		t.Fatalf("uninstall answered no: %d %q %q", code, stdout.String(), stderr.String())
	}
	result := runApp("", "uninstall", "--yes")
	if result.code != 0 || !strings.Contains(result.stdout, "Signed out of "+server.URL) || !strings.Contains(result.stdout, "Removed the agent skill") {
		t.Fatalf("uninstall: %+v", result)
	}
	if len(server.Revoked) == 0 {
		t.Fatal("the sign-in was not ended on the server")
	}
	if _, err := os.Stat(program); !os.IsNotExist(err) {
		t.Fatalf("the program is still there: %v", err)
	}
	if _, err := os.Stat(store.Dir()); !os.IsNotExist(err) {
		t.Fatalf("the settings are still there: %v", err)
	}

	// A copy that npm installed stays: npm removes it.
	npm := filepath.Join(dir, "lib", "node_modules", "sortdo", "node_modules", "@sortdo", "cli-linux-x64", "bin", "sortdo")
	executablePath = func() string { return npm }
	if result := runApp("", "uninstall", "--yes"); result.code != 0 || !strings.Contains(result.stdout, "npm uninstall -g sortdo") {
		t.Fatalf("npm uninstall: %+v", result)
	}
}

// A person at a terminal can choose commands with the arrow keys; agents and CI never get a menu.
func TestMenus(t *testing.T) {
	server := setup(t)
	signIn(t, server)
	for _, name := range noPromptVariables {
		t.Setenv(name, "")
	}
	person := func(keys string, args ...string) run {
		var stdout, stderr bytes.Buffer
		app := &App{Version: "1.2.3", Stdin: strings.NewReader(keys), Stdout: &stdout, Stderr: &stderr, Interactive: true, Width: 100, Height: 30}
		code := app.Run(args)
		return run{code, stdout.String(), stderr.String()}
	}
	// Type to find the command, then choose the action; the command that runs is shown.
	if result := person("teams\rlist\r"); result.code != 0 || !strings.Contains(result.stdout, "› sortdo teams list") || !strings.Contains(result.stdout, `"action":"list"`) {
		t.Fatalf("menu: %+v", result)
	}
	if result := person("search\rroad map\r"); result.code != 0 || !strings.Contains(result.stdout, "› sortdo search 'road map'") || !strings.Contains(result.stdout, "Notes") {
		t.Fatalf("search from the menu: %+v", result)
	}
	// A tool command without its action asks for it.
	if result := person("\x1b[B\r", "teams"); result.code != 0 || !strings.Contains(result.stdout, "› sortdo teams ") {
		t.Fatalf("action menu: %+v", result)
	}
	// Esc leaves without running anything.
	if result := person("\x1b"); result.code != 0 || strings.Contains(result.stdout, "› sortdo") {
		t.Fatalf("cancel: %+v", result)
	}
	// In CI, under a coding agent or with SORTDO_NO_INPUT, the same commands print help or an error and never wait.
	for _, name := range []string{"CI", "CLAUDECODE", "SORTDO_NO_INPUT"} {
		t.Setenv(name, "1")
		if result := person(""); !strings.Contains(result.stdout, "Usage:") {
			t.Fatalf("%s: bare sortdo should print help: %+v", name, result)
		}
		if result := person("", "teams"); result.code != 1 || !strings.Contains(result.stderr, "Choose an action") {
			t.Fatalf("%s: teams without an action should fail: %+v", name, result)
		}
		t.Setenv(name, "")
	}
}

func TestUpdateAndSignInMessages(t *testing.T) {
	setup(t)
	// npm, Homebrew and winget copies get their command before any download is tried.
	t.Setenv("SORTDO_RELEASES_URL", "http://127.0.0.1:9")
	saved := executablePath
	executablePath = func() string { return "/opt/homebrew/Caskroom/sortdo/0.1.2/sortdo" }
	t.Cleanup(func() { executablePath = saved })
	if result := runApp("", "update"); result.code != 1 || !strings.Contains(result.stderr, "brew upgrade sortdo") || strings.Contains(result.stderr, "Could not") {
		t.Fatalf("update for Homebrew: %+v", result)
	}
	// A workspace command before sign-in says to sign in.
	if result := runApp("", "teams", "list"); result.code != 1 || !strings.Contains(result.stderr, "Sign in to use the workspace commands, such as teams: sortdo login") {
		t.Fatalf("not signed in: %+v", result)
	}
}
