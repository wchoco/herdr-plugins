# bero: can a herdr plugin know when a shortcut is being entered

Research for issue #45 (map: #43). herdr version examined: 0.9.5 (socket
protocol 22, schema_version 1), from docs, the bundled API schema, the v0.9.5
source, and a live throwaway server with a client in tmux.

## Answer

- **No.** A herdr 0.9.5 plugin cannot learn when the user presses the prefix
  key or enters any keybinding mode, nor when that mode ends. Nothing in the
  API carries it: no event, no subscription, no query field. [S1][S2][L1][L2]
- **Why: the mode lives only in the client.** Prefix, Navigate, Resize and Copy
  mode are `ClientShellMode` values in the client's `ClientShellState`
  (`src/client/*`). The server, which is where plugins run, never receives
  them. [C1][C2][R1]
- **The prefix chord cannot be bound to a plugin action.** Binding the same
  chord as `keys.prefix` and a `[[keys.command]]` makes herdr keep the prefix
  and disable the command (`herdr config check`: "kept keys.prefix, disabled
  keys.command[0].key"). [L3]
- **`prefix+<key>` plugin bindings only mark the end.** A
  `[[keys.command]] key = "prefix+y"` runs when `y` is pressed, i.e. when the
  shortcut finishes. There is no signal at the prefix press, and no signal for
  a cancelled prefix (Esc) or for modes that a built-in action enters. [D3][L1]
- **herdr already does this itself, client-side, for Korean only.**
  `[experimental] switch_ascii_input_source_in_prefix = true` turns the input
  source to ASCII while Prefix, Navigate, Resize or Copy mode (or the help,
  navigator, confirm-close, worktree-remove and context menus) is active, and
  restores it when returning to terminal input or entering a herdr text field.
  It works only in a client running natively on macOS or Windows, and on
  Windows only with the Korean IME. Elsewhere it does nothing. [D1][D2][W1][C1][C3]
  In bero's WSL topology the herdr client is a Linux process, so this is a
  no-op there.

Confidence: **high** that no plugin-visible signal exists (schema enumerated,
docs read, mode state traced to client-only code, and confirmed live: no events
and an identical session snapshot while in prefix mode). **High** on the
built-in feature's scope (docs plus source). The workarounds below are
**untested**.

## Evidence

### Schema [S1][S2]

`herdr api schema --json` (0.9.5):

- `EventKind` (pushed events): `workspace_*`, `worktree_*`, `tab_*`,
  `pane_created/closed/updated/focused/moved/output_changed/exited`,
  `pane_agent_detected`, `pane_agent_status_changed`, `layout_updated`.
  Nothing about keys or modes.
- `SubscriptionEventKind` (parameterised subscriptions):
  `pane.output_matched`, `pane.agent_status_changed`, `pane.scroll_changed`.
- `SessionSnapshot` has focused workspace/tab/pane, layouts, panes, agents.
  No mode field.
- The only "mode" in the request schema is `PaneZoomMode`.
  `ClientShellSurfaceSetParams` and `command.invoke` belong to the client-shell
  protocol (a client asks the server to run a command); they do not expose the
  client's keyboard mode to others.

### Docs

- [D1] `configuration.mdx`, "Prefix input source switching": "On macOS and
  Windows, Herdr can temporarily switch to an ASCII-capable input source while
  prefix commands and prefix-launched modes are active ... Herdr restores the
  previous input source when returning to terminal input or entering a text
  field. This setting has no effect on other platforms." Caution box: "On
  Windows, support is currently limited to the Korean IME."
- [D2] `config-reference.json`, `experimental.switch_ascii_input_source_in_prefix`:
  "... Windows support is currently limited to the Korean IME; with an IME for
  any other language, the input source is left unchanged. macOS and Windows
  only; a no-op elsewhere and a best-effort no-op if the switch fails."
- [D3] `configuration.mdx`, "Custom command keybindings" and `plugins.mdx`,
  "Keybindings": `[[keys.command]]` with `type = "plugin_action"` binds a key
  string to a plugin action. Key strings are either `prefix+<key>` or a direct
  chord.
- [D4] `concepts.mdx`, "Modes": "Herdr has terminal mode, prefix mode, and
  navigate mode. ... Prefix mode waits for one Herdr action after the prefix
  key." `configuration.mdx` also names Prefix, Navigate, Copy and Resize mode
  bars.
- [W1] `windows-beta.mdx`: "Prefix input-source switching (Korean IME) |
  partial"; "Other Windows IMEs are not supported by this option yet."

### Source (herdr v0.9.5)

- [C1] `src/client/shell/input_source.rs`: `wants_ascii_input()` is true for
  `ClientShellMode::Prefix | Navigate | Resize | Copy` and for the overlays
  `ConfirmClose`, `Help`, `Navigator`, `WorktreeRemove`, `ContextMenu`,
  `GlobalMenu`. `reconcile_input_source()` queues on/off changes, and skips
  them while the outer window is unfocused.
- [C2] Code search for `ClientShellMode` in `herdrdev/herdr` hits only files
  under `src/client/shell/`. Nothing in the server or the API layer refers to
  it.
- [C3] `src/client/shell_runtime.rs` applies the queued changes through
  `platform::PrefixInputSource` (`switch_to_ascii()` / `restore()`).
  `src/platform/windows.rs` implements them for Windows. Its comments are
  useful for bero's IME Agent (#43) too:
  - `WM_IME_CONTROL` / `IMC_GETOPENSTATUS` sent to `ImmGetDefaultIMEWnd(GetForegroundWindow())`
    reads the open state reliably across processes. It is sent with
    `SendMessageTimeoutW` (`SMTO_ABORTIFHUNG`, 200 ms).
  - "Writing the state back (`IMC_SETOPENSTATUS` / `IMC_SETCONVERSIONMODE`)
    changes the flag value but does NOT affect real input in terminal/TSF
    hosts". `ImmGetContext` across processes returns null.
  - So herdr switches by injecting the IME toggle key with `SendInput`, chosen
    by the foreground layout's language. Only Korean (`VK_HANGUL`) is mapped:
    "Japanese (half/full-width) and Chinese use different keys per IME, so
    they return `None` and are left untouched".
- [R1] Issue #47's research (`docs/research/bero-windows-client.md` on branch
  `research/bero-windows-client`) reached the same conclusion independently:
  plugins run on the server; the client handles keybindings and prefix mode
  locally and has no extension point.

