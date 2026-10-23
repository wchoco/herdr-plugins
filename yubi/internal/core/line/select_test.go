package line_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// selectScreen has 6 Line Numbers: 6 "top", 5 blank, 4 "middle  " (trailing
// spaces), 3 "  indented", 2 blank (spaces only), 1 "bottom"; then a blank
// row below Line 1.
var selectScreen = screen.ParseANSI("top\r\n\r\nmiddle  \r\n  indented\r\n   \r\nbottom\r\n")

func TestCopyTextOfASelection(t *testing.T) {
	n := line.NewNumbering(selectScreen, 10)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "single line", input: "1", want: "bottom"},
		{name: "range in either order", input: "4-3", want: "middle\n  indented"},
		{name: "screen order regardless of input order", input: "1,6", want: "top\nbottom"},
		{name: "deduplicated", input: "3,4-3,3", want: "middle\n  indented"},
		{name: "overlapping ranges", input: "4-2,3-1", want: "middle\n  indented\n\nbottom"},
		{name: "blank rows inside become empty lines", input: "6-4", want: "top\n\nmiddle"},
		{name: "blank row alone", input: "5", want: ""},
		{name: "whole screen", input: "1-6", want: "top\n\nmiddle\n  indented\n\nbottom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges, err := line.Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := n.Select(ranges)
			if err != nil {
				t.Fatal(err)
			}
			if got := line.Copy(selectScreen, rows); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectAboveTopLineIsAllOrNothing(t *testing.T) {
	n := line.NewNumbering(selectScreen, 10)
	for _, input := range []string{"7", "1,7", "5-7", "99999999999999999999"} {
		ranges, err := line.Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := n.Select(ranges)
		var oor line.OutOfRangeError
		if !errors.As(err, &oor) || rows != nil {
			t.Errorf("Select(%q): rows %v, err %v; want OutOfRangeError", input, rows, err)
		}
	}
}

func TestOutOfRangeErrorNamesTheNumber(t *testing.T) {
	n := line.NewNumbering(selectScreen, 10)
	_, err := n.Select([]line.Range{{2, 9}})
	if err == nil || err.Error() != "no line 9" {
		t.Errorf("got %v, want no line 9", err)
	}
}

func TestPreviewRowsKeepOnlyLinesOnScreen(t *testing.T) {
	n := line.NewNumbering(selectScreen, 10)
	// Rows top to bottom: Line 6 is row 0, Line 1 is row 5.
	got := n.Rows([]line.Range{{1, 1}, {5, 9}, {4, 4}})
	if want := []int{0, 1, 2, 5}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
