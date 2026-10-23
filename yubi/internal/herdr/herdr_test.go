package herdr_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

// replay starts a fake herdr socket that answers one connection per
// captured exchange: it checks the request's method and params against the
// capture and sends the captured response. It returns the socket path.
func replay(t *testing.T, captures ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	type exchange struct{ req, resp string }
	var exs []exchange
	for _, name := range captures {
		b, err := os.ReadFile(filepath.Join(fixture.Dir(), "herdr-"+fixture.HerdrVersion, "ndjson", name+".ndjson"))
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.SplitN(string(b), "\n", 2)
		exs = append(exs, exchange{req: lines[0], resp: lines[1]})
	}
	go func() {
		for _, ex := range exs {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			got, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil {
				t.Errorf("reading request: %v", err)
			} else if !sameRequest(t, got, ex.req) {
				t.Errorf("request\n got %s\nwant %s", got, ex.req)
			}
			conn.Write([]byte(ex.resp))
			conn.Close()
		}
	}()
	return path
}

// sameRequest compares two request lines by method and params, ignoring id.
func sameRequest(t *testing.T, a, b string) bool {
	t.Helper()
	var x, y struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}
	if err := json.Unmarshal([]byte(a), &x); err != nil {
		t.Errorf("request is not JSON: %v", err)
		return false
	}
	if err := json.Unmarshal([]byte(b), &y); err != nil {
		t.Fatal(err)
	}
	return x.ID != "" && x.Method == y.Method && reflect.DeepEqual(x.Params, y.Params)
}

func TestReadVisible(t *testing.T) {
	want := fixture.Load(t, "cjk")
	tests := []struct {
		capture string
		format  session.Format
		want    string
	}{
		{capture: "pane-read-ansi", format: session.FormatANSI, want: want.ANSI},
		{capture: "pane-read-text", format: session.FormatText, want: want.Text},
	}
	for _, tt := range tests {
		t.Run(tt.capture, func(t *testing.T) {
			c := herdr.NewClient(replay(t, tt.capture))
			got, err := c.ReadVisible(context.Background(), "w6:p1", tt.format)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestReadVisiblePaneNotFound(t *testing.T) {
	c := herdr.NewClient(replay(t, "pane-read-not-found"))
	_, err := c.ReadVisible(context.Background(), "w9:p99", session.FormatANSI)
	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "pane_not_found" {
		t.Fatalf("got %v, want a pane_not_found APIError", err)
	}
	if err.Error() != "pane_not_found" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestReadVisibleSocketUnreachable(t *testing.T) {
	c := herdr.NewClient(filepath.Join(t.TempDir(), "missing.sock"))
	_, err := c.ReadVisible(context.Background(), "w1:p1", session.FormatANSI)
	if err == nil || err.Error() != "socket unreachable" {
		t.Fatalf("got %v, want socket unreachable", err)
	}
}

func TestExists(t *testing.T) {
	tests := []struct {
		capture string
		pane    string
		want    bool
	}{
		{capture: "pane-get", pane: "w1:p1", want: true},
		{capture: "pane-get-not-found", pane: "w9:p99", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.capture, func(t *testing.T) {
			c := herdr.NewClient(replay(t, tt.capture))
			got, err := c.Exists(context.Background(), tt.pane)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExistsSocketUnreachable(t *testing.T) {
	c := herdr.NewClient(filepath.Join(t.TempDir(), "missing.sock"))
	if _, err := c.Exists(context.Background(), "w1:p1"); err == nil {
		t.Fatal("got no error, want socket unreachable")
	}
}

func TestCloseOverlay(t *testing.T) {
	c := herdr.NewClient(replay(t, "plugin-pane-close"))
	if err := c.CloseOverlay(context.Background(), "w1:p2"); err != nil {
		t.Fatal(err)
	}
}

func TestCloseOverlayNotFound(t *testing.T) {
	c := herdr.NewClient(replay(t, "plugin-pane-close-not-found"))
	err := c.CloseOverlay(context.Background(), "w1:p2")
	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "plugin_pane_not_found" {
		t.Fatalf("got %v, want a plugin_pane_not_found APIError", err)
	}
}

func TestOSC52Copy(t *testing.T) {
	var out bytes.Buffer
	if err := herdr.NewOSC52(&out).Copy("emoji 🎉 party"); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;ZW1vamkg8J+OiSBwYXJ0eQ==\x07"
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOSC52Limits(t *testing.T) {
	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{name: "at the 192 KiB limit", text: strings.Repeat("a", 192*1024), ok: true},
		{name: "over the limit", text: strings.Repeat("a", 192*1024+1)},
		{name: "empty", text: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := herdr.NewOSC52(&out).Copy(tt.text)
			if tt.ok {
				if err != nil {
					t.Fatal(err)
				}
				want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(tt.text)) + "\x07"
				if out.String() != want {
					t.Error("wrong bytes")
				}
				return
			}
			if err == nil || out.Len() != 0 {
				t.Errorf("err %v, wrote %d bytes; want an error and nothing written", err, out.Len())
			}
		})
	}
}
