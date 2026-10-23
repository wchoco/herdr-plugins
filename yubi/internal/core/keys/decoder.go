package keys

import "time"

// EscTimeout is how long a lone Esc byte waits for the rest of an Alt
// chord or escape sequence before it counts as the Esc key. Under kitty
// flag 1 Esc arrives as `CSI 27u` and never waits.
const EscTimeout = 50 * time.Millisecond

// Decoder decodes a stream of reads. Unlike Decode it holds back a read's
// unfinished tail (a lone ESC, or a CSI sequence without its final byte)
// until the next read completes it or its timeout passes. Time is passed
// in by the caller, so the decoder has no clock of its own.
type Decoder struct {
	timeout time.Duration
	pending []byte
	since   time.Time
}

// NewDecoder returns a decoder whose held-back tail times out after timeout.
func NewDecoder(timeout time.Duration) *Decoder { return &Decoder{timeout: timeout} }

// Feed decodes one read taken at now. A tail held back from an earlier read
// whose timeout has already passed is decoded on its own first.
func (d *Decoder) Feed(b []byte, now time.Time) []Key {
	out := d.Expire(now)
	buf := append(d.pending, b...)
	d.pending = nil
	for len(buf) > 0 {
		if unfinished(buf) {
			d.pending = append([]byte(nil), buf...)
			d.since = now
			break
		}
		k, n := decodeOne(buf)
		out = append(out, k)
		buf = buf[n:]
	}
	return out
}

// Expire decodes the held-back tail as it is once its timeout has passed
// at now; before that it returns nothing.
func (d *Decoder) Expire(now time.Time) []Key {
	if d.pending == nil || now.Before(d.since.Add(d.timeout)) {
		return nil
	}
	out := Decode(d.pending)
	d.pending = nil
	return out
}

// Deadline returns when the held-back tail times out, if there is one.
func (d *Decoder) Deadline() (time.Time, bool) {
	if d.pending == nil {
		return time.Time{}, false
	}
	return d.since.Add(d.timeout), true
}

// unfinished reports whether b is the start of a key that a later read may
// complete: a lone ESC, or ESC [ without a final byte yet.
func unfinished(b []byte) bool {
	if b[0] != 0x1b {
		return false
	}
	if len(b) == 1 {
		return true
	}
	if b[1] != '[' {
		return false
	}
	for _, c := range b[2:] {
		if c >= 0x40 && c <= 0x7e {
			return false
		}
	}
	return true
}
