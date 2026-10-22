# What `herdr pane read` returns (herdr 0.9.5)

Research for #5 (parent map #3, yubi). Everything below is pinned to herdr **v0.9.5** (socket protocol 22). Sources are the official docs, the bundled API schema (`herdr api schema --json`), and the herdr source at tag `v0.9.5`. The empirical checks ran against the local 0.9.5 server, in throwaway tabs that were closed afterwards.

Source shorthand: `SRC/<path>#L<n>` means `https://github.com/herdrdev/herdr/blob/v0.9.5/<path>#L<n>`.

## Short answer

- **Soft-wrap boundaries are not exposed as metadata.** No source returns per-row wrap flags, cursor position, or a mapping between screen rows and logical lines. Internally herdr has the data (`ScreenTextRow { soft_wrapped, wrap_continuation }`, SRC/crates/ghostty-vt/src/lib.rs#L478), but no API method returns it.
- **Logical lines are available as a separate snapshot:** `--source recent-unwrapped` (API `recent_unwrapped`) joins soft-wrapped rows into one line. It is a *recent/bottom-anchored* read, not a viewport read, so it does not always match the visible screen (see "Mapping logical lines to the visible screen").
- `--source visible` is the viewport (it follows the user's scroll position). There is one `\n`-terminated line per screen row, trailing whitespace is trimmed, and trailing blank rows are dropped.
- For yubi this means one Line Number per logical line is *possible*: derive it from `visible` plus `recent` plus `recent_unwrapped`. It is not a field you can read directly, and it has edge cases. Per-screen-row numbering from `visible` alone is exact and simple.

## API surface

`pane.read` params (`PaneReadParams`, schema `request.$defs.PaneReadParams`):

| field | type | notes |
|---|---|---|
| `pane_id` | string | required |
| `source` | `visible` \| `recent` \| `recent_unwrapped` \| `detection` | required over the socket; the CLI defaults to `recent` |
| `lines` | uint32 \| null | capped at 1000 (SRC/src/app/api_helpers.rs#L117) |
| `format` | `text` \| `ansi` | default `text` |
| `strip_ansi` | bool | default true; **ignored by `pane.read`**. It is used only by `pane.wait_for_output` / output-match subscriptions (SRC/src/api/wait.rs#L66). `format` alone decides whether escapes appear. |

Result (`PaneReadResult`): `pane_id`, `workspace_id`, `tab_id`, `source`, `format`, `text`, `revision`, `truncated`.

- `revision` is **always 0** for `pane.read`. It is hardcoded (SRC/src/app/api/panes.rs#L1547), so it cannot be used to detect that the screen changed between two reads.
- `truncated` is true when more content exists than was returned: for `recent*` when `total_rows > lines`, for `visible`/`detection` when `--lines` cut lines (SRC/src/pane/terminal.rs `finish_recent_snapshot`, SRC/src/app/api_helpers.rs `limit_snapshot_lines`).
- The CLI prints only `text`. A Go plugin that needs the metadata must speak the socket protocol (`$HERDR_SOCKET_PATH`, newline-delimited JSON `{"id","method":"pane.read","params":{...}}`).

CLI flags (SRC/src/cli/pane.rs#L470-L535): `--ansi` sets `format=ansi`. `--raw` sets `format=ansi` and `strip_ansi=false`, which produces the same output as `--ansi` for reads (verified byte-identical).

Docs: CLI reference "Read output" and "Read sources" (`https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/cli-reference.mdx`, lines 243-251 and 550-559). Socket API "Reading panes" (`.../socket-api.mdx`, line 843ff).

## Sources in detail

Dispatch: SRC/src/app/api_helpers.rs#L109-L142 (`read_terminal_snapshot`). Implementations: SRC/src/pane/terminal.rs#L2813-L3050.

### `visible`

- **text:** iterates the render state's viewport rows, so it is exactly what is on screen, **including when the user has scrolled up** (test `detection_text_stays_at_bottom_when_viewport_is_scrolled`, SRC/src/pane/terminal.rs#L5864). Each row is `trim_end`ed, trailing blank rows are dropped (`trim_trailing_blank_rows`), and the rows are joined with `\n` plus a final `\n`.
- **ansi:** `read_ansi_viewport(..., unwrap=false, trim=true)`. Rows are still screen rows. SGR is emitted as `\e[0m\e[38;5;1m...` (observed). Explicitly written trailing spaces **are kept**, empty cells are not, and there is no final `\n`.
- `--lines N` keeps the last N newline-delimited lines of that snapshot.

### `recent` (wrapped)

- One line per rendered screen row, same trimming as `visible`, read from the screen/scrollback buffer.
- **Anchored at the bottom of the active screen, not at the viewport.** The range ends at `max(last non-blank row of the active screen, cursor row)` and starts `lines-1` rows above that (SRC/src/pane/terminal.rs `ghostty_recent_read_range`, L3039). On the alternate screen it ends at the last physical row.
- Default `lines` is 80 when omitted.

### `recent_unwrapped`

- Same row range as `recent`, formatted by ghostty's formatter with `unwrap=true, trim=true` (SRC/crates/ghostty-vt/src/lib.rs#L1593 `read_text_screen`). **Soft-wrapped rows are joined into one logical line**, and hard newlines stay as line breaks.
- The text has no final `\n` (observed).
- If the range starts in the middle of a wrapped logical line, the first returned line is only the tail fragment (observed: a 404-char line whose first 2 rows were above the range came back as its last 116 chars).
- `--format ansi` with this source gives an unwrapped VT stream (`read_ansi_screen(..., unwrap=true)`).

### `detection`

- Plain text only, always bottom-anchored (`recent` with `lines = viewport rows`). This is what agent-state detection sees. It is not useful for yubi.

## Representation details (verified empirically, 144x53 pane)

| aspect | `text` format | `ansi` format |
|---|---|---|
| Line separator | `\n` per row (or per logical line for `recent_unwrapped`) | same |
| Trailing whitespace in a row | trimmed, even spaces the program wrote | written spaces kept, empty cells trimmed |
| Trailing blank rows | dropped (`visible`, `recent`) | dropped |
| Leading/interior blank rows | kept as empty lines | kept |
| Wide (CJK) chars | one char per glyph; the spacer-tail cell is skipped, so **char count != column count** (`CJK: 日本語…` was 25 chars / 40 columns) | same |
| Wide char at wrap edge | no placeholder in the text; the unwrapped join is clean (`漢`x120 split 68/52 across rows, joined back to 131 chars) | same |
| Emoji | single code point; it occupies 2 columns | same |
| Tab (`\t`) | expanded to spaces (tab stops are not preserved) | same |
| Empty cell inside a row | space | space |
| Kitty unicode placeholder | rendered as a space (SRC/src/pane/terminal.rs `ghostty_screen_row`) | — |
| SGR styling | stripped | emitted per run, reset with `\e[0m` |

## What is *not* exposed

- **Cursor position:** not in `pane.read`, `PaneInfo`, or `pane.layout`.
- **Wrap flags per row:** not exposed (see above).
- **Terminal column count:** not exposed directly. `PaneInfo.scroll.viewport_rows` gives the rows. `pane.layout` gives `rect.width/height`, but in the test the rect was 145 wide while `tput cols` in the pane reported 144, so the rect is not the terminal grid width. Columns can only be inferred: a soft-wrapped row in `recent` is full width.
- **Scroll position** is exposed separately: `PaneInfo.scroll {offset_from_bottom, max_offset_from_bottom, viewport_rows}` (`herdr pane get`).
- **Content revision / atomic multi-source read:** none. Two reads are two snapshots, and `revision` is always 0.
- `pane.selection.read` (row/col `PaneTextPoint`s plus `content_revision`) exists in the schema but is used internally by the client for word selection and is not documented as public API.

## Mapping logical lines to the visible screen

The `recent*` range equals the viewport only when **(a)** the pane is scrolled to the bottom (`offset_from_bottom == 0`) and **(b)** the range end is the last viewport row. If the cursor/last content sits higher (for example after `\e[H\e[2J`, which in ghostty pushes the old screen into scrollback), then `recent_unwrapped --lines <viewport_rows>` reaches **above** the visible top into scrollback. This was observed: visible had 3 lines, while `recent_unwrapped --lines 53` returned 48 history lines plus those 3.

A workable recipe if yubi wants logical-line numbers:

1. Read `pane.get` and require `scroll.offset_from_bottom == 0`, or fall back to per-row numbering when scrolled, since `recent*` cannot see a scrolled viewport.
2. Read `visible` (V rows), then `recent` and `recent_unwrapped` with the same `lines = viewport_rows`.
3. Locate V as the contiguous **suffix-aligned** window inside the `recent` rows. Both use the same per-row trimming, so rows compare equal. This bounds which rows are on screen.
4. Walk `recent_unwrapped` lines against `recent` rows greedily: each logical line consumes the rows whose concatenation equals it. Compare with whitespace at row joins relaxed, because per-row trimming removes spaces at wrap boundaries that the unwrapped line keeps. Keep the logical lines that intersect the visible window. A line whose first rows are above the window is a clipped fragment.
5. Because the reads are not atomic and there is no revision, re-read `visible` at the end and retry if it changed.

Caveats: the cursor row may be a blank row below the last content, which shifts the `recent` end relative to `visible`'s trimmed end (handled by the alignment in step 3). On the alternate screen (full-screen TUIs), "logical lines" rarely mean anything, and wrap flags there come from the app's own cursor movement.

**Simpler alternative:** number per screen row from `visible` alone. It is exact, it works while scrolled, and it needs one read. That is the fallback the map already allows.

## Reproduction

Fixtures were printed in fresh tabs created with `herdr tab create --workspace <ws> --env ZDOTDIR=<dir>`, where `<dir>/.zshrc` ran the fixture script. The tabs were then read with `herdr pane read <id> --source {visible,recent,recent-unwrapped} [--lines 53] [--ansi|--raw]`, and with raw socket `pane.read` calls to see `revision`/`truncated`. The fixtures covered a 250-char ASCII line, CJK text, 120 x `漢`, SGR colors, a tab, an emoji, explicit trailing spaces, interior and trailing blank rows, a 400-char line straddling the viewport top, and 100 history lines followed by `\e[H\e[2J`.
