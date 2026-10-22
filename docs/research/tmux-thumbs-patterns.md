# tmux-thumbs patterns and hint assignment

Research for issue #7 (parent: #3, yubi). It is reference material for yubi's
Hint Mode: which patterns the existing tools match, how they assign Hints, how
they handle duplicate Matches, and which config knobs they expose.

Sources (read at these commits on 2026-10-22):

- fcsonline/tmux-thumbs @ `ae91d5f7c0d989933e86409833c46a1eca521b6a`:
  `README.md`, `src/state.rs`, `src/alphabets.rs`, `src/view.rs`, `src/main.rs`, `src/swapper.rs`
- Morantron/tmux-fingers @ `fc3c750b8d73ac3e29675aae3b3ac6a00a0718f1`:
  `README.md`, `src/fingers/constants.cr`, `src/fingers/hinter.cr`, `src/huffman.cr`,
  `src/fingers/options.cr`, `src/fingers/commands/load_config.cr`, `src/fingers/commands/start.cr`,
  `spec/lib/huffman_spec.cr`
- wezterm/wezterm @ `main`: `wezterm-gui/src/overlay/quickselect.rs`, `config/src/config.rs`,
  `mux/src/pane.rs`, `docs/config/lua/config/quick_select_alphabet.md`, https://wezterm.org/quickselect.html
- kovidgoyal/kitty @ `master`: `kittens/hints/marks.go`, `kittens/hints/hints_generator.go`,
  `kittens/hints/main.go`, https://sw.kovidgoyal.net/kitty/kittens/hints/

## Summary

| | tmux-thumbs | tmux-fingers | wezterm quick-select | kitty hints |
|---|---|---|---|---|
| Built-in patterns | 15 regexes + 1 exclude | 14 named regexes | 14 regexes (thumbs set minus `diff_summary`) | one `--type` per run |
| Default alphabet | `qwerty` = `asdfqwerzxcvjklmiuopghtybn` | same, minus `c i m q n` | same string as thumbs `qwerty` | `0123456789abcdefghijklmnopqrstuvwxyz` |
| Hint scheme | prefix-free, 1-2 chars (steal from the end of the alphabet) | prefix-free, n-ary Huffman tree | same algorithm as thumbs | breadth-first base-N; prefix-free only with `--prefix-free` |
| Shortest Hints go to | top of screen (bottom with `--reverse`) | bottom of screen | bottom-right | bottom (`--ascending` flips) |
| Duplicate Matches | separate Hints; `--unique` shares one | share one Hint by default; jump mode does not | always share one Hint | separate Hints |
| Uppercase | last char uppercase runs `upcase-command` (copy and paste by default) | Shift runs `shift-action` (`:paste:`) | copy and paste | n/a (`--program`) |

## tmux-thumbs

### Built-in patterns

From `PATTERNS` in `src/state.rs` (lines 7-26), in priority order:

| Name | Regex | Copied part |
|---|---|---|
| `markdown_url` | `\[[^]]*\]\(([^)]+)\)` | group 1 (the URL) |
| `url` | `(?P<match>(https?://\|git@\|git://\|ssh://\|ftp://\|file:///)[^ ]+)` | `match` |
| `diff_summary` | `diff --git a/([.\w\-@~\[\]]+?/[.\w\-@\[\]]++) b/([.\w\-@~\[\]]+?/[.\w\-@\[\]]++)` | groups 1 and 2, as two Matches |
| `diff_a` | `--- a/([^ ]+)` | group 1 |
| `diff_b` | `\+\+\+ b/([^ ]+)` | group 1 |
| `docker` | `sha256:([0-9a-f]{64})` | group 1 |
| `path` | `(?P<match>([.\w\-@$~\[\]]+)?(/[.\w\-@$\[\]]+)+)` | `match` |
| `color` | `#[0-9a-fA-F]{6}` | whole |
| `uid` | `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}` | whole |
| `ipfs` | `Qm[0-9a-zA-Z]{44}` | whole |
| `sha` | `[0-9a-f]{7,40}` | whole |
| `ip` | `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}` | whole |
| `ipv6` | `[A-f0-9:]+:+[A-f0-9:]+[%\w\d]+` | whole |
| `address` | `0x[0-9a-fA-F]+` | whole |
| `number` | `[0-9]{4,}` | whole |

