// Package config parses and validates yubi's config.toml. It is pure: the
// caller reads the file. Validation is strict and the first error wins,
// with its location, e.g. `config: hint.patterns[1].regex: missing
// closing )`. There is no fallback to the defaults on error.
//
// Errors are found in schema order: within a table its unknown keys
// first (sorted), then its settings in the order the schema lists them,
// [hint], [line], then [clipboard]. Array indexes count from 0.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
)

// Config is a validated config.
type Config struct {
	Alphabet string         // Hint alphabet
	Patterns []hint.Pattern // Custom Patterns in written order, then the enabled Built-ins

	// Line Mode commit keys, in keys.Canon form.
	Copy, Joined []keys.Key
	Separator    string // joins the lines of a Joined Copy

	// CopyCommand is the argv that receives copied text on stdin instead
	// of OSC 52; nil means OSC 52.
	CopyCommand []string
}

// Default returns the config used when there is no config file.
func Default() Config {
	return Config{
		Alphabet: hint.DefaultAlphabet,
		Patterns: hint.Builtins(),
		Copy:     []keys.Key{{Code: keys.Enter}},
		Joined:   []keys.Key{{Code: keys.Enter, Mod: keys.Ctrl}, {Code: 'j', Mod: keys.Ctrl}},
	}
}

// The default commit key names, for reporting a conflict with them.
var (
	defaultCopy   = []string{"enter"}
	defaultJoined = []string{"ctrl-enter", "ctrl-j"}
)

// Parse parses the contents of config.toml. Every setting is optional.
func Parse(data []byte) (Config, error) {
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return Config{}, fmt.Errorf("config: line %d: %s", pe.Position.Line, pe.Message)
		}
		return Config{}, fmt.Errorf("config: %v", err)
	}
	c := Default()
	if err := parse(raw, &c); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

// locError is an error at a key path.
type locError struct{ path, msg string }

func (e *locError) Error() string { return e.path + ": " + e.msg }

func errAt(path, format string, args ...any) error {
	return &locError{path, fmt.Sprintf(format, args...)}
}

func parse(raw map[string]any, c *Config) error {
	if err := unknown("", raw, "hint", "line", "clipboard"); err != nil {
		return err
	}
	if v, ok := raw["hint"]; ok {
		t, ok := v.(map[string]any)
		if !ok {
			return wrongType("hint", "table", v)
		}
		if err := parseHint(t, c); err != nil {
			return err
		}
	}
	if v, ok := raw["line"]; ok {
		t, ok := v.(map[string]any)
		if !ok {
			return wrongType("line", "table", v)
		}
		if err := parseLine(t, c); err != nil {
			return err
		}
	}
	if v, ok := raw["clipboard"]; ok {
		t, ok := v.(map[string]any)
		if !ok {
			return wrongType("clipboard", "table", v)
		}
		if err := parseClipboard(t, c); err != nil {
			return err
		}
	}
	return nil
}

func parseClipboard(t map[string]any, c *Config) error {
	if err := unknown("clipboard.", t, "copy_command"); err != nil {
		return err
	}
	v, ok := t["copy_command"]
	if !ok {
		return nil
	}
	argv, err := stringList("clipboard.copy_command", v)
	if err != nil {
		return err
	}
	if len(argv) == 0 {
		return errAt("clipboard.copy_command", "must not be empty")
	}
	if argv[0] == "" {
		return errAt("clipboard.copy_command[0]", "required")
	}
	c.CopyCommand = argv
	return nil
}

func parseHint(t map[string]any, c *Config) error {
	if err := unknown("hint.", t, "alphabet", "disable", "patterns"); err != nil {
		return err
	}
	if v, ok := t["alphabet"]; ok {
		s, ok := v.(string)
		if !ok {
			return wrongType("hint.alphabet", "string", v)
		}
		if err := checkAlphabet(s); err != nil {
			return err
		}
		c.Alphabet = s
	}
	disabled := map[string]bool{}
	if v, ok := t["disable"]; ok {
		ds, err := stringList("hint.disable", v)
		if err != nil {
			return err
		}
		known := map[string]bool{}
		for _, p := range hint.Builtins() {
			known[p.Name] = true
		}
		for i, d := range ds {
			if !known[d] {
				return errAt(fmt.Sprintf("hint.disable[%d]", i), "unknown Built-in Pattern %q", d)
			}
			disabled[d] = true
		}
	}
	var pats []hint.Pattern
	if v, ok := t["patterns"]; ok {
		var err error
		if pats, err = parsePatterns(v); err != nil {
			return err
		}
	}
	for _, p := range hint.Builtins() {
		if !disabled[p.Name] {
			pats = append(pats, p)
		}
	}
	c.Patterns = pats
	return nil
}

func checkAlphabet(s string) error {
	seen := map[rune]bool{}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return errAt("hint.alphabet", "%q is not a lowercase letter a-z", string(r))
		}
		if seen[r] {
			return errAt("hint.alphabet", "%q repeats", string(r))
		}
		seen[r] = true
	}
	if len(s) < 2 {
		return errAt("hint.alphabet", "needs at least 2 letters")
	}
	return nil
}

