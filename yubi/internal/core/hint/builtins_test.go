package hint_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// digest is a 64-hex sha256 digest (a real image ID).
const digest = "bfce0254aed363ef761a90786544493d68c2a988d9d33f6908ea299bb2e06c07"

func TestBuiltinsNamesInPriorityOrder(t *testing.T) {
	want := []string{
		"markdown_url", "url", "diff_a", "diff_b", "docker", "path", "color",
		"uuid", "sha", "ipv4", "ipv6", "address", "number",
	}
	var got []string
	for _, p := range hint.Builtins() {
		got = append(got, p.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// builtin returns the Built-in Pattern called name, alone.
func builtin(t *testing.T, name string) []hint.Pattern {
	t.Helper()
	for _, p := range hint.Builtins() {
		if p.Name == name {
			return []hint.Pattern{p}
		}
	}
	t.Fatalf("no Built-in Pattern %q", name)
	return nil
}

// found is a Match without its row, as "pattern:text@col+width".
type found struct {
	pattern, text string
	col, width    int
}

func findAll(screenText string, pats []hint.Pattern) [][]found {
	scr := screen.ParseANSI(screenText)
	out := make([][]found, len(scr.Rows))
	for _, m := range hint.Find(scr.Rows, pats) {
		out[m.Row] = append(out[m.Row], found{m.Pattern, m.Text, m.Col, m.Width})
	}
	return out
}

// Each Built-in Pattern on its own: what it matches and copies, and what it
// must not match.
func TestBuiltinPatternCases(t *testing.T) {
	tests := []struct {
		pattern string
		line    string
		want    []string // copied texts; nil = no Match
	}{
		{"markdown_url", "see [docs](https://example.com/a?b=c) now", []string{"https://example.com/a?b=c"}},
		{"markdown_url", "[](./rel/x.md)", []string{"./rel/x.md"}},
		{"markdown_url", "[docs] (https://x.io)", nil},
		{"markdown_url", "[docs]()", nil},

		{"url", "open https://x.io/p?q=1 now", []string{"https://x.io/p?q=1"}},
		{"url", "http://a ssh://h/r git://h/r ftp://h/f file:///etc/hosts", []string{"http://a", "ssh://h/r", "git://h/r", "ftp://h/f", "file:///etc/hosts"}},
		{"url", "git@github.com:wchoco/herdr-plugins.git", []string{"git@github.com:wchoco/herdr-plugins.git"}},
		{"url", "https:/x.io http//x.io mailto:a@b", nil},

		{"diff_a", "--- a/yubi/main.go", []string{"yubi/main.go"}},
		{"diff_a", "--- b/yubi/main.go", nil},
		{"diff_a", "-- a/yubi/main.go", nil},

		{"diff_b", "+++ b/yubi/main.go", []string{"yubi/main.go"}},
		{"diff_b", "+++ a/yubi/main.go", nil},
		{"diff_b", "++ b/yubi/main.go", nil},

		{"docker", "Digest: sha256:" + digest, []string{digest}},
		{"docker", "sha256:" + digest[:63], nil},
		{"docker", "sha256:" + strings.ToUpper(digest), nil},
		{"docker", "sha1:" + digest, nil},

		{"path", "ls /usr/local/bin", []string{"/usr/local/bin"}},
		{"path", "./rel/x.txt ~/y ../up/z", []string{"./rel/x.txt", "~/y", "../up/z"}},
		{"path", "at yubi/main.go:12:3", []string{"yubi/main.go"}},
		{"path", "no slashes here", nil},

		{"color", "fg #ff00AA bg #000000", []string{"#ff00AA", "#000000"}},
		{"color", "#ff00A #gggggg ff00aa", nil},

		{"uuid", "id 123e4567-e89b-12d3-a456-426614174000", []string{"123e4567-e89b-12d3-a456-426614174000"}},
		{"uuid", "123E4567-E89B-12D3-A456-426614174000", nil},
		{"uuid", "123e4567-e89b-12d3-a456-42661417400", nil},

		{"sha", "commit 3011470 and 4ca25c0a3724dfa28162210991c6c022b67ac1e6", []string{"3011470", "4ca25c0a3724dfa28162210991c6c022b67ac1e6"}},
		{"sha", "abcdef ABCDEF1 xyz", nil},

		{"ipv4", "ping 192.0.2.10 or 10.0.0.1", []string{"192.0.2.10", "10.0.0.1"}},
		{"ipv4", "1.2.3 a.b.c.d", nil},

		{"ipv6", "to 2001:db8::ff00:42:8329 or fe80::1%eth0", []string{"2001:db8::ff00:42:8329", "fe80::1%eth0"}},
		{"ipv6", "key: value x:y 1:", nil},

		{"address", "pc=0x7ffde000 sp=0xDEADbeef", []string{"0x7ffde000", "0xDEADbeef"}},
		{"address", "0x 0xg x7f", nil},

		{"number", "port 8080 pid 123456", []string{"8080", "123456"}},
		{"number", "1 22 333", nil},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"/"+tt.line, func(t *testing.T) {
			var got []string
			for _, row := range findAll(tt.line, builtin(t, tt.pattern)) {
				for _, f := range row {
					got = append(got, f.text)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// All Built-in Patterns together: the leftmost match wins, a tie goes to
// the higher-priority Pattern, and the highlight covers the copied text.
func TestBuiltinPriority(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []found
	}{
		{"markdown link vs bare url", "[a](https://x.io) https://x.io",
			[]found{{"markdown_url", "https://x.io", 4, 12}, {"url", "https://x.io", 18, 12}}},
		{"url vs path", "file:///etc/hosts", []found{{"url", "file:///etc/hosts", 0, 17}}},
		{"diff_a vs path", "--- a/x/y.go", []found{{"diff_a", "x/y.go", 6, 6}}},
		{"diff_b vs path", "+++ b/x/y.go", []found{{"diff_b", "x/y.go", 6, 6}}},
		{"docker vs sha", "sha256:" + digest, []found{{"docker", digest, 7, 64}}},
		{"path vs ipv4", "10.0.0.0/8", []found{{"path", "10.0.0.0/8", 0, 10}}},
		{"color vs sha", "#deadbeef", []found{{"color", "#deadbe", 0, 7}}},
		{"uuid vs sha", "123e4567-e89b-12d3-a456-426614174000",
			[]found{{"uuid", "123e4567-e89b-12d3-a456-426614174000", 0, 36}}},
		{"sha vs number", "1234567 123456",
			[]found{{"sha", "1234567", 0, 7}, {"number", "123456", 8, 6}}},
		{"ipv6 vs number", "2001:db8::1", []found{{"ipv6", "2001:db8::1", 0, 11}}},
		{"address vs sha", "0xdeadbeef", []found{{"address", "0xdeadbeef", 0, 10}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAll(tt.line, hint.Builtins())[0]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// Real command output, row by row.
func TestBuiltinsOnCaptures(t *testing.T) {
	tests := []struct {
		name   string
		screen string
		want   [][]string // per row, "pattern:text"
	}{
		{"git log", `commit 4ca25c0a3724dfa28162210991c6c022b67ac1e6
Author: riku.okajima
Date:   Sat Oct 24 02:03:33 2026 +0900

    feat(yubi): add Hint Mode with url and path Patterns
3011470 docs(yubi): count Line Numbers from the last displayed line (#16)`,
			[][]string{
				{"sha:4ca25c0a3724dfa28162210991c6c022b67ac1e6"},
				nil,
				// ipv6 is greedy: a clock time is an ipv6 Match.
				{"ipv6:02:03:33", "number:2026", "number:0900"},
				nil,
				nil,
				{"sha:3011470"},
			}},
		{"git diff", `diff --git a/yubi/main.go b/yubi/main.go
index 4fa6d57..7b574dc 100644
--- a/yubi/main.go
+++ b/yubi/main.go
@@ -51,4 +51,4 @@ func main() {
+import "github.com/wchoco/herdr-plugins/yubi/internal/core/hint"`,
			[][]string{
				{"path:a/yubi/main.go", "path:b/yubi/main.go"},
				{"sha:4fa6d57", "sha:7b574dc", "number:100644"},
				{"diff_a:yubi/main.go"},
				{"diff_b:yubi/main.go"},
				nil,
				{"path:github.com/wchoco/herdr-plugins/yubi/internal/core/hint"},
			}},
		{"docker", `REPOSITORY   TAG       IMAGE ID                                                                  CREATED      SIZE
alpine       latest    sha256:` + digest + `   5 days ago   7.8MB
CONTAINER ID   IMAGE    COMMAND     CREATED       STATUS                  PORTS                    NAMES
1787b132db60   alpine   "/bin/sh"   5 weeks ago   Exited (0) 5 days ago   0.0.0.0:6006->6006/tcp   web-1`,
			[][]string{
				nil,
				{"docker:" + digest},
				nil,
				{"sha:1787b132db60", "path:/bin/sh", "ipv4:0.0.0.0", "number:6006", "path:6006/tcp"},
			}},
		{"ip addr", `2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc mq state UP group default qlen 1000
    link/ether 00:15:5d:0b:a8:79 brd ff:ff:ff:ff:ff:ff
    inet 192.0.2.10/24 brd 192.0.2.255 scope global eth0
       valid_lft forever preferred_lft forever
    inet6 2001:db8::215:5dff:fe0b:a879/64 scope link`,
			[][]string{
				{"number:1500", "number:1000"},
				{"path:link/ether", "ipv6:00:15:5d:0b:a8:79", "ipv6:ff:ff:ff:ff:ff:ff"},
				// path outranks ipv4 at the same start, so CIDR copies whole.
				{"path:192.0.2.10/24", "ipv4:192.0.2.255"},
				nil,
				{"ipv6:2001:db8::215:5dff:fe0b:a879", "path:/64"},
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := findAll(tt.screen, hint.Builtins())
			if len(rows) != len(tt.want) {
				t.Fatalf("got %d rows, want %d", len(rows), len(tt.want))
			}
			for y, row := range rows {
				var got []string
				for _, f := range row {
					got = append(got, f.pattern+":"+f.text)
				}
				if !reflect.DeepEqual(got, tt.want[y]) {
					t.Errorf("row %d: got %q, want %q", y, got, tt.want[y])
				}
			}
		})
	}
}
