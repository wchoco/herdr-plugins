# herdr behaviour notes for plugin authors

What herdr 0.9.5 (socket protocol 22) does that its schema and docs leave out, each seen on a live server. Request and response shapes come from `herdr api schema --json`; the plugin guide is in herdr's docs (`https://herdr.dev/llms.txt`). When `min_herdr_version` is raised, re-check what a plugin relies on here; yubi's `scripts/e2e.sh` and its socket captures (`yubi/testdata/README.md`) do most of it.

## Running a throwaway server

- Use `scripts/throwaway-herdr`. An inherited `HERDR_SOCKET_PATH` overrides the XDG dirs, so setting only `XDG_*` still reaches the user's server.
- The socket is `$XDG_CONFIG_HOME/herdr/herdr.sock` and must fit in `sun_path`: keep the dir short (`/tmp/...`, not a deep scratch path).
- A headless `herdr server` sizes new panes 120x40 and never resizes a pane's PTY after a split, because no client renders. For geometry, run herdr as a client inside `tmux -L <name>`.
- A client's first run shows a welcome (Enter continues) and then the settings dialog (Esc closes it). Both take keys meant for the panes.
- `plugin action invoke` acts on the focused pane. `workspace create` does not always move focus, so `workspace focus <id>` first.

## Plugins

- `herdr plugin link <dir>` does not run `[[build]]`; build the binary first. Only `plugin install` builds.
- An action has no TTY. Its stdout and stderr go to `herdr plugin log list`, where a non-empty `stderr` is the only sign of a failure that still exits 0.
- An action run by a keybinding gets `HERDR_PANE_ID` = the focused pane. While an overlay is focused, that is the overlay itself.
- `herdr plugin config-dir <id>` creates the config and state dirs as a side effect.

## Socket

- One request per connection: a second request on the same connection is reset. Only `events.subscribe` keeps its connection, and streams.
- `params` is required on every request, `{}` when empty. An unparseable line gets an error with `"id":""`.
- `pane.send_input` answers `{"type":"ok"}`. It wraps `text` in bracketed paste only while the pane has DECSET 2004 on, and does not strip control characters.
- `pane.layout`'s `focused_pane_id` is herdr's focused pane, not the `pane_id` asked about.

## Reading panes

- `pane read --format ansi` separates rows with `\r\n` and has no final newline; `text` uses `\n` plus a final `\n`. `ansi` keeps written trailing spaces; `text` trims them.
- ANSI reads carry truecolor (`38;2;r;g;b`) and 256-colour SGR.

## Overlays, focus and zoom

- An `overlay` pane zooms the tab and focuses itself. When its process exits, herdr restores the previous focus and zoom.
- `plugin.pane.close` restores focus but not a previous zoom: the tab is left un-zoomed. Re-zoom with `pane.zoom` `{"mode":"on"}`. Two calls show one un-zoomed frame in between.
- While a tab is zoomed, `pane.layout` says `zoomed:true` but keeps the split rects; the zoomed pane is drawn framed over the whole tab.
- An overlay opens one column narrower than it ends up: herdr drops the scrollbar gutter after the pane enters the alternate screen, at its next layout pass (a plugin action triggers one). The pane gets a SIGWINCH with no change to the tab's `pane.layout` `area`. An outer terminal resize sends no event, but does change `area`.
- herdr refuses to move a pane out of a zoomed tab (`pane.move` → `changed:false, reason:"zoomed_tab"`).
- Closing any pane of a zoomed tab, or its process exiting, un-zooms the tab, even when the zoomed pane's rect does not change (herdr 0.9.5).

## Events

- Pushed events use snake_case names (`pane_focused`) and repeat them in `data.type`. One focus change arrives as `pane_focused`, `tab_focused`, `workspace_focused`, in that order.
- A process exit gives `pane_exited` only, with no `pane_closed`.
- An `events.subscribe` naming a missing `pane_id` answers `pane_not_found` and closes the connection.

## Clipboard

- OSC 52 written to a plugin pane's stdout reaches the client's clipboard, with a "copied" toast. herdr drops it silently when it is over 192 KiB decoded or no client is in the foreground, and never answers a read-back.
