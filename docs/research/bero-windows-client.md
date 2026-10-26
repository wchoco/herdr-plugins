# bero: can the Windows herdr client attach to a WSL or remote server

Research for issue #47 (map: #43). herdr version examined: 0.9.5 (stable),
both the Windows build (`0.9.5-x86_64-pc-windows-msvc`) and the Linux build.

## Answer

- **Yes, over SSH.** A native Windows herdr client can attach to a herdr
  server on a Linux, macOS or Windows host with `herdr --remote <ssh-target>`
  or a saved machine (`herdr machine add`). It uses the Windows OpenSSH client.
  This is documented as supported. [W1][R1][M1]
- **There is no WSL-specific transport.** A Windows client reaches a WSL server
  the same way it reaches any host: over SSH. That means sshd running inside
  WSL (or some other SSH route into the distro). Nothing in the docs or source
  connects a Windows client to a WSL server's local socket directly. [S3]
- **Plugins run on the server, never on the client.** "Custom commands and
  plugins advertised by the selected server still run there. Herdr does not
  copy local command plugins, configuration, executables, or secrets onto SSH
  hosts." [M2] So bero itself runs in WSL or on the SSH host, not on Windows.
- **Which processes run on Windows:** the `herdr.exe` client, running inside
  the outer terminal (for example Windows Terminal), plus the `ssh.exe` it
  starts. The client draws the UI and handles keybindings and prefix mode
  locally. [R2] It has **no** extension point for third-party code: no
  client-side plugins or hooks. The IME Agent therefore has to be a separate
  Windows process. It cannot be hosted inside herdr's client.
- **Notable: the herdr client already switches the Windows IME.** The
  experimental `switch_ascii_input_source_in_prefix` feature runs in the client
  (`src/client/shell/input_source.rs`, `src/platform/windows.rs`). It turns the
  IME to ASCII while prefix mode or a few overlays are active, then restores
  the previous state. On Windows it supports only the Korean IME. [C1][C2][W2]
  This is the closest existing "IME Agent" there is. It also suggests a
  possible upstream route for the "herdr shortcut" Trigger (see Implications).

Confidence: **high** for attach support and where plugins run (both stated
in the docs and consistent with `--help`). **High** for the in-client IME code
(read from the v0.9.5 source). **Medium** for "no WSL transport": nothing
documents one, and the only WSL code found is detection for cursor rendering.
No live Windows-client-to-WSL attach was run for this ticket.

## Evidence

### Attach from Windows

- Windows support table: "`herdr --remote` and saved SSH machines to
  Linux/macOS/Windows hosts | supported". [W1]
- "From Windows Terminal, use the same remote command as Linux and macOS:
  `herdr --remote workbox`. … Herdr uses the installed Windows OpenSSH client
  and your SSH configuration. Windows OpenSSH does not use Herdr's Unix
  control-socket reuse, so key authentication through Windows `ssh-agent` is
  recommended to avoid repeated prompts during remote setup." [W1]
- "Remote attach and saved SSH machines support Linux, macOS, and Windows
  local clients connecting to Linux or macOS hosts on x86_64 and aarch64, or
  Windows hosts on x86_64." [R1] The same sentence appears for saved machines. [M1]
- Local and remote versions do not need to match once both support the stable
  endpoint generation. Interactive attach offers to install herdr at
  `~/.local/bin/herdr` on Linux hosts. [R1]
- `--help` of the Windows binary lists `--remote <target>`,
  `--remote-keybindings <local|server>` and `machine` with
  `add/status/reconnect/...`, identical to the Linux binary. The only
  differences between the two `--help` outputs are the config and log paths
  (`C:\Users\jiji\AppData\Roaming\herdr\config.toml`, plus `herdr-client.log`
  and `herdr-server.log`). [H1]
- Windows caveats that bear on this topology: "Local Windows recovery is not
  supported yet" for SSH authentication recovery, so a Windows client cannot
  use the shared ControlMaster connection. [M1] Direct terminal attach
  (`herdr terminal attach`) is unsupported on native Windows. [W1]

### Client/server split

- "In this mode, the remote server owns the running panes and sends their
  terminal content and session state over SSH. Your local Herdr draws the UI,
  including its sidebar, menus, and theme." [R2]
- "By default, `herdr --remote` uses your local Herdr keybindings for that
  attach. … Local custom command keybindings are not sent, because those
  commands would run on the remote host." [R2]
- "The UI uses the client's local theme, sidebar settings, and keybindings by
  default. Custom commands and plugins advertised by the selected server still
  run there." [M2]
- Status-bar `hostname`, `datetime` and `command` entries "resolve on the
  Herdr server". [CFG]
- `pane.focused` "reports manual pane selection changes from any client
  attached to this server … its payload does not identify the client". [API]
  So server-side bero sees focus changes but not which client made them. That
  is fine under #43's one-client assumption.

### WSL

- The docs mention WSL only for cursor rendering and Kitty graphics, never as
  an attach target. [W1][CFG]
- In source, `src/platform/linux.rs` detects WSL (`WSL_DISTRO_NAME`,
  `WSL_INTEROP`, `/proc/.../osrelease`, `/run/WSL`), but only to choose the
  drawn host cursor and skip the palette query. [S3]
- So Windows client → WSL server needs an SSH endpoint into the distro, e.g.
  sshd in WSL reached via `localhost` (WSL2 port forwarding or mirrored
  networking), or an ssh config `ProxyCommand` that tunnels through
  `wsl.exe`. Neither route was tested here.

