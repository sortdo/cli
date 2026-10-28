package app

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/store"
)

// builtin is a command made by hand. Every other command comes from the server's tools.
type builtin struct {
	usage   string // shown in help after "sortdo "
	summary string
	details string
	run     func(a *App, args []string) error
}

var builtins map[string]builtin

func init() {
	builtins = map[string]builtin{
		"login": {"login [--email <address>] [--workspace-name <name>] [--no-browser] [--no-wait | --wait]\n  sortdo login poll",
			"Sign in. In a terminal it opens the browser and waits; an agent gets a link and `sortdo login poll`.",
			`Signs in with a code the person allows in the browser. The page shows the same code: allow only if it matches.

In a terminal, sortdo opens the browser and waits. Without a terminal (a coding agent), it prints the link and the code,
saves the sign-in and exits; ` + "`sortdo login poll`" + ` then waits for the person. If poll stops before they answer, run it again.

  --email <address>          The person's email, filled in on the sign-in page.
  --workspace-name <name>    A name for a new workspace, when the person has none yet.
  --no-browser               Do not open the browser.
  --no-wait                  Print the link and exit, also in a terminal.
  --wait                     Wait for the person, also without a terminal.`,
			(*App).login},
		"logout": {"logout", "End the sign-in on the server and remove it here.", "", (*App).logout},
		"status": {"status", "Show the server, the workspace, the sign-in and updates.", "", (*App).status},
		"token": {"token", "Print the access token for scripts. It is refreshed when needed.",
			"Prints a valid access token for the server, for scripts that call https://trysort.com/mcp themselves. Keep it secret.",
			(*App).token},
		"tools": {"tools [--refresh]", "List the tools and their commands.",
			"Lists every tool the server offers you, with its command. --refresh reloads the list from the server.\n--json prints the tools with their input schemas.",
			(*App).tools},
		"call": {"call <tool> [<json> | - | @file]", "Call any tool by its MCP name with JSON arguments.",
			"Calls a tool by its MCP name, for example: sortdo call manage_members '{\"action\":\"list\"}'\nGive - to read the JSON from stdin, or @file to read it from a file.",
			(*App).call},
		"search": {"search <words>", "Find pages. Titles and URLs.", "Finds pages and prints their titles and URLs. For filters, people and the web: sortdo do-search --help", (*App).search},
		"read":   {"read <page id or URL>", "Read a page as Markdown, with line numbers.", "", (*App).read},
		"pages": {"pages create | edit | <action>", "Create and edit pages, and every other page action.",
			`  sortdo pages create --title <title> [--content <markdown> | --file <path | ->] [--team <id> | --parent <id>] [--publish]
  sortdo pages create --draft <draft id> --team <id> | --parent <id> [--publish]
  sortdo pages edit <page id or URL> --replace <old text> --with <new text>
  sortdo pages edit <page id or URL> --edits '<json array of line edits>' [--title <title>]

A new page is a draft that only you see until it is published (--publish). Without --team or --parent, Sort places it
when one place clearly fits; otherwise it lists places and keeps a draft: run sortdo pages create --draft <id> --team <id>.
Team ids come from sortdo teams list. Edits are saved at once as a new version that can be restored.

The other page actions (publish, move, versions, restore-version, trash and more): sortdo pages <action> --help`,
			(*App).pages},
		"approval": {"approval <id>", "Check an action that waits for the person's approval.", "", (*App).approval},
		"update": {"update [--check]", "Update sortdo to the latest version.",
			"Downloads the latest release, checks its SHA-256 checksum and replaces this program. --check only reports.\nWhen npm, Homebrew or winget installed sortdo, it prints that tool's update command instead.",
			(*App).update},
		"skill": {"skill install [--dir <folder>] | skill uninstall [--dir <folder>] | skill print", "Teach coding agents to use sortdo.",
			`Installs the sortdo Agent Skill (SKILL.md) for your coding agents:
  ~/.agents/skills/sortdo    Codex, Cursor, GitHub Copilot, Gemini CLI and other agents that read ~/.agents/skills
  ~/.claude/skills/sortdo    Claude Code (when ~/.claude exists)

  --dir <folder>    Install into this skills folder instead, for example .claude/skills in a project.
  skill print       Print the skill.`,
			(*App).skill},
		"version": {"version", "Print the version.", "", func(a *App, _ []string) error {
			fmt.Fprintf(a.Stdout, "sortdo %s\n", a.Version)
			return nil
		}},
	}
}

func (a *App) workspaceName() string {
	result, err := client.CallTool(a.origin, "manage_workspace", map[string]any{"action": "get"})
	if err != nil || result.IsError {
		return ""
	}
	workspace, _ := result.Data()["workspace"].(map[string]any)
	return client.Text(workspace, "name")
}

