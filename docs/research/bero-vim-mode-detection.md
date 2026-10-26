# Detecting vim and neovim normal mode from pane output (zero-config)

Research for #48 (parent map #43, bero). The question: with no vimrc or init.lua changes, what do vim and neovim write to the terminal that tells normal mode apart from insert and cmdline modes? How reliable is it? If zero-config is impossible, what is the smallest app-side config that works?

Versions: Vim 9.1.2141 (Debian package, tested locally), Vim `master` source, Neovim `master` source and docs, plus older Neovim release docs where noted. herdr facts are pinned to **v0.9.5**. Neovim is not installed on this machine, so the Neovim findings come from source and docs only. Nothing was measured against a running nvim.

Source shorthand:

- `VIM/<path>` = `https://github.com/vim/vim/blob/master/<path>`
- `NVIM/<path>` = `https://github.com/neovim/neovim/blob/master/<path>`
- `HERDR/<path>` = `https://github.com/herdrdev/herdr/blob/v0.9.5/<path>`

## Short answer

| | Zero-config signal? | What it is | Reliability |
|---|---|---|---|
| **Neovim** | **Yes** | DECSCUSR cursor shape, `ESC [ N SP q`, from the default `'guicursor'`. Steady block `2` = normal/visual/cmdline. Steady bar `6` = insert. Steady underline `4` = replace/operator-pending. Blinking block `1` = `:terminal` mode (0.11+). | High. It has been the default since at least v0.2.1 (2017), `--clean` and `-u NONE` keep it, and herdr's `TERM=xterm-256color` turns it on. Users can switch it off with `set guicursor=`. |
| **Vim** | **No** reliable signal | Vim sends no cursor-shape sequence by default (no `t_SI`/`t_EI`). The only mode-bearing output is the `'showmode'` text (`-- INSERT --`, `-- REPLACE --`, `-- VISUAL --`) drawn on the last screen row. | Low. The text is localised (`-- 挿入 --` under a Japanese locale), it is off in `compatible` mode, and statusline setups often turn it off. Reading it means scraping the screen. Not recommended. |

**Minimal app-side config for Vim** (3 lines in `~/.vimrc`). This makes Vim emit the same signal Neovim emits by default, so bero needs only one detector:

```vim
let &t_SI = "\e[6 q"   " insert: steady bar
let &t_SR = "\e[4 q"   " replace: steady underline
let &t_EI = "\e[2 q"   " leaving insert/replace: steady block
```

**Caveats for both editors. They matter for the 50 ms target and for correctness:**

1. **Latency.** The "back to normal" sequence is emitted only after the editor has decided that `Esc` is a lone key, which takes `'ttimeoutlen'`. That was about 126 ms in Vim with `defaults.vim` (`ttimeoutlen=100`) and about 1027 ms with no `ttimeout`. In Neovim the default `ttimeoutlen` is 50. The shape stream alone therefore misses a ~50 ms target. bero can meet it by acting on the `Esc` keystroke that herdr forwards into the pane, when that pane currently shows a bar cursor, and then using the shape stream to confirm or correct.
2. **Exit and suspend leave a block cursor.** Neovim's reset sequence with herdr's terminfo is `ESC[2 q`, the same bytes as normal mode. Vim leaves the last shape in place. A block cursor alone does not mean "an editor is in normal mode". The Trigger has to be gated on the pane's foreground program being vim/nvim, or at least on having seen that program start.
3. **The shape-to-mode mapping is not one-to-one.** Block also covers visual and cmdline (`:`, `/`). Underline covers both replace (typing text) and operator-pending (`d`, `c`, `y` …, which is normal-mode-ish). See "Mapping shapes to Triggers".
4. **herdr 0.9.5 does not expose cursor shape to plugins.** herdr parses DECSCUSR internally, but no socket API method returns it. Whether bero can observe the byte stream or the parsed shape at all belongs to #44.

Confidence: high for "Vim has no default signal" (upstream source, three distro system vimrcs, and empirical capture agree). High for Neovim's default and its terminal detection (source and docs), but not verified empirically here. High for the latency numbers in Vim (measured). Neovim's ~50 ms is inferred from its default `ttimeoutlen`, not measured.