There is one exclude pattern, `bash`: `[[:cntrl:]]\[([0-9]{1,2};)?([0-9]{1,2})?m`. It
matches SGR color escapes. They are consumed but never get a Hint, so an escape
never splits a path (`src/state.rs:5`, `:129-140`).

The README also lists "Docker images" and "kubernetes resources", but no pattern
for them exists in `src/state.rs` at this commit. The README list is out of date.

Matching algorithm (`src/state.rs:91-151`):

- Priority is exclude patterns, then custom (`--regexp`) patterns, then built-ins
  (`all_patterns = [exclude, custom, builtin]`).
- Each line is scanned left to right. Every pattern is run on the rest of the
  line, and the match with the lowest start index wins. On a tie the
  earlier pattern in the list wins (`min_by` keeps the first minimum). The scan
  then continues after the end of the winning match, so Matches never overlap.
- Capture rules: a named group `match` is copied if present. Otherwise, if the
  regex has groups, every group that took part becomes a separate Match.
  Otherwise the whole match is copied.
- Input is `tmux capture-pane -J -p`, so wrapped lines are joined first
  (`src/swapper.rs:218`).

### Alphabets

There are 22 named alphabets (`src/alphabets.rs:3-26`). Custom alphabet strings
are not accepted (`get_alphabet` panics on unknown names).

`numeric` `1234567890`, `abcd` `abcd`, `qwerty` `asdfqwerzxcvjklmiuopghtybn` (default),
`qwerty-homerow` `asdfjklgh`, `qwerty-left-hand` `asdfqwerzcxv`, `qwerty-right-hand` `jkluiopmyhn`,
`azerty` `qsdfazerwxcvjklmuiopghtybn`, `azerty-homerow` `qsdfjkmgh`, `azerty-left-hand` `qsdfazerwxcv`,
`azerty-right-hand` `jklmuiophyn`, `qwertz` `asdfqweryxcvjkluiopmghtzbn`, `qwertz-homerow` `asdfghjkl`,
`qwertz-left-hand` `asdfqweryxcv`, `qwertz-right-hand` `jkluiopmhzn`, `dvorak` `aoeuqjkxpyhtnsgcrlmwvzfidb`,
`dvorak-homerow` `aoeuhtnsid`, `dvorak-left-hand` `aoeupqjkyix`, `dvorak-right-hand` `htnsgcrlmwvz`,
`colemak` `arstqwfpzxcvneioluymdhgjbk`, `colemak-homerow` `arstneiodh`, `colemak-left-hand` `arstqwfpzxcv`,
`colemak-right-hand` `neioluymjhk`.

Each alphabet lists the easiest keys first. wezterm's docs describe the
ordering: left four fingers on the home, top, then bottom row, then the right
four fingers in the same order, then the hard-to-reach middle keys
(`quick_select_alphabet.md`).

### Hint generation (`Alphabet::hints`, `src/alphabets.rs:37-64`)

- Start with every letter as a one-character Hint.
- While there are fewer Hints than Matches, pop the last remaining single
  letter and replace it with `letter + each alphabet letter` (taking only as many
  as needed). The new two-letter Hints go before the earlier expansions.
- Result: single letters first, then two-letter Hints. The set is prefix-free
  because a letter that becomes a prefix is no longer a Hint itself. Tests:
  `abcd` with 6 Matches gives `a b c da db dc`, with 8 gives `a b ca cb da db dc dd`.
- The maximum is `len(alphabet)^2` Hints (676 for `qwerty`). Matches past that
  get no Hint.

### Hint ordering and `--reverse` (`src/state.rs:153-187`)

- Matches are collected top to bottom and left to right. Hints are handed out in
  that order, so by default the **top** Match gets `a` (test `match_reverse`: the
  first of three IPs gets `a`, the last gets `c`).
- `@thumbs-reverse enabled` hands them out from the bottom, so the shortest Hints
  are nearest the prompt and cursor. The README says it is "useful to get shorter
  hints closer to the cursor".

