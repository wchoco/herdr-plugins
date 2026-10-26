# Where a herdr plugin can show persistent status

Research for #46 (map #43, bero). Question: where can a herdr 0.9.5 plugin draw something that stays visible, to show the IME State, what can each place carry, and how is it updated?

Sources: herdr 0.9.5 docs from `https://herdr.dev/llms.txt` (Plugins, Socket API, Configuration, CLI reference, config reference JSON), `herdr api schema --json` (protocol 22), `herdr --default-config`, and a live throwaway server (`scripts/throwaway-herdr`, client in `tmux`) on 2026-10-26. "Live" below marks what that run showed.

## Short answer

herdr has no plugin-owned status bar segment and no API for border colours. Plugin UI is terminal panes only: "Runtime action registration and native non-terminal plugin UI are not part of plugin v1" (Plugins). What a plugin can do is **push text into places herdr already draws**. The ones that stay visible and update in tens of milliseconds:

1. **Workspace metadata token** rendered in the Space sidebar (`$ime`). Pushed by `workspace.report_metadata`; on screen about 40 ms after the call (live). Needs one line of user config and an expanded sidebar.
2. **Outer terminal window title** via `client.window_title.set`. Pushed; about 40 ms (live). Visible in the Windows Terminal tab/title bar, outside herdr's frame. Replaces the whole title.
3. **Tab bar right `command` entry** (tmux-style status area). Always on screen, but **polled** at `interval_seconds` >= 1, so 0.2 to 1 s late (live), and it is user config, not something the plugin declares.

Pane and tab labels and pane metadata tokens also work but either clobber user-owned names or only show for agent panes. Notifications are transient and rate limited. A split plugin pane works but costs layout space in one tab.

## Options

| Place | How a plugin writes it | Carries | Update | Visible when | Notes |
|---|---|---|---|---|---|
| Space sidebar row token `$name` | `workspace.report_metadata` `{workspace_id, source, tokens:{ime:"あ"}}` / `herdr workspace report-metadata` | one string per key, <= 80 chars after normalisation; colour/bold/dim via user `rules` matched on the value | push, ~40 ms to screen (live) | expanded desktop sidebar only; per workspace, so every workspace row needs it or only the focused one shows it | user must add `"$ime"` to `ui.sidebar.spaces.rows` |
| Agent sidebar row token `$name` | `pane.report_metadata` `{pane_id, source, tokens:{...}}` | same as above | push | only for panes herdr treats as agents (live: a plain shell pane got no row; it appeared once the pane was reported as an agent) | a vim pane is not an agent, so useless for the vim Trigger |
| Outer window title | `client.window_title.set` `{title}`; `client.window_title.clear` returns it to `ui.window_title` | one string; no styling | push, ~40 ms (live, seen as tmux `pane_title`) | always, in the outer terminal's title/tab, not inside herdr | overrides the user's whole title; only the foreground client; reason `no_foreground_client` otherwise |
| Tab bar right status area | user config `ui.tab_bar_right = [{type="command", command=..., interval_seconds=1}]`; also `text`, `datetime`, `hostname`, `zoom` | last line of stdout; ESC sequences stripped, no styling | poll, interval 1 s minimum; 177 to 653 ms after a state-file change (live, 1 s interval) | always on the tab row, unless the row is narrow (status area yields to tabs) or hidden (`hide_tab_bar_when_single_tab`) | runs on the server via `/bin/sh -lc`; the plugin can only supply a script for the user to wire in |
| Pane label (border) | `pane.rename` `{pane_id, label}` / `--clear` | string | push, immediate (live) | on the pane's top border only while the tab is split (live: not drawn on a single framed pane); also feeds `{pane}` in `ui.window_title` | overwrites the user's manual pane name |
| Tab label | `tab.rename` `{tab_id, label}` | string | push, immediate (live) | always on the tab row | overwrites the user's tab name; there is no "clear" to restore an auto name |
| Workspace label | `workspace.rename` | string | push | sidebar | same clobbering problem |
| Pane metadata `title` / `display_agent` / `state_labels` | `pane.report_metadata` | strings | push | agent rows and pane titles of agent panes | display-only overrides for agents; same agent-only limit |
| Toast | `notification.show` `{title, body?, position?, sound?}` | title <= 80, body <= 240 chars | push | transient; suppressed for the active tab; one at a time (`busy`) | `rate_limited` from the 2nd call in a burst (live: 1 shown, 10 refused); not persistent |
| Plugin pane `split` | manifest `[[panes]]` + `plugin.pane.open` `{placement:"split", focus:false, direction}` then `pane.resize` | anything a TTY program can draw, full colour | the plugin redraws itself | only in the tab it was opened in; a real pane in the layout | takes space and a slot in pane focus cycling; would need one per tab or moving it on `tab_focused` |
| Plugin pane `popup` / `overlay` | same | full TTY | — | modal / zooms the tab | not usable as a status display |
| Pane border colour / theme | none | — | — | — | colours come only from `[theme]` config; no API |

## Details per option

### Metadata tokens (Socket API, "pane.report_metadata"; Configuration, "Sidebar row layouts")