### The client's existing Windows IME switching

- Config key `experimental.switch_ascii_input_source_in_prefix` (default
  false): "While prefix mode is active, temporarily switch the host input
  source to an ASCII-capable mode … then restore the previous input source
  when prefix mode exits. … on Windows it switches the IME to English (ASCII)
  input. Windows support is currently limited to the Korean IME; with an IME
  for any other language, the input source is left unchanged." [CFG][W2]
- It lives in the client. `ClientShellState::wants_ascii_input` is true in
  `Prefix`, `Navigate`, `Resize` and `Copy` modes and in several overlays
  (help, navigator, menus, confirm-close). `reconcile_input_source` holds off
  while the outer window is unfocused, "Restoring through a global key
  injection is only safe after this client regains focus". [C1]
- The Windows implementation's own notes are directly useful for designing
  the IME Agent: [C2]
  - "Windows IMEs live in the terminal-emulator process, not in herdr."
  - Reading: `WM_IME_CONTROL`/`IMC_GETOPENSTATUS` sent to
    `ImmGetDefaultIMEWnd(foreground)` "reads whether the IME is open … reliably
    across the process boundary". It goes through `SendMessageTimeoutW`
    (`SMTO_ABORTIFHUNG`, 200 ms).
  - Writing: "`IMC_SETOPENSTATUS` / `IMC_SETCONVERSIONMODE` changes the flag
    value but does NOT affect real input in terminal/TSF hosts", and
    `ImmGetContext` returns null across processes.
  - So herdr switches "the way kren-select does: inject the IME toggle key
    with `SendInput`". Only Korean (`VK_HANGUL`) is mapped. Japanese
    "(half/full-width) and Chinese use different keys per IME, so they return
    `None` and are left untouched".
  - On restore, it re-reads the state first and leaves it alone if the user
    already changed it manually during prefix mode. That matches the bero rule
    "manual changes during a Trigger are left alone".

## Implications for bero

1. The IME Agent cannot live inside the Windows herdr client. That process
   has no plugin or hook surface. The Agent is a separate Windows process,
   and the herdr client is not a "natural home". It is still a useful
   neighbour, because the client process already proves that IME read/toggle
   from a sibling process works for Korean.
2. bero (the plugin) runs wherever the server runs: WSL or the SSH host. The
   bero → IME Agent channel has to cross from that host back to Windows
   itself. herdr's SSH bridge offers no general server-to-client channel for
   plugins. (OSC 52 from a plugin pane does reach the client's clipboard, see
   `docs/herdr.md`, but that is clipboard only.)
3. The "herdr shortcut" Trigger is decided in the client when a remote client
   is used: keybindings and prefix mode run locally. No server-side plugin
   event for prefix mode was found in the docs. For that Trigger the
   strongest option may be the client's own feature. That means either
   extending `switch_ascii_input_source_in_prefix` upstream to the Japanese
   IME, or bero just recommending it, instead of bero detecting the shortcut
   itself. This should be confirmed in the Trigger-detection ticket.
4. The Japanese IME needs another write method. herdr's notes say
   `IMC_SETOPENSTATUS` does not take effect in terminal/TSF hosts, and herdr
   has no Japanese toggle key mapped. The IME Agent design should test this
   claim in Windows Terminal with the Microsoft Japanese IME. It may hold only
   for some IMEs or hosts.

## Not verified

- No live Windows client → WSL or SSH attach was run. The ticket forbids
  starting a Windows herdr server or touching the user's session, and the
  docs plus `--help` were consistent.
- Whether `IMC_SETOPENSTATUS` really fails for the Japanese IME in Windows
  Terminal. herdr's comment speaks of terminal/TSF hosts in general.

## Sources

- [W1] herdr docs, Windows support (v0.9.5):
  https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/windows-beta.mdx
- [W2] same page, "Partial support" → Prefix input-source switching.
- [R1] [R2] herdr docs, Persistence and remote access → Remote attach over SSH:
  https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/persistence-remote.mdx
- [M1] [M2] herdr docs, Connecting machines ("Before you connect",
  "Recovering SSH authentication", "Settings and automation"):
  https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/connecting-machines.mdx
- [CFG] herdr docs, Configuration → Prefix input source switching, status
  area; config reference key `experimental.switch_ascii_input_source_in_prefix`
  (via https://herdr.dev/llms-full.txt).
- [API] herdr docs, Socket API → event subscriptions (`pane.focused`)
  (via https://herdr.dev/llms-full.txt).
- [C1] herdr source v0.9.5, `src/client/shell/input_source.rs`:
  https://github.com/herdrdev/herdr/blob/v0.9.5/src/client/shell/input_source.rs
- [C2] herdr source v0.9.5, `src/platform/windows.rs` (section "Prefix-mode
  ASCII input source support", around line 3029):
  https://github.com/herdrdev/herdr/blob/v0.9.5/src/platform/windows.rs
- [S3] herdr source v0.9.5, `src/platform/linux.rs` (`running_inside_wsl`):
  https://github.com/herdrdev/herdr/blob/v0.9.5/src/platform/linux.rs
- [H1] `herdr --help`, `herdr machine --help` and `herdr plugin --help` of
  `/mnt/c/Users/jiji/.herdr/packages/standalone/releases/0.9.5-x86_64-pc-windows-msvc/herdr.exe`
  and `~/.local/bin/herdr`, both reporting `herdr 0.9.5`.
