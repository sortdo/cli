package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sortdo/cli/internal/client"
	"github.com/sortdo/cli/internal/store"
)

// Every tool the server lists is a command: manage_teams is `sortdo teams`, its action add_members is
// `sortdo teams add-members`, and the input schema's properties are options (teamId is --team-id). New server tools
// appear without a new sortdo release. The list is cached per server for a day.

const toolsCacheName = "tools"
const toolsCacheAge = 24 * time.Hour

// toolCommand is one tool as a command.
type toolCommand struct {
	name    string
	tool    client.Tool
	actions []string // the schema's action values, as the server spells them
}

// commandName is the command for a tool: without "manage_", with hyphens.
func commandName(tool string) string {
	return strings.ReplaceAll(strings.TrimPrefix(tool, "manage_"), "_", "-")
}

func actionName(action string) string { return strings.ReplaceAll(action, "_", "-") }

// kebab turns a property name such as memberIds into the option name member-ids.
func kebab(name string) string {
	var out strings.Builder
	for index, r := range name {
		if unicode.IsUpper(r) {
			if index > 0 {
				out.WriteByte('-')
			}
			out.WriteRune(unicode.ToLower(r))
		} else if r == '_' {
			out.WriteByte('-')
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func toolCommands(tools []client.Tool) map[string]*toolCommand {
	commands := map[string]*toolCommand{}
	for _, tool := range tools {
		command := &toolCommand{name: commandName(tool.Name), tool: tool}
		if action, ok := schemaProperties(tool.InputSchema)["action"]; ok {
			command.actions = enumValues(action)
		}
		commands[command.name] = command
	}
	return commands
}

func schemaProperties(schema map[string]any) map[string]map[string]any {
	result := map[string]map[string]any{}
	properties, _ := schema["properties"].(map[string]any)
	for name, value := range properties {
		if property, ok := value.(map[string]any); ok {
			result[name] = property
		}
	}
	return result
}

func enumValues(property map[string]any) []string {
	var values []string
	if list, ok := property["enum"].([]any); ok {
		for _, value := range list {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if list, ok := property[key].([]any); ok {
			for _, option := range list {
				if entry, ok := option.(map[string]any); ok {
					if text, ok := entry["const"].(string); ok {
						values = append(values, text)
					}
				}
			}
		}
	}
	return values
}

func requiredNames(schema map[string]any) []string {
	var names []string
	if list, ok := schema["required"].([]any); ok {
		for _, value := range list {
			if text, ok := value.(string); ok {
				names = append(names, text)
			}
		}
	}
	return names
}

// option is one schema property as a command-line option.
type option struct {
	property    string
	flag        string
	kind        string // string, integer, number, boolean, array, object, or json for anything else
	items       string // the item type of an array
	enum        []string
	description string
	required    bool
}

func (command *toolCommand) options() []option {
	required := requiredNames(command.tool.InputSchema)
	var options []option
	for name, property := range schemaProperties(command.tool.InputSchema) {
		if name == "action" && len(command.actions) > 0 {
			continue
		}
		kind, _ := property["type"].(string)
		switch kind {
		case "string", "integer", "number", "boolean", "array", "object":
		default:
			kind = "json"
		}
		item := ""
		if items, ok := property["items"].(map[string]any); ok {
			item, _ = items["type"].(string)
		}
		description, _ := property["description"].(string)
		options = append(options, option{
			property: name, flag: kebab(name), kind: kind, items: item, enum: enumValues(property),
			description: description, required: slices.Contains(required, name),
		})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].flag < options[j].flag })
	return options
}

func (o option) placeholder() string {
	switch o.kind {
	case "boolean":
		return ""
	case "integer", "number":
		return "<number>"
	case "array":
		if o.items == "object" {
			return "<json array>"
		}
		return "<value>…"
	case "object", "json":
		return "<json>"
	}
	if len(o.enum) > 0 && len(o.enum) <= 6 {
		return "<" + strings.Join(o.enum, "|") + ">"
	}
	return "<text>"
}

// convert turns one option value into the JSON value the schema asks for.
func (o option) convert(value string) (any, error) {
	bad := func(want string) error {
		return client.Errorf("--%s needs %s, not %q.", o.flag, want, value)
	}
	switch o.kind {
	case "string":
		if len(o.enum) > 0 && !slices.Contains(o.enum, value) {
			return nil, client.Errorf("--%s must be one of: %s.", o.flag, strings.Join(o.enum, ", "))
		}
		return value, nil
	case "integer":
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, bad("a whole number")
		}
		return number, nil
	case "number":
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, bad("a number")
		}
		return number, nil
	case "boolean":
		switch strings.ToLower(value) {
		case "true", "yes", "1", "":
			return true, nil
		case "false", "no", "0":
			return false, nil
		}
		return nil, bad("true or false")
	case "object":
		var parsed map[string]any
		if json.Unmarshal([]byte(value), &parsed) != nil {
			return nil, bad(`a JSON object, for example '{"read":true}'`)
		}
		return parsed, nil
	case "json":
		var parsed any
		if json.Unmarshal([]byte(value), &parsed) == nil {
			return parsed, nil
		}
		return value, nil
	}
	return value, nil
}

