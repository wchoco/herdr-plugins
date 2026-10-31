# yubi architecture

Where things live and the rules that are not obvious from reading one file. Paths are relative to `yubi/`.

## Layers

- `internal/core/*` does no I/O. Everything herdr-specific is in `internal/herdr/`, and `main.go` wires the two together.
- `internal/core/session/` is the overlay's behaviour. `Run(ctx, Config, Deps)` is the loop and the test seam: every adapter (`PaneSource`, `Clipboard`, `Paster`, `Terminal`, `EventStream`, `Layouts`, `Clock`) comes in through `Deps`.
- The pure packages under `internal/core/` take one pane at a time: `hint` (find Matches, assign Hints), `line` (Line Mode input grammar, `Numbering`, selection), `layout` (`Place` maps a herdr tab onto the overlay), `render` (`Grid` and its golden text form `Dump`), and `config`.

## Session

- `Machine` (machine.go) holds one `visiblePane` (pane.go) per Visible Pane. `panes[0]` is always the Target Pane. The Neighbour Panes follow in `layout.Placement.Others` order, which is tab order, not reading order.
- Pane Labels are assigned once, at construction, in both modes, by `labelPanes`. It sorts by overlay position (top to bottom, then left to right) and takes letters from the end of the alphabet, keeping `minTargetLetters` for the Target's Hints. Labels never change afterwards, even when a read fails. Look panes up by label (`paneByLabel`); never rely on slice order.
- Neighbour Panes are read once, concurrently (`readNeighbours`, layout.go). Each read arrives as its own event, so the overlay draws before they land, and every code path has to handle a pane that is unread, read, or unreadable.
- Only the Target, the overlay, focus, zoom and tab size end the session (`stale`, events.go). Neighbour events are ignored.
- Paste always goes to the Target, whichever pane the text came from.

## Tests

- Session tests go through `Run` with fakes from session_test.go. `fakeSource` serves captures by pane id; `paneErr`, `hang` and `gate` make a read fail, never return, or wait until the test releases it, so a test controls the order reads arrive in.
- Split-tab tests: `startSplit` and the `splitRun` helpers in neighbour_test.go, and `runLive` with the layouts in layout_test.go. `rightTall` is a layout whose tab order differs from reading order. The live terminal draws one frame per input read, so press keys one at a time when you need each frame.
- Render goldens are in `testdata/golden/`, driven by `TestLayoutRenderGoldens`. Rewrite them with `go test ./yubi/internal/core/session -update` and read the diff.
- Fixtures are real herdr captures loaded by `fixture.Load`; see `testdata/README.md`.
- `scripts/e2e.sh` runs real herdr through `scripts/throwaway-herdr`. The manual test is `docs/manual-test.md`; keep `docs/manual-test.ja.md` in step with it.
