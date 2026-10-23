package config_test

import (
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
)

// Every validation rule, with the exact first error. Pattern indexes count
// from 0, in written order.
func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		// TOML syntax
		{"syntax", "[hint]\nalphabet = ", "config: line 2: unexpected EOF; expected value"},
		{"duplicate key", "[line]\nseparator = \"\"\nseparator = \" \"", "config: line 3: Key 'line.separator' has already been defined."},

		// unknown keys
		{"unknown top-level key", "version = 1", "config: version: unknown key"},
		{"unknown table", "[style]\nhint = \"red\"", "config: style: unknown key"},
		{"unknown hint key", "[hint]\nalphabets = \"ab\"", "config: hint.alphabets: unknown key"},
		{"unknown line key", "[line]\npaste = [\"alt-enter\"]", "config: line.paste: unknown key"},
		{"unknown pattern key", "[[hint.patterns]]\nname = \"a\"\nregex = \"a\"\ngroup = 1", "config: hint.patterns[0].group: unknown key"},
		{"unknown keys in sorted order", "zz = 1\naa = 1", "config: aa: unknown key"},
		{"unknown key before a bad value", "[hint]\nalphabet = \"A\"\nzz = 1", "config: hint.zz: unknown key"},

		// wrong types
		{"hint not a table", "hint = 1", "config: hint: expected table, got integer"},
		{"line not a table", "line = []", "config: line: expected table, got array"},
		{"alphabet not a string", "hint.alphabet = [\"a\", \"b\"]", "config: hint.alphabet: expected string, got array"},
		{"disable not an array", "hint.disable = \"number\"", "config: hint.disable: expected array, got string"},
		{"disable entry not a string", "hint.disable = [1]", "config: hint.disable[0]: expected string, got integer"},
		{"patterns not tables", "hint.patterns = \"jira\"", "config: hint.patterns: expected array of tables, got string"},
		{"pattern not a table", "hint.patterns = [\"jira\"]", "config: hint.patterns[0]: expected table, got string"},
		{"pattern name not a string", "[[hint.patterns]]\nname = 1\nregex = \"a\"", "config: hint.patterns[0].name: expected string, got integer"},
		{"pattern regex not a string", "[[hint.patterns]]\nname = \"a\"\nregex = true", "config: hint.patterns[0].regex: expected string, got boolean"},
		{"copy not an array", "line.copy = \"enter\"", "config: line.copy: expected array, got string"},
		{"joined entry not a string", "line.joined = [1.5]", "config: line.joined[0]: expected string, got float"},
		{"separator not a string", "line.separator = 1979-05-27", "config: line.separator: expected string, got datetime"},

		// Custom Patterns
		{"name missing", "[[hint.patterns]]\nregex = \"a\"", "config: hint.patterns[0].name: required"},
		{"name empty", "[[hint.patterns]]\nname = \"\"\nregex = \"a\"", "config: hint.patterns[0].name: required"},
		{"regex missing", "[[hint.patterns]]\nname = \"a\"", "config: hint.patterns[0].regex: required"},
		{"regex empty", "[[hint.patterns]]\nname = \"a\"\nregex = \"\"", "config: hint.patterns[0].regex: required"},
		{"name repeated", "[[hint.patterns]]\nname = \"jira\"\nregex = \"a\"\n[[hint.patterns]]\nname = \"jira\"\nregex = \"b\"", "config: hint.patterns[1].name: duplicate \"jira\""},
		{"regex does not compile", "[[hint.patterns]]\nname = \"a\"\nregex = \"a\"\n[[hint.patterns]]\nname = \"b\"\nregex = \"(b\"", "config: hint.patterns[1].regex: missing closing )"},
		{"regex is not RE2", "[[hint.patterns]]\nname = \"a\"\nregex = '(?=a)'", "config: hint.patterns[0].regex: invalid or unsupported Perl syntax"},

		// disable
		{"unknown disable name", "hint.disable = [\"number\", \"jira\"]", "config: hint.disable[1]: unknown Built-in Pattern \"jira\""},

		// alphabet
		{"alphabet uppercase", "hint.alphabet = \"aB\"", "config: hint.alphabet: \"B\" is not a lowercase letter a-z"},
		{"alphabet digit", "hint.alphabet = \"a1\"", "config: hint.alphabet: \"1\" is not a lowercase letter a-z"},
		{"alphabet non-ascii", "hint.alphabet = \"aé\"", "config: hint.alphabet: \"é\" is not a lowercase letter a-z"},
		{"alphabet repeats", "hint.alphabet = \"abca\"", "config: hint.alphabet: \"a\" repeats"},
		{"alphabet too short", "hint.alphabet = \"a\"", "config: hint.alphabet: needs at least 2 letters"},
		{"alphabet empty", "hint.alphabet = \"\"", "config: hint.alphabet: needs at least 2 letters"},

		// commit keys
		{"copy empty", "line.copy = []", "config: line.copy: must not be empty"},
		{"joined empty", "line.joined = []", "config: line.joined: must not be empty"},
		{"unknown key name", "line.copy = [\"return\"]", "config: line.copy[0]: invalid key \"return\""},
		{"uppercase key name", "line.copy = [\"Enter\"]", "config: line.copy[0]: invalid key \"Enter\""},
		{"shift before ctrl", "line.copy = [\"shift-ctrl-y\"]", "config: line.copy[0]: invalid key \"shift-ctrl-y\""},
		{"digit key", "line.copy = [\"enter\", \"1\"]", "config: line.copy[1]: invalid key \"1\""},
		{"modifier only", "line.copy = [\"ctrl-\"]", "config: line.copy[0]: invalid key \"ctrl-\""},
		{"alt key", "line.copy = [\"alt-enter\"]", "config: line.copy[0]: alt- keys are reserved for Paste Copy"},
		{"alt after ctrl", "line.joined = [\"ctrl-alt-j\"]", "config: line.joined[0]: alt- keys are reserved for Paste Copy"},
		{"input key ctrl-u", "line.copy = [\"ctrl-u\"]", "config: line.copy[0]: \"ctrl-u\" is an input key"},
		{"input key ctrl-c", "line.joined = [\"ctrl-c\"]", "config: line.joined[0]: \"ctrl-c\" is an input key"},
		{"key in both lists", "[line]\ncopy = [\"enter\", \"tab\"]\njoined = [\"ctrl-j\", \"tab\"]", "config: line.joined[1]: \"tab\" is also in line.copy"},
		{"same bytes in both lists", "[line]\ncopy = [\"enter\"]\njoined = [\"ctrl-m\"]", "config: line.joined[0]: \"ctrl-m\" is also in line.copy"},
		// Only the list the user wrote can be located.
		{"against the default joined", "line.copy = [\"ctrl-j\"]", "config: line.copy[0]: \"ctrl-j\" is also in line.joined"},
		{"against the default copy", "line.joined = [\"ctrl-m\"]", "config: line.joined[0]: \"ctrl-m\" is also in line.copy"},
		{"shift letter is the uppercase letter", "[line]\ncopy = [\"shift-y\"]\njoined = [\"shift-y\"]", "config: line.joined[0]: \"shift-y\" is also in line.copy"},

		// first error wins, in schema order: hint before line
		{"first error wins", "[line]\ncopy = []\n[hint]\nalphabet = \"a\"", "config: hint.alphabet: needs at least 2 letters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.Parse([]byte(tt.in))
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want %q", tt.in, tt.want)
			}
			if err.Error() != tt.want {
				t.Errorf("Parse(%q)\n got %q\nwant %q", tt.in, err, tt.want)
			}
		})
	}
}
