// Package hint implements Hint Mode's pure parts: finding Matches on the
// screen with Patterns, and giving them Hints.
package hint

// DefaultAlphabet is the default Hint alphabet, easiest keys first
// (tmux-thumbs' `qwerty`).
const DefaultAlphabet = "asdfqwerzxcvjklmiuopghtybn"

// Assign gives Hints from alphabet to the Matches ms (in reading order, as
// Find returns them) and returns them by copied text. Identical texts share
// one Hint. The shortest Hints go to the Matches nearest the bottom, right
// to left within a row, and a shared Hint takes the place of its lowest
// occurrence. Texts beyond the cap of len(alphabet)^2 get no Hint.
func Assign(ms []Match, alphabet string) map[string]string {
	var texts []string // unique texts, bottom-right first
	seen := map[string]bool{}
	for i := len(ms) - 1; i >= 0; i-- {
		if t := ms[i].Text; !seen[t] {
			seen[t] = true
			texts = append(texts, t)
		}
	}
	hints := Generate(alphabet, len(texts))
	out := make(map[string]string, len(hints))
	for i, h := range hints {
		out[texts[i]] = h
	}
	return out
}

// Generate returns up to n prefix-free Hints of one or two characters from
// alphabet, shortest first, with tmux-thumbs' scheme: start from the single
// letters and, while there are too few Hints, turn the last single letter
// left into a prefix of two-letter Hints. At most len(alphabet)^2 Hints are
// returned.
func Generate(alphabet string, n int) []string {
	var letters []string
	for _, r := range alphabet {
		letters = append(letters, string(r))
	}
	singles := append([]string(nil), letters...)
	var doubles []string
	for len(singles)+len(doubles) < n && len(singles) > 0 {
		prefix := singles[len(singles)-1]
		singles = singles[:len(singles)-1]
		want := n - len(singles) - len(doubles)
		var sub []string
		for _, l := range letters[:min(want, len(letters))] {
			sub = append(sub, prefix+l)
		}
		doubles = append(sub, doubles...)
	}
	singles = singles[:min(len(singles), max(n-len(doubles), 0))]
	return append(singles, doubles...)
}
