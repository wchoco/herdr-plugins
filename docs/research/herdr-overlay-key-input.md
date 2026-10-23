# How keys reach yubi's overlay pane (herdr 0.9.5)

Research for wchoco/herdr-plugins#17 (map: #3, yubi). Target: herdr **0.9.5**.
This builds on the overlay-keys research for #4
(`docs/research/herdr-plugin-overlay-keys.md` on branch
`research/herdr-plugin-overlay-keys`). That note covered routing (prefix and
direct bindings) and the legacy bytes for a few keys. This note covers
modified keys, the kitty keyboard protocol and `modifyOtherKeys`, and Esc
timing.

Sources:

- The herdr 0.9.5 docs ([Keyboard], [Troubleshooting]).
- The herdr source at tag `v0.9.5`. `SRC/<path>#L<n>` means
  `https://github.com/herdrdev/herdr/blob/v0.9.5/<path>#L<n>`.
- Probes against the installed `herdr 0.9.5` binary, run on a **separate**
  herdr server. It had its own `XDG_CONFIG_HOME`, state, runtime dir and
  socket, a clean environment (`env -i`, no inherited `HERDR_*`), and a
  throwaway plugin. The plugin's `overlay` pane puts its TTY in raw mode,
  which also clears ISIG. It can optionally write a keyboard-mode sequence
  first, sends `CSI ? u`, and logs every `read()` with timestamps. The server
  was stopped afterwards.

Two kinds of "outer terminal" were used:

1. **A fake host.** A Python pty runs the herdr client and writes chosen bytes
   to it. In "kitty" mode it answers herdr's startup `CSI ? u` query, so herdr
   thinks the host speaks the kitty protocol. In "legacy" mode it doesn't
   answer. This shows how herdr decodes each host encoding and re-encodes it
   for the pane.
2. **A private tmux 3.6 server** (`tmux -L`, `-f /dev/null`) running the
   herdr client with `TMUX` set, driven by `tmux send-keys`.

"Probe" below marks a fact seen in these logs.

## Answer in brief

