# How a herdr plugin draws an overlay and captures keys

Research for wchoco/herdr-plugins#4 (map: #3, yubi). Target: herdr **0.9.5**.

Sources: the herdr 0.9.5 docs ([Plugins], [Socket API]), the herdr source at tag
`v0.9.5` (links below), and probes run against a local herdr 0.9.5. The probes
used a separate herdr server (own `XDG_CONFIG_HOME`, inside a private tmux
server at 120x40) with a throwaway plugin. The plugin had one action that opens
a pane, and one pane that puts its TTY in raw mode and logs every `read()`.

## Answer in brief

- A plugin can't draw on top of another pane. The only plugin UI is a
  **terminal pane that the plugin owns** (`[[panes]]`). "Runtime action
  registration and native non-terminal plugin UI are not part of plugin v1"
  ([Plugins]). The external pane-graphics overlay API was removed in 0.9.x
  ([Socket API], "Pane graphics").
- An action has no TTY, so it can't be interactive. A keybinding runs the action
  (`[[keys.command]] type = "plugin_action"`). The action then opens a plugin
  pane with `$HERDR_BIN_PATH plugin pane open ...` and passes it the Target Pane
  id.
- Two placements fit yubi:
  - **`overlay`** (the manifest default): a real pane, zoomed to fill the
    **whole tab area**. On close, the previous focus and zoom come back.
  - **`popup`**: a session-modal floating terminal, centred on the tab area,
    with a border. It is half-size by default, and `width`/`height` take cells
    or `"N%"`. Neither placement can sit exactly on the Target Pane's rect.
- Keys:
  - An **overlay** pane gets raw terminal bytes. herdr first takes its own
    **prefix key** (default `ctrl+b`), any key after the prefix, and every
    **direct (non-prefix) keybinding**, including the key that launched yubi.
  - A **popup** gets **all** input, including the prefix key, `Esc` and the
    launching key.
- To dismiss, the pane process exits (or something calls
  `plugin pane close <id>` / `popup.close`). herdr then restores the previous
  focus, plus the previous zoom for an overlay. Probes confirmed this for both.

## 1. Presenting the UI

### Overlay placement (`placement = "overlay"`, default)

- How it works: herdr splits the **focused pane**, marks the tab `zoomed`, and
  focuses the new pane. It records `previous_focus` and `previous_zoomed`. When
  the overlay process exits, herdr puts focus and zoom back
  ([custom_commands.rs `spawn_overlay_argv_command`][overlay-spawn],
  [api.rs PaneDied handling][overlay-exit]).
- It is a normal herdr pane, with a pane id (for example `w1:p2`),
  `HERDR_PANE_ID`, and pane lifecycle events. `plugin pane close` and
  `plugin pane focus` work on it ([Plugins], "Panes").
- **It always fills the whole tab content area, not the Target Pane's rect.** In
  the probe, a 93x39 single Target Pane got a 91x37 overlay. With the Target
  Pane split to 44x37, the overlay was still 91x37. The two cells lost come from
  the pane frame. Whether a frame is drawn depends on the user's
  `pane_borders` / `pane_outer_borders` config. The overlay counts as a split
  pane, so `pane_borders = "auto"` draws its frame.
- **The hidden Target Pane is not resized.** A probe ran a `SIGWINCH` logger in
  the Target Pane, and it got no resize while the overlay opened, ran, or
  closed. So the Target Pane's screen doesn't reflow underneath the overlay.
- The overlay's title bar shows the manifest pane `title`.
- **Request constraint:** overlay and popup reject `target_pane_id`,
  `workspace_id` and `direction`, with the error "overlay and popup plugin panes
  target the active pane" ([plugins/mod.rs `handle_plugin_pane_open`][open]).
  They always attach to the focused pane at open time. When a keybinding fires,
  that is the Target Pane.

### Popup placement (`placement = "popup"`)

- A session-modal floating terminal. It leaves the tiled layout alone and is
  not a pane: no pane id, no `HERDR_PANE_ID`, no pane events, and it is outside
  the `pane.*` APIs ([Plugins], "Panes"; [Socket API], `plugin.pane.open`).
- Geometry: centred on the tab content area (the sidebar is excluded), with a
  1-cell border. The default size is half the area. `width`/`height` take cells
  or `"1%".."100%"`, set in the manifest or on the open request
  ([popup_size.rs][popup-size]). The probe's default popup was 44x17 inner
  cells. It **can't be placed at an arbitrary rect**, such as over a split
  Target Pane.
