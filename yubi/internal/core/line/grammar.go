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

// ErrMixed is returned for input whose items name more than one pane.
var ErrMixed = errors.New("one pane at a time")

// ParsePane parses Line Mode input that may name a pane: Parse's grammar,
// with an optional Pane Label (one lowercase letter) at the start of the
// input. It returns the label, "" for none, and the ranges. A later item
// may repeat the same label, but an item that names another pane is
// ErrMixed: `3,s2` mixes the unlabelled pane with pane s.
func ParsePane(input string) (string, []Range, error) {
	items := strings.Split(input, ",")
	labels := make([]string, len(items))
	for i, item := range items {
		labels[i], items[i] = cutLabel(item)
	}
	ranges, err := Parse(strings.Join(items, ","))
	if err != nil {
		return "", nil, err
	}
	for _, l := range labels[1:] {
		if l != "" && l != labels[0] {
			return "", nil, ErrMixed
		}
	}
	return labels[0], ranges, nil
}

// PreviewPane returns the pane and the selection typed so far, for the
// live preview: the Pane Label at the start of input, "" for none, and
// Preview's ranges of the items that name no other pane.
func PreviewPane(input string) (string, []Range) {
	items := strings.Split(input, ",")
	label, _ := cutLabel(items[0])
	var kept []string
	for _, item := range items {
		if l, rest := cutLabel(item); l == "" || l == label {
			kept = append(kept, rest)
		}
	}
	return label, Preview(strings.Join(kept, ","))
}

// cutLabel splits a leading Pane Label off item.
func cutLabel(item string) (label, rest string) {
	if item != "" && item[0] >= 'a' && item[0] <= 'z' {
		return item[:1], item[1:]
	}
	return "", item
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
