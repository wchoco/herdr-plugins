package session_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// runConfig runs a session on c with the config file toml.
func runConfig(t *testing.T, mode session.Mode, c fixture.Capture, toml string, reads ...string) scenario {
	t.Helper()
	cfg := session.Config{Mode: mode, Target: "w1:p1"}
	settings, err := config.Parse([]byte(toml))
	if err != nil {
		cfg.SettingsErr = err
	} else {
		cfg.Settings = &settings
	}
	s := scenario{
		src:  &fakeSource{captures: map[string]fixture.Capture{"w1:p1": c}},
		clip: &fakeClipboard{},
		term: newTerminal(120, 39, reads...),
	}
	s.err = session.Run(context.Background(), cfg,
		session.Deps{Source: s.src, Clipboard: s.clip, Terminal: s.term})
	return s
}

func screenOf(rows ...string) fixture.Capture {
	s := strings.Join(rows, "\n")
	return fixture.Capture{Text: s + "\n", ANSI: strings.ReplaceAll(s, "\n", "\r\n")}
}

func TestNoConfigFileBehavesLikeTheDefaults(t *testing.T) {
	for _, mode := range []session.Mode{session.ModeHint, session.ModeLine} {
		var frames [2]string
		for i, set := range []bool{false, true} {
			cfg := session.Config{Mode: mode, Target: "w1:p1"}
			if set {
				d := config.Default()
				cfg.Settings = &d
			}
			term := newTerminal(80, 12)
			src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "links")}}
			if err := session.Run(context.Background(), cfg, session.Deps{Source: src, Clipboard: &fakeClipboard{}, Terminal: term}); err != nil {
				t.Fatal(err)
			}
			frames[i] = term.frames[0].Dump()
		}
		if frames[0] != frames[1] {
			t.Errorf("%s: no config differs from the defaults:\n%s\n%s", mode, frames[0], frames[1])
		}
	}
}

func TestConfigErrorIsShownAndAnyKeyCloses(t *testing.T) {
	for _, mode := range []session.Mode{session.ModeHint, session.ModeLine} {
		s := runConfig(t, mode, fixture.Load(t, "cjk"), "[hint]\nalphabet = \"a\"", "1", "\r")
		if s.err != nil {
			t.Fatal(s.err)
		}
		if got := statusRow(t, s.term); got != "config: hint.alphabet: needs at least 2 letters" {
			t.Errorf("%s: status row %q", mode, got)
		}
		if len(s.clip.copies) != 0 || len(s.term.frames) != 1 {
			t.Errorf("%s: copies %q, %d frames", mode, s.clip.copies, len(s.term.frames))
		}
		// Nothing is drawn above the status row.
		if got := strings.TrimSpace(s.term.frames[0].RowText(0)); got != "" {
			t.Errorf("%s: row 0 %q", mode, got)
		}
	}
}

// copiedBy returns what typing typed copies in Hint Mode on c.
func copiedBy(t *testing.T, c fixture.Capture, toml, typed string) []string {
	t.Helper()
	return runConfig(t, session.ModeHint, c, toml, typed).clip.copies
}

func TestCustomPatternOutranksOverlappingBuiltins(t *testing.T) {
	// PROJ-1234 holds the number 1234; with the Jira Pattern the whole key
	// is one Match, so there is one Hint ("a") and it copies the key.
	c := screenOf("see PROJ-1234 now")
	jira := "[[hint.patterns]]\nname = \"jira\"\nregex = '\\b[A-Z]+-\\d+\\b'"
	if got, want := copiedBy(t, c, jira, "a"), []string{"PROJ-1234"}; !reflect.DeepEqual(got, want) {
		t.Errorf("with jira: copies %q, want %q", got, want)
	}
	if got, want := copiedBy(t, c, "", "a"), []string{"1234"}; !reflect.DeepEqual(got, want) {
		t.Errorf("without jira: copies %q, want %q", got, want)
	}
}