- **herdr does not pass host bytes through.** It decodes every host key into
  a key event, then **re-encodes it per pane** with libghostty, using that
  pane's own keyboard modes. A pane gets whatever encoding *it* asked for,
  whatever the host sent. This is per pane. Source:
  `encode_terminal_key` / `encode_key_with` (SRC/src/pane/terminal.rs#L2041,
  #L2504). Probe: the same host bytes reached three panes three different
  ways (legacy, kitty, `modifyOtherKeys`).
- **The kitty push `CSI > flags u` from yubi's pane is honoured, per pane.**
  The pane's terminal answers `CSI ? u` with its own flags: `CSI ? 0 u`
  before a push, `CSI ? 1 u` after `CSI > 1 u` (probe). `CSI > 4 ; 2 m`
  (`modifyOtherKeys` 2) is honoured too (probe). When the pane exits, its
  modes go with it.
- **A legacy pane (nothing pushed) can't tell modified Enter apart.** herdr
  deliberately turns Ctrl-Enter and Shift-Enter into a plain `\r`
  (`legacy_shell_key`, SRC/src/pane/input.rs#L109). Probe: true from a kitty
  host, from a legacy host, and from tmux. Alt-Enter is `ESC \r`. Ctrl-J is
  `\n`. So in a legacy pane, **Ctrl-J (`\n`) is the only Enter variant that
  differs from Enter.**
- **After yubi pushes kitty flag 1 (disambiguate)**, it gets (probe):
  - Enter `\r`
  - Ctrl-Enter `CSI 13;5u`
  - Shift-Enter `CSI 13;2u`
  - Alt-Enter `CSI 13;3u`
  - Ctrl-J `CSI 106;5u`
  - Esc `CSI 27u`
  - Alt-a `CSI 97;3u`
  - Ctrl-C `CSI 99;5u`
  - Backspace `\x7f`
  - Ctrl-Backspace `CSI 127;5u`
  - `A` / Shift+a: plain `A`
- **The outer terminal still decides which chords exist.** herdr can only
  re-encode what the host reported. At startup it pushes kitty flags 7 to the
  host and queries them. Under tmux it also sets `modifyOtherKeys` 2, and
  under WezTerm (or Alacritty on Windows) it sets `modifyOtherKeys` 1
  (SRC/src/client/terminal_setup.rs#L79, SRC/src/input/model.rs#L279,
  #L300-L332). If the host sends Ctrl-Enter as `\r`, every pane sees Enter.
  Probe: legacy host sends `\r` for Ctrl-Enter, so the pane gets `\r` even
  with kitty flags pushed.
- **Esc vs Alt-key.** In a legacy pane, Esc is `\x1b` and Alt-a is `\x1ba`.
  herdr writes each key in one write, so the pane normally gets them in
  separate reads. But herdr *itself* resolves the host's ambiguity first. On
  a legacy host with mouse capture on (the default), it holds a lone ESC for
  **up to 150 ms**. A key arriving within that window joins it: Esc, then `a`
  30 ms later, reached the pane as Alt-a (probe). With kitty flags pushed,
  yubi gets `CSI 27u` for Esc and never needs its own timeout. The 150 ms
  merge on a legacy host still happens upstream.
- **Uppercase and Shift+letter** arrive as the plain letter (`A`) in legacy
  and kitty-flag-1 panes (probe). Do **not** use `modifyOtherKeys` 2 in yubi:
  libghostty then sends Shift+a as `CSI 27;2;65~` (probe).
- **Ctrl-C** reaches a raw-mode pane as `\x03`, or `CSI 99;5u` with kitty
  flags (probe). One exception: herdr takes Ctrl-C (and Super-C) to copy a
  visible **mouse selection**, if there is one
  (`is_retained_selection_copy_key`, SRC/src/client/shell/input.rs#L8).
- **Backspace** is `\x7f` in every mode tested (probe). In a legacy pane,
  Ctrl-Backspace and Ctrl-H are both `\x08`.
- **What herdr intercepts in `overlay` placement** (unchanged from #4):
  - Every direct (non-prefix) binding, including the one that launched yubi.
  - The prefix key (default `ctrl+b`) and the key after it.
  - Ctrl-C while a mouse selection is visible.

  None of the keys in this ticket are bound directly by default. Probe: bare
  `h j k l`, Up and Down reached the overlay. `ctrl+b` and the Esc after it
  were swallowed. Routing runs *before* encoding, so pushing kitty flags
  doesn't change what herdr intercepts.

## 1. Pipeline: host bytes → key event → per-pane encoding

1. **Client startup.** The herdr client pushes kitty flags
   `DISAMBIGUATE | REPORT_EVENT_TYPES | REPORT_ALTERNATE_KEYS` (= 7) to the
   outer terminal. It then sends `CSI ? u` to see whether the host
   understood. Separately, it enables `modifyOtherKeys`, but only based on
   the environment: mode 2 when `TMUX` is set; mode 1 for WezTerm, or
   Alacritty on Windows. (SRC/src/client/terminal_setup.rs#L76-L121,
   SRC/src/input/model.rs#L279-L332.) Probe: the fake host saw `CSI > 7 u`,
   then `CSI ? u`.
2. **Decoding.** Host bytes (legacy, kitty CSI-u, or `CSI 27;m;k~`) are
   parsed into a `TerminalKey`. Probe: from a legacy host, `\n` decodes as
   Ctrl-J and `\x08` as Ctrl-H. A kitty pane receives them as `CSI 106;5u` and
   `CSI 104;5u`. A legacy host's `CSI 27;5;13~` (`modifyOtherKeys` form)
   decodes as Ctrl-Enter.
3. **Routing.** Bindings and the prefix are checked first
   (SRC/src/client/shell/input.rs#L610-L690; see #4).
4. **Encoding for the focused pane.** `encode_terminal_key` checks whether the
   pane "negotiated nothing" (kitty flags 0 and `modifyOtherKeys` off, read
   from libghostty's live state; SRC/src/pane/terminal.rs#L2531).
   - If so, herdr applies its legacy table. Any modified Enter becomes Enter
     (Alt is kept as an ESC prefix). Ctrl-M becomes Enter, Ctrl-I becomes Tab,
     Ctrl-[ becomes Esc, Ctrl+Shift+letter becomes the control byte, and Super
     chords are dropped (SRC/src/pane/input.rs#L104-L151).
   - Otherwise, libghostty encodes the key for the pane's flags.

   The docs say the same: "Pane input follows the keyboard modes requested by
   the app … Apps using Kitty keyboard input receive the modifiers and key
   events they request. Without report-all mode, modified Enter, Tab, and
   Backspace do not receive release reports." ([Keyboard], line 69).
5. **Report-all upgrade.** If the focused pane asks for report-all-keys (flag
   8), or for event types plus `modifyOtherKeys`, the server tells the client
   to raise the host's flags to 31. The flags go back to 7 when focus leaves
   (SRC/src/server/headless/render.rs#L182-L227,
   SRC/src/client/shell_runtime.rs#L98-L116,
   SRC/src/terminal_modes.rs#L41-L58). Probe: opening a pane that pushed
   `CSI > 31 u` made the host see `CSI < 1 u CSI > 31 u`. Closing it gave
   `CSI < 1 u CSI > 7 u`. Flags 1, and `modifyOtherKeys` 2 alone, didn't
   touch the host.

`KittyKeyboardTracker` (SRC/src/pane/kitty_keyboard.rs#L1-L6) only records
modes so they survive an update handoff. Live keyboard state lives in
libghostty.

## 2. Bytes the overlay pane received (probe logs)

Columns:

- **kitty host / legacy pane**: the fake host answered the kitty query and
  sent kitty-encoded keys; the pane pushed nothing.
- **pane `CSI > 1 u`**: same host, pane pushed flag 1.
- **pane `CSI > 31 u`**: same host, pane pushed flags 31.
- **pane `CSI > 4;2 m`**: same host, pane enabled `modifyOtherKeys` 2.

Host bytes per key:

| Key | Host bytes |
| --- | --- |
| Enter | `\r` |
| Ctrl-Enter | `CSI 13;5u` |
| Shift-Enter | `CSI 13;2u` |
| Alt-Enter | `CSI 13;3u` |
| Ctrl-J | `CSI 106;5u` |
| `A` | `A` |
| Esc | `CSI 27u` |
| Alt-a | `CSI 97;3u` |
| Ctrl-C | `CSI 99;5u` |
| Backspace | `\x7f` |
| Ctrl-Backspace | `CSI 127;5u` |

What the pane received:

| Key | kitty host / legacy pane | pane `CSI > 1 u` | pane `CSI > 31 u` | pane `CSI > 4;2 m` |
| --- | --- | --- | --- | --- |
| Enter | `\r` | `\r` | `CSI 13u` | `\r` |
| Ctrl-Enter | `\r` | `CSI 13;5u` | `CSI 13;5u` + release `CSI 13;5:3u` | `CSI 27;5;13~` |
| Shift-Enter | `\r` | `CSI 13;2u` | `CSI 13;2u` + release | `CSI 27;2;13~` |
| Alt-Enter | `ESC \r` | `CSI 13;3u` | `CSI 13;3u` + release | `CSI 27;3;13~` |
| Ctrl-J | `\n` | `CSI 106;5u` | `CSI 106;5u` + release | `CSI 27;5;106~` |
| `a` | `a` | `a` | `a`¹ | `a` |
| `A` (Shift+a) | `A` | `A` | `A`¹ | `CSI 27;2;65~` |
| Esc | `\x1b` | `CSI 27u` | `CSI 27u` + release | `\x1b` |
| Alt-a | `ESC a` | `CSI 97;3u` | `CSI 97;3u` + release | `CSI 27;3;97~` |
| Ctrl-C | `\x03` | `CSI 99;5u` | `CSI 99;5u` + release | `CSI 27;5;99~` |
| Backspace | `\x7f` | `\x7f` | `CSI 127u` | `\x7f` |
| Ctrl-Backspace | `\x08` | `CSI 127;5u` | `CSI 127;5u` | `\x08` |

Release events sent by the host were dropped for the legacy and flag-1 panes.

¹ The fake host kept sending plain `a`/`A` after herdr asked it for flags 31.
A real kitty host would send `CSI 97u` and so on, so this cell isn't what a
real terminal produces.

Legacy host (no kitty answer, no `modifyOtherKeys`), legacy bytes in:

| Key (host bytes) | legacy pane | pane `CSI > 1 u` |
| --- | --- | --- |
| Enter / Ctrl-Enter / Shift-Enter (`\r`) | `\r` | `\r` |
| Ctrl-J (`\n`) | `\n` | `CSI 106;5u` |
| Alt-Enter (`ESC \r`) | `ESC \r` | `CSI 13;3u` |
| `A` | `A` | `A` |
| Esc (`\x1b`) | `\x1b`, delivered about 150 ms late | `CSI 27u`, about 150 ms late |
| Alt-a (`ESC a`) | `ESC a` | `CSI 97;3u` |
| Ctrl-C (`\x03`) | `\x03` | `CSI 99;5u` |
| Backspace (`\x7f`) | `\x7f` | `\x7f` |
| Ctrl-H (`\x08`) | `\x08` | `CSI 104;5u` |
| Tab (`\t`) | `\t` | `\t` |
| `CSI 27;5;13~` (Ctrl-Enter, `modifyOtherKeys` form) | n/a | `CSI 13;5u` |

Real tmux 3.6 outer terminal (`extended-keys on`, herdr set `modifyOtherKeys`
2), keys sent with `tmux send-keys`:

- With a **legacy pane**, results matched the legacy column above: C-Enter and
  S-Enter gave `\r`, M-Enter `ESC \r`, C-j `\n`, Escape `\x1b`, M-a `ESC a`,
  C-c `\x03`, BSpace `\x7f`, C-BSpace `\x08`.
- With **flag 1 pushed**, results matched the `CSI > 1 u` column: C-Enter
  `CSI 13;5u`, S-Enter `CSI 13;2u`, M-Enter `CSI 13;3u`, C-j `CSI 106;5u`,
  Escape `CSI 27u`, M-a `CSI 97;3u`, C-c `CSI 99;5u`, C-BSpace `CSI 127;5u`.
- `A` and `S-a` both gave `A`.

`send-keys` injects key events inside tmux. It doesn't show that a real
terminal in front of tmux reports Ctrl-Enter. With `extended-keys off`, the
results were the same, so this tmux probe doesn't separate the tmux settings.

## 3. Esc and Alt timing

- On the pane side, herdr writes each key's bytes in one PTY write. In every
  probe, a lone `\x1b` and a following key arrived in separate `read()`s when
  they were ≥150 ms apart.
- The ambiguity is resolved **upstream**, in herdr's host input framer
  (SRC/src/raw_input.rs#L42-L54, #L435-L457).
  - A pending lone ESC from the host is flushed as Esc after **10 ms** idle.
  - With host mouse capture on (`mouse_capture`, default true), that becomes
    **150 ms**.
  - A pending `ESC [` gets 50 ms.

  Probe (legacy host, mouse capture default):
  - Esc with nothing after it reached the pane about 150 ms late.
  - Esc, then `a` 2 ms or 30 ms later, reached the pane as Alt-a.
  - Esc, then `[A` 2 ms later, reached the pane as Up.
  - `ESC ESC` in one write reached the pane as two Escs, 150 ms apart.
- With a kitty-capable host, Esc arrives as `CSI 27u` and has no ambiguity
  upstream. In a pane with kitty flags pushed, Esc is `CSI 27u` all the way
  through, so yubi needs no Esc timeout of its own.

## 4. Can yubi change what it receives?

- **Yes, per pane.** Write `CSI > 1 u` (or `CSI = 1 u`) to the pane's stdout
  at startup, and pop with `CSI < u` on exit (optional, because the pane dies
  anyway). From then on, herdr encodes keys for yubi's pane with kitty flag 1.
  No manifest field or API call is needed.
- yubi can confirm the mode took effect by sending `CSI ? u`. The pane
  answers on stdin with `CSI ? <flags> u` (probe).
- Flags beyond 1 have costs:
  - Flag 2 (event types) or 8 (report all) brings release events.
  - Flag 8 also makes herdr raise the **host's** flags to 31 while yubi is
    focused. This is harmless but unnecessary.
  - `modifyOtherKeys` 2 turns Shift+letter into `CSI 27;2;<cp>~`, which is
    worse for typing Hints.
- **The outer terminal still decides which chords exist.** Ctrl-Enter and
  Shift-Enter only reach yubi as distinct keys when the user's terminal
  reports them, meaning:
  - a terminal that speaks the kitty protocol and answered herdr's query, or
  - tmux with extended keys, which herdr enables via `modifyOtherKeys` 2.

  On a legacy host (or tmux without extended keys, or many SSH setups),
  Ctrl-Enter is `\r` at the source, and no pane mode can recover it (probe).

## Points for the "Copy-and-paste trigger" decision

- **Always distinguishable** (any host, any pane mode):
  - Enter (`\r`) vs Ctrl-J (`\n`, or `CSI 106;5u` with kitty flags).
  - Lowercase vs uppercase letters (`a` vs `A`).
  - Alt-letter (`ESC a`) vs the letter, though on a legacy host an Alt-letter
    can be faked by typing Esc and then the letter within 150 ms.
  - Ctrl-C, Backspace, Tab, Esc (Esc is about 150 ms late on legacy hosts).
- **Distinguishable only with kitty flags pushed by yubi *and* a host that
  reports them:** Ctrl-Enter, Shift-Enter, Alt-Enter (Alt-Enter is `ESC \r`
  even in legacy, which still differs from `\r`), and Ctrl-Backspace vs
  Ctrl-H.
- **Never distinguishable in a legacy pane:** Ctrl-Enter and Shift-Enter vs
  Enter (herdr folds them by design).
- So an uppercase-Hint trigger (tmux-thumbs style) works on every host.
  Ctrl-Enter for Joined Copy needs the Ctrl-J alias, which the Line Mode
  grammar (#8) already has.

## Unknowns

- Real terminals weren't exercised directly. kitty, Ghostty, WezTerm,
  iTerm2, Windows Terminal and SSH were only *simulated* through the fake
  host and `tmux send-keys`. Which ones report Ctrl-Enter and Shift-Enter
  distinctly to herdr is up to each terminal.
- Whether tmux's `extended-keys` setting (off / on / always) and
  `terminal-features … extkeys` need user configuration in front of herdr
  wasn't settled. `send-keys` gave the same result with `extended-keys off`.
- With flags 31, a real kitty host would send `CSI 97u` for `a`, and herdr
  would re-encode it. The fake host didn't model that.
- `herdr --remote` / `--machine`: the input path through the remote client
  wasn't probed.
- Windows: herdr uses a ConPTY fallback encoding there
  (SRC/src/pane/terminal.rs#L2052-L2057). Not examined.

[Keyboard]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/keyboard.mdx
[Troubleshooting]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/troubleshooting.mdx