- Schema: `tokens` is a map of up to 16 keys per report, names `^[A-Za-z0-9_-]{1,32}$`, values string or `null` (clear). Optional `ttl_ms` 1..86400000 and `seq` for ordering. `source` <= 80 chars, e.g. `plugin:wchoco.bero`.
- "Pane tokens are exposed by pane and agent get/list responses and can be rendered as `$name` in Agent sidebar rows." "Space sidebar rows can render values such as `$jj_status`."
- Styling stays in the user's config: `{ token = "$ime", fg = "#888", rules = [{ equals = "あ", fg = "#f55", bold = true }] }`. "Metadata reporters provide values only." A plugin cannot colour the value itself.
- "Sidebar row settings affect only the expanded desktop sidebar. Collapsed and mobile views keep their compact layouts." So a user who collapses the sidebar (`prefix+b`) loses it.
- "Token metadata is not restored after a server restart." bero must re-report on startup (`[[startup]]` hook) or on its next state change.
- A workspace accepts sequenced token reports from at most 32 distinct sources in its lifetime. bero uses one source, so fine.
- Live: with `rows = [["state_icon","workspace","$ime"],["branch"]]`, the row read `· agent-a3a8… · IME-ON`. The workspace name is truncated to make room (sidebar width 26 by default), so a short value (one glyph) is better.
- The IME State is per client (one Windows machine), not per workspace. To show it whichever workspace is focused, bero would write the same token to every workspace, or to the focused one and follow `workspace_focused` events.

### Window title (Socket API, "Set or clear the foreground client's outer terminal window title"; Configuration, "Outer terminal window title")

- `client.window_title.set` "overrides the configured title until `client.window_title.clear` hands it back." The user's `ui.window_title` template (`{hostname}: {workspace}` by default) is not readable through the API, so bero cannot prepend to it faithfully; it would have to reconstruct a title (e.g. from `workspace.get`) and keep it current on focus changes.
- Applies to the foreground client only, which matches v1's one-client assumption.
- Whether the outer terminal shows the title depends on the terminal (Windows Terminal shows it in the tab). Over ssh and WSL the title travels in the client's output stream, so it reaches Windows Terminal in all three topologies; not checked live on Windows.

### Tab bar right `command` (Configuration, "Configure an ordered tmux-style status area")

- "Command entries run immediately and then at `interval_seconds` without blocking rendering or overlapping a previous run. The interval can be 1–31,536,000 seconds." "Herdr uses the last line of successful output, removes ESC-prefixed terminal control sequences instead of interpreting styles, clears the entry after failure, empty output, or `timeout_seconds`."
- The command gets "the same active workspace, tab, pane, socket, binary, and working-directory context as custom command keybindings", so a script could ask herdr for a token (`herdr workspace get`) or read a state file bero writes.
- No API pushes a refresh, so the status lags up to one interval. Fine for "is the IME on" at a glance, too slow to confirm that a Trigger's Off landed.
- It is user config, so it needs install docs ("add this entry"), and a plugin cannot add it itself; there is no config-write method in the schema (only `server.reload_config`).

### Labels (`pane.rename`, `tab.rename`, `workspace.rename`)

- Live: a pane label showed in the split pane's top border (`┌ IME:ON ───`). With `ui.pane_borders = "always"` and a single pane, no label was drawn. `ui.show_agent_labels_on_pane_borders` only shows agent labels when no manual label is set.
- All three replace a name the user may have chosen; `tab.rename` takes a required `label` with no clear, so the original auto name cannot be restored. Not recommended.

### Notifications (`notification.show`)

- Text is collapsed to one line and trimmed to 80/240 chars. "Herdr suppresses popups for the active tab" (Configuration), and a burst is `rate_limited` (live). Good for a one-off warning such as "IME Agent unreachable", not for persistent state.
- Live oddity: with `ui.toast.delivery` unset (documented default `"off"`) the first call still answered `shown:true, reason:"shown"`, while nothing appeared in the captured pane text. The docs say `off` yields `disabled`. Not investigated further.

### Plugin pane (Plugins, "Panes")

- Placements: `overlay` (default, zooms and focuses), `popup` (modal, receives all input), `split`, `tab`, `zoomed`. `plugin.pane.open` takes `focus` (default false), `direction`, `target_pane_id`; `pane.resize` can then shrink it.
- A split pane is a normal pane in one tab's layout; herdr keeps plugin ownership if it is moved with `pane.move`. It would need one per tab or a move on every `tab_focused`, takes at least a bordered row or column, and joins focus cycling. Heavyweight for a one-glyph indicator.

## Recommendation for bero

- Primary: a workspace token, e.g. `ime`, reported with `workspace.report_metadata` to every workspace (or the focused one), plus a documented sidebar snippet with a `rules` colour. Push, ~40 ms, no clobbering, per-value styling by the user. Re-report from a `[[startup]]` hook since tokens do not survive restart.
- Optional: `client.window_title.set` for users who hide the sidebar, accepting that it replaces their title template.
- Optional: ship a tiny script for `ui.tab_bar_right` that prints the state, for users who want it on the tab row and accept up to 1 s lag.
- Avoid labels (clobber user names), pane tokens (agent panes only), toasts (transient) and plugin panes (layout cost).

## Confidence and caveats

- High for what each API accepts (schema plus docs agree) and for the placements seen live: Space-row token, Agent-row token only for agent panes, tab-bar command polling lag, pane label on split borders, tab label, window title change, toast rate limiting.
- Latencies were measured on Linux/WSL with a local `tmux` client: CLI call ~28 ms of which most is process start; screen update ~40 to 70 ms including a `socat` or `herdr` spawn. A raw socket call from a running bero process should be faster. Not measured over ssh or on a Windows client.
- Window-title display in Windows Terminal across the three topologies is inferred, not observed.
- herdr 0.9.5 only; the plugin docs say non-terminal plugin UI is out of scope for plugin v1, so a later herdr may add a real status surface.