// convertItem turns one array item. A value that starts with "[" is a whole JSON array.
func (o option) convertItems(value string) ([]any, error) {
	if strings.HasPrefix(strings.TrimSpace(value), "[") {
		var parsed []any
		if json.Unmarshal([]byte(value), &parsed) != nil {
			return nil, client.Errorf("--%s: %q is not a JSON array.", o.flag, value)
		}
		return parsed, nil
	}
	item := option{flag: o.flag, kind: o.items}
	if item.kind == "" {
		item.kind = "json"
	}
	converted, err := item.convert(value)
	if err != nil {
		return nil, err
	}
	return []any{converted}, nil
}

// readValue reads "-" (stdin) or a file for --args and --<name>-file.
func readValue(source string, stdin io.Reader) (string, error) {
	if source == "-" {
		data, err := io.ReadAll(stdin)
		return string(data), err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", client.Errorf("Could not read %s: %v", source, err)
	}
	return string(data), nil
}

// parseToolArguments builds the tool's arguments from options. --args gives a JSON object (or - for stdin, or
// @file); options win over it. A string option also takes --<name>-file <path> (or - for stdin).
func (command *toolCommand) parseToolArguments(tokens []string, action string, stdin io.Reader) (map[string]any, error) {
	usage := command.name
	if action != "" {
		usage += " " + actionName(action)
	}
	arguments := map[string]any{}
	byFlag := map[string]option{}
	for _, o := range command.options() {
		byFlag[o.flag] = o
		byFlag[o.property] = o
	}
	set := map[string]bool{}
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if !strings.HasPrefix(token, "--") || token == "--" {
			return nil, client.Errorf("Unexpected %q. Give values as options, for example --page-id <id>. See: sortdo %s --help", token, usage)
		}
		name, value, inline := strings.Cut(token[2:], "=")
		next := func() (string, error) {
			if inline {
				return value, nil
			}
			if index+1 >= len(tokens) {
				return "", client.Errorf("--%s needs a value. See: sortdo %s --help", name, usage)
			}
			index++
			return tokens[index], nil
		}
		if name == "args" {
			raw, err := next()
			if err != nil {
				return nil, err
			}
			if raw == "-" || strings.HasPrefix(raw, "@") {
				if raw, err = readValue(strings.TrimPrefix(raw, "@"), stdin); err != nil {
					return nil, err
				}
			}
			var given map[string]any
			if json.Unmarshal([]byte(raw), &given) != nil {
				return nil, client.Errorf(`--args must be a JSON object, for example --args '{"teamId":"…"}'.`)
			}
			for key, item := range given {
				if !set[key] {
					arguments[key] = item
				}
			}
			continue
		}
		o, known := byFlag[name]
		fromFile := false
		if !known && strings.HasSuffix(name, "-file") {
			if candidate, ok := byFlag[strings.TrimSuffix(name, "-file")]; ok && candidate.kind == "string" {
				o, known, fromFile = candidate, true, true
			}
		}
		if !known {
			return nil, client.Errorf("Unknown option --%s for sortdo %s. See: sortdo %s --help", name, usage, usage)
		}
		if o.kind == "boolean" && !inline {
			// A boolean option takes an explicit true or false right after it, else it is true.
			if index+1 < len(tokens) && slices.Contains([]string{"true", "false"}, tokens[index+1]) {
				index++
				value = tokens[index]
			} else {
				value = "true"
			}
		} else {
			var err error
			if value, err = next(); err != nil {
				return nil, err
			}
		}
		if fromFile {
			var err error
			if value, err = readValue(value, stdin); err != nil {
				return nil, err
			}
		}
		if o.kind == "array" {
			items, err := o.convertItems(value)
			if err != nil {
				return nil, err
			}
			if !set[o.property] {
				arguments[o.property] = []any{}
			}
			arguments[o.property] = append(arguments[o.property].([]any), items...)
		} else {
			converted, err := o.convert(value)
			if err != nil {
				return nil, err
			}
			arguments[o.property] = converted
		}
		set[o.property] = true
	}
	if action != "" {
		arguments["action"] = action
	}
	var missing []string
	for _, o := range command.options() {
		if _, ok := arguments[o.property]; o.required && !ok {
			missing = append(missing, "--"+o.flag)
		}
	}
	if len(missing) > 0 {
		return nil, client.Errorf("Missing %s. See: sortdo %s --help", strings.Join(missing, ", "), usage)
	}
	return arguments, nil
}