### Duplicate Matches (`@thumbs-unique`)

- Default (disabled): every occurrence gets its own Hint.
- Enabled: a `HashMap<text, hint>` reuses the first Hint given to the same text
  (test `match_unique`: both `127.0.0.1` get `a`). The Hint list is still built
  for the total Match count, not the unique count, so Hints can be longer than
  they need to be.
- Selecting a Hint copies `mat.text`, so for the copy result it does not matter
  which occurrence was hit.

### Input handling (`src/view.rs:186-290`)

- Typed characters are lowercased and appended. The first Match whose Hint equals
  the buffer is picked. Hints that start with the buffer stay visible, the rest
  are hidden (`render`, `hint.starts_with(typed_hint)`).
- Uppercase: if the **final** typed character was uppercase
  (`key != lower_key`), output is flagged `%U=true` and `@thumbs-upcase-command`
  runs (default: set-buffer and paste-buffer, so copy and paste).
- If no Match is found once the buffer is as long as the longest Hint, thumbs
  exits.
- Arrow keys move a selection cursor and `Enter` picks it. `Space` turns on multi
  select, and a second `Space` finishes it (`@thumbs-multi-command`).
  `Backspace` deletes a typed char. `Esc` exits.

### Config knobs (README "Configuration", CLI flags in `src/main.rs`)

| tmux option | Default | Meaning |
|---|---|---|
| `@thumbs-key` | `space` | Trigger key after the prefix |
| `@thumbs-alphabet` | `qwerty` | Named alphabet |
| `@thumbs-reverse` | disabled | Assign Hints from the bottom |
| `@thumbs-unique` | disabled | Same text gets the same Hint |
| `@thumbs-position` | `left` | `left`, `right`, `off_left`, `off_right` (Hint on or next to the Match) |
| `@thumbs-regexp-N` | none | Extra regexes. They take priority over built-ins. No way to turn built-ins off |
| `@thumbs-command` | `tmux set-buffer -- {} && tmux display-message "Copied {}"` | Lowercase pick |
| `@thumbs-upcase-command` | set-buffer, paste-buffer, display-message | Uppercase pick |
| `@thumbs-multi-command` | set-buffer, paste-buffer, send space | Multi select |
| `@thumbs-fg-color` / `bg-color` | `green` / `black` | Match text |
| `@thumbs-hint-fg-color` / `hint-bg-color` | `yellow` / `black` | Hint |
| `@thumbs-select-fg-color` / `select-bg-color` | `blue` / `black` | Arrow selection |
| `@thumbs-multi-fg-color` / `multi-bg-color` | `yellow` / `black` | Multi-selected items |
| `@thumbs-contrast` | `0` | Draw the Hint as `[a]` |
| `@thumbs-osc52` | `0` | Also emit OSC 52 for the system clipboard |

Colors are the 8 named ANSI colors plus `default`, or `#RRGGBB`.

## tmux-fingers (the original; thumbs is a Rust port of it)

### Built-in patterns (`BUILTIN_PATTERNS`, `src/fingers/constants.cr`)

| Name | Regex (abridged) |
|---|---|
| `ip` | `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}` |
| `uuid` | `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}` |
| `sha` | `[0-9a-f]{7,128}` |
| `digit` | `[0-9]{4,}` |
| `url` | `((https?://\|git@\|git://\|ssh://\|ftp://\|file:///)[^\s()"']+)` |
| `path` | `(([.\w\-~\$@]+)?(/[.\w\-@]+)+/?)` |
| `hex` | `(0x[0-9a-fA-F]+)` |
| `kubernetes` | `(pod\|service\|deployment.apps\|configmap\|...)` followed by `[[:alnum:]_#$%&+=/@-]+` (about 60 resource kinds) |
| `kubernetes-pod` | `[a-z][a-z0-9-]*[a-z0-9]-[bcdfghjklmnpqrstvwxz2456789]{5,10}-[bcdfghjklmnpqrstvwxz2456789]{5}` |
| `git-status` | `(modified\|deleted\|deleted by us\|new file): +(?<match>.+)` |
| `git-status-branch` | `Your branch is up to date with '(?<match>.*)'.` |
| `diff` | `(---\|\+\+\+) [ab]/(?<match>.*)` |

