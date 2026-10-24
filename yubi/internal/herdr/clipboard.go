package herdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
)

// NewClipboard picks the Clipboard: the copy_command argv when set,
// otherwise OSC 52 written to tty. A copy_command replaces OSC 52, it is
// not a fallback, and none is ever auto-detected.
func NewClipboard(copyCommand []string, tty io.Writer) session.Clipboard {
	if len(copyCommand) > 0 {
		return NewCommand(copyCommand)
	}
	return NewOSC52(tty)
}

// Command is the copy_command Clipboard: the text is piped to the stdin of
// argv, run directly (no shell) on the herdr server host.
type Command struct {
	argv    []string
	timeout time.Duration
}

// NewCommand returns a clipboard that runs argv (non-empty) per copy.
func NewCommand(argv []string) *Command {
	return &Command{argv: argv, timeout: 5 * time.Second}
}

// detachDelay is how long Copy waits, after the command exits, for its
// stderr to close. Tools like xclip leave a background process holding
// the clipboard and the pipe.
const detachDelay = 200 * time.Millisecond

// Copy runs the command with text on stdin. A non-zero exit is an error
// carrying the first line of its stderr. stdout is discarded so it cannot
// reach the overlay's tty.
func (c *Command) Copy(text string) error {
	if text == "" {
		return errors.New("nothing to copy")
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.argv[0], c.argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.WaitDelay = detachDelay
	err := cmd.Run()
	name := c.argv[0]
	switch {
	case err == nil, errors.Is(err, exec.ErrWaitDelay):
		// ErrWaitDelay: exited 0, a background child kept stderr open.
		return nil
	case ctx.Err() != nil:
		return fmt.Errorf("%s: timed out after %v", name, c.timeout)
	}
	if line, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n"); line != "" {
		return fmt.Errorf("%s: %v: %s", name, err, line)
	}
	return fmt.Errorf("%s: %v", name, err)
}
