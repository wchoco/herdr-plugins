package herdr_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

func TestLayoutOfANonFocusedPane(t *testing.T) {
	c := herdr.NewClient(replay(t, "pane-layout-split3"))
	got, err := c.Layout(context.Background(), "w1:p2")
	if err != nil {
		t.Fatal(err)
	}
	r := func(x, y, w, h int) layout.Rect { return layout.Rect{X: x, Y: y, Width: w, Height: h} }
	want := &layout.Tab{
		Area: r(0, 0, 120, 40),
		// herdr names w1:p1 as focused; the Target is the pane asked about.
		Target: "w1:p2",
		Panes: []layout.Pane{
			{ID: "w1:p1", Rect: r(0, 0, 60, 40)},
			{ID: "w1:p2", Rect: r(60, 0, 60, 20)},
			{ID: "w1:p3", Rect: r(60, 20, 60, 20)},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestLayoutPaneNotFound(t *testing.T) {
	c := herdr.NewClient(replay(t, "pane-layout-not-found"))
	_, err := c.Layout(context.Background(), "w9:p99")
	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "pane_not_found" {
		t.Fatalf("got %v, want a pane_not_found APIError", err)
	}
}
