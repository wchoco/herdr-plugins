package layout_test

import (
	"reflect"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
)

// The tabs and overlay sizes below were observed on a headless herdr 0.9.5
// server with the default ui config (pane_borders auto, outer borders,
// scrollbars and gaps on): a 120x40 tab gives a 117x38 overlay, and a
// pane in a split sits inside a 1-cell frame with a scrollbar gutter, so a
// 60x20 rect holds a 57x18 grid.

func rect(x, y, w, h int) layout.Rect { return layout.Rect{X: x, Y: y, Width: w, Height: h} }

func tab(target string, panes ...layout.Pane) *layout.Tab {
	return &layout.Tab{Area: rect(0, 0, 120, 40), Target: target, Panes: panes}
}

func TestPlaceWithoutLayoutPutsTargetAtOriginFillingTheOverlay(t *testing.T) {
	got := layout.Place(nil, 117, 38)
	want := layout.Placement{W: 117, H: 38, Target: rect(0, 0, 117, 38)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPlaceFullTabTargetIsTopAnchoredAndClipped(t *testing.T) {
	// A lone pane has no frame: its 119x40 grid starts at the tab's
	// corner, one cell up and left of the framed overlay, so it is drawn
	// from the overlay's corner and clipped on the right and bottom.
	got := layout.Place(tab("w1:p1", layout.Pane{ID: "w1:p1", Rect: rect(0, 0, 120, 40)}), 117, 38)
	want := layout.Placement{W: 117, H: 38, Target: rect(0, 0, 117, 38)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPlaceThreePaneSplitKeepsEveryPaneWhereItIs(t *testing.T) {
	tb := tab("w1:p2",
		layout.Pane{ID: "w1:p1", Rect: rect(0, 0, 60, 40)},
		layout.Pane{ID: "w1:p2", Rect: rect(60, 0, 60, 20)},
		layout.Pane{ID: "w1:p3", Rect: rect(60, 20, 60, 20)},
	)
	got := layout.Place(tb, 117, 38)
	want := layout.Placement{
		W: 117, H: 38,
		// The overlay's grid starts at tab cell (1,1), as does every
		// framed pane's grid relative to its rect.
		Target: rect(60, 0, 57, 18),
		Others: []layout.Pane{
			{ID: "w1:p1", Rect: rect(0, 0, 57, 38)},
			{ID: "w1:p3", Rect: rect(60, 20, 57, 18)},
		},
		// Each pane's frame, in overlay cells; edges on the tab's outer
		// frame fall outside the overlay.
		Frames: []layout.Rect{rect(-1, -1, 60, 40), rect(59, -1, 60, 20), rect(59, 19, 60, 20)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got\n %+v\nwant\n %+v", got, want)
	}
}

func TestPlaceTwoPaneSplitLeftTarget(t *testing.T) {
	tb := tab("w1:p1",
		layout.Pane{ID: "w1:p1", Rect: rect(0, 0, 60, 40)},
		layout.Pane{ID: "w1:p2", Rect: rect(60, 0, 60, 40)},
	)
	got := layout.Place(tb, 117, 38)
	if want := rect(0, 0, 57, 38); got.Target != want {
		t.Errorf("target %+v, want %+v", got.Target, want)
	}
	if want := []layout.Pane{{ID: "w1:p2", Rect: rect(60, 0, 57, 38)}}; !reflect.DeepEqual(got.Others, want) {
		t.Errorf("others %+v, want %+v", got.Others, want)
	}
}

func TestPlaceUnframedOverlayAssumesNoPaneFrames(t *testing.T) {
	// pane_borders = "off": the overlay loses only the scrollbar column.
	tb := tab("w1:p2",
		layout.Pane{ID: "w1:p1", Rect: rect(0, 0, 60, 40)},
		layout.Pane{ID: "w1:p2", Rect: rect(60, 0, 60, 40)},
	)
	got := layout.Place(tb, 119, 40)
	want := layout.Placement{
		W: 119, H: 40,
		Target: rect(60, 0, 59, 40),
		Others: []layout.Pane{{ID: "w1:p1", Rect: rect(0, 0, 59, 40)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPlaceTargetMissingFromLayoutFallsBackToFullOverlay(t *testing.T) {
	got := layout.Place(tab("w1:p9", layout.Pane{ID: "w1:p1", Rect: rect(0, 0, 120, 40)}), 117, 38)
	if want := rect(0, 0, 117, 38); got.Target != want || got.Others != nil {
		t.Errorf("got %+v", got)
	}
}

func TestEncodeDecodeRoundTripsHerdrLayoutJSON(t *testing.T) {
	// The layout object of a herdr pane_layout result decodes as is.
	raw := `{"area":{"height":40,"width":120,"x":0,"y":0},"focused_pane_id":"w1:p1","panes":[{"focused":true,"pane_id":"w1:p1","rect":{"height":40,"width":60,"x":0,"y":0}},{"focused":false,"pane_id":"w1:p2","rect":{"height":40,"width":60,"x":60,"y":0}}],"splits":[],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false}`
	tb, err := layout.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := &layout.Tab{
		Area:   rect(0, 0, 120, 40),
		Target: "w1:p1",
		Panes:  []layout.Pane{{ID: "w1:p1", Rect: rect(0, 0, 60, 40)}, {ID: "w1:p2", Rect: rect(60, 0, 60, 40)}},
	}
	if !reflect.DeepEqual(tb, want) {
		t.Errorf("decoded %+v, want %+v", tb, want)
	}
	again, err := layout.Decode(layout.Encode(tb))
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Errorf("round trip: %+v, %v", again, err)
	}
	if _, err := layout.Decode("{"); err == nil {
		t.Error("bad JSON decoded")
	}
}