func (a *App) login(args []string) error {
	f, err := parseFlags(args, []string{"email", "workspace-name"}, []string{"no-browser", "no-wait", "wait"}, "login")
	if err != nil {
		return err
	}
	if len(f.words) > 0 {
		if f.words[0] == "poll" {
			return a.loginPoll()
		}
		return client.Errorf("Unknown login command %q. See: sortdo login --help", f.words[0])
	}
	wait := a.Interactive
	if f.bool("no-wait") {
		wait = false
	}
	if f.bool("wait") {
		wait = true
	}
	pending, err := client.StartDevice(a.origin, client.Hints{Email: f.get("email"), WorkspaceName: f.get("workspace-name")})
	if err != nil {
		return err
	}
	server := client.ServerFlag(a.origin)
	if !wait {
		if err := store.SavePending(a.origin, pending); err != nil {
			return err
		}
		fmt.Fprintf(a.Stdout, "To sign in, open this link and allow Sort CLI:\n  %s\nThe page shows the code %s.\nThen run: sortdo login poll%s\n\n", pending.Link, pending.UserCode, server)
		fmt.Fprintf(a.Stdout, "For agents: give the person the link, then run `sortdo login poll%s` at once, without asking. It returns when they allow or cancel. If it stops before they answer, run it again. The code expires in %d minutes.\n", server, int(time.Until(pending.ExpiresAt).Round(time.Minute).Minutes()))
		return nil
	}
	opened := false
	if !f.bool("no-browser") && a.Interactive {
		opened = openBrowser(pending.Link)
	}
	if opened {
		fmt.Fprintf(a.Stdout, "Opened the browser. If it did not open, visit:\n  %s\n", pending.Link)
	} else {
		fmt.Fprintf(a.Stdout, "Open this link to sign in:\n  %s\n", pending.Link)
	}
	fmt.Fprintf(a.Stdout, "Allow Sort CLI only if the page shows the code %s.\nWaiting…\n", pending.UserCode)
	return a.finishLogin(pending)
}

func (a *App) loginPoll() error {
	pending, ok := store.LoadPending(a.origin)
	if !ok {
		return client.Errorf("No sign-in is waiting. Run: sortdo login%s", client.ServerFlag(a.origin))
	}
	if time.Now().After(pending.ExpiresAt) {
		_ = store.SavePending(a.origin, nil)
		return client.Errorf("The code expired. Run sortdo login%s again.", client.ServerFlag(a.origin))
	}
	fmt.Fprintf(a.Stdout, "Waiting for the person to allow Sort CLI (code %s)…\n", pending.UserCode)
	return a.finishLogin(pending)
}

func (a *App) finishLogin(pending *store.Pending) error {
	place, keychainError, err := client.PollDevice(a.origin, pending)
	// The code is used up whatever the answer was; a stopped poll keeps it for the next try.
	_ = store.SavePending(a.origin, nil)
	if err != nil {
		return err
	}
	store.ClearCache(toolsCacheName, a.origin)
	if keychainError != nil {
		fmt.Fprintf(a.Stderr, "No OS keychain is available (%v). The sign-in is in %s, readable only by you.\n", keychainError, place)
	}
	if name := a.workspaceName(); name != "" {
		fmt.Fprintf(a.Stdout, "Signed in to %s on %s.\n", name, a.origin)
	} else {
		fmt.Fprintf(a.Stdout, "Signed in to %s.\n", a.origin)
	}
	return nil
}

func (a *App) logout(args []string) error {
	if _, err := parseFlags(args, nil, nil, "logout"); err != nil {
		return err
	}
	store.ClearCache(toolsCacheName, a.origin)
	if client.Revoke(a.origin) {
		fmt.Fprintln(a.Stdout, "Signed out.")
	} else {
		fmt.Fprintln(a.Stdout, "Not signed in.")
	}
	return nil
}

