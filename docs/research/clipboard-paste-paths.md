# Clipboard and paste paths from a herdr plugin

Research for [#6](https://github.com/wchoco/herdr-plugins/issues/6) (map [#3](https://github.com/wchoco/herdr-plugins/issues/3), yubi). Feeds the "Clipboard and paste strategy" decision ([#11](https://github.com/wchoco/herdr-plugins/issues/11)).

Investigated against herdr **0.9.5** (tag `v0.9.5`, commit `2ecf19a`) source and docs, plus live probes on WSL2 (Ubuntu, herdr 0.9.5). Source links point at the `v0.9.5` tag.

## Question

How can a herdr plugin process put text on the system clipboard across WSL2, Linux (X11/Wayland), macOS, and SSH/remote herdr sessions? Does herdr pass OSC 52 through, and can a plugin emit it? Which external commands are viable? Can the plugin paste into the Target Pane via `pane send-text` (bracketed paste)?

## Short answer

1. **herdr has no clipboard API.** No CLI command or socket method writes the clipboard. The socket API method list has nothing clipboard-related ([socket-api.mdx](https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx)). A plugin has to emit OSC 52 itself or run an OS tool.
2. **OSC 52 from a plugin pane or popup works, and it is herdr's own copy path.** herdr's embedded terminal (libghostty-vt) captures OSC 52 writes from any pane process. The server forwards them to the **foreground client only**. That client writes the clipboard with the same function herdr uses for its own mouse and copy-mode copies, and shows herdr's "copied" toast. So the copy lands on the clipboard of the machine the user is sitting at, whatever the topology.
3. **Plugin action and hook commands cannot use OSC 52.** They have no PTY: their stdout goes to the plugin log. Only `[[panes]]` entrypoints (overlay/split/tab/zoomed/popup) have a terminal.
4. **External commands are wrong for remote users and unreliable locally.** Plugin processes are spawned by the **herdr server**, so they inherit the server's environment, not the client's. Live on this machine: the user is attached over SSH (Tailscale) from another device. The server env has `DISPLAY`, `WAYLAND_DISPLAY` and `WSL_DISTRO_NAME` but no `SSH_*`. A plugin calling `clip.exe` or `wl-copy` would silently fill the WSL host's clipboard, not the user's.
5. **`pane send-text` is raw and never bracketed.** The socket method `pane.send_input` with `text` and no `keys` *is* bracketed-paste-aware and sends no Enter. That is the right paste primitive. It has no CLI wrapper (`pane run` adds Enter), so a Go plugin must speak NDJSON to `HERDR_SOCKET_PATH`.

## Findings

### 1. How herdr handles OSC 52 from a pane

- **Capture.** `capture_clipboard_write` accepts only the **standard clipboard** (`c`; `p`/primary and others return UNSUPPORTED). It also requires a single `text/plain` content and a non-empty payload of at most **192 KiB** decoded (`MAX_CLIPBOARD_BYTES = 192 * 1024`). See [crates/ghostty-vt/src/lib.rs L498, L602-L660](https://github.com/herdrdev/herdr/blob/v0.9.5/crates/ghostty-vt/src/lib.rs#L602-L660). Both BEL and ST terminators work. **Read queries (`52;c;?`) are not answered** (tests at [L5040-L5055](https://github.com/herdrdev/herdr/blob/v0.9.5/crates/ghostty-vt/src/lib.rs#L5040-L5055)), so a plugin cannot read or back up the clipboard via OSC 52.
- **Pane to server.** The PTY reader turns captured writes into `AppEvent::ClipboardWrite` ([src/pane.rs L2614-L2622](https://github.com/herdrdev/herdr/blob/v0.9.5/src/pane.rs#L2614-L2622)).
- **Server to client.** The headless server forwards it as `ServerMessage::Clipboard` to the **foreground client only** ("Clipboard writes are client-local side effects"). See [src/server/headless/notifications.rs L334-L340](https://github.com/herdrdev/herdr/blob/v0.9.5/src/server/headless/notifications.rs#L334-L340).
- **Client to OS.** `forward_clipboard` calls `selection::write_osc52_bytes`, then shows copy feedback ([src/client/clipboard_forwarding.rs](https://github.com/herdrdev/herdr/blob/v0.9.5/src/client/clipboard_forwarding.rs), [src/client/mod.rs L1986-L1996](https://github.com/herdrdev/herdr/blob/v0.9.5/src/client/mod.rs#L1986-L1996)). The toast is configurable via `[ui.toast.clipboard]` (`herdr --default-config`).
- **`write_osc52_bytes`** ([src/selection.rs L299-L377](https://github.com/herdrdev/herdr/blob/v0.9.5/src/selection.rs#L299-L377)) works like this:
  - It **prefers OSC 52** to the outer terminal when the *client* sees `SSH_CONNECTION`, `SSH_TTY` or `VSCODE_IPC_HOOK_CLI`, or detects WSL (env vars, `/proc` kernel strings, `/run/WSL`, binfmt `WSLInterop`). It emits `ESC ] 52 ; c ; <base64> BEL`.
  - Otherwise it tries native tools first, then falls back to OSC 52:
    - Linux: `wl-copy --type text/plain;charset=utf-8` if `WAYLAND_DISPLAY`, then `xclip -selection clipboard -in` and `xsel --clipboard --input` if `DISPLAY` ([src/platform/linux.rs L1057-L1079](https://github.com/herdrdev/herdr/blob/v0.9.5/src/platform/linux.rs#L1057-L1079)).
    - macOS: `pbcopy` ([src/platform/macos.rs L640-L648](https://github.com/herdrdev/herdr/blob/v0.9.5/src/platform/macos.rs#L640-L648)).
    - Native Windows: Win32 `CF_UNICODETEXT` ([src/platform/windows.rs L2670](https://github.com/herdrdev/herdr/blob/v0.9.5/src/platform/windows.rs#L2670)).
- **Popups take the same path.** A plugin popup is spawned through `TerminalRuntime::spawn_argv_command` with the app's `event_tx`, the same runtime as panes ([src/app/popup.rs L67-L97](https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/popup.rs#L67-L97)). So OSC 52 from a popup flows the same way. (This is from reading the source; I did not probe it live.)
- **Remote attach.** With `herdr --remote <ssh-target>`, the server runs remotely and the client runs locally. The same forwarding puts the copy on the local clipboard. herdr's changelog records this design intent, e.g. "Pane text copied through VS Code Remote Tunnels now reaches the viewing machine's clipboard instead of overwriting the remote host clipboard" ([CHANGELOG.md](https://github.com/herdrdev/herdr/blob/v0.9.5/CHANGELOG.md)).

Practical consequence: a yubi pane or popup only has to write `\x1b]52;c;<base64>\x07` to its own stdout. Choosing between a native tool and OSC 52 for the outer terminal is herdr's job, and it is decided on the right machine (the client's).

Caveats:
- When herdr chooses OSC 52 (SSH/WSL/VS Code), the **outer terminal** must allow OSC 52 writes. xterm gates them behind `allowWindowOps`, per [ctlseqs](https://invisible-island.net/xterm/ctlseqs/ctlseqs.html) ("These controls may be disabled using the allowWindowOps resource"). Other terminals have their own settings. If the user's herdr mouse-copy works, yubi's copy works too, because it is the same function.
- The write is dropped if no client is in the foreground (`send_to_foreground_client`).
- Payloads over 192 KiB are rejected silently, which matters only for huge selections.

### 2. Plugin process types and their I/O

From [plugins.mdx](https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/plugins.mdx):

- `[[actions]]`, `[[events]]` and `[[startup]]` are argv commands whose output is captured in plugin command logs (`herdr plugin log list`). They have no terminal, so OSC 52 written there goes nowhere.
- `[[panes]]` entrypoints run in a herdr terminal (overlay/split/tab/zoomed) or in a session-modal popup. The popup "receives all terminal input, including Escape" and has no `HERDR_PANE_ID`; the underlying pane is in `HERDR_PLUGIN_CONTEXT_JSON`.
- All runtime commands get `HERDR_SOCKET_PATH`, `HERDR_BIN_PATH`, `HERDR_PLUGIN_CONTEXT_JSON`, and so on. Nothing tells the plugin where the foreground client is, or whether that client is local, SSH or remote.

So the copy must be performed by the overlay/popup process (the one showing Hints or Line Numbers), not by a bare action. That fits a design where the action opens the pane or popup.

### 3. External commands

| Command | Platform | Notes |
| --- | --- | --- |
| `clip.exe` | WSL2 | Write-only, fast (about 50 ms here). **Reads stdin in the Windows ANSI code page**: raw UTF-8 `日本語 é` came out as mojibake. UTF-16LE **without** BOM round-trips correctly. With a BOM, a U+FEFF is kept in the clipboard. Converts `\n` to `\r\n`. Live probe on this machine. |
| `powershell.exe Set-Clipboard` | WSL2 | Correct UTF-8 if `[Console]::InputEncoding` is set to UTF-8. Keeps `\n`. About 0.9 s startup, too slow for an interactive copy. |
| `wl-copy` | Linux Wayland (incl. WSLg) | herdr's own first choice when `WAYLAND_DISPLAY` is set. It forks to serve the selection (herdr fixed a freeze around this: "Wayland clipboard copies no longer freeze Herdr while `wl-copy` serves the selection", CHANGELOG). Not installed on this machine. |
| `xclip` / `xsel` | Linux X11 / XWayland | Need `DISPLAY`. Not installed here. |
| `pbcopy` | macOS | Always present. |

The big problem with all of them is **whose machine they run on**. Plugin commands are children of the herdr server. Live on this machine:
- The foreground client process has `SSH_CONNECTION`/`SSH_TTY` set (the user is on Tailscale SSH from another device).
- The server process has `DISPLAY`, `WAYLAND_DISPLAY` and `WSL_DISTRO_NAME`, and no `SSH_*`.
- So a plugin that "detects WSL and calls clip.exe" would write the host's Windows clipboard, which the user is not looking at.

The same happens with `herdr --remote`, where the plugin runs on the remote server host. An OSC 52 probe from a throwaway pane did **not** change the WSL host's Windows clipboard, because herdr routed it to the SSH client's terminal. That is the correct behaviour, and it shows the trap.

### 4. Go clipboard libraries

- **atotto/clipboard** ([clipboard_unix.go @6a3273b](https://github.com/atotto/clipboard/blob/6a3273b56adb47de891896ab1ce8c6c0b7dbae60/clipboard_unix.go#L52-L96)) shells out to the same tools. It picks `wl-copy`/`wl-paste` if `WAYLAND_DISPLAY` is set and both exist, then `xclip`, `xsel`, Termux, and finally `clip.exe` + `powershell.exe`. It writes raw bytes, so on WSL it inherits the `clip.exe` code-page mojibake. On macOS it uses `pbcopy`. It decides once, in `init()`, from the server-side environment, with the same wrong-machine problem.
- **golang.design/x/clipboard** ([README "Platform notes" @d95c794](https://github.com/golang-design/clipboard/blob/d95c7941706e35d9bf1d7a7560b0d8976c034712/README.md#platform-notes)) speaks X11/Wayland natively (pure Go now, no Cgo on desktop). But "your program owns what it copies … When it exits, the data goes with it, unless a clipboard manager has kept a copy". A short-lived overlay process would lose the copy on exit. Wayland needs a data-control compositor. `Init` fails with no display. It has no WSL-to-Windows path except through WSLg's X11/Wayland bridge.
- Neither library knows OSC 52 or herdr's client/server split. Neither adds anything over a few lines of Go that emit OSC 52.

### 5. Pasting into the Target Pane

- **`pane send-text <pane> <text>`** maps to `pane.send_text`, which calls `runtime.try_send_bytes(params.text)`: raw bytes, **never bracketed**, whatever the pane's mode ([src/app/api/panes.rs L1912-L1928](https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/panes.rs#L1912-L1928)). A pasted line containing `\n` would execute in a shell.
- **`pane.send_input {pane_id, text, keys}`** encodes `text` with `encode_api_text`. That wraps it in `ESC[200~ … ESC[201~` **when the pane currently has bracketed paste (DECSET 2004) enabled**, and sends it plain otherwise ([src/app/api_helpers.rs L25-L32](https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api_helpers.rs#L25-L32), [panes.rs L1930-L1956](https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/panes.rs#L1930-L1956)). `keys` is optional (`skip_serializing_if = Vec::is_empty`), so **text with no keys is a pure paste with no Enter**.
- **`pane run`** is `pane.send_input` with `keys: ["Enter"]` ([src/cli/pane.rs L1039-L1052](https://github.com/herdrdev/herdr/blob/v0.9.5/src/cli/pane.rs#L1039-L1052)): bracketed, but it submits. **`agent prompt`** is the same idea for agents. Neither suits a paste-only variant.
- **Live probe** (throwaway pane, DECSET 2004 on, `cat -v`):
  - `send-text "A-sendtext"` showed `A-sendtext`.
  - Raw socket `pane.send_input` (text only) showed `^[[200~B-sendinput^[[201~`.
- Transport: the socket is newline-delimited JSON. On Unix it is a socket at `HERDR_SOCKET_PATH`; on Windows it is a named pipe ([socket-api.mdx](https://github.com/herdrdev/herdr/blob/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx)). The request shape is `{"id":"…","method":"pane.send_input","params":{"pane_id":"w1:p1","text":"…"}}`.
- On macOS, `pane.send_input` checks the canonical line-input limit and rejects oversized lines (socket-api.mdx). A long paste into a canonical-mode program could be refused.
- **Safety:** herdr does not sanitize `text`. If copied screen text contained `ESC[201~` or other control bytes, it could break out of the bracketed paste. yubi should strip C0/C1 controls (keeping `\t`, and `\n` for multi-line pastes) before pasting. Screen text read via `pane read --format text` should already be plain, but it is cheap to enforce.
- The pane in `pane.send_input` is the **Target Pane**, which is the underlying pane from `HERDR_PLUGIN_CONTEXT_JSON` or the overlay's previous focus. The popup itself has no pane id. Paste only after the popup/overlay closes or loses input focus. While a popup is open it takes all terminal input, but `send_input` writes straight to the target PTY, so ordering is about UX, not correctness.

## Options matrix

| Option | WSL2 local | Linux X11/Wayland local | macOS local | SSH into herdr host | `herdr --remote` | Needs | Verdict |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **A. OSC 52 from the overlay/popup process** (stdout) | Yes (herdr re-emits OSC 52 to the outer terminal) | Yes (herdr client uses wl-copy/xclip/xsel, else OSC 52) | Yes (client uses pbcopy) | Yes (to the user's terminal) | Yes (local client) | Copy done inside a `[[panes]]` process; outer terminal allows OSC 52 where herdr picks it | **Recommended primary.** Same path and toast as herdr's own copy. Text only, ≤192 KiB, no read-back. |
| B. Shell out from the plugin (`clip.exe` / `wl-copy` / `xclip` / `pbcopy`), self-detected | Yes, if UTF-16LE is used | Yes, if the tool is installed and the server has `DISPLAY`/`WAYLAND_DISPLAY` | Yes | **Wrong clipboard** (host) | **Wrong clipboard** (remote) | Per-OS code; tool installed; encoding fixes | Fallback only, opt-in via config (e.g. `clipboard = "command"` + argv). |
| C. Go library (atotto) | Mojibake (raw UTF-8 to clip.exe) | Like B | Like B | Wrong clipboard | Wrong clipboard | Dependency | Adds nothing over B, and has the encoding bug. |
| D. Go library (golang.design/x/clipboard) | Only via WSLg bridge | Data lost when the short-lived process exits (no manager) | Yes | Wrong clipboard | Wrong clipboard | Dependency; display | Not suitable. |
| E. OSC 52 from an action command (no PTY) | No | No | No | No | No | n/a | Does not work: output goes to the plugin log. |
| F. herdr clipboard API | n/a | n/a | n/a | n/a | n/a | n/a | Does not exist in 0.9.5. |

| Paste option | Bracketed-aware | Sends Enter | Available as | Verdict |
| --- | --- | --- | --- | --- |
| `pane send-text` | No (raw) | No | CLI and socket | Avoid for multi-line text, which runs line by line in shells. |
| `pane.send_input` with text only | **Yes** (follows the live DECSET 2004 state) | No | **Socket only** (NDJSON) | **Recommended** for the copy-and-paste variant. |
| `pane run` / `pane.send_input` + `Enter` | Yes | Yes | CLI and socket | Only if the variant should also submit. |
| Self-wrapping `ESC[200~…ESC[201~` + `send-text` | Faked | No | CLI | Wrong when the target app has bracketed paste off; no API to query the mode. |

## Open points for the strategy decision

- Whether to offer the command fallback (B), and how to configure it (argv in config; default off).
- Trailing newline: send none by default, so the paste does not execute. `clip.exe` would turn `\n` into `\r\n`, one more reason to avoid it.
- Failure feedback: OSC 52 is fire-and-forget, so yubi cannot confirm success. herdr's client toast is the user-visible confirmation. If the content exceeds 192 KiB, yubi should warn before emitting.
- `min_herdr_version`: `pane.send_input` and OSC 52 forwarding exist in 0.9.5. I did not find the first version that had each.

## Method

- Read the herdr v0.9.5 source (cloned at tag `v0.9.5`) and the docs linked from <https://herdr.dev/llms.txt>. Checked `herdr --help`, `herdr pane send-text --help`, `herdr --skill`, `herdr --default-config`.
- Live probes on WSL2, Ubuntu, herdr 0.9.5:
  - `clip.exe` encodings.
  - PowerShell `Set-Clipboard` timing.
  - OSC 52 from a throwaway pane.
  - `send-text` vs `pane.send_input` against `cat -v` with DECSET 2004.
  - Server and client process environments.
  - The user's clipboard was saved first and restored afterwards (verified byte-identical). The throwaway pane was closed.
- Read atotto/clipboard (`6a3273b`) and golang.design/x/clipboard (`d95c794`) sources and READMEs, and xterm ctlseqs for OSC 52 and DECSET 2004.
