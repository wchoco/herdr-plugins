package herdr

import (
	"io"
	"os"

	"golang.org/x/term"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
)

// Terminal setup written when the overlay starts, and its undo on Close:
// alternate screen, hidden cursor, kitty keyboard flag 1 (disambiguate).
const (
	ttySetup   = "\x1b[?1049h\x1b[?25l\x1b[>1u"
	ttyRestore = "\x1b[<u\x1b[?25h\x1b[?1049l"
)

// TTY is the overlay pane's terminal; it implements session.Terminal.
type TTY struct {
	in, out *os.File
	state   *term.State
	input   chan []byte
}

// OpenTTY puts the terminal on in/out into raw mode and starts reading keys.
func OpenTTY(in, out *os.File) (*TTY, error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	t := &TTY{in: in, out: out, state: state, input: make(chan []byte)}
	if _, err := io.WriteString(out, ttySetup); err != nil {
		t.Close()
		return nil, err
	}
	go t.read()
	return t, nil
}

func (t *TTY) read() {
	defer close(t.input)
	buf := make([]byte, 4096)
	for {
		n, err := t.in.Read(buf)
		if n > 0 {
			t.input <- append([]byte(nil), buf[:n]...)
		}
		if err != nil {
			return
		}
	}
}

// Size returns the terminal size in cells.
func (t *TTY) Size() (int, int, error) { return term.GetSize(int(t.out.Fd())) }

// Input delivers raw key bytes.
func (t *TTY) Input() <-chan []byte { return t.input }

// Draw writes a full frame.
func (t *TTY) Draw(g *render.Grid) error {
	_, err := t.out.Write(g.Encode())
	return err
}

// Writer is the terminal output, for the OSC 52 clipboard.
func (t *TTY) Writer() io.Writer { return t.out }

// Close undoes the terminal setup.
func (t *TTY) Close() error {
	io.WriteString(t.out, ttyRestore)
	return term.Restore(int(t.in.Fd()), t.state)
}
