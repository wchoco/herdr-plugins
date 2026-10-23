package hint_test

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

func TestAssign(t *testing.T) {
	tests := []struct {
		name     string
		screen   string
		alphabet string
		want     map[string]string // copied text -> Hint
	}{
		{"none", "nothing", "abcd", map[string]string{}},
		// Shortest Hints nearest the bottom, right to left within a row.
		{"reverse reading order", "a/1 a/2\na/3 a/4\n", "abcd",
			map[string]string{"a/4": "a", "a/3": "b", "a/2": "c", "a/1": "d"}},
		{"two-letter hints go to the top", "a/1 a/2 a/3\na/4 a/5 a/6\n", "abcd",
			map[string]string{"a/6": "a", "a/5": "b", "a/4": "c", "a/3": "da", "a/2": "db", "a/1": "dc"}},
		// Identical text shares one Hint, placed by its lowest occurrence;
		// the list is sized by unique texts, so all four fit in one letter.
		{"duplicates share one hint", "x/d a/1\na/2 x/d\na/3 x/d", "abcd",
			map[string]string{"x/d": "a", "a/3": "b", "a/2": "c", "a/1": "d"}},
		// Beyond k*k unique texts, the top Matches get no Hint.
		{"cap", "a/1 a/2 a/3\na/4 a/5", "ab",
			map[string]string{"a/5": "aa", "a/4": "ab", "a/3": "ba", "a/2": "bb"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := hint.Find(screen.ParseANSI(tt.screen).Rows, hint.Builtins())
			got := hint.Assign(ms, tt.alphabet)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFind(t *testing.T) {
	type m struct {
		row, col, width int
		text            string
	}
	tests := []struct {
		name   string
		screen string
		want   []m
	}{
		{"none", "nothing here\n", nil},
		{"url and path on one row", "see https://x.io/a and /tmp/f.txt",
			[]m{{0, 4, 14, "https://x.io/a"}, {0, 23, 10, "/tmp/f.txt"}}},
		// The url Pattern outranks path where both start at the same cell.
		{"tie goes to url", "file:///etc/hosts", []m{{0, 0, 17, "file:///etc/hosts"}}},
		// Leftmost wins: the path in the URL is consumed by the URL.
		{"leftmost wins, no overlap", "git@github.com:a/b.git x/y",
			[]m{{0, 0, 22, "git@github.com:a/b.git"}, {0, 23, 3, "x/y"}}},
		{"rows in order", "a/b\n\n~/c d/e", []m{{0, 0, 3, "a/b"}, {2, 0, 3, "~/c"}, {2, 4, 3, "d/e"}}},
		// Columns count display cells: each wide glyph is two.
		{"wide glyphs before a match", "日本 /var/log x", []m{{0, 5, 8, "/var/log"}}},
		{"prompt path keeps the dollar", "user@host:~/proj$ ls", []m{{0, 10, 7, "~/proj$"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scr := screen.ParseANSI(tt.screen)
			var got []m
			for _, x := range hint.Find(scr.Rows, hint.Builtins()) {
				got = append(got, m{x.Row, x.Col, x.Width, x.Text})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindCopiesMatchGroup(t *testing.T) {
	pats := []hint.Pattern{
		{Name: "key", Re: regexp.MustCompile(`key=(?P<match>\w+)`)},
		{Name: "word", Re: regexp.MustCompile(`[a-z]+`)},
	}
	got := hint.Find(screen.ParseANSI("key=abc def").Rows, pats)
	want := []hint.Match{
		{Row: 0, Col: 4, Width: 3, Text: "abc", Pattern: "key"},
		{Row: 0, Col: 8, Width: 3, Text: "def", Pattern: "word"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGenerate(t *testing.T) {
	tests := []struct {
		alphabet string
		n        int
		want     string // space-separated
	}{
		{"abcd", 0, ""},
		{"abcd", 1, "a"},
		{"abcd", 4, "a b c d"},
		// tmux-thumbs' own test vectors.
		{"abcd", 6, "a b c da db dc"},
		{"abcd", 8, "a b ca cb da db dc dd"},
		{"abcd", 5, "a b c da db"},
		{"abcd", 16, "aa ab ac ad ba bb bc bd ca cb cc cd da db dc dd"},
		// Capped at k*k.
		{"ab", 9, "aa ab ba bb"},
	}
	for _, tt := range tests {
		got := strings.Join(hint.Generate(tt.alphabet, tt.n), " ")
		if got != tt.want {
			t.Errorf("Generate(%q, %d) = %q, want %q", tt.alphabet, tt.n, got, tt.want)
		}
	}
}

func TestGenerateDefaultAlphabetIsPrefixFreeUpToCap(t *testing.T) {
	hs := hint.Generate(hint.DefaultAlphabet, 1000)
	if len(hs) != 676 {
		t.Fatalf("got %d hints, want 676", len(hs))
	}
	seen := map[string]bool{}
	for _, h := range hs {
		if seen[h] {
			t.Fatalf("duplicate hint %q", h)
		}
		seen[h] = true
	}
	some := hint.Generate(hint.DefaultAlphabet, 30)
	for _, h := range some {
		for _, o := range some {
			if h != o && strings.HasPrefix(o, h) {
				t.Fatalf("%q is a prefix of %q", h, o)
			}
		}
	}
	if got := some[:2]; !reflect.DeepEqual(got, []string{"a", "s"}) {
		t.Errorf("first hints %q", got)
	}
}
