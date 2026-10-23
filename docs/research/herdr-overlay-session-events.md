# What yubi's overlay can observe about the session (herdr 0.9.5)

Research for [#19](https://github.com/wchoco/herdr-plugins/issues/19) (map [#3](https://github.com/wchoco/herdr-plugins/issues/3), yubi). It feeds "Error and edge behavior" ([#21](https://github.com/wchoco/herdr-plugins/issues/21)).

Builds on the earlier research: "How a herdr plugin draws an overlay and captures keys" (#4), "What herdr pane read returns" (#5) and "Clipboard and paste paths" (#6). Those cover overlay geometry, key routing, re-entrancy stacking, `pane.read` output and `pane.send_input`. This note does not repeat them.

Sources:
- herdr docs at tag `v0.9.5`: [Socket API] ("Event subscriptions", "Socket transport") and [Plugins] ("Panes", "Commands and environment").
- herdr source at tag `v0.9.5` (commit `2ecf19a`). `SRC/<path>#L<n>` means `https://github.com/herdrdev/herdr/blob/v0.9.5/<path>#L<n>`.
- Probes against the installed herdr 0.9.5 binary. They ran on an **isolated** server: its own `XDG_CONFIG_HOME`/`XDG_STATE_HOME`/`HERDR_SOCKET_PATH`, all inherited `HERDR_*` variables removed, inside a private tmux server (`tmux -L`) at 120x40. A throwaway plugin had one action (bound to `F5`) that opens an `overlay` pane with `--env TARGET=$HERDR_PANE_ID --focus`. The pane put its TTY in raw mode, logged `SIGWINCH` with the new size, and held an `events.subscribe` connection that logged every pushed line. A second subscriber ran outside the plugin for comparison.

Facts marked **(probe)** were observed. The rest come from the source or the docs.

## Short answer

- **The overlay gets SIGWINCH only when its own PTY size changes.** That happens on outer terminal resize, and when the tab loses its zoom (so the overlay becomes a small split). Focus moves, tab switches and split-ratio changes don't resize it. **(probe)**
- **A socket subscription is the only way to learn about the rest of the session.** `events.subscribe` on `$HERDR_SOCKET_PATH` pushes `pane_closed`, `pane_exited`, `pane_focused`, `tab_focused`, `layout_updated` and so on as NDJSON. Outer terminal resize emits **no** event. **(probe)**
- **Wire names are not the documented names.** Lifecycle events arrive as `{"event":"pane_focused","data":{"type":"pane_focused",...}}` (snake_case). Subscription *requests* use the dotted names (`"pane.focused"`). The per-pane subscription events keep dotted names on the wire (`"event":"pane.scroll_changed"`), and their `data` has no `type`. **(probe; source)**
- **Target Pane gone:**
  - Closed by `pane close` (or another client): `pane_closed`.
  - Its process exits: `pane_exited` only, with no `pane_closed`.
  - Either way the overlay **stays open**, the tab is **un-zoomed**, and the overlay shrinks to a split (SIGWINCH). **(probe)**
- **Calls on a pane id that no longer exists:**
  - `pane.read`, `pane.send_input`, `pane.get` and `pane.layout` return `{"id":…,"error":{"code":"pane_not_found",…}}`.
  - `events.subscribe` naming that pane errors **and closes the connection**.
  - The CLI prints the error JSON to stderr and exits 1.
  - Public pane numbers are never reused within a workspace. **(probe; source)**
- **Second overlay from the same plugin:** herdr neither refuses nor deduplicates it. The new overlay splits the focused pane, which is the first overlay, and stacks on top **(probe, as in #4)**. There is **no `plugin pane list`** CLI or `plugin.pane.list` method in 0.9.5. The action can still detect the case:
  - The focused pane's `pane get` shows `label` = the manifest pane `title` and `cwd` = the plugin root.
  - Or yubi keeps its own record in `HERDR_PLUGIN_STATE_DIR`.

## 1. What reaches the overlay pane, per session change

All rows are **(probe)**. Setup: tab `w1:t1` with Target `w1:p1` and a sibling `w1:p2`. The overlay was `w1:p3`.

| Change while the overlay is open | SIGWINCH in overlay | Socket events pushed (wire names) | Other effects |
| --- | --- | --- | --- |
| Overlay opens (for reference) | n/a (starts at 91x37) | `workspace_focused`, `tab_focused`, `pane_created` (overlay), `pane_focused` (overlay), `layout_updated` (`zoomed:true`) | Event order as listed, all in the same millisecond. |
| Outer terminal resized (120x40 → 100x30 → back) | **Yes** (`71x27`, then `91x37`) | **None** | The hidden Target keeps its old PTY size (44x37) until it is shown again. |
| Focus moves to the Target (`prefix+h`) | No | `workspace_focused`, `tab_focused`, `pane_focused {pane_id: w1:p1}` | The tab **stays zoomed**, now showing the Target. The Target PTY is resized to the full tab (44→91 cols), so its screen reflows. The overlay keeps running, hidden, and gets no keys. |
| Focus moves back (`prefix+l`) | No | same triple, `pane_id: w1:p3` | |
| New tab created and focused (`prefix+c`) | No | `workspace_focused`, `tab_created`, `tab_focused {tab_id: w1:t2}`, `pane_created`, `pane_focused`, `layout_updated` (for `w1:t2`) | The overlay stays alive in `w1:t1`. |
| Back to the first tab (`prefix+p`) | No | `workspace_focused`, `tab_focused {w1:t1}`, `pane_focused {w1:p3}` | Focus returns to the overlay, still zoomed. |
| Split ratio changed via `pane resize` (API) | No | `layout_updated` | Zoom is kept. |
| Split ratio changed via resize mode (`prefix+r`, `h`, `h`, Esc) | No | `layout_updated` once per step | Zoom is kept. Resize mode acts on the hidden split between the Target and the overlay. |
| Another pane in the tab is split (`pane split w1:p2`) | **Yes** (`20x37`) | `pane_created`, `layout_updated` (`zoomed:false`), and `pane.scroll_changed` for the Target (subscribed) | **The tab un-zooms.** The overlay becomes a narrow split, and the Target is visible and resized to its split rect (15 cols). `pane zoom w1:p3` re-zooms it (SIGWINCH back to `91x37`, `layout_updated`). |
| Target closed via `pane close w1:p1` | **Yes** (`35x37`) | `pane_closed {pane_id: w1:p1}`, `layout_updated` (`zoomed:false`) | The overlay stays open and focused, now a split. |
| Target's shell exits (`exit`) | **Yes** (`44x18`) | `pane_exited {pane_id: w1:p2}`, `layout_updated` (`zoomed:false`) | Same as above. There is **no `pane_closed`** for a process exit. |
| A stacked second overlay exits | No (for the first) | `pane_exited` (inner overlay), `layout_updated` | Focus and zoom return to the first overlay. **No `pane_focused`** was emitted for the restore. |
| The orphaned overlay exits after its Target closed | n/a | `pane_exited`, `layout_updated` | herdr restores focus only if the recorded `previous_focus` still exists, else leaves it where the layout puts it (`w1:p2` here). Zoom goes back to `previous_zoomed` ([SRC/src/app/api.rs#L499-L535]). |

How to read the table:

- **SIGWINCH tells yubi only that its own size changed.** It doesn't say why. A Target close, a sibling split and an outer resize all look the same from inside the PTY.
- **The tab losing its zoom is the risky case.** Any split or close in the tab un-zooms it ([SRC/src/app/api/panes.rs#L1967-L2030] closes and emits; the probe showed the un-zoom). The overlay is then a small split, and the Target is visible next to it at a different size. The Target has also been resized, so the screen yubi read is stale.
- **The overlay can't tell "the user looked at the Target" from "focus left the tab" by SIGWINCH.** Neither resizes it. Only `pane_focused` / `tab_focused` show it.

## 2. Event subscription: request, response, pushed lines

Transport ([Socket API] "Socket transport"): newline-delimited JSON on the Unix socket at `$HERDR_SOCKET_PATH`, which plugin panes receive ([Plugins] "Commands and environment"). Use one dedicated connection for the subscription. The connection stays open after the first response.

Request ([Socket API] "Event subscriptions"; type list from `Subscription`, [SRC/src/api/schema/events.rs#L18]):

```json
{"id":"sub","method":"events.subscribe","params":{"subscriptions":[
  {"type":"pane.closed"},{"type":"pane.exited"},{"type":"pane.focused"},
  {"type":"tab.focused"},{"type":"layout.updated"},
  {"type":"pane.scroll_changed","pane_id":"w1:p1"}]}}
```

- Lifecycle types (`workspace.*`, `tab.*`, `pane.created|closed|updated|focused|moved|exited|agent_detected`, `layout.updated`, `worktree.*`) take **no filter**. They deliver events for the whole session, so yubi has to filter on `pane_id` / `tab_id` itself.
- Only `pane.output_matched`, `pane.agent_status_changed` and `pane.scroll_changed` take a `pane_id`.
- There is no subscribable "output changed" event. `pane.output_changed` exists only as an `events.wait` match ([SRC/src/api/schema/events.rs#L173-L177]). `Subscription` ([SRC/src/api/schema/events.rs#L18]) has no such variant. The nearest thing is `pane.output_matched` with a regex.

Lines observed on the wire **(probe)**:

```json
{"id":"sub","result":{"type":"subscription_started"}}
{"data":{"type":"workspace_focused","workspace_id":"w1"},"event":"workspace_focused"}
{"data":{"tab_id":"w1:t1","type":"tab_focused","workspace_id":"w1"},"event":"tab_focused"}
{"data":{"pane_id":"w1:p1","type":"pane_focused","workspace_id":"w1"},"event":"pane_focused"}
{"data":{"pane_id":"w1:p1","type":"pane_closed","workspace_id":"w1"},"event":"pane_closed"}
{"data":{"pane_id":"w1:p2","type":"pane_exited","workspace_id":"w1"},"event":"pane_exited"}
{"data":{"layout":{"area":{"height":39,"width":94,"x":0,"y":0},"focused_pane_id":"w1:p3","panes":[{"focused":true,"pane_id":"w1:p3","rect":{"height":39,"width":38,"x":0,"y":0}},…],"splits":[…],"tab_id":"w1:t1","workspace_id":"w1","zoomed":false},"type":"layout_updated"},"event":"layout_updated"}
{"data":{"pane_id":"w1:p1","scroll":{"max_offset_from_bottom":20,"offset_from_bottom":0,"viewport_rows":37},"workspace_id":"w1"},"event":"pane.scroll_changed"}
```

Shape notes:

- Pushed events carry **no `id`**. Lifecycle events are `EventEnvelope {event, data}`, with `event` serialized snake_case and `data` tagged `type` in snake_case ([SRC/src/api/schema/events.rs#L193], [L362], [L421]). Per-pane subscription events use `SubscriptionEventEnvelope`, whose `event` is renamed with dots and whose `data` is untagged ([SRC/src/api/schema/events.rs#L368-L392]). The docs only ever show the dotted names. A parser should match on the snake_case names (or normalize `.`↔`_`).
- One pane focus change emits three events: `workspace_focused`, `tab_focused`, `pane_focused` ([SRC/src/app/api.rs#L847-L873]). `pane_focused` is not re-emitted for a pane that is already selected ([Socket API]). It also doesn't identify the client that moved focus.
- `layout_updated.layout` is the same `PaneLayoutSnapshot` that `pane layout` returns. **While the overlay is zoomed, the Target's `rect` in it is the hidden split rect, not the Target's real size** **(probe)**. Example: the Target was 44x37 on its PTY while the layout said `width: 24`, because the overlay split the Target in half. The Target's own rect must be read **before** the overlay opens. The action can do that with `pane layout`, or `pane get`'s `scroll.viewport_rows`, and pass it along.
- Error/overrun: if the subscriber falls behind the shared event history, the server sends `{"id":"sub","error":{"code":"events_lost",…}}` and closes the connection ([Socket API]).
- All entries must be valid. If any `pane_id` doesn't exist, the whole request fails with `pane_not_found` and the connection is **closed**. That was verified with a lifecycle entry plus a stale-pane entry **(probe)**.

Manifest `[[events]]` hooks are no substitute. They run a fresh command per event, with no TTY and no link to the open pane. `layout.updated` and `pane.updated` are deliberately excluded from them ([SRC/src/api/schema/events.rs#L286-L309]).

## 3. Calls on a pane id that no longer exists

All **(probe)**, on the raw socket after `w1:p1` was closed:

| Request | Response |
| --- | --- |
| `pane.read {pane_id:"w1:p1", source:"visible"}` | `{"id":"r1","error":{"code":"pane_not_found","message":"pane w1:p1 not found"}}` |
| `pane.send_input {pane_id:"w1:p1", text:"hello"}` | `{"id":"r2","error":{"code":"pane_not_found","message":"pane w1:p1 not found"}}` |
| `pane.get` | same code |
| `pane.layout {pane_id:"w1:p1"}` | `{"error":{"code":"pane_not_found","message":"pane not found"}}` |
| `pane.send_input` to a never-existing `w9:p99`, `pane.read` of `"garbage"` | same `pane_not_found` |
| `events.subscribe` with `pane.scroll_changed` for `w1:p1` | `pane_not_found`, then the server closes the connection |
| `plugin.pane.focus` / `plugin.pane.close` on a non-plugin or gone pane | `{"error":{"code":"plugin_pane_not_found","message":"plugin pane not found"}}` |

CLI (`herdr pane read w1:p1 …`, `herdr pane send-text w1:p1 …`): the error JSON goes to **stderr**, nothing goes to stdout, exit code **1** **(probe)**.

Can a stale id hit a *different* pane? Not within a workspace. `next_public_pane_number` only grows and is persisted in the session snapshot ([SRC/src/workspace.rs#L1143-L1146], [SRC/src/persist/snapshot.rs#L63]). A new pane after `w1:p1` closed became `w1:p7` **(probe)**. A pane **moved** to another workspace gets a new id, though. `pane_moved` carries `previous_pane_id` and the new `pane` ([SRC/src/api/schema/events.rs#L507-L520]). The old id then answers `pane_not_found`.

## 4. Opening an overlay while one from the same plugin is open

- herdr has no plugin-pane singleton for `overlay`. That check exists only for `popup` (`ui_busy`, see #4). `plugin pane open` always succeeds and splits the **focused** pane ([SRC/src/app/api/plugins/mod.rs#L436-L541]).
- If yubi's key is a direct binding and the overlay is focused, the action runs with `HERDR_PANE_ID` = the overlay itself. The new overlay then "targets" the first one **(probe: second overlay `w1:p6` got `TARGET=w1:p3`)**. If the user has moved focus elsewhere (another tab, or the Target while zoomed), a second yubi opens there and the first stays alive, hidden.
- Listing: 0.9.5 has `plugin.pane.open|focus|close` only. There is **no `plugin.pane.list`** method and no `plugin pane list` CLI ([Socket API] method table; `herdr plugin pane --help`). `PaneInfo` has no plugin-owner field ([SRC/src/api/schema/panes.rs#L449-L484]).
- Ways the action can detect "this is already a yubi pane":
  1. **`label`**: herdr sets a plugin pane's manual label to the manifest pane `title` ([SRC/src/app/api/plugins/panes.rs#L294]). `pane get $HERDR_PANE_ID` for the overlay showed `"label":"probe-ui"`, and `pane list` shows it too **(probe)**. The user can rename the pane (`prefix+shift+p`), so this is a heuristic.
  2. **`cwd` / `foreground_cwd`** = `HERDR_PLUGIN_ROOT`. Plugin panes start in the plugin root unless `--cwd` is passed **(probe)**. The action's context also exposes it as `focused_pane_cwd`.
  3. **State file**: the action records the opened overlay's `pane_id` (from the `plugin_pane_opened` response) in `HERDR_PLUGIN_STATE_DIR`. Next time it checks whether that pane still exists with `pane get` (`pane_not_found` → stale record). If it does exist, it closes or focuses it with `plugin pane close|focus`. `plugin.pane.focus` returns `plugin_id` and `entrypoint`, which confirms ownership, but it moves focus as a side effect ([SRC/src/app/api/plugins/mod.rs#L549-L578]).
  4. Use `popup` placement instead, where a second open returns `ui_busy` (#4).

## 5. Focus and zoom restore details (source)

On overlay exit, herdr uses `restore_overlay_after_exit` ([SRC/src/app/api.rs#L499-L535]):

- If the overlay was focused in its tab at exit, focus goes back to `previous_focus` (only if that pane still exists) and zoom back to `previous_zoomed`.
- If the overlay was **not** focused at exit, for example because the user had moved to the Target and yubi was closed by `plugin pane close` or exited by itself, herdr just keeps the current zoom. The tab can then stay zoomed on the Target.
- If the Target's process exits, the pane may **respawn a shell** instead of closing, when the terminal has `respawn_shell_on_exit` ([SRC/src/app/api.rs#L537-L551]). Then the id stays valid and only `pane_exited` is seen. This was not probed.

## Points for "Error and edge behavior"

- Watch the Target with one `events.subscribe` connection: `pane.closed` + `pane.exited` (both, because a process exit gives no `pane_closed`), `pane.moved`, `pane.focused`, `tab.focused`, `layout.updated`. Match snake_case wire names.
- Target closed or exited: the overlay survives as an un-zoomed split. yubi should exit (or show a status and exit) on `pane_closed`/`pane_exited` for the Target id. Otherwise it is left as an orphan split.
- Sibling split/close in the tab: the zoom drops, the overlay shrinks (SIGWINCH), and the Target is resized and visible. Options: re-zoom itself with `pane.zoom`, re-read and re-render, or exit. Copying from a stale render could copy text no longer on screen.
- Focus left the overlay (`pane_focused` ≠ own id) or the tab (`tab_focused` ≠ own tab): yubi gets no keys. Decide whether to exit, so a hidden yubi doesn't linger, or stay.
- Outer terminal resize: SIGWINCH only, no event. Re-layout to the new size. The hidden Target keeps its old size until shown, so the Target rect captured at open time stays valid while the overlay is zoomed.
- Treat `pane_not_found` from `pane.read` / `pane.send_input` as "Target gone". The copy-and-paste variant must handle the paste failing after the copy succeeded.
- A subscription that names a stale pane id fails and closes the connection. Subscribe to `pane.scroll_changed` only after confirming the Target exists, or drop it.
- On `events_lost` the connection is closed. Re-check the Target with `pane get`, or simply exit.
- Re-entrancy: decide how the action detects an open yubi (label/cwd heuristic vs a state-file record of the pane id) and what it does (close it, focus it, or no-op).
- Read the Target's rect before opening the overlay. The layout snapshot reports a hidden half-size rect while zoomed.

## Unknowns

- Multiple attached clients with different terminal sizes: which size the overlay gets, and whether SIGWINCH fires when another client resizes. Not probed.
- Whether a mouse click on another pane, or the sidebar, while the overlay is zoomed behaves like the keyboard focus move. Not probed.
- `pane.moved` of the Target while the overlay is open (the overlay is a split of the Target's tab). Not probed.
- `respawn_shell_on_exit` panes: when the flag is set, and whether the overlay sees anything besides `pane_exited`. Not probed.
- Whether outer-terminal resize emits `layout_updated` when the tab is **not** zoomed. Only the zoomed case was probed (no event). The server's `ClientResize` path resizes and repaints without emitting an event ([SRC/src/server/headless.rs#L2170-L2210]), but I did not trace every downstream path.

[Socket API]: https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx
[Plugins]: https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/plugins.mdx
[SRC/src/api/schema/events.rs#L18]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L18
[SRC/src/api/schema/events.rs#L173-L177]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L173-L177
[SRC/src/api/schema/events.rs#L193]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L193
[L362]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L362
[L421]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L421
[SRC/src/api/schema/events.rs#L368-L392]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L368-L392
[SRC/src/api/schema/events.rs#L286-L309]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L286-L309
[SRC/src/api/schema/events.rs#L507-L520]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/events.rs#L507-L520
[SRC/src/api/schema/panes.rs#L449-L484]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/panes.rs#L449-L484
[SRC/src/app/api.rs#L499-L535]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api.rs#L499-L535
[SRC/src/app/api.rs#L537-L551]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api.rs#L537-L551
[SRC/src/app/api.rs#L847-L873]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api.rs#L847-L873
[SRC/src/app/api/panes.rs#L1967-L2030]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/panes.rs#L1967-L2030
[SRC/src/app/api/plugins/mod.rs#L436-L541]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/mod.rs#L436-L541
[SRC/src/app/api/plugins/mod.rs#L549-L578]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/mod.rs#L549-L578
[SRC/src/app/api/plugins/panes.rs#L294]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/panes.rs#L294
[SRC/src/workspace.rs#L1143-L1146]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/workspace.rs#L1143-L1146
[SRC/src/persist/snapshot.rs#L63]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/persist/snapshot.rs#L63
[SRC/src/server/headless.rs#L2170-L2210]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/server/headless.rs#L2170-L2210