func TestCustomPatternsRankInWrittenOrderAndMatchGroupIsCopied(t *testing.T) {
	c := screenOf("ticket id=42 open")
	toml := `
[[hint.patterns]]
name  = "id"
regex = 'id=(?P<match>\d+)'

[[hint.patterns]]
name  = "word"
regex = 'id=\d+ \w+'
`
	if got, want := copiedBy(t, c, toml, "a"), []string{"42"}; !reflect.DeepEqual(got, want) {
		t.Errorf("copies %q, want %q", got, want)
	}
}

func TestDisableNumberStopsNumberHints(t *testing.T) {
	c := screenOf("took 12345 ms")
	if got, want := copiedBy(t, c, "", "a"), []string{"12345"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default: copies %q, want %q", got, want)
	}
	s := runConfig(t, session.ModeHint, c, `hint.disable = ["number"]`, "a")
	if got := statusRow(t, s.term); got != "no matches" || len(s.clip.copies) != 0 {
		t.Errorf("disabled: status %q, copies %q", got, s.clip.copies)
	}
}

func TestDisablingEveryBuiltinGivesNoMatches(t *testing.T) {
	all := `hint.disable = ["markdown_url", "url", "diff_a", "diff_b", "docker",
  "path", "color", "uuid", "sha", "ipv4", "ipv6", "address", "number"]`
	s := runConfig(t, session.ModeHint, fixture.Load(t, "links"), all, "a")
	if got := statusRow(t, s.term); got != "no matches" {
		t.Errorf("status row %q", got)
	}
}

func TestCustomAlphabetChangesTheHints(t *testing.T) {
	c := screenOf("/one /two")
	// Bottom-right first: /two gets the first letter of the alphabet.
	if got, want := copiedBy(t, c, `hint.alphabet = "jk"`, "j"), []string{"/two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("j: copies %q, want %q", got, want)
	}
	if got, want := copiedBy(t, c, `hint.alphabet = "jk"`, "k"), []string{"/one"}; !reflect.DeepEqual(got, want) {
		t.Errorf("k: copies %q, want %q", got, want)
	}
	if got := copiedBy(t, c, `hint.alphabet = "jk"`, "a"); len(got) != 0 {
		t.Errorf("a: copies %q", got)
	}
}

func TestSeparatorChangesJoinedCopy(t *testing.T) {
	c := screenOf("first", "second")
	s := runConfig(t, session.ModeLine, c, `line.separator = " "`, "1-2", "\n")
	if want := []string{"first second"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies %q, want %q", s.clip.copies, want)
	}
}

func TestRebindingCommitKeys(t *testing.T) {
	c := screenOf("first", "second")
	toml := "[line]\ncopy = [\"space\"]\njoined = [\"ctrl-y\", \"tab\"]"
	tests := []struct {
		name  string
		reads []string
		want  []string
	}{
		{"space copies", []string{"1-2", " "}, []string{"first\nsecond"}},
		{"enter is no longer bound", []string{"1-2", "\r", " "}, []string{"first\nsecond"}},
		{"ctrl-y joins", []string{"1-2", "\x19"}, []string{"firstsecond"}},
		{"tab joins", []string{"1-2", "\t"}, []string{"firstsecond"}},
		{"ctrl-j is no longer bound", []string{"1-2", "\n", "\t"}, []string{"firstsecond"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := runConfig(t, session.ModeLine, c, toml, tt.reads...)
			if !reflect.DeepEqual(s.clip.copies, tt.want) {
				t.Errorf("copies %q, want %q", s.clip.copies, tt.want)
			}
		})
	}
}

func TestCommitKeyBoundByLetterAliasMatchesItsLegacyBytes(t *testing.T) {
	// ctrl-m arrives as "\r" from a legacy terminal and as CSI 109;5u
	// from a kitty one.
	c := screenOf("first", "second")
	for _, key := range []string{"\r", "\x1b[109;5u"} {
		s := runConfig(t, session.ModeLine, c, "[line]\ncopy = [\"ctrl-m\"]\njoined = [\"ctrl-j\"]", "2", key)
		if want := []string{"first"}; !reflect.DeepEqual(s.clip.copies, want) {
			t.Errorf("%q: copies %q, want %q", key, s.clip.copies, want)
		}
	}
}
