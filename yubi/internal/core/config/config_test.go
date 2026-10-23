package config_test

import (
	"reflect"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
)

func names(ps []hint.Pattern) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func TestEmptyConfigIsTheDefaults(t *testing.T) {
	for _, in := range []string{"", "# nothing\n", "[hint]\n[line]\n"} {
		got, err := config.Parse([]byte(in))
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		want := config.Default()
		if got.Alphabet != want.Alphabet || got.Separator != want.Separator ||
			!reflect.DeepEqual(names(got.Patterns), names(want.Patterns)) ||
			!reflect.DeepEqual(got.Copy, want.Copy) || !reflect.DeepEqual(got.Joined, want.Joined) {
			t.Errorf("Parse(%q) = %+v, want the defaults", in, got)
		}
	}
}

func TestDefaults(t *testing.T) {
	d := config.Default()
	if d.Alphabet != "asdfqwerzxcvjklmiuopghtybn" {
		t.Errorf("alphabet %q", d.Alphabet)
	}
	if !reflect.DeepEqual(names(d.Patterns), names(hint.Builtins())) {
		t.Errorf("patterns %v", names(d.Patterns))
	}
	if want := []keys.Key{{Code: keys.Enter}}; !reflect.DeepEqual(d.Copy, want) {
		t.Errorf("copy %v", d.Copy)
	}
	if want := []keys.Key{{Code: keys.Enter, Mod: keys.Ctrl}, {Code: 'j', Mod: keys.Ctrl}}; !reflect.DeepEqual(d.Joined, want) {
		t.Errorf("joined %v", d.Joined)
	}
	if d.Separator != "" {
		t.Errorf("separator %q", d.Separator)
	}
}

func TestValidSettings(t *testing.T) {
	got, err := config.Parse([]byte(`
[hint]
alphabet = "jk"
disable  = ["number", "sha"]

[[hint.patterns]]
name  = "jira"
regex = '\b[A-Z]+-\d+\b'

[[hint.patterns]]
name  = "url"
regex = 'id=(?P<match>\d+)'

[line]
copy      = ["space", "ctrl-shift-y"]
joined    = ["tab", "shift-a", "ctrl-space"]
separator = " "
`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Alphabet != "jk" || got.Separator != " " {
		t.Errorf("alphabet %q, separator %q", got.Alphabet, got.Separator)
	}
	// A Custom Pattern named like a Built-in sits next to it, it does
	// not replace it.
	want := []string{"jira", "url", "markdown_url", "url", "diff_a", "diff_b", "docker", "path",
		"color", "uuid", "ipv4", "ipv6", "address"}
	if !reflect.DeepEqual(names(got.Patterns), want) {
		t.Errorf("patterns %v, want %v", names(got.Patterns), want)
	}
	if got.Patterns[0].Re.String() != `\b[A-Z]+-\d+\b` {
		t.Errorf("jira regex %q", got.Patterns[0].Re)
	}
	if want := []keys.Key{{Code: ' '}, {Code: 'y', Mod: keys.Ctrl | keys.Shift}}; !reflect.DeepEqual(got.Copy, want) {
		t.Errorf("copy %v, want %v", got.Copy, want)
	}
	if want := []keys.Key{{Code: keys.Tab}, {Code: 'A'}, {Code: ' ', Mod: keys.Ctrl}}; !reflect.DeepEqual(got.Joined, want) {
		t.Errorf("joined %v, want %v", got.Joined, want)
	}
}

func TestInlinePatternsAndEmptyLists(t *testing.T) {
	got, err := config.Parse([]byte(`hint = { patterns = [{ name = "a", regex = "a+" }], disable = [] }`))
	if err != nil {
		t.Fatal(err)
	}
	if n := names(got.Patterns); len(n) != len(hint.Builtins())+1 || n[0] != "a" {
		t.Errorf("patterns %v", n)
	}
	if _, err := config.Parse([]byte("hint.patterns = []")); err != nil {
		t.Errorf("empty patterns: %v", err)
	}
}

func TestDisablingEveryBuiltinLeavesNoPatterns(t *testing.T) {
	got, err := config.Parse([]byte(`hint.disable = ["markdown_url", "url", "diff_a", "diff_b", "docker",
  "path", "color", "uuid", "sha", "ipv4", "ipv6", "address", "number"]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Patterns) != 0 {
		t.Errorf("patterns %v", names(got.Patterns))
	}
}
