package line_test

import (
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

func TestJoinedCopy(t *testing.T) {
	wrap := screen.ParseANSI(fixture.Load(t, "wrap").ANSI)
	joined := screen.ParseANSI(fixture.Load(t, "joined").ANSI)
	shell := screen.ParseANSI(fixture.Load(t, "shell").ANSI)
	url := "https://example.com/a/very/long/path/segment01/segment02/segment03/segment04/segment05/segment06/segment07/segment08/segment09/segment10/segment11/segment12/segment13/segment14/segment15/segment16/segment17/segment18/segment19/segment20/segment21/segment22/segment23/segment24/segment25/segment26/segment27/segment28/segment29/segment30/"
	tests := []struct {
		name string
		scr  screen.Screen
		rows []int
		sep  string
		want string
	}{
		{name: "URL soft-wrapped over three rows", scr: wrap, rows: []int{1, 2, 3}, want: url},
		{name: "wrap on a space keeps the space", scr: joined, rows: []int{1, 2}, want: strings.Repeat("x", 118) + " after the wrap"},
		{name: "intermediate written trailing spaces kept, last trimmed", scr: joined, rows: []int{3, 4}, want: "kept   trimmed"},
		{name: "single row is trimmed", scr: joined, rows: []int{3}, want: "kept"},
		{name: "separator between lines", scr: joined, rows: []int{2, 3, 4}, sep: " ", want: "after the wrap kept    trimmed"},
		{name: "blank rows join as empty lines", scr: shell, rows: []int{15, 16, 17}, sep: "|", want: "trailing spaces   ||"},
		{name: "no rows", scr: joined, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := line.JoinedCopy(tt.scr, tt.rows, tt.sep); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