How patterns combine: custom `@fingers-pattern-N` values are parsed first and
the enabled built-ins are appended after them (`load_config.cr`
`parse_tmux_conf`, `add_builtin_patterns`). All of them become one alternation
`(p1|p2|...)` that goes through `gsub` (`hinter.cr` `pattern`). Priority is therefore regex-alternation
order at the same start position. Only the named group `match` narrows the copied
text (`captured_text_for_match`).

Compared with thumbs, fingers adds kubernetes, git-status and git-status-branch.
It lacks markdown_url, color, ipfs, ipv6, docker and diff_summary. `url` stops at
whitespace, brackets and quotes rather than only at a space.

### Alphabet and Hint generation

- `@fingers-keyboard-layout` picks from the same 20 layouts as thumbs (no
  `numeric` or `abcd`). The letters `c i m q n` are always removed
  (`DISALLOWED_CHARS = /[cimqn]/`). `q` exits, and the others are left free for
  key bindings (`load_config.cr` binds `a`-`z` except those letters).
- Hints come from an n-ary Huffman tree (`src/huffman.cr`) with arity equal to the
  alphabet size. Leaves are prefix-free by construction, so Hints are
  variable-length and never prefixes of each other. The result is sorted by
  length and cached on disk per `(alphabet, n)`.
  - If `n <= len(alphabet)`, the alphabet itself is returned.
  - Example (`spec/lib/huffman_spec.cr`): alphabet `asdf`, n=5 gives
    `s d f aa as`. With n=50, Hints are 3 to 6 chars long. The weights `-i` skew
    the tree, so it is not a balanced minimum-length code. A balanced scheme
    would cap at 3 chars for 50 with 4 letters.
- Ordering: Hints are sorted shortest first and taken with `pop` (longest first)
  while lines are processed top to bottom. So the **bottom** Matches get the
  shortest Hints, with no option to reverse.
- A Match whose Hint would be longer than its text is skipped and the Hint goes
  back on the stack (`hinter.cr` `replace`).

### Duplicate Matches

- Default mode: `reuse_hints: mode != "jump"` (`start.cr:287`). Identical text gets
  one shared Hint, and the Hint count is computed from unique texts
  (`count_unique_matches`), so Hints stay short.
- Jump mode (`@fingers-jump-key`, default `J`) places the cursor on the Match, so
  every occurrence needs its own Hint and reuse is off. Capture is also not
  joined (`join: mode != "jump"`).

### Input and modifiers

- Lowercase `a`-`z`: `@fingers-main-action` (default `:copy:`).
- Shift (uppercase): `@fingers-shift-action` (default `:paste:`).
- Ctrl: `@fingers-ctrl-action` (default `:open:`). Alt: `@fingers-alt-action` (no default).
- `Tab`: toggle multi mode (results are joined with a space). `Space`: fzf.
  `?`: help. `q`, `Esc`, `C-c`: exit.
- Typing narrows the display: Hints that do not start with the input are drawn
  unhighlighted.

### Config knobs (README "Configuration", `src/fingers/options.cr`)

`@fingers-key` (`F`), `@fingers-jump-key` (`J`), `@fingers-pattern-N` (custom
regexes, `(?<match>...)` supported, also usable by name with `--patterns`),
`@fingers-enabled-builtin-patterns` (`all`, or a comma list of names),
`@fingers-main-action`/`ctrl`/`alt`/`shift-action` (`:copy:`, `:paste:`, `:open:`, or
a shell command that gets the Match on stdin), `@fingers-hint-style`
(`fg=green,bold`), `@fingers-highlight-style` (`fg=yellow`),
`@fingers-selected-hint-style` (`fg=blue,bold`), `@fingers-selected-highlight-style`
(`fg=blue`), `@fingers-backdrop-style` (non-matched text, default none),
`@fingers-hint-position` (`left`/`right`), `@fingers-keyboard-layout` (`qwerty`),
`@fingers-show-copied-notification`, `@fingers-use-system-clipboard` (on),
`@fingers-enable-bindings` (on). CLI: `start <pane> --mode jump --patterns a,b --main-action ...`.