## Vim

### What Vim emits by default: nothing mode-specific

- `:h termcap-cursor-shape` (`VIM/runtime/doc/term.txt`): "When Vim enters Insert mode the 't_SI' escape sequence is sent … When leaving Insert mode or Replace mode 't_EI' is used. … These are not standard termcap/terminfo entries, you need to set them yourself." The help also says that when `t_EI` is empty, `t_SI` and `t_SR` are never sent.
- Source: `term_cursor_mode()` (`VIM/src/term.c`, around L4479) returns right away when `*T_CEI == NUL`. The only way `t_SI`/`t_EI` get filled automatically is from the termcap capabilities `SI`/`EI` (`VIM/src/term.c` around L1874). Those are non-standard and do not exist in terminfo. The builtin xterm table has `t_SH` (`ESC[%d q`) and `t_RS`. They are used for `:terminal` window cursor handling (`term_cursor_shape()`) and for querying the terminal's cursor style. They are not used for mode changes.
- `defaults.vim` (Vim 9.1) does not touch `t_SI`/`t_EI`/`t_SR`. Neither do the distro system vimrcs checked: Debian `/etc/vim/vimrc` (local), Fedora `vimrc` (`https://src.fedoraproject.org/rpms/vim/raw/rawhide/f/vimrc`, which only appends `a:blinkon0` to `'guicursor'`, a GUI-only option in Vim), and Arch `archlinux.vim` (`https://gitlab.archlinux.org/archlinux/packaging/packages/vim/-/raw/main/archlinux.vim`).

**Empirical check.** Vim 9.1.2141 ran in a pty with `TERM=xterm-256color`, both with `-Nu NONE` and with the default config (empty `$HOME`, so Debian vimrc plus `defaults.vim`). I also tried answering Vim's `ESC[>c` probe as xterm 390. Across `i`, `Esc`, `R`, `v`, `:`, `o`, `Ctrl-O` and `:q!`, **no DECSCUSR (`ESC[… q`), OSC 12, or other mode-specific sequence was written** in any of these runs. The only differences between modes were the redraws of the last row.

### The `'showmode'` text (screen-scrape only)

With the default config, entering insert writes `ESC[24;1H ESC[1m -- INSERT -- ESC[m`, and leaving it writes `ESC[24;1H ESC[K` (observed). The same happens for `-- REPLACE --`, `-- VISUAL --` and `-- (insert) --` (Ctrl-O). Why this is not a reliable signal:

