package app

import (
	"strings"

	"github.com/sortdo/cli/internal/client"
)

// globals are the options every command takes, in any position.
type globals struct {
	server  string
	json    bool
	help    bool
	version bool
	rest    []string // everything else, in order
}

// splitGlobals takes --server, --json, --help (-h) and --version out of the arguments.
func splitGlobals(argv []string) (globals, error) {
	var result globals
	for index := 0; index < len(argv); index++ {
		arg := argv[index]
		if arg == "--" {
			result.rest = append(result.rest, argv[index:]...)
			break
		}
		switch {
		case arg == "-h" || arg == "--help":
			result.help = true
		case arg == "--json":
			result.json = true
		case arg == "--version" || arg == "-v":
			result.version = true
		case arg == "--server":
			if index+1 >= len(argv) {
				return result, client.Errorf("--server needs an address, for example --server https://trysort.com")
			}
			index++
			result.server = argv[index]
		case strings.HasPrefix(arg, "--server="):
			result.server = strings.TrimPrefix(arg, "--server=")
		default:
			result.rest = append(result.rest, arg)
		}
	}
	return result, nil
}

// flags are one command's parsed arguments: positional words, and option values (repeatable).
type flags struct {
	words  []string
	values map[string][]string
}

func (f flags) get(name string) string {
	if values := f.values[name]; len(values) > 0 {
		return values[len(values)-1]
	}
	return ""
}

func (f flags) has(name string) bool {
	_, ok := f.values[name]
	return ok
}

func (f flags) bool(name string) bool {
	value := f.get(name)
	return f.has(name) && value != "false" && value != "0"
}

// parseFlags reads --name value, --name=value and boolean --name. A word after "--" is always positional.
// Options that are not in valueOptions or boolOptions are refused, so a typo never changes what runs.
func parseFlags(tokens []string, valueOptions, boolOptions []string, command string) (flags, error) {
	result := flags{values: map[string][]string{}}
	takesValue := map[string]bool{}
	for _, name := range valueOptions {
		takesValue[name] = true
	}
	isBool := map[string]bool{}
	for _, name := range boolOptions {
		isBool[name] = true
	}
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token == "--" {
			result.words = append(result.words, tokens[index+1:]...)
			break
		}
		if !strings.HasPrefix(token, "--") || len(token) == 2 {
			result.words = append(result.words, token)
			continue
		}
		name, value, inline := strings.Cut(token[2:], "=")
		switch {
		case isBool[name]:
			if !inline {
				value = "true"
			}
		case takesValue[name]:
			if !inline {
				if index+1 >= len(tokens) {
					return result, client.Errorf("--%s needs a value. See: sortdo %s --help", name, command)
				}
				index++
				value = tokens[index]
			}
		default:
			return result, client.Errorf("Unknown option --%s. See: sortdo %s --help", name, command)
		}
		result.values[name] = append(result.values[name], value)
	}
	return result, nil
}