- Only one popup can be open at a time. Opening a second returns `ui_busy`, and
  so does opening one while Settings, Copy mode or another herdr modal is up.
- The Target Pane stays visible around the popup, unless the popup is sized to
  100%.
- Popup support arrived in herdr **0.7.4** (CHANGELOG).

### Other placements

`split`, `tab` and `zoomed` also exist:

- `zoomed` is like `overlay`, but it takes an explicit `target_pane_id` and does
  **not** track or restore the previous focus and zoom.
- `split` and `tab` change the layout. They don't suit a transient overlay.

### What this means for "draw over the Target Pane"

Neither placement shares the Target Pane's cells or geometry. So yubi has to
**re-render the Target Pane's text itself** inside its own pane:

1. Read the screen with `herdr pane read <target> --source visible`. Add
   `--format ansi` to keep colours (available since 0.5.7).
2. Paint that text into the yubi pane.
3. Draw Hints or Line Numbers on top.

Overlay gives the most screen-like result: full tab size, and the real screen is
hidden. The overlay is at least as large as a single Target Pane minus the
frame, but can be a few cells smaller in each direction. Rows or columns that
don't fit need a clipping policy. Popup is smaller by default and keeps the real
screen visible around it.

## 2. How yubi learns the Target Pane id

- **Action process** (run by the keybinding): `HERDR_PANE_ID` is the focused
  pane, which is the Target Pane. `HERDR_PLUGIN_CONTEXT_JSON.focused_pane_id`
  holds the same id, with `invocation_source = "keybinding"`. Probe result:
  `HERDR_PANE_ID=w1:p1`, `focused_pane_id=w1:p1`.
- **Overlay pane process**: `HERDR_PANE_ID` is the **overlay's own** pane id,
  for example `w1:p2`. `HERDR_PLUGIN_CONTEXT_JSON.focused_pane_id` is the
  Target Pane, because the context is captured before the split. Its
  `invocation_source` is `"api"`.
- **Popup process**: `HERDR_PANE_ID` is **not set**.
  `HERDR_PLUGIN_CONTEXT_JSON.focused_pane_id` is the Target Pane, the tiled pane
  under the popup.
- Recommended: have the action pass the id explicitly:
  `plugin pane open --plugin <id> --entrypoint <pane> --env YUBI_TARGET_PANE=$HERDR_PANE_ID --focus`.
  `--env` works for both overlay and popup (probe-verified). This way yubi
  doesn't depend on the context JSON's shape. Only the `HERDR_*` keys that
  herdr manages are protected from override
  ([panes.rs `plugin_pane_protected_env_key`][panes]).
- Reading the Target Pane **while the overlay is open** still works. Probe:
  `pane read w1:p1 --source visible` returned the full screen. So the action can
  open the pane right away and let the pane read the screen, or the action can
  read first and hand over a file under `HERDR_PLUGIN_STATE_DIR`.
- Getting the Target Pane's size or rect: `pane get <id>` returns
  `scroll.viewport_rows`. `pane layout --pane <id>` returns pane rects for the
  tab.

## 3. What input the pane receives

The pane is an ordinary PTY. With the TTY in raw mode, the probe logged these
bytes (outer terminal: tmux):

| Key pressed | Overlay pane | Popup |
| --- | --- | --- |
| `a` / `A` / `1` / Space | `a` / `A` / `1` / ` ` (uppercase comes through as itself) | same |
| Enter | `\r` | `\r` |
| Esc | `\x1b` (lone byte) | `\x1b` |
| Backspace | `\x7f` | `\x7f` |
| Tab | `\t` | (not tested) |
| Up | `\x1b[A` | (not tested) |
| `ctrl+u` / `ctrl+c` | `\x15` / `\x03` | (not tested) |
| `ctrl+b` (herdr prefix) | **swallowed**: herdr enters prefix mode | `\x02` (delivered) |
| `ctrl+b` then `Esc` | **both swallowed** (Esc cancels prefix mode) | n/a |
| F5 (bound to the yubi action) | **swallowed**: herdr ran the action again, which **stacked a second overlay** on top of the first | `\x1b[15~` (delivered, no re-entry) |
| F6 (another direct binding) | swallowed | `\x1b[17~` (delivered) |

The routing behind this is in [client/shell/input.rs `route_key_press`][input]:

- When a popup is open, every key goes to it before any binding is checked.
- In Terminal mode, `resolve_direct_binding` runs first, then `matches_prefix`.
  Only keys that pass both reach the focused pane.