// resolveAction finds the action a word names, with hyphens or underscores.
func (command *toolCommand) resolveAction(word string) (string, bool) {
	for _, action := range command.actions {
		if action == word || actionName(action) == word {
			return action, true
		}
	}
	return "", false
}

// help is the --help text of the command, or of one of its actions.
func (command *toolCommand) help(action string) string {
	var out strings.Builder
	title := command.tool.Title
	if title == "" {
		title = command.tool.Name
	}
	fmt.Fprintf(&out, "sortdo %s — %s\n\n", command.name, title)
	fmt.Fprintln(&out, wrap(command.tool.Description, 0))
	fmt.Fprintln(&out)
	switch {
	case action != "":
		fmt.Fprintf(&out, "Usage: sortdo %s %s [options]\n", command.name, actionName(action))
	case len(command.actions) > 0:
		fmt.Fprintf(&out, "Usage: sortdo %s <action> [options]\n\nActions:\n", command.name)
		names := make([]string, len(command.actions))
		for index, value := range command.actions {
			names[index] = actionName(value)
		}
		fmt.Fprintln(&out, wrap(strings.Join(names, ", "), 2))
	default:
		fmt.Fprintf(&out, "Usage: sortdo %s [options]\n", command.name)
	}
	options := command.options()
	// For an action, the options whose description names it come first.
	var first, rest []option
	for _, o := range options {
		if action != "" && mentions(o.description, action) {
			first = append(first, o)
		} else {
			rest = append(rest, o)
		}
	}
	if len(first) > 0 {
		fmt.Fprintf(&out, "\nOptions for %s:\n", actionName(action))
		writeOptions(&out, first)
		if len(rest) > 0 {
			fmt.Fprintln(&out, "\nOther options:")
			writeOptions(&out, rest)
		}
	} else if len(rest) > 0 {
		fmt.Fprintln(&out, "\nOptions:")
		writeOptions(&out, rest)
	}
	fmt.Fprintln(&out, "\nAlso:")
	writeRows(&out, [][2]string{
		{"--args <json | - | @file>", "All arguments as one JSON object, from stdin with -, or from a file. Options above win."},
		{"--<option>-file <path | ->", "Read a text option from a file, or from stdin with -."},
		{"--json", "Print the tool's data as JSON."},
	})
	return out.String()
}

func mentions(description, action string) bool {
	for _, field := range strings.FieldsFunc(description, func(r rune) bool { return !unicode.IsLetter(r) && r != '_' }) {
		if field == action {
			return true
		}
	}
	return false
}

func writeOptions(out *strings.Builder, options []option) {
	rows := make([][2]string, 0, len(options))
	for _, o := range options {
		left := "--" + o.flag
		if placeholder := o.placeholder(); placeholder != "" {
			left += " " + placeholder
		}
		description := o.description
		if o.required {
			description = "Required. " + description
		}
		if o.kind == "array" && o.items != "object" {
			description += " Repeat the option for each value, or give a JSON array."
		}
		if len(o.enum) > 6 {
			description += " One of: " + strings.Join(o.enum, ", ") + "."
		}
		rows = append(rows, [2]string{left, strings.TrimSpace(description)})
	}
	writeRows(out, rows)
}

// writeRows prints two columns; long left sides put the description on the next line.
func writeRows(out *strings.Builder, rows [][2]string) {
	const column = 30
	for _, row := range rows {
		if len(row[0]) <= column-4 {
			text := wrap(row[1], column)
			fmt.Fprintf(out, "  %-*s%s\n", column-2, row[0], strings.TrimLeft(text, " "))
		} else {
			fmt.Fprintf(out, "  %s\n%s\n", row[0], wrap(row[1], column))
		}
	}
}

// wrap breaks text into lines of at most 100 characters, indented.
func wrap(text string, indent int) string {
	const width = 100
	var lines []string
	line := strings.Repeat(" ", indent)
	for _, word := range strings.Fields(text) {
		if len(line) > indent && len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = strings.Repeat(" ", indent)
		}
		if len(line) > indent {
			line += " "
		}
		line += word
	}
	if strings.TrimSpace(line) != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// loadTools returns the server's tools from the cache, or from the server when the cache is old, missing or refresh
// is set. When the server cannot be reached, an old cache still serves.
func loadTools(origin string, refresh bool) ([]client.Tool, error) {
	var cached []client.Tool
	savedAt, ok := store.ReadCache(toolsCacheName, origin, &cached)
	if ok && !refresh && time.Since(savedAt) < toolsCacheAge {
		return cached, nil
	}
	tools, err := client.ListTools(origin)
	if err != nil {
		if ok {
			return cached, nil
		}
		return nil, err
	}
	store.WriteCache(toolsCacheName, origin, tools)
	return tools, nil
}

// cachedTools returns the cached tools without a request, for help.
func cachedTools(origin string) []client.Tool {
	var cached []client.Tool
	store.ReadCache(toolsCacheName, origin, &cached)
	return cached
}
