package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/relaunch"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

// stateFile is the re-launch record under $HERDR_PLUGIN_STATE_DIR.
const stateFile = "overlay.json"

// overlays is what launch needs from herdr besides opening an overlay.
type overlays interface {
	relaunch.Panes
	CloseOverlay(ctx context.Context, pane string) error
}

// launch runs one press of a yubi binding in mode, with focused as herdr's
// focused pane: it toggles the recorded overlay closed, switches it to
// mode on its original Target Pane, or opens a fresh overlay on focused.
// open opens an overlay on a Target Pane and returns the overlay's id.
// stateDir holds the record; with stateDir empty nothing is recorded and
// every press opens a fresh overlay.
func launch(ctx context.Context, h overlays, stateDir, focused string, mode session.Mode,
	open func(ctx context.Context, target string) (string, error)) error {
	plan, err := relaunch.Decide(ctx, h, loadRecord(stateDir), focused, mode)
	if err != nil {
		return err
	}
	if plan.Kind == relaunch.None {
		clearRecord(stateDir)
		return nil
	}
	if plan.Kind != relaunch.Open {
		err := h.CloseOverlay(ctx, plan.Overlay)
		var apiErr *herdr.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.Code == "plugin_pane_not_found":
			// Not yubi's overlay after all (e.g. a pane id reused after a
			// server restart): treat the record as stale.
			plan = relaunch.Plan{Kind: relaunch.Open, Target: focused}
		case err != nil:
			return err
		case plan.Kind == relaunch.Close:
			clearRecord(stateDir)
			return nil
		}
	}
	id, err := open(ctx, plan.Target)
	if err != nil {
		return err
	}
	return saveRecord(stateDir, relaunch.Record{Overlay: id, Target: plan.Target, Mode: mode})
}

// loadRecord returns the recorded overlay, or nil when there is none or it
// cannot be read (e.g. left half-written by a crash).
func loadRecord(dir string) *relaunch.Record {
	if dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return nil
	}
	var rec relaunch.Record
	if json.Unmarshal(b, &rec) != nil {
		return nil
	}
	return &rec
}

// saveRecord replaces the record atomically.
func saveRecord(dir string, rec relaunch.Record) error {
	if dir == "" {
		return nil
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, stateFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, stateFile))
}

// clearRecord forgets the overlay that was just closed.
func clearRecord(dir string) {
	if dir != "" {
		os.Remove(filepath.Join(dir, stateFile))
	}
}