func (a *App) status(args []string) error {
	if _, err := parseFlags(args, nil, nil, "status"); err != nil {
		return err
	}
	report := map[string]any{"version": a.Version, "server": a.origin, "signedIn": false}
	if source := client.TokenSource(a.origin); source != "" {
		if _, err := client.AccessToken(a.origin); err == nil {
			report["signedIn"] = true
			report["tokenFrom"] = source
			if name := a.workspaceName(); name != "" {
				report["workspace"] = name
			}
		} else {
			report["problem"] = err.Error()
		}
	}
	if pending, ok := store.LoadPending(a.origin); ok && time.Now().Before(pending.ExpiresAt) {
		report["pendingCode"] = pending.UserCode
	}
	if latest := latestKnownVersion(a.Version); latest != "" {
		report["latestVersion"] = latest
	}
	if a.json {
		return a.printJSON(report)
	}
	fmt.Fprintf(a.Stdout, "sortdo %s\nServer: %s\n", a.Version, a.origin)
	if report["signedIn"] == true {
		if name, ok := report["workspace"]; ok {
			fmt.Fprintf(a.Stdout, "Workspace: %s\n", name)
		}
		fmt.Fprintf(a.Stdout, "Signed in: yes (token from %s)\n", report["tokenFrom"])
	} else if problem, ok := report["problem"]; ok {
		fmt.Fprintf(a.Stdout, "Signed in: no. %s\n", problem)
	} else {
		fmt.Fprintf(a.Stdout, "Signed in: no. Run: sortdo login%s\n", client.ServerFlag(a.origin))
	}
	if code, ok := report["pendingCode"]; ok {
		fmt.Fprintf(a.Stdout, "Waiting sign-in: code %s. Finish it with: sortdo login poll%s\n", code, client.ServerFlag(a.origin))
	}
	if latest, ok := report["latestVersion"]; ok {
		fmt.Fprintf(a.Stdout, "Update: %s is available. Run: %s\n", latest, updateCommand())
	}
	return nil
}

func (a *App) token(args []string) error {
	if _, err := parseFlags(args, nil, nil, "token"); err != nil {
		return err
	}
	token, err := client.AccessToken(a.origin)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Stdout, token)
	return nil
}

func (a *App) tools(args []string) error {
	f, err := parseFlags(args, nil, []string{"refresh"}, "tools")
	if err != nil {
		return err
	}
	tools, err := loadTools(a.origin, f.bool("refresh"))
	if err != nil {
		return err
	}
	if a.json {
		return a.printJSON(tools)
	}
	for _, tool := range tools {
		summary := firstSentence(tool.Description)
		if tool.Title != "" {
			summary = tool.Title + ": " + summary
		}
		fmt.Fprintf(a.Stdout, "sortdo %-18s %s\n", commandName(tool.Name), summary)
	}
	fmt.Fprintln(a.Stdout, "\nHelp for one: sortdo <command> --help. Call one by its MCP name: sortdo call <tool> '<json>'")
	return nil
}

func (a *App) call(args []string) error {
	f, err := parseFlags(args, nil, nil, "call")
	if err != nil {
		return err
	}
	if len(f.words) == 0 {
		return client.Errorf("Name the tool: sortdo call <tool> [<json arguments>]")
	}
	arguments := map[string]any{}
	if len(f.words) > 1 {
		raw := f.words[1]
		if raw == "-" || strings.HasPrefix(raw, "@") {
			if raw, err = readValue(strings.TrimPrefix(raw, "@"), a.Stdin); err != nil {
				return err
			}
		}
		if err := jsonObject(raw, &arguments); err != nil {
			return err
		}
	}
	result, err := client.CallTool(a.origin, f.words[0], arguments)
	if err != nil {
		return err
	}
	return a.printResult(result)
}

func (a *App) search(args []string) error {
	f, err := parseFlags(args, nil, nil, "search")
	if err != nil {
		return err
	}
	query := strings.TrimSpace(strings.Join(f.words, " "))
	if query == "" {
		return client.Errorf("Say what to search for: sortdo search <words>")
	}
	result, err := client.CallTool(a.origin, "search", map[string]any{"query": query})
	if err != nil {
		return err
	}
	if a.json {
		return a.printResult(result)
	}
	results, _ := result.Data()["results"].([]any)
	if len(results) == 0 {
		if result.IsError {
			a.failed = true
			fmt.Fprintln(a.Stdout, result.Text)
		} else {
			fmt.Fprintln(a.Stdout, "No pages found.")
		}
	}
	for _, item := range results {
		entry, _ := item.(map[string]any)
		fmt.Fprintf(a.Stdout, "%s\n  %s\n", client.Text(entry, "title"), client.Text(entry, "url"))
	}
	return nil
}

func (a *App) read(args []string) error {
	f, err := parseFlags(args, nil, nil, "read")
	if err != nil {
		return err
	}
	if len(f.words) != 1 {
		return client.Errorf("Name one page: sortdo read <page id or URL>")
	}
	result, err := client.CallTool(a.origin, "fetch", map[string]any{"id": f.words[0]})
	if err != nil {
		return err
	}
	if a.json {
		return a.printResult(result)
	}
	if text := client.Text(result.Data(), "text"); text != "" && !result.IsError {
		fmt.Fprintln(a.Stdout, text)
		return nil
	}
	return a.printResult(result)
}

var createdPagePattern = regexp.MustCompile(`^Created page .* with id: ([0-9a-f-]{36})`)

