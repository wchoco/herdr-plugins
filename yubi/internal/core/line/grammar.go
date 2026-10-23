package line

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Range is an inclusive span of Line Numbers, From <= To. A single Line
// Number is a Range with From == To.
type Range struct{ From, To int }

// ErrMalformed is returned for input that does not fit Line Mode's grammar.
var ErrMalformed = errors.New("invalid input")

// Parse parses Line Mode input: comma-separated items, each a Line Number
// or a range `a-b` in either order, with no spaces. 0 is not a Line Number.
// The ranges are returned in input order, normalised so From <= To.
func Parse(input string) ([]Range, error) {
	var out []Range
	for _, item := range strings.Split(input, ",") {
		r, ok := parseItem(item)
		if !ok {
			return nil, ErrMalformed
		}
		out = append(out, r)
	}
	return out, nil
}

// Preview returns the selection typed so far, for the live preview: the
// items of input that parse, plus `a` for a trailing `a-` that is still
// being typed. Items that do not parse are left out.
func Preview(input string) []Range {
	var out []Range
	for _, item := range strings.Split(input, ",") {
		if r, ok := parseItem(strings.TrimSuffix(item, "-")); ok {
			out = append(out, r)
		}
	}
	return out
}

func parseItem(item string) (Range, bool) {
	a, b, isRange := strings.Cut(item, "-")
	from, ok := parseNumber(a)
	if !ok {
		return Range{}, false
	}
	to := from
	if isRange {
		if to, ok = parseNumber(b); !ok {
			return Range{}, false
		}
	}
	return Range{From: min(from, to), To: max(from, to)}, true
}

// parseNumber parses a Line Number: one or more ASCII digits, not 0. A
// number too large for int saturates, so it is reported as out of range
// rather than malformed.
func parseNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		n = math.MaxInt
	}
	return n, n > 0
}