// parsePatterns returns the Custom Patterns in written order.
func parsePatterns(v any) ([]hint.Pattern, error) {
	var tables []any
	switch v := v.(type) {
	case []map[string]any: // [[hint.patterns]]
		for _, t := range v {
			tables = append(tables, t)
		}
	case []any: // patterns = [{...}]
		tables = v
	default:
		return nil, wrongType("hint.patterns", "array of tables", v)
	}
	var out []hint.Pattern
	seen := map[string]bool{}
	for i, v := range tables {
		at := fmt.Sprintf("hint.patterns[%d]", i)
		t, ok := v.(map[string]any)
		if !ok {
			return nil, wrongType(at, "table", v)
		}
		if err := unknown(at+".", t, "name", "regex"); err != nil {
			return nil, err
		}
		name, err := required(at+".name", t["name"])
		if err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, errAt(at+".name", "duplicate %q", name)
		}
		seen[name] = true
		expr, err := required(at+".regex", t["regex"])
		if err != nil {
			return nil, err
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			var se *syntax.Error
			if errors.As(err, &se) {
				return nil, errAt(at+".regex", "%s", se.Code)
			}
			return nil, errAt(at+".regex", "%v", err)
		}
		out = append(out, hint.Pattern{Name: name, Re: re})
	}
	return out, nil
}

// required returns v as a non-empty string.
func required(path string, v any) (string, error) {
	if v == nil {
		return "", errAt(path, "required")
	}
	s, ok := v.(string)
	if !ok {
		return "", wrongType(path, "string", v)
	}
	if s == "" {
		return "", errAt(path, "required")
	}
	return s, nil
}

func parseLine(t map[string]any, c *Config) error {
	if err := unknown("line.", t, "copy", "joined", "separator"); err != nil {
		return err
	}
	copyNames, joinedNames := defaultCopy, defaultJoined
	_, copySet := t["copy"]
	_, joinedSet := t["joined"]
	if copySet {
		var err error
		if copyNames, c.Copy, err = commitKeys("line.copy", t["copy"]); err != nil {
			return err
		}
	}
	if joinedSet {
		var err error
		if joinedNames, c.Joined, err = commitKeys("line.joined", t["joined"]); err != nil {
			return err
		}
	}
	if v, ok := t["separator"]; ok {
		s, ok := v.(string)
		if !ok {
			return wrongType("line.separator", "string", v)
		}
		c.Separator = s
	}
	// A conflict is reported in a list the user wrote: joined if both.
	in := func(k keys.Key, ks []keys.Key) bool {
		for _, x := range ks {
			if x == k {
				return true
			}
		}
		return false
	}
	if joinedSet {
		for i, k := range c.Joined {
			if in(k, c.Copy) {
				return errAt(fmt.Sprintf("line.joined[%d]", i), "%q is also in line.copy", joinedNames[i])
			}
		}
	} else {
		for i, k := range c.Copy {
			if in(k, c.Joined) {
				return errAt(fmt.Sprintf("line.copy[%d]", i), "%q is also in line.joined", copyNames[i])
			}
		}
	}
	return nil
}

// commitKeys parses a non-empty list of commit key names.
func commitKeys(path string, v any) ([]string, []keys.Key, error) {
	names, err := stringList(path, v)
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, errAt(path, "must not be empty")
	}
	var out []keys.Key
	for i, n := range names {
		at := fmt.Sprintf("%s[%d]", path, i)
		k, err := parseKey(n)
		if err != nil {
			return nil, nil, errAt(at, "%s", err.Error())
		}
		out = append(out, k)
	}
	return names, out, nil
}

// inputKeys are the Line Mode keys that edit or cancel, which cannot
// commit. Digits, `-` and `,` cannot be named at all.
var inputKeys = []keys.Key{{Code: 'u', Mod: keys.Ctrl}, {Code: 'c', Mod: keys.Ctrl}}

// parseKey parses `[ctrl-][shift-]<name>`, name being enter, tab, space or
// one lowercase letter, into its keys.Canon form.
func parseKey(s string) (keys.Key, error) {
	if strings.HasPrefix(s, "alt-") || strings.Contains(s, "-alt-") {
		return keys.Key{}, errors.New("alt- keys are reserved for Paste Copy")
	}
	var k keys.Key
	rest := s
	if r, ok := strings.CutPrefix(rest, "ctrl-"); ok {
		k.Mod |= keys.Ctrl
		rest = r
	}
	if r, ok := strings.CutPrefix(rest, "shift-"); ok {
		k.Mod |= keys.Shift
		rest = r
	}
	switch {
	case rest == "enter":
		k.Code = keys.Enter
	case rest == "tab":
		k.Code = keys.Tab
	case rest == "space":
		k.Code = keys.Space
	case len(rest) == 1 && rest[0] >= 'a' && rest[0] <= 'z':
		k.Code = rune(rest[0])
	default:
		return keys.Key{}, fmt.Errorf("invalid key %q", s)
	}
	k = keys.Canon(k)
	for _, in := range inputKeys {
		if k == in {
			return keys.Key{}, fmt.Errorf("%q is an input key", s)
		}
	}
	return k, nil
}

// stringList returns v as an array of strings.
func stringList(path string, v any) ([]string, error) {
	a, ok := v.([]any)
	if !ok {
		return nil, wrongType(path, "array", v)
	}
	out := make([]string, 0, len(a))
	for i, e := range a {
		s, ok := e.(string)
		if !ok {
			return nil, wrongType(fmt.Sprintf("%s[%d]", path, i), "string", e)
		}
		out = append(out, s)
	}
	return out, nil
}

// unknown reports the first key of t, in sorted order, that is not known.
func unknown(prefix string, t map[string]any, known ...string) error {
	var ks []string
	for k := range t {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		ok := false
		for _, n := range known {
			ok = ok || k == n
		}
		if !ok {
			return errAt(prefix+k, "unknown key")
		}
	}
	return nil
}

func wrongType(path, want string, v any) error {
	return errAt(path, "expected %s, got %s", want, typeName(v))
}

// typeName names a decoded TOML value's type.
func typeName(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case int64:
		return "integer"
	case float64:
		return "float"
	case bool:
		return "boolean"
	case time.Time:
		return "datetime"
	case []any, []map[string]any:
		return "array"
	case map[string]any:
		return "table"
	}
	return fmt.Sprintf("%T", v)
}
