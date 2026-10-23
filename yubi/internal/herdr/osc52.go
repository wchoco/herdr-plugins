package herdr

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// OSC52MaxBytes is the largest text herdr forwards in an OSC 52 copy
// (decoded size). herdr silently drops anything larger.
const OSC52MaxBytes = 192 * 1024

// OSC52 is the default Clipboard: an OSC 52 sequence written to yubi's own
// pane, which herdr forwards to the attached client.
type OSC52 struct{ w io.Writer }

// NewOSC52 returns a clipboard that writes to w, the overlay pane's tty.
func NewOSC52(w io.Writer) *OSC52 { return &OSC52{w: w} }

// Copy writes ESC ] 52 ; c ; <base64> BEL.
func (o *OSC52) Copy(text string) error {
	if text == "" {
		return errors.New("nothing to copy")
	}
	if len(text) > OSC52MaxBytes {
		return fmt.Errorf("text is %d KiB, over the OSC 52 limit of 192 KiB", (len(text)+1023)/1024)
	}
	_, err := io.WriteString(o.w, "\x1b]52;c;"+base64.StdEncoding.EncodeToString([]byte(text))+"\x07")
	return err
}