- `'showmode'` is on only when `nocompatible` (`:h 'showmode'`). Debian's `vim.tiny` with `vimrc.tiny` runs `compatible`.
- The text is translated. `VIM/src/po/ja.po` maps `" INSERT"` to `" 挿入"` and `" REPLACE"` to `" 置換"`. bero's users are by definition likely to run a Japanese locale.
- Statusline plugins (airline, lightline) commonly `set noshowmode`.
- Cmdline mode has no marker at all.
- It needs reading screen contents (#44 territory), not watching a byte pattern.

### Minimal Vim config and its observed behaviour

With the three-line snippet above (plus `defaults.vim`), the captured sequences were:

| action | emitted |
|---|---|
| startup | nothing (Vim does not send `t_EI` until the first mode change) |
| `i` / `o` | `ESC[6 q` |
| `Esc` | `ESC[2 q` |
| `R` | `ESC[4 q` |
| `v`, `:` | nothing (block was already set; cmdline and visual count as non-insert) |
| `Ctrl-O` in insert | `ESC[2 q`, then `ESC[6 q` when it returns to insert |
| `:q!` | nothing. The block stays after exit (`:h termcap-cursor-shape`: "When Vim exits the shape for Normal mode will remain"). |

Because nothing is sent at startup, bero should treat "vim just started" as normal mode. Vim always starts in normal mode unless it is started with `+startinsert`.

Optional extra lines: `let &t_ti ..= "\e[2 q"` and `let &t_te ..= "\e[0 q"` (from `:h termcap-cursor-shape`) make startup explicit and reset the shape on exit. `\e[0 q` is distinct from `\e[2 q`, which helps with caveat 2.

Vim 9.1 also has `echoraw()` and the `ModeChanged` autocommand. A config could emit a dedicated private sequence carrying the exact mode name. That is more precise (it separates cmdline from normal) but longer, and it gives up the "same detector as Neovim" property. The cursor-shape snippet is the smallest config that works.

## Neovim

### The default `'guicursor'` makes the TUI emit DECSCUSR

- Default (`NVIM/runtime/doc/options.txt`): `"n-v-c-sm:block,i-ci-ve:ver25,r-cr-o:hor20,t:block-blinkon500-blinkoff500-TermCursor"`. The `t:` part first appears in the 0.11 docs. `release-0.10`, `release-0.9`, `v0.5.0` and `v0.2.1` all have `"n-v-c-sm:block,i-ci-ve:ver25,r-cr-o:hor20"`. It is an option default, so `nvim --clean` and `-u NONE` keep it. Popular distributions (LazyVim and similar) do not clear it as far as I know (not verified).
- `:h tui-cursor-shape` (`NVIM/runtime/doc/tui.txt`): Nvim uses the terminfo extensions `Ss`/`Se`. If they are missing, it adds them based on `$TERM` and environment variables.
- Source: `tui_set_mode()` (`NVIM/src/nvim/tui/tui.c`, around L1493) maps block→1, hor→3, ver→5. It adds 1 when the mode does not blink, then prints `set_cursor_style` (Ss). With the default (no blink on n/i/r), that gives **block = `ESC[2 q`, bar = `ESC[6 q`, underline = `ESC[4 q`**. The `t:` entry blinks, so it gives `ESC[1 q`. The function runs on every `mode_change` UI event, so a sequence is emitted at startup and on each mode change. If `'guicursor'` is empty it emits the reset sequence (`Se`) instead.
- Terminal detection under herdr: herdr sets `TERM=xterm-256color`, `COLORTERM=truecolor` and `TERM_PROGRAM=herdr` in every pane (`HERDR/src/pane.rs` L100-L108, `HERDR/crates/ghostty-vt/src/lib.rs` L34). Nvim treats any `xterm*` TERM as the xterm family (`patch_terminfo_bugs`, `NVIM/src/nvim/tui/tui.c` around L2182). For a plain xterm it does `terminfo_set_if_empty(set_cursor_style, "\x1b[%p1%d q")` (around L2427). The local ncurses 6.6 `xterm-256color` entry already has `Ss=\E[%p1%d q` and `Se=\E[2 q`, so DECSCUSR is on either way. Over ssh, TERM is passed to the remote side, so a remote nvim behaves the same. The only blacklist (st, old VTE, Konsole) clears the *reset* sequence, not `Ss`. It keys off env vars such as `VTE_VERSION`/`KONSOLE_VERSION`, which could leak into the pane from the outer terminal. Even then, mode shapes still flow.
- Exit/suspend: the TUI prints `reset_cursor_style` (`NVIM/src/nvim/tui/tui.c` L666, L1410). For an xterm-family TERM with the ncurses entry, that is `Se=\E[2 q`, **the same bytes as normal mode**. If a stray `TMUX` env or another detected terminal is present, it is `ESC[0 q`.

### Mapping shapes to Triggers (default `'guicursor'`)

| bytes | Neovim modes (default) | Vim modes (with the snippet) | suggested bero meaning |
|---|---|---|---|
| `ESC[2 q` steady block | normal, visual, cmdline (`:`, `/`, `?`), showmatch blink in insert (`sm`, only with `'showmatch'`, off by default) | normal, visual, cmdline, Ctrl-O | Trigger starts |
| `ESC[6 q` steady bar | insert, cmdline-insert (`ci`, after `<Insert>` in cmdline, rare), visual-exclusive (`ve`, only with `selection=exclusive`) | insert | Trigger ends (Restore) |
| `ESC[4 q` steady underline | replace, cmdline-replace, **operator-pending** (`d`, `c`, `y` … waiting for a motion) | replace | ambiguous; "no change" is safest, because operator-pending always follows normal mode where IME is already Off |
| `ESC[1 q` blinking block | `:terminal` job mode (0.11+) | – | Trigger ends (the user is typing into a shell) |
| `ESC[0 q` / reset at exit | after exit or suspend (some terminals) | – | Trigger ends |

Cmdline mode is indistinguishable from normal mode in both editors. For IME purposes, that is probably what you want, since ex commands are ASCII. Searching for Japanese text with `/` would need a manual IME toggle, and the map already says manual changes during a Trigger are left alone.

### Minimal Neovim config if a user has disabled it

If someone has `set guicursor=`, restoring the default is enough:

```lua
vim.o.guicursor = "n-v-c-sm:block,i-ci-ve:ver25,r-cr-o:hor20,t:block-blinkon500-blinkoff500-TermCursor"
```

Any value that keeps n=block and i=ver works. Blink settings change the parameter (block 1 vs 2, bar 5 vs 6), so bero should treat odd and even parameters of the same shape as equivalent, except that blinking block marks `:terminal` mode in the 0.11+ default.

## Latency: why the shape alone misses ~50 ms

Measured with a pty harness. The time runs from writing the key to reading the DECSCUSR bytes, 5 runs each, all values identical:

| Vim config | `i` → bar | `Esc` → block |
|---|---|---|
| snippet + `defaults.vim` (`ttimeout ttimeoutlen=100`) | 0-2 ms | **126 ms** |
| snippet only (no `ttimeout`, so `timeoutlen=1000` applies) | 0-1 ms | **1027 ms** |
| snippet + `defaults.vim`, next key `l` sent 20 ms after `Esc` | – | still about 120 ms after `Esc`. The `l` was already delivered before the block sequence. |

The editor cannot tell a lone `Esc` from the start of an escape sequence until `'ttimeoutlen'` expires (`:h 'ttimeout'`; Neovim's default `ttimeoutlen` is 50). So the "normal mode" signal arrives at least `ttimeoutlen` after the user pressed `Esc`, and a fast typist's next key gets ahead of it. Entering insert has no such delay.

Implication for the spec: either accept about 50 ms (nvim) or about 100 ms (Vim with `defaults.vim`) and call it out, or have bero act **predictively on the `Esc` key** that herdr forwards to a pane whose last seen shape is bar or underline, then use the shape stream to confirm, or to Restore if the shape does not turn to block. How a plugin sees forwarded keys is #45's subject.

## Open questions for other tickets

- Can a herdr plugin observe DECSCUSR bytes, or the parsed per-pane cursor shape? herdr 0.9.5 tracks it internally (`DecscusrTracker`, `HERDR/src/pane/cursor.rs`, and `TerminalCursorState.shape` in `HERDR/src/pane/terminal.rs` around L2578), but the socket API exposes only the cursor position (`HERDR/src/api/schema/panes.rs`). → #44
- Can bero know the pane's foreground program, so that a block cursor counts only while vim/nvim runs? → #44
- Can a plugin see keys forwarded to a pane, for the predictive `Esc`? → #45

## Method

- Vim: `:h termcap-cursor-shape`, `:h t_SH`, `defaults.vim` (local Vim 9.1 runtime). `src/term.c` and `src/po/ja.po` from Vim `master`. Distro vimrcs from Debian (local), Fedora and Arch packaging.
- Empirical: a Python pty harness that sets `TERM=xterm-256color` and a 24x80 window size, writes keys step by step, and logs every escape sequence written. A second harness timed key-to-DECSCUSR latency. Vim was run with `-Nu NONE`, with the default config under an empty `$HOME`, and with the snippet with and without `defaults.vim`.
- Neovim: `runtime/doc/options.txt` (`master`, `release-0.11`, `release-0.10`, `release-0.9`, `v0.5.0`, `v0.2.1`), `runtime/doc/tui.txt`, `runtime/doc/vim_diff.txt`, and `src/nvim/tui/tui.c` and `src/nvim/cursor_shape.c` from `master`. Downloading a Neovim binary for empirical capture was not done.
- herdr: source tarball for tag v0.9.5.