Caveats:

- In a raw-mode reader, `\x1b` alone (Esc) and `\x1b[...` (an escape sequence)
  arrive in separate `read()` calls in practice, but yubi should still
  disambiguate Esc with a short timeout or a proper key parser.
- Keys reach the pane encoded the way the pane's terminal modes ask for. The
  probe used the legacy encoding. A pane that turns on the kitty keyboard
  protocol would get CSI-u sequences instead. Not tested.
- Ctrl+C reaches the overlay as `\x03` only because the probe turned off ISIG.
  With a cooked TTY it becomes SIGINT.
- **Re-entrancy:** with overlay, pressing the yubi key again inside yubi
  launches yubi on top of yubi, with yubi's own pane as the "Target Pane".
  Either the action detects that the focused pane is a yubi pane and does
  nothing (or closes it), or yubi uses popup, which swallows the key. If the
  user's prefix is a plain key, or Esc is bound directly, an overlay pane never
  sees that key.

## 4. Dismissal and focus return

- The pane process exits, for example after a copy or on Esc. Overlay: the pane
  closes, and the previous focus and zoom come back. Probe: focus returned to
  `w1:p1` and the tab was unzoomed. Popup: it closes, and input goes back to the
  tiled pane.
- From outside, `herdr plugin pane close <overlay-pane-id>` gives the same
  restore (probe: focus `w1:p1`, `zoomed: false`). For a popup, use the socket
  method `popup.close` (it returns `popup_not_open` if none is open).
- Not verified: what happens if the user moves focus away while the overlay is
  open (prefix + direction), or if the Target Pane closes. Those are for the
  error/edge ticket.

## 5. Manifest fields and version constraints

```toml
id = "wchoco.yubi"            # ASCII letters/digits . : _ -
name = "yubi"
version = "0.1.0"
min_herdr_version = "0.7.4"   # required; popup needs >= 0.7.4. Pin to what we test (0.9.5) unless we test older.
platforms = ["linux", "macos"]

[[actions]]
id = "hint"                   # local ids: no dots; qualified as wchoco.yubi.hint
title = "yubi: hint mode"
contexts = ["pane"]
command = ["./yubi", "action", "hint"]   # argv, no shell; cwd = plugin root

[[panes]]
id = "ui"
title = "yubi"
placement = "overlay"         # or "popup" (+ width/height, cells or "N%")
command = ["./yubi", "ui"]
```

User keybinding (in herdr `config.toml`, not the manifest):

```toml
[[keys.command]]
key = "prefix+y"
type = "plugin_action"
command = "wchoco.yubi.hint"
```

- `min_herdr_version` is required. herdr refuses to link or install a plugin
  whose minimum is newer than the running binary ([Plugins]).
- Plugin v1 landed in 0.7.0, popup placement in 0.7.4, and ANSI pane reads in
  0.5.7 (CHANGELOG at v0.9.5).
- Plugin panes run with the plugin root as cwd, unless `--cwd` is passed. Env
  passed to them: `HERDR_BIN_PATH`, `HERDR_SOCKET_PATH`, `HERDR_PLUGIN_ID`,
  `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`, `HERDR_PLUGIN_STATE_DIR`,
  `HERDR_PLUGIN_CONTEXT_JSON` and `HERDR_PLUGIN_ENTRYPOINT_ID`, plus
  `HERDR_PANE_ID`, `HERDR_TAB_ID` and `HERDR_WORKSPACE_ID` (not for popup).

## Open points for the "Overlay presentation" decision

- **Overlay**: shows the screen at full tab size and hides the real one, but the
  geometry doesn't match a split Target Pane. It is exposed to prefix and
  direct-binding interception and to re-entrancy.
- **Popup**: owns every key and can't re-enter, but it is centred and smaller
  by default. A 100% popup still differs from a split Target Pane's rect.
- Either way, yubi re-renders the Target Pane's text rather than painting over
  it.
- `pane read --source visible` seemed to trim trailing blank rows: 29 content
  lines came back from a 39-row screen. The screen-reading ticket should confirm
  row alignment and wrap info.

[Plugins]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/plugins.mdx
[Socket API]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx
[open]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/mod.rs#L436-L541
[panes]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/panes.rs
[overlay-spawn]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/custom_commands.rs#L440-L522
[overlay-exit]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api.rs#L281-L300
[popup-size]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/popup_size.rs#L49-L80
[input]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/client/shell/input.rs#L610-L680
