package session_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// Neighbour Pane fates in the fuzz input: fate%3 says what its read does,
// fate/3 the key read it comes before (past the last one: after them all).
const (
	fateArrives = iota
	fateFails
	fateNever
)

// fuzzSep separates the key reads in the fuzz script; an empty read lets
// the lone-Esc timeout pass instead of sending anything.
const fuzzSep = 0xff

// FuzzSession runs a session on a split tab with every input derived from
// the fuzz data: the mode, the layout, each Neighbour Pane's screen and
// whether and when its read arrives or fails, the Hint alphabet, the
// clipboard failing, and the raw key reads. It checks only that the
// session never panics and ends cleanly once its input closes.
//
// The session runs in a synctest bubble, so every key read and every
// neighbour read is handled before the next one happens, in the order the
// fuzz data gives, and a session that blocks forever fails as a deadlock.
func FuzzSession(f *testing.F) {
	tabs := []*layout.Tab{twoPanes, threePanes, rightTall}
	var tall []string
	for i := range 40 {
		tall = append(tall, fmt.Sprintf("row %d /tall/%d", i, i))
	}
	screens := []fixture.Capture{
		fixture.Load(f, "links"),
		fixture.Load(f, "shell"),
		fixture.Load(f, "cjk"),
		fixture.Load(f, "wrap"),
		fixture.Load(f, "joined"),
		fiveRows,
		screenOf(""),
		screenOf("   ", "", "\t"),
		pathsScreen(27),
		pathsScreen(800), // more Matches than two-letter Hints
		{},               // replaced by the fuzzed screen
		screenOf(tall...),
		screenOf(strings.Repeat("漢", 19) + "字 /wide/clip https://example.com/" + strings.Repeat("x", 60)),
		screenOf("x"),
		{ANSI: "\x1b[1;31m\x1b[0m\r\n\x1b[38;2;1;2;3m\x1b[K"},
		{}, // replaced by the fuzzed screen
	}

	// line, tab, fates, screens (4 bits per pane: Target first), alphabet,
	// fuzzed screen, clipboard fails, key reads.
	type seed struct {
		line         bool
		tab          uint8
		fate1, fate2 uint8
		screens      uint16
		alphabet     string
		custom       string
		clipFails    bool
		script       string
	}
	seeds := []seed{
		// The panic of #59/#60: a Line Mode commit on a Neighbour Pane
		// that could not be read.
		{line: true, fate1: fateFails, screens: 0x052, script: "n1\r"},
		{line: true, fate1: fateFails, screens: 0x052, script: "n\xff1\xff\r"},
		// The neighbour fails after its label is typed.
		{line: true, fate1: fateFails + 3*2, screens: 0x052, script: "n\xff1\xff\r"},
		{fate1: fateFails + 3, screens: 0x010, script: "n\xffa"},
		// Never read: commits wait, the session stays open.
		{line: true, fate1: fateNever, screens: 0x052, script: "n1\r"},
		{fate1: fateNever, fate2: fateNever, tab: 1, screens: 0x000, script: "na\xffb"},
		// Arrivals between keys, in both modes, with and without Matches.
		{line: true, tab: 1, fate1: fateArrives + 3, fate2: fateFails, screens: 0x250, script: "b2\xff\x7f\xffn2-4\xff\x1b[13;2u"},
		{tab: 2, fate1: fateArrives, fate2: fateArrives + 3, screens: 0x018, script: "n\xffb\xffa"},
		{tab: 2, fate1: fateFails, fate2: fateFails, screens: 0x766, script: "a\xffs"},
		// Blank screens everywhere: nothing to copy.
		{line: true, tab: 1, fate1: fateArrives, fate2: fateFails, screens: 0x666, script: "1\r"},
		// A two-letter alphabet leaves no room for Pane Labels.
		{alphabet: "jk", fate1: fateArrives, screens: 0x098, script: "jk\xffkj"},
		{line: true, alphabet: "jk", fate1: fateArrives, screens: 0x0a5, custom: "\x1b[31mred\x1b[0m /r\r\n\r\n漢字", script: "n1\r"},
		// A lone Esc that times out, an Alt chord, a failing clipboard.
		{line: true, fate1: fateArrives, screens: 0x05b, clipFails: true, script: "\x1b\xff\xff3\r\xff\x1bq"},
		{fate1: fateArrives, screens: 0x0c0, clipFails: true, script: "n\xffa\xff\x1b"},
	}
	for _, s := range seeds {
		f.Add(s.line, s.tab, s.fate1, s.fate2, s.screens, s.alphabet, s.custom, s.clipFails, []byte(s.script))
	}

	f.Fuzz(func(t *testing.T, line bool, tabSel, fate1, fate2 uint8, screenSel uint16, alphabet, custom string, clipFails bool, script []byte) {
		tab := tabs[int(tabSel)%len(tabs)]
		mode := session.ModeHint
		if line {
			mode = session.ModeLine
		}
		cfg := session.Config{Mode: mode, Target: tab.Target, Layout: tab}
		if validAlphabet(alphabet) {
			settings, err := config.Parse([]byte("[hint]\nalphabet = \"" + alphabet + "\"\n"))
			if err != nil {
				t.Fatalf("alphabet %q: %v", alphabet, err)
			}
			cfg.Settings = &settings
		}
		reads := bytes.Split(script, []byte{fuzzSep})
		if len(reads) > 64 {
			reads = reads[:64]
		}

		synctest.Test(t, func(t *testing.T) {
			src := &fakeSource{
				captures: map[string]fixture.Capture{},
				paneErr:  map[string]error{},
				hang:     map[string]bool{},
				gate:     map[string]chan struct{}{},
			}
			pick := func(i int) fixture.Capture {
				j := int(screenSel>>(4*i)) & 0xf
				if j == 10 || j == 15 {
					return fixture.Capture{Text: custom, ANSI: custom}
				}
				return screens[j]
			}
			src.captures[tab.Target] = pick(0)
			// release[k] lists the neighbours whose read ends before key
			// read k.
			release := make([][]string, len(reads)+1)
			fates := []uint8{fate1, fate2}
			n := 0
			for _, p := range tab.Panes {
				if p.ID == tab.Target {
					continue
				}
				fate := fates[n%len(fates)]
				n++
				src.captures[p.ID] = pick(n)
				switch fate % 3 {
				case fateNever:
					src.hang[p.ID] = true
					continue
				case fateFails:
					src.paneErr[p.ID] = errors.New("pane gone")
				}
				src.gate[p.ID] = make(chan struct{})
				k := min(int(fate/3), len(reads))
				release[k] = append(release[k], p.ID)
			}

			term := &fuzzTerminal{w: 77, h: 18, input: make(chan []byte)}
			c := &calls{}
			if clipFails {
				c.copyErr = errors.New("no clipboard")
			}
			done := make(chan error, 1)
			go func() {
				done <- session.Run(context.Background(), cfg, session.Deps{
					Source: src, Clipboard: logClipboard{c}, Paster: logPaster{c}, Terminal: term,
				})
			}()
			ended := func() bool {
				synctest.Wait()
				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("Run: %v", err)
					}
					return true
				default:
					return false
				}
			}
			for k := 0; k <= len(reads); k++ {
				if ended() {
					return
				}
				for _, id := range release[k] {
					close(src.gate[id])
					if ended() {
						return
					}
				}
				if k == len(reads) {
					break
				}
				if len(reads[k]) == 0 {
					time.Sleep(keys.EscTimeout) // fake time: the timer fires
					continue
				}
				term.input <- reads[k]
			}
			if ended() {
				return
			}
			// The session is still open; closing its input must end it.
			close(term.input)
			if err := <-done; err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	})
}

// validAlphabet reports whether s can be a Hint alphabet in config.toml.
func validAlphabet(s string) bool {
	if len(s) < 2 {
		return false
	}
	seen := map[rune]bool{}
	for _, r := range s {
		if r < 'a' || r > 'z' || seen[r] {
			return false
		}
		seen[r] = true
	}
	return true
}

// fuzzTerminal is a Terminal whose input the fuzz test sends one read at a
// time, and which draws nowhere.
type fuzzTerminal struct {
	w, h  int
	input chan []byte
}

func (t *fuzzTerminal) Size() (int, int, error)  { return t.w, t.h, nil }
func (t *fuzzTerminal) Input() <-chan []byte     { return t.input }
func (t *fuzzTerminal) Resized() <-chan struct{} { return nil }
func (t *fuzzTerminal) Draw(*render.Grid) error  { return nil }