### Live checks (throwaway herdr 0.9.5, client in `tmux -L`)

- [L1] Subscribed to every parameterless `events.subscribe` type, then sent
  `ctrl+b` (status bar showed `PREFIX`), `y` (bound to a shell command),
  `ctrl+b` then Esc, and `ctrl+b r` (status bar showed `RESIZE`) then Esc.
  **No events arrived** across any of it. The same subscriber did get
  `tab_created` etc. for a `herdr tab create` straight after, so it was
  working. The `prefix+y` command ran about 60 ms after `y` was sent.
- [L2] `herdr api snapshot` taken in terminal mode and again while `PREFIX`
  was showing: **identical**.
- [L3] Config with `keys.prefix = "ctrl+b"` and `[[keys.command]] key = "ctrl+b"`:
  `herdr config check` exits 1 with "ctrl+b: kept keys.prefix, disabled
  keys.command[0].key".
- Side observation: `prefix+c` opens a "new tab" name dialog (a herdr text
  field) rather than creating the tab at once. Entering a shortcut can
  therefore end in a text field where the user may want their IME back. herdr's
  own switch restores the input source there for this reason [D1].

## Implications for bero

The "entering a herdr shortcut" Trigger cannot be zero-config from the herdr
(server) side in 0.9.5. Options, none tested:

1. **Detect on the Windows side, in the IME Agent.** A low-level keyboard hook
   (`WH_KEYBOARD_LL`) sees keys before the IME does. When the terminal hosting
   herdr is foreground and the prefix chord goes down, turn the IME Off; when
   the next key completes the shortcut, Restore. bero could read `keys.prefix`
   from the herdr config and pass it to the Agent, which keeps it close to
   zero-config. Weak points: the Agent must model herdr's keymap. Some keys
   enter persistent modes (`prefix+w` navigate, `prefix+r` resize, `prefix+[`
   copy, `prefix+?` help, `prefix+g` goto) that end on Esc, `q` or Enter.
   Others open text fields (`prefix+c`, renames). Remote or local keybindings
   may differ (`--remote-keybindings server`). And the Agent would be
   keylogging the terminal window, which needs care.
2. **Upstream.** The cleanest fix lives in herdr: extend
   `switch_ascii_input_source_in_prefix` to other IMEs (Japanese is excluded
   only because the toggle key varies per IME [C3]), or expose client mode
   changes as an event. The built-in switch only acts in a client running
   natively on Windows, such as a Windows herdr client attached over SSH
   (#47). A client running in WSL would need the event route plus bero.
3. **Config-side fallback.** Users who switch to prefix-free `ctrl+alt+...`
   chords [keyboard.mdx] avoid typing a bare key into herdr. Modifier chords
   are generally not composed by the IME, but this assumption has not been
   checked against the Japanese IME. Navigate/resize/copy modes still take
   bare keys.
4. **Drop the Trigger from v1**, or limit it to "a `prefix+<key>` plugin
   binding ran", which only gives an end signal and is useless for setting the
   IME Off beforehand.

## Sources

- [S1] `herdr api schema --json`, herdr 0.9.5: `schemas.event.$defs.EventKind`,
  `schemas.success_response.$defs.SessionSnapshot`, `schemas.request.$defs`.
- [S2] same, `schemas.subscription_event.$defs.SubscriptionEventKind`.
- [D1] https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/configuration.mdx
- [D2] https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/data/config-reference.json
- [D3] configuration.mdx (above) and https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/plugins.mdx
- [D4] https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/concepts.mdx
- [keyboard.mdx] https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/keyboard.mdx
- [W1] https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/windows-beta.mdx
- [C1] https://github.com/herdrdev/herdr/blob/v0.9.5/src/client/shell/input_source.rs
- [C2] GitHub code search `ClientShellMode repo:herdrdev/herdr`
- [C3] https://github.com/herdrdev/herdr/blob/v0.9.5/src/client/shell_runtime.rs,
  https://github.com/herdrdev/herdr/blob/v0.9.5/src/platform/windows.rs
- [R1] `docs/research/bero-windows-client.md` (branch `research/bero-windows-client`, issue #47)
- [L1]-[L3] live runs via `scripts/throwaway-herdr`, 2026-10-26.
