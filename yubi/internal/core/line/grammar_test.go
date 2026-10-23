package line_test

import (
	"reflect"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
)

func TestParseAcceptsNumbersAndRanges(t *testing.T) {
	tests := []struct {
		in   string
		want []line.Range
	}{
		{"3", []line.Range{{3, 3}}},
		{"15", []line.Range{{15, 15}}},
		{"3-7", []line.Range{{3, 7}}},
		{"7-3", []line.Range{{3, 7}}},
		{"2,5-7", []line.Range{{2, 2}, {5, 7}}},
		{"2,5-7,10", []line.Range{{2, 2}, {5, 7}, {10, 10}}},
		{"4,4", []line.Range{{4, 4}, {4, 4}}},
		{"05", []line.Range{{5, 5}}},
	}
	for _, tt := range tests {
		got, err := line.Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Parse(%q): got %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPreviewShowsTheSelectionTypedSoFar(t *testing.T) {
	tests := []struct {
		in   string
		want []line.Range
	}{
		{"", nil},
		{"5", []line.Range{{5, 5}}},
		{"5-", []line.Range{{5, 5}}},
		{"5-7", []line.Range{{5, 7}}},
		{"5-7,", []line.Range{{5, 7}}},
		{"5-7,,2", []line.Range{{5, 7}, {2, 2}}},
		{"0", nil},
		{"-", nil},
		{"3--", nil},
	}
	for _, tt := range tests {
		if got := line.Preview(tt.in); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Preview(%q): got %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	for _, in := range []string{"3-", "-3", ",,", ",3", "3,", "0", "0-2", "2-0", "1-2-3", "3--4", "-"} {
		if got, err := line.Parse(in); err == nil {
			t.Errorf("Parse(%q): got %v, want an error", in, got)
		}
	}
}