## Comparison: wezterm quick-select

- Patterns (`quickselect.rs:26-55`): the thumbs list without `diff_summary`, using
  `\S+` instead of `[^ ]+`. The `url` scheme is `file://`, not `file:///`.
  Config: `quick_select_patterns` (added to the defaults),
  `disable_default_quick_select_patterns`, `quick_select_remove_styling` (nightly),
  and `quick_select_alphabet`. The `QuickSelectArgs` key assignment can override
  patterns and the alphabet per binding.
- Labels (`compute_labels_for_alphabet_impl`): the same "steal from the end of the
  alphabet" algorithm as thumbs, so labels are 1-2 chars. Default alphabet
  `asdfqwerzxcvjklmiuopghtybn`, any custom string allowed.
- Ordering: results are walked in reverse, "so that we assign eg: `a` to the
  bottom-right-most result first".
- Duplicates: every result has a `match_id`, "an identifier that can be used to
  group results that have the same textual content" (`mux/src/pane.rs`). Labels
  are computed for unique `match_id`s and shared by identical text. This is always
  on.
- Matches past the label capacity are skipped, not errored.
- An uppercase label copies and pastes. Lowercase copies. `Esc` cancels.

## Comparison: kitty hints kitten

- No combined pattern set. One `--type` per invocation: `url` (default), `path`,
  `line`, `hash`, `word`, `ip`, `linenum`, `hyperlink`, or `regex` (custom
  `--regex`, Perl syntax). `--minimum-match-length` defaults to 3.
- Alphabet: default `0123456789abcdefghijklmnopqrstuvwxyz`
  (`DEFAULT_HINT_ALPHABET`, `marks.go:32`), set with `--alphabet`.
- Encoding (`hints_generator.go`): `encode_hint(index)` walks the alphabet's prefix
  tree breadth-first (`a b c aa ab ...`). That is not prefix-free, so `--prefix-free`
  raises the start offset by `hints_to_skip(n, k) = (n-2)/(k-1)` so the next n Hints
  are prefix-free. `--hints-offset` (default 1) skips the first label.
- Ordering: index 0 goes to the **last** (bottom) match by default
  (`m.Index = largest_index - m.Index + offset`). `--ascending` flips this.
- Duplicates: no dedup in `marks.go`. Each occurrence gets its own index.
- Actions: `--program` (`-` paste, `@` clipboard, `*` primary selection, a command),
  `--multiple` with `--multiple-joiner`, plus Hint and text colors.

## Takeaways for yubi Hint Mode

- **Pattern set**: the thumbs/wezterm list is the common base (markdown_url, url,
  diff paths, docker sha256, path, hex color, uuid, ipfs, git sha, ipv4, ipv6, 0x
  address, 4+ digit number). Fingers adds kubernetes and git-status patterns.
  Both tmux tools support a named `match` capture group that narrows the copied
  text of a custom pattern.
- **Overlap resolution**: thumbs takes the leftmost match, then the first pattern
  in the list, with no overlaps. A single alternation (fingers, wezterm) gives the
  same result if the patterns are concatenated in priority order. Custom patterns
  go before built-ins in both thumbs and fingers.
- **Hint scheme**: every mature tool uses prefix-free Hints so a Hint can be
  picked without a confirm key. The thumbs/wezterm "steal from the end" algorithm
  is simple and gives at most 2 chars (k^2 capacity). Fingers' Huffman tree allows
  longer Hints but is more complex and not length-optimal.
- **Ordering**: the newer tools (fingers, wezterm, kitty) give the shortest Hints to
  the bottom of the screen, nearest the prompt. thumbs defaults to the top and needs
  `--reverse`.
- **Duplicates**: sharing a Hint for identical text is the default in fingers and
  wezterm and opt-in in thumbs. Compute the Hint count from unique texts (as
  fingers and wezterm do) so Hints stay short. Only position-based actions (jump)
  need per-occurrence Hints.
- **Config knobs seen everywhere**: custom regexes, enable or disable built-ins,
  alphabet (named layouts or a raw string), Hint position, colors for Hint, Match
  and backdrop, an uppercase or Shift action (copy and paste), and multi select.
