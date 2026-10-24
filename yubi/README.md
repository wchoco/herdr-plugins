# yubi

yubi is a [herdr](https://herdr.dev) plugin that copies text from a pane's visible screen to your clipboard, using only the keyboard. Press a key and yubi redraws the tab in place, so nothing on screen moves. It has two modes:

- **Hint Mode** puts a short lowercase **Hint** over every **Match** on the screen: URLs, paths, git shas, UUIDs, IPs, numbers and your own **Custom Patterns**. Type the Hint and that Match is copied.
- **Line Mode** gives every screen row a **Line Number**, counting up from 1 at the last non-blank row. Type numbers and ranges (`2,5-7`) and press Enter to copy those lines. Press Ctrl-Enter or Ctrl-J instead for a **Joined Copy**: the lines joined into one line, with no line breaks.

Both modes can also do a **Paste Copy**, which copies the text and then pastes it into the pane. By default yubi copies with OSC 52, so the text reaches the clipboard of the machine you are sitting at, whether herdr runs locally, over SSH or with `herdr --remote`.

The pane yubi copies from is the **Target Pane**: the pane that had focus when you pressed the key.

## Hint Mode

```
user@host:a/proj$ git push
remote: Create a pull request:
remote:   zttps://github.com/wchoco/herdr-plugins/pull/new/feat
To github.com:rchoco/herdr-plugins.git
see sttps://example.com/docs and sttps://example.com/docs again
nothing to hint on this row
user@host:a/proj$ go build e/...
w/yubi/internal/core/hint/hint.go:12:3: undefined: x
log written to qtmp/yubi/build.log and f/proj/out.txt
日本語 dvar/log/syslog 漢字
docs again: sttps://example.com/docs
user@host:a/proj$

HINT
```

Each Hint is drawn over the first cells of its Match, in bold black on yellow. The rest of the Match is underlined in its own colours, and all other text is faint. Here, typing `z` copies `https://github.com/wchoco/herdr-plugins/pull/new/feat` and closes the overlay. Typing `q` copies `/tmp/yubi/build.log`. A URL printed three times shares the one Hint `s`. The Matches nearest the bottom of the screen get the shortest Hints.

## Line Mode

```
7ser@host:~/proj$ cat notes.txt
6 本語のテキスト: wide glyphs
5ixed ASCII 漢字 and かな, then a URL https://example.com/路径
4ruecolor 色 and 256-colour
3moji 🎉 party
2 角 reverse
1ser@host:~/proj$

LINE  6-4,1
```

Each Line Number is drawn over the first cells of its row, in bold black on cyan, so the text does not shift. The rows you have selected so far are shown in reverse video after every key. Here they are rows 6, 5, 4 and 1. Enter copies the four lines, top to bottom and joined by newlines, and closes the overlay.

## Requirements

- herdr 0.9.5 or newer.
- Linux or macOS.
- A Go toolchain, version 1.26.10 or newer (the `go` line of this repo's `go.mod`). herdr builds yubi from source with `go build` when it installs the plugin.

## Install

```sh
herdr plugin install wchoco/herdr-plugins/yubi
```

To pin a release, pass its tag:

```sh
herdr plugin install wchoco/herdr-plugins/yubi --ref yubi/vX.Y.Z
```

## Keybindings

yubi has two actions, `wchoco.yubi.hint` and `wchoco.yubi.line`. Bind them in your herdr `config.toml` (not yubi's). `prefix+f` and `prefix+y` are not bound in herdr's defaults:

```toml
[[keys.command]]
key = "prefix+f"
type = "plugin_action"
command = "wchoco.yubi.hint"

[[keys.command]]
key = "prefix+y"
type = "plugin_action"
command = "wchoco.yubi.line"
```

Then run `herdr server reload-config`, or restart herdr.

## Usage

Focus the pane you want to copy from and press a yubi key. yubi opens a full-tab overlay that redraws the tab as it was. In a split tab, the Target Pane is drawn at its own position and the other panes are drawn faint at theirs, with faint rules between them. The other panes are for orientation only: only the Target Pane has Hints and Line Numbers. The bottom row of the overlay is the status row. It shows the mode, what you have typed, and errors.

When a copy succeeds, the overlay closes. With OSC 52, herdr shows its own "copied" toast.

### Hint Mode keys

| Key | Action |
|---|---|
| Hint letters | Type a Hint. The copy happens as soon as the Hint is complete, with no Enter. After the first letter, Hints that no longer fit are hidden, and the letters you typed are drawn faint. |
| Uppercase first letter | Paste Copy: copy the Match, then paste it into the Target Pane. A `paste` marker appears in the status row. The case of the second letter does not matter, so `Ab` and `AB` both paste. |
| Backspace | Remove the last typed letter. |
| Esc, Ctrl-C | Cancel and close. |
| Any other key | Ignored, including a letter that continues no Hint. |

Notes on Hints:

- Hints are one or two letters, and no Hint is the start of another.
- Identical Matches share one Hint.
- With the default 26-letter alphabet there are at most 676 Hints. Matches beyond that get no Hint.
- A Match that runs past the right edge of the Target Pane still gets a Hint, right-aligned to the last visible cell, and its full text is copied.

### Line Mode keys

| Key | Action |
|---|---|
| `0`-`9`, `-`, `,` | Edit the input. |
| Backspace | Delete one character. |
| Ctrl-U | Clear the input. |
| Enter | Copy the selected lines. |
| Ctrl-Enter, Ctrl-J | Joined Copy of the selected lines. |
| Alt-Enter | Paste Copy of the selected lines. |
| Alt-Ctrl-J, Alt-Ctrl-Enter | Paste Copy of the Joined Copy. |
| Esc, Ctrl-C | Cancel and close. |
| Any other key | Ignored. |

The input is a comma-separated list of Line Numbers and ranges, with no spaces, such as `3`, `3-7` (same as `7-3`) or `2,5-7,10`. Line Numbers start at 1 on the last non-blank row yubi displays and count upward. Blank rows above that row are numbered too.

On commit, the whole input is checked before anything is copied:

- Malformed input, such as `3-`, `,,` or `0`, shows `invalid input`. A number above the top row shows `no line N`. A selection of only blank rows shows `nothing to copy`. In each case nothing is copied and the overlay stays open, so you can fix the input. The error goes away on your next edit.
- Empty input does nothing.

What gets copied:

- **Copy**: the selected rows, top to bottom as on screen, each row once, joined by `\n`. Trailing whitespace is trimmed from each line. A blank row inside the selection becomes an empty line. There is no trailing newline.
- **Joined Copy**: the same rows in the same order, joined by `line.separator` (empty by default). Each row except the last keeps its trailing spaces, so a soft wrap that fell on a space does not glue two words together. Only the last row is trimmed.

The commit keys can be changed in the config. Alt on any commit key always does the Paste Copy for that key.

### Paste Copy

A Paste Copy first copies the text, then sends it to the Target Pane as typed input:

- yubi never presses Enter, so a pasted command is never run.
- herdr wraps the text in bracketed paste when the Target Pane has bracketed paste turned on, which most shells and editors do. A multi-line paste therefore stays a single paste.
- Control characters other than newline and tab are removed before pasting, so screen text cannot inject terminal sequences.
- If the copy fails, nothing is pasted and only the copy error is shown.
- If the copy works but the paste fails, the status row shows `copied; paste failed: <reason>`. The text is still on the clipboard. This message stays up until you press a key, even if the overlay is resized or the Target Pane goes away.

### Pressing a yubi key again

herdr handles its prefix and keybindings before the overlay sees them, so your yubi keys still work while the overlay is open:

- The same mode's key closes the overlay.
- The other mode's key switches to that mode on the same Target Pane.

### Messages

In these states nothing can be selected, and any key closes the overlay. Errors are shown in bold red; `no matches` and `nothing to copy` are not errors and are shown plain:

| Status row | Meaning |
|---|---|
| `no matches` | Hint Mode found no Match on the displayed rows. |
| `nothing to copy` | The screen is empty or blank. |
| `cannot read target: <reason>` | The Target Pane could not be read, e.g. `pane_not_found`. |
| `cannot read target: socket unreachable` | yubi could not reach herdr's socket. |
| `cannot watch session: <reason>` | herdr refused yubi's event subscription. |
| `config: ...` | The config file is invalid (see [Configuration](#configuration)). |
| `copy failed: <reason>` | The copy failed, e.g. the text is over the OSC 52 limit, or `copy_command` failed. |
| `copied; paste failed: <reason>` | The text was copied but could not be pasted. |

### When the overlay closes by itself

yubi never copies from an out-of-date screen. It closes silently, with no message, when:

- the Target Pane closes, exits or moves;
- the overlay pane moves;
- the tab is un-zoomed;
- focus moves to another pane, tab or workspace;
- the overlay is resized.

A pending `copied; paste failed` message is the only exception (see [Paste Copy](#paste-copy)).

## Configuration

yubi reads `config.toml` from its herdr plugin config dir. To find that dir:

```sh
herdr plugin config-dir wchoco.yubi
```

This command also creates the dir. The file is read fresh every time yubi starts, so changes take effect on the next launch, with no reload step. Without a file, yubi uses the defaults. The dir survives plugin updates and reinstalls.

Every setting is optional. This is the full schema with its defaults:

```toml
[hint]
alphabet = "asdfqwerzxcvjklmiuopghtybn"
disable  = []

# Any number of Custom Patterns, none by default:
# [[hint.patterns]]
# name  = "jira"
# regex = '\b[A-Z]+-\d+\b'

[line]
copy      = ["enter"]
joined    = ["ctrl-enter", "ctrl-j"]
separator = ""

[clipboard]
# copy_command = ["wl-copy"]   # unset: OSC 52
```

### `[hint]`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `alphabet` | string | `"asdfqwerzxcvjklmiuopghtybn"` | The letters Hints are made from, easiest first. Single-letter Hints are used up from the end of the alphabet first, to make room for two-letter ones. Rules: only lowercase `a`-`z`, no letter twice, at least 2 letters. With k letters there are at most k² Hints. |
| `disable` | array of strings | `[]` | Built-in Patterns to turn off, by name (see [Built-in Patterns](#built-in-patterns)). Each name must be a Built-in Pattern. You may disable them all. |
| `patterns` | array of tables | none | Custom Patterns, each with `name` and `regex`. See below. |

Each `[[hint.patterns]]` entry has:

- `name` (string, required, non-empty): must be unique among Custom Patterns. A Custom Pattern named like a Built-in Pattern does not replace it; use `disable` for that.
- `regex` (string, required, non-empty): a [Go RE2 regular expression](https://pkg.go.dev/regexp/syntax). RE2 has no lookahead or lookbehind. If the regex has a named group `match`, only that group is copied and hinted. Otherwise the whole match is. Use a TOML literal string (`'...'`) to avoid escaping backslashes.

The inline form `patterns = [{ name = "...", regex = '...' }]` works too.

How Matches are found:

- Each row is scanned left to right. The leftmost match wins, and a tie goes to the Pattern with the higher priority.
- Matches never overlap: the scan goes on after the winning match.
- Custom Patterns rank above every Built-in Pattern, in the order you wrote them.
- A Match never spans two rows.

### `[line]`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `copy` | array of strings | `["enter"]` | Keys that copy the selected lines. |
| `joined` | array of strings | `["ctrl-enter", "ctrl-j"]` | Keys that do a Joined Copy. |
| `separator` | string | `""` | Joins the lines of a Joined Copy, e.g. `" "`. |

A commit key name is `[ctrl-][shift-]<name>`, where `<name>` is `enter`, `tab`, `space` or one lowercase letter, e.g. `ctrl-enter`, `ctrl-s`, `shift-enter`. Rules:

- Lists must not be empty.
- `alt-` keys are not allowed, because Alt on a commit key is the Paste Copy.
- Line Mode's own input keys cannot be commit keys: `ctrl-u` and `ctrl-c`. Digits, `-`, `,`, Backspace and Esc cannot be written as key names at all.
- A key may not be in both `copy` and `joined`. Keys a terminal sends as the same bytes count as the same key: `ctrl-m` is `enter`, and `ctrl-i` is `tab`. If you set only one of the lists, it is checked against the default of the other.

Setting a list replaces the default; it does not add to it.

### `[clipboard]`

| Key | Type | Default | Meaning |
|---|---|---|---|
| `copy_command` | array of strings | unset (OSC 52) | A program that gets the copied text on its stdin **instead of** OSC 52, e.g. `["wl-copy"]`, `["pbcopy"]` or `["xclip", "-selection", "clipboard"]`. |

`copy_command` is an argv array, not a shell command line. Use `["sh", "-c", "..."]` if you need a shell. The string form `copy_command = "wl-copy"` is an error, and so are an empty array and an empty program name. Its stdout is discarded. If it exits non-zero, the status row shows `copy failed: <program>: exit status N: <first line of its stderr>`. If it runs longer than 5 seconds, it shows `copy failed: <program>: timed out after 5s`. It runs on the machine where the herdr server runs (see [Terminal notes](#terminal-notes)). yubi never looks for a clipboard tool by itself.

### Errors

The config is validated strictly. Any of these is an error:

- TOML syntax errors;
- unknown keys;
- wrong types;
- a Custom Pattern without `name` or `regex`, with a duplicate name, or with a regex that does not compile;
- an unknown `disable` name;
- a bad `alphabet`;
- a bad or conflicting commit key;
- a bad `copy_command`.

yubi shows the first error it finds, with its location, in the status row, and nothing can be selected. Press any key to close. yubi does not fall back to the defaults, so a mistake cannot go unnoticed. Array indexes count from 0. Examples:

```
config: line 3: expected '.' or '=', but got 'x' instead
config: hint.colour: unknown key
config: hint.alphabet: expected string, got integer
config: hint.alphabet: "a" repeats
config: hint.patterns[1].regex: missing closing )
config: hint.disable[0]: unknown Built-in Pattern "numbers"
config: line.joined[0]: alt- keys are reserved for Paste Copy
config: line.joined[1]: "enter" is also in line.copy
config: clipboard.copy_command: expected array, got string
```

## Built-in Patterns

In priority order, highest first. These are the names `hint.disable` takes. The regexes come from [tmux-thumbs](https://github.com/fcsonline/tmux-thumbs).

| Name | Matches | Copies |
|---|---|---|
| `markdown_url` | a Markdown link `[text](url)` | the URL only |
| `url` | text starting with `https://`, `http://`, `git@`, `git://`, `ssh://`, `ftp://` or `file:///`, up to the next space | the whole URL |
| `diff_a` | `--- a/<path>` in a diff header | the path after `a/` |
| `diff_b` | `+++ b/<path>` in a diff header | the path after `b/` |
| `docker` | `sha256:` followed by 64 hex digits | the 64 hex digits |
| `path` | a path with at least one `/`, such as `/tmp/x`, `./a/b`, `~/proj` or `src/main.go` | the path |
| `color` | a hex colour `#rrggbb` | the whole colour |
| `uuid` | a lowercase UUID (`8-4-4-4-12` hex) | the whole UUID |
| `sha` | 7 to 40 lowercase hex digits, such as a git sha | the digits |
| `ipv4` | four dot-separated groups of 1 to 3 digits | the address |
| `ipv6` | hex groups separated by colons | the address |
| `address` | `0x` followed by hex digits | the whole address |
| `number` | 4 or more digits | the digits |

Some of these patterns are loose, and these false positives are known:

- clock times such as `02:03:33` match as `ipv6`;
- the `0900` in a time zone offset `+0900` matches as `number`;
- a CIDR block such as `192.0.2.10/24` matches as one `path`, because `path` ranks above `ipv4`;
- `link/ether` (from `ip addr`) matches as `path`;
- an IPv6 prefix such as `2001:db8::1/64` splits into an `ipv6` Match and a `path` Match `/64`.

To silence a noisy pattern, add it to `hint.disable`. To catch these cases more precisely, add a Custom Pattern, which ranks above all Built-in Patterns.

## Terminal notes

- **Ctrl-Enter and Alt-Ctrl-Enter.** Your terminal can only tell these apart from Enter and Alt-Enter if it supports the [kitty keyboard protocol](https://sw.kovidgoyal.net/kitty/keyboard-protocol/) (e.g. kitty, Ghostty, WezTerm, foot). Elsewhere, Ctrl-Enter usually arrives as a plain Enter and does a normal copy. **Ctrl-J and Alt-Ctrl-J always work**, in every terminal. Alt-Enter also works without the protocol.
- **OSC 52.** yubi writes OSC 52 to its own pane. herdr forwards it to the client you are attached with, which writes your clipboard and shows a "copied" toast. Your outer terminal must allow OSC 52 clipboard writes; some terminals ask first or need it turned on. If herdr runs inside tmux, set `set -g set-clipboard on` in tmux.
- **`copy_command` over SSH and `--remote`.** `copy_command` runs on the machine where the herdr server runs. Over SSH or with `herdr --remote`, that is the server, not the machine you are sitting at, so `wl-copy`, `pbcopy` or `xclip` write the *server's* clipboard. OSC 52, the default, writes the clipboard where you are. Set `copy_command` only if your terminal cannot do OSC 52 and herdr runs on the same machine.
- **Esc lag.** If your terminal does not support the kitty keyboard protocol, herdr holds a lone Esc for up to about 150 ms (to tell it apart from an Alt key), so cancelling feels slightly slow. A key pressed in that window can combine with the Esc into an Alt key. With the kitty protocol there is no lag.
- **Intercepted keys.** herdr handles some keys before the overlay sees them: its prefix key (`ctrl+b` by default), the key after the prefix, and every direct (non-prefix) keybinding. If one of those collides with a yubi key, such as a commit key you bound, yubi never gets it. herdr also takes Ctrl-C to copy a mouse selection when one is visible.

## Known limits

- **Visible screen only.** yubi sees what is on the Target Pane's screen, not its scrollback.
- **Soft-wrapped lines.** herdr does not say where a line was soft-wrapped, so yubi works per screen row. A long line wrapped over three rows gets three Line Numbers, and a Match wrapped onto the next row is two separate Matches (or none). Use a Joined Copy in Line Mode to copy a wrapped URL or command as one line.
- **A Target Pane that fills its tab.** The overlay is a little smaller than the tab, because of herdr's frame around it. yubi draws the Target from the overlay's top-left corner and clips its rightmost columns and bottom rows. The status row covers the last row shown. Clipped rows get no Hints or Line Numbers. A Match cut off on the right still gets a Hint and is copied in full.
- **Pane borders.** Split layouts are placed exactly only with herdr's default border settings. With `pane_outer_borders = false`, the Target Pane can be drawn one cell off.
- **Other panes** are read once when the overlay opens, and drawn faint without Hints. A pane that cannot be read stays blank.
- **No mouse.** yubi is keyboard-only.
- **No theming.** Styles are fixed and use only your terminal's 16-colour palette and SGR attributes.
- **192 KiB limit.** OSC 52 copies are limited to 192 KiB, which is herdr's cap. A larger copy shows `copy failed: text is N KiB, over the OSC 52 limit of 192 KiB`.
- **No Windows.** Linux and macOS only.

## Reporting bugs

Include the output of `yubi version`, which prints `<version>+<commit>`, with `.dirty` appended for a modified build. The binary is in the plugin's install dir: the `plugin_root` that `herdr plugin list --plugin wchoco.yubi --json` prints. Run `./yubi version` there.

## Development

`go vet ./...` and `go test ./...` from the repo root need no running herdr. Test fixtures and how to re-capture them are described in [`testdata/README.md`](testdata/README.md). Run the [manual test checklist](docs/manual-test.md) before tagging a `yubi/vX.Y.Z` release.
