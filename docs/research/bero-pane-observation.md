# How a herdr plugin can observe pane changes (herdr 0.9.5)

Research for [#44](https://github.com/wchoco/herdr-plugins/issues/44) (map [#43](https://github.com/wchoco/herdr-plugins/issues/43), bero). It feeds "how each Trigger is detected" ([#52](https://github.com/wchoco/herdr-plugins/issues/52)) and the per-Trigger research ([#48](https://github.com/wchoco/herdr-plugins/issues/48) vim, [#49](https://github.com/wchoco/herdr-plugins/issues/49) Claude Code).

Question: what can a plugin observe about a pane's output in near real time (escape sequences such as DECSCUSR and DECSET, screen content, foreground process), through which mechanism, with what latency, and does any of it work without polling?

Builds on two earlier notes and does not repeat them: "What `herdr pane read` returns" (branch `research/herdr-pane-read`, `docs/research/herdr-pane-read.md`) and "What yubi's overlay can observe about the session" (branch `research/herdr-overlay-session-events`, `docs/research/herdr-overlay-session-events.md`, which covers the `events.subscribe` wire format).

Sources:

- herdr docs at tag `v0.9.5`: [Socket API] and [Plugins].
- `herdr api schema --json` from the installed 0.9.5 binary.
- herdr source at tag `v0.9.5` (commit `2ecf19a`). `SRC/<path>#L<n>` means `https://github.com/herdrdev/herdr/blob/v0.9.5/<path>#L<n>`.
- Probes against a throwaway headless 0.9.5 server (`scripts/throwaway-herdr herdr server`, WSL2, 16 cores), driven by a small Python client on the raw socket. Facts marked **(probe)** were observed. The rest come from the source or the docs.

## Short answer

- **Escape sequences are not observable.** No API returns the bytes a pane writes, its cursor shape (DECSCUSR) or its DECSET modes (alternate screen, bracketed paste, application cursor, mouse tracking). herdr tracks all of these internally, and even forwards the focused pane's cursor shape to its own clients, but none of it is in the socket API. `pane.read --format ansi` carries only SGR styling: a vim insert/normal switch with `t_SI`/`t_EI` set to DECSCUSR showed no `CSI … SP q` and no `CSI ? …` in the read **(probe; source; schema)**.
- **Screen content is observable by pull.** `pane.read` returns the rendered screen (see the pane-read note). It is cheap: median 1.8 ms per request, and polling it at about 90 Hz cost the server about 6 % of one core **(probe)**. Output written by the pane was visible to a tight read loop a median of 30 ms after a `send_text`, and that figure includes the shell running the command **(probe)**.
- **The foreground process is observable by pull.** `pane.process_info` returns the shell pid, the foreground process group id, and each foreground process's pid, name, argv, cmdline and cwd. It showed `vim` while vim ran and `zsh` right after it quit **(probe)**. No event fires when the foreground process changes, unless herdr recognizes it as a coding agent (`pane.agent` changes, which emits `pane.updated`). Starting and quitting vim emitted nothing **(probe; source)**.
- **Nothing pane-content-related is push.** Every `events.subscribe` stream, lifecycle events included, is served by a per-connection loop that polls the server's event history (and, for `pane.output_matched`, re-reads the pane), then sleeps a fixed 100 ms (`CONNECTION_POLL_INTERVAL`, SRC/src/api/server.rs#L30, loop at #L961-L993). So:
  - `pane_focused` arrived 26-101 ms after the focus request (median 64 ms, n=20) **(probe)**.
  - A terminal-title change (`OSC 2`) arrived as `pane_updated` 50-152 ms after the `send_text` that caused it (median 100 ms, n=15) **(probe)**.
  - `pane.output_matched` (server-side substring/regex over a `pane.read`) arrived a median of 110 ms after the output was sent (52-978 ms, n=10) **(probe)**.
  - There is no subscribable "output changed" event. `pane.output_changed` exists in the schema but is never emitted (SRC/src/app/api/plugins/mod.rs#L3655-L3676, a test asserting that).
- **Consequence for bero's ~50 ms target:** any event-driven path, including `pane_focused`, adds 0-100 ms of delivery delay on its own. Only client-side polling of `pane.read` / `pane.process_info` / `pane.get` at a short interval (10-20 ms) can land inside 50 ms, and that is polling, not push. Zero-config detection from what herdr exposes therefore means **polling the focused pane's screen and process**, with `pane_focused` events as a hint rather than the clock.

## 1. What each kind of pane state looks like through the API

| Pane state | Exposed? | How | Push or pull |
| --- | --- | --- | --- |
| Raw output bytes | No | No method streams PTY output. The full method list (schema `request` enum, SRC/src/api/schema.rs) has no `attach`/`output` stream. | n/a |
| Cursor shape (DECSCUSR) | No | Tracked per pane (`DecscusrTracker`, SRC/src/pane/cursor.rs#L9-L94; used in `cursor_state_from_render_state`, SRC/src/pane/terminal.rs#L2578-L2595). Sent only to herdr's own clients in `FrameData.cursor.shape` over the private client socket (`herdr-client.sock`, bincode; SRC/src/protocol/wire.rs#L740-L775). Not in any socket API type: the schema's only `cursor` fields are selection/copy-mode request points. | n/a |
| Cursor position | No | Same as above (see also the pane-read note). | n/a |
| DECSET modes (alt screen, 2004, DECCKM, mouse) | No | Internal only (`alternate_screen`, `bracketed_paste`, … in SRC/src/pane/terminal.rs#L134-L160, #L440-L465). Observable side effect: `pane.send_input` wraps text in bracketed paste while 2004 is on (`docs/herdr.md`). | n/a |
| Rendered screen text and SGR | Yes | `pane.read` with `source` `visible`/`recent`/`recent_unwrapped`/`detection`, `format` `text`/`ansi`. | Pull. `revision` is always 0, so no cheap "changed since" check. |
| Screen matches a pattern | Yes | `events.subscribe` `pane.output_matched` (per `pane_id`, substring or regex; fires on the transition to matching, re-arms when the match disappears, SRC/src/api/subscriptions.rs#L361-L397), or one-shot `pane.wait_for_output`. | Server-side poll every 100 ms. |
| Foreground process | Yes | `pane.process_info` → `foreground_process_group_id`, `foreground_processes[] {pid, name, argv0, argv, cmdline, cwd}` (SRC/src/api/schema/panes.rs#L493-L518; [Socket API] "pane.process_info returns …"). | Pull only. |
| Foreground cwd | Yes | `pane.get` / `pane.list` → `foreground_cwd`. | Pull. |
| Detected coding agent (claude, codex, …) | Yes | `pane.get` → `agent`, `agent_status`; events `pane.agent_detected`, `pane.agent_status_changed`, `pane.updated`. Agents come from manifests (SRC/src/detect/manifests/*.toml). vim/neovim are not agents. | Events, each subject to the 100 ms loop; detection itself has its own debounce (SRC/src/pane/agent_detection.rs#L5-L13). |
| Terminal title (OSC 0/2) | Yes | `pane.get` → `terminal_title`, `terminal_title_stripped`; `pane.updated` when the stripped title changes ([Socket API]; SRC/src/app/terminal_titles.rs#L58-L73). | Event (100 ms loop) or pull. Needs the app to set a title, so it is app-side config, not zero-config. |
| Focus | Yes | `pane_focused` / `tab_focused` / `workspace_focused` events; `pane.get` `focused`; `pane.layout` `focused_pane_id`. | Event (100 ms loop) or pull. |
| Scroll position | Yes | `pane.scroll_changed` (per `pane_id`) or `pane.get` `scroll`. | Server-side poll every 100 ms. |

Manifest `[[events]]` hooks do not help either: they spawn one command per event (high latency, no state), and `pane.updated`, `layout.updated` and `pane.output_changed` are not hook-eligible (`PLUGIN_HOOK_EVENT_KINDS`, SRC/src/api/schema/events.rs#L286-L309).

## 2. Latency measurements

All **(probe)** on the throwaway server, one client, two 60-column panes running zsh, timings from Python's `time.monotonic()` on the client.

| Measurement | Result |
| --- | --- |
| `pane.get` round trip (new connection per request), n=300 | median 0.4 ms, p90 0.6 ms, max 4.2 ms |
| `pane.read` `visible` `ansi` round trip, n=50 | median 1.8 ms (one cold-start outlier at 102 ms) |
| `pane.process_info` round trip, n=50 | median 2.3 ms (cold-start outlier at 102 ms) |
| `pane.focus` request → `pane_focused` event | n=20: min 26, median 64, max 101 ms |
| `send_text` of `printf '\033]2;…\007'` → `pane_updated` with the new `terminal_title` | n=15: min 50, median 100, max 152 ms |
| `send_text` of `echo` → `pane.output_matched` | n=10: min 52, median 110, max 978 ms |
| `send_text` of `clear; echo …` → line present in a tight `pane.read` loop | n=15: min 1.7, median 30, p90 33, max 56 ms |
| Server CPU, idle | 0.2 % of one core |
| Server CPU, `pane.read visible ansi` at ~87 req/s | 6.3 % |
| Server CPU, `pane.read visible ansi` at ~45 req/s | 3.6 % |
| Server CPU, `pane.process_info` at ~46 req/s | 4.3 % |

Reading the numbers:

- Event delivery is uniformly spread over the 100 ms sleep, as the source predicts. A Trigger driven by an event alone will miss 50 ms about half the time.
- The 30 ms for the read loop is mostly the shell (fork, `clear`, `echo`). A program that is already running and only redraws (vim switching modes) should show up faster. That was not timed separately.
- `pane.process_info` on Linux walks `/proc` for the foreground group (SRC/src/platform/linux.rs `foreground_job`), so it costs a little more than `pane.read`.
- Not measured: the herdr-over-ssh topology (the plugin runs next to the server, so socket latency is local in every topology), a busy server with many panes, and the round trip for a large `recent` read.

## 3. What a mode switch inside vim looks like from the API

**(probe)**: vim 9 started as `vim -u NONE -N -c 'set showmode ttimeoutlen=10' -c 'let &t_SI="\e[6 q"' -c 'let &t_EI="\e[2 q"'`, driven with `pane.send_keys`.

- `pane.process_info`: `foreground_processes = [{name: "vim", argv: [...], cmdline: ..., cwd: ...}]`; `zsh` again after `:q!`.
- `pane.get`: `agent`, `title`, `terminal_title` all empty; `revision` 0.
- `pane.read visible`: the last row is `-- INSERT --` in insert mode, `-- VISUAL --` in visual, `:` on the command line, and the file message or blank in normal mode. That relies on `showmode` (on by default in nocompatible vim, but neovim and many configs differ). Pinning down vim/neovim signals is [#48](https://github.com/wchoco/herdr-plugins/issues/48).
- `pane.read visible ansi`: differs only in the status row's SGR. No DECSCUSR, no DECSET.
- Subscribed to `pane.updated`, `pane.focused`, `layout.updated`, `pane.agent_detected`, `pane.created`, `pane.exited` and `pane.scroll_changed` for that pane: **zero events** across insert/visual/cmdline/normal switches and the vim exit.

Side note seen while probing: `pane.send_input` delivered `i` and Esc to vim as bracketed paste (`^[[200~i^[[201~`), because vim had DECSET 2004 on. Use `pane.send_keys` to drive TUIs in tests.

## 4. Constraints on where the observer runs

- The observer must hold a socket connection and poll, so it is a long-lived process. herdr has no supported long-lived plugin process: `[[startup]]` hooks are "one-shot initialization commands rather than supervised daemons", and "Plugins must stop detached background processes before their parent command or pane exits" ([Plugins] "Startup hooks", "Install and link"). The supported long-lived host is a plugin pane, whose placements are `overlay`, `popup`, `split`, `tab` and `zoomed` (SRC/src/api/schema/plugins.rs#L445-L452). None is hidden. Where bero's loop lives belongs to [#53](https://github.com/wchoco/herdr-plugins/issues/53).
- The socket path comes from `HERDR_SOCKET_PATH` in the plugin's runtime environment ([Plugins] "Commands and environment"). One request per connection, except `events.subscribe` (`docs/herdr.md`).
- Polling `pane.read` cannot tell "nothing changed" cheaply: `revision` is always 0 (pane-read note), so the client has to compare text itself.

## 5. Prior art inside herdr itself

Relevant to the IME Agent and the shortcut Trigger, not to this question, but found on the way:

- `[experimental] switch_ascii_input_source_in_prefix` (SRC/src/main.rs#L420-L426): the herdr **client** switches the host input source while prefix/navigate/resize/copy mode or some herdr overlays are active, and restores it afterwards (SRC/src/client/shell/input_source.rs#L4-L36). On Windows it reads the IME open state with `WM_IME_CONTROL`/`IMC_GETOPENSTATUS` through `SendMessageTimeoutW`, notes that `IMC_SETOPENSTATUS` "does NOT affect real input in terminal/TSF hosts", and toggles by injecting `VK_HANGUL` with `SendInput`. Only Korean is mapped (SRC/src/platform/windows.rs#L3028-L3080). This runs only in a herdr client process on Windows.
- `reveal_hidden_cursor_for_cjk_ime` and `cjk_ime_cursor_shape` (SRC/src/main.rs#L427-L441) handle candidate-window placement, not the IME state.

## Confidence and caveats

- High: escape sequences, cursor shape and DECSET modes are not exposed in 0.9.5. Checked against the schema, the method list and the source, and probed.
- High: the 100 ms subscription loop. It is in the source and the measured spread matches it.
- Medium: the absolute latency and CPU numbers. They come from one WSL2 machine, a headless server, two panes and no attached client. An attached client adds rendering work on the server.
- Not checked: whether a future herdr release adds cursor/mode fields or a push output stream. Re-check this note when `min_herdr_version` moves past 0.9.5.
- Not checked: attaching to `herdr-client.sock` as a second client to read `FrameData.cursor.shape`. It is a private bincode protocol, a second client is out of v1 scope (#43), and an attached client can affect pane sizing. It is listed only for completeness.

[Socket API]: https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx
[Plugins]: https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/plugins.mdx