// createdPageID is the id of a page that create_page created. A draft that still waits for a location has none.
func createdPageID(result *client.Result) string {
	if data, ok := result.Structured.(map[string]any); ok {
		if id := client.Text(data, "pageId"); id != "" {
			return id
		}
	}
	if match := createdPagePattern.FindStringSubmatch(result.Text); match != nil {
		return match[1]
	}
	return ""
}

func (a *App) pages(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return client.Errorf("Choose an action: sortdo pages create, sortdo pages edit, or sortdo pages <action>. See: sortdo pages --help")
	}
	switch args[0] {
	case "create":
		return a.pageCreate(args[1:])
	case "edit":
		return a.pageEdit(args[1:])
	}
	return a.runTool("pages", args)
}

func (a *App) pageCreate(args []string) error {
	f, err := parseFlags(args, []string{"title", "content", "file", "team", "parent", "draft"}, []string{"publish"}, "pages create")
	if err != nil {
		return err
	}
	if len(f.words) > 0 {
		return client.Errorf("Unexpected %q. See: sortdo pages --help", f.words[0])
	}
	title, draft := f.get("title"), f.get("draft")
	location := map[string]any{}
	if parent := f.get("parent"); parent != "" {
		location["parentId"] = parent
	} else if team := f.get("team"); team != "" {
		location["teamId"] = team
	}
	if title == "" && draft == "" {
		return client.Errorf("Give the page a title: sortdo pages create --title <title>")
	}
	if draft != "" && len(location) == 0 {
		return client.Errorf("Say where the draft goes: sortdo pages create --draft <id> --team <id> (or --parent <id>)")
	}
	arguments := location
	if draft != "" {
		arguments["draftId"] = draft
	} else {
		arguments["title"] = title
		content := f.get("content")
		if file := f.get("file"); file != "" {
			if content, err = readValue(file, a.Stdin); err != nil {
				return err
			}
		}
		if content != "" {
			arguments["content"] = content
		}
	}
	created, err := client.CallTool(a.origin, "create_page", arguments)
	if err != nil {
		return err
	}
	if err := a.printResult(created); err != nil || created.IsError || !f.bool("publish") {
		return err
	}
	id := createdPageID(created)
	if id == "" {
		// No page yet: the draft waits for a location, and the result above lists where it can go.
		return client.Errorf("Not published: choose a location first, then run sortdo pages create --draft <id> --team <id> --publish")
	}
	published, err := client.CallTool(a.origin, "manage_pages", map[string]any{"action": "publish", "pageId": id})
	if err != nil {
		return err
	}
	return a.printResult(published)
}

func (a *App) pageEdit(args []string) error {
	f, err := parseFlags(args, []string{"replace", "with", "edits", "title"}, nil, "pages edit")
	if err != nil {
		return err
	}
	if len(f.words) != 1 {
		return client.Errorf("Name one page: sortdo pages edit <page id or URL> --replace <old text> --with <new text>")
	}
	arguments := map[string]any{"pageId": f.words[0]}
	if f.has("replace") || f.has("with") {
		if !f.has("replace") || !f.has("with") {
			return client.Errorf("Give both: --replace <old text> --with <new text>")
		}
		arguments["replace"] = map[string]any{"oldText": f.get("replace"), "newText": f.get("with")}
	}
	if raw := f.get("edits"); raw != "" {
		var edits []any
		if err := jsonValue(raw, &edits); err != nil {
			return client.Errorf(`--edits must be a JSON array, for example '[{"startLine":3,"endLine":3,"newContent":"…"}]'.`)
		}
		arguments["edits"] = edits
	}
	if title := f.get("title"); title != "" {
		arguments["title"] = title
	}
	if len(arguments) == 1 {
		return client.Errorf("Say what to change: --replace <old> --with <new>, --edits '<json>' or --title <title>")
	}
	result, err := client.CallTool(a.origin, "update_page", arguments)
	if err != nil {
		return err
	}
	return a.printResult(result)
}

func (a *App) approval(args []string) error {
	f, err := parseFlags(args, nil, nil, "approval")
	if err != nil {
		return err
	}
	if len(f.words) != 1 {
		return client.Errorf("Give the approval id: sortdo approval <id>")
	}
	result, err := client.CallTool(a.origin, "get_approval", map[string]any{"approvalId": f.words[0]})
	if err != nil {
		return err
	}
	return a.printResult(result)
}

func openBrowser(link string) bool {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", link)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", link)
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return false
		}
		command = exec.Command("xdg-open", link)
	}
	return command.Start() == nil
}

var sentenceEnd = regexp.MustCompile(`\.(\s|$)`)

func firstSentence(text string) string {
	if location := sentenceEnd.FindStringIndex(text); location != nil {
		return text[:location[0]+1]
	}
	return text
}
