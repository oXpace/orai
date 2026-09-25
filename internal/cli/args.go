package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// UsageError is a command-line mistake (exit code 2).
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

func usagef(format string, args ...any) error { return &UsageError{fmt.Sprintf(format, args...)} }

// options parses GNU-style flags interspersed with positionals: `--name value`,
// `--name=value`, bool `--name`, and `--` to end flags. The standard flag package stops
// at the first positional, which `orai msg send staff --body x` needs to get past.
type options struct {
	values map[string]string
	bools  map[string]bool
	args   []string
}

func (o options) has(name string) bool   { _, ok := o.values[name]; return ok || o.bools[name] }
func (o options) get(name string) string { return o.values[name] }
func (o options) flag(name string) bool  { return o.bools[name] }
func (o options) intValue(name string) (int, bool, error) {
	raw, ok := o.values[name]
	if !ok {
		return 0, false, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, usagef("--%s expects an integer, got %q", name, raw)
	}
	return n, true, nil
}

// spec maps flag names (without dashes) to whether they take a value.
func parse(argv []string, spec map[string]bool) (options, error) {
	o := options{values: map[string]string{}, bools: map[string]bool{}}
	for i := 0; i < len(argv); i++ {
		token := argv[i]
		if token == "--" {
			o.args = append(o.args, argv[i+1:]...)
			break
		}
		if !strings.HasPrefix(token, "-") || token == "-" {
			o.args = append(o.args, token)
			continue
		}
		name, value, inline := strings.Cut(strings.TrimLeft(token, "-"), "=")
		if token == "-h" {
			name = "help"
		}
		takesValue, known := spec[name]
		if name == "help" {
			o.bools["help"] = true
			continue
		}
		if !known {
			return o, usagef("unrecognized option %s", token)
		}
		if !takesValue {
			if inline {
				return o, usagef("option --%s takes no value", name)
			}
			o.bools[name] = true
			continue
		}
		if !inline {
			if i+1 >= len(argv) {
				return o, usagef("option --%s needs a value", name)
			}
			i++
			value = argv[i]
		}
		if _, dup := o.values[name]; dup {
			return o, usagef("option --%s given twice", name)
		}
		o.values[name] = value
	}
	return o, nil
}

// firstWord finds the command word, skipping a leading --project PATH.
func firstWord(argv []string) (int, string) {
	for i := 0; i < len(argv); i++ {
		switch token := argv[i]; {
		case token == "--project":
			i++
		case strings.HasPrefix(token, "--project="):
		default:
			return i, token
		}
	}
	return -1, ""
}
