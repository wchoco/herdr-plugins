# yubi manual test checklist

Run this checklist before every `yubi/vX.Y.Z` tag, after `scripts/check.sh` passes with herdr and tmux installed (its `yubi/scripts/e2e.sh` already drives the re-launch and zoom cases through a private herdr). It covers what those checks cannot: real terminals, the clipboard, SSH, and how things look. Copy the checklist into the release PR and tick each item there. Note the terminal (and version) used for each section. A Japanese copy is in [manual-test.ja.md](manual-test.ja.md); change both together.

## Setup

1. Check out the commit to be tagged. Then build it and link it. `plugin link` does not run the manifest's build commands, so build first, and build again after every change:

   ```sh
   herdr plugin unlink wchoco.yubi 2>/dev/null; herdr plugin uninstall wchoco.yubi 2>/dev/null
   (cd yubi && go build -o yubi .)
   herdr plugin link ./yubi
   ```

2. Add the keybindings from the [README](../README.md#keybindings) to your herdr `config.toml`, then run `herdr server reload-config`.
3. Move any `config.toml` out of `$(herdr plugin config-dir wchoco.yubi)`, so the defaults apply. Put it back when you are done.
4. Create the test screen script once:

   ```sh
   cat > /tmp/yubi-screen.sh <<'EOF'
   clear
   printf '%s\n' \
     'see https://example.com/docs and https://example.com/docs again' \
     'log written to /tmp/yubi/build.log' \
     'commit 1a2b3c4d5e6f7a8b' \
     'first line   ' \
     'second line'
   echo "https://example.com/$(printf 'w%.0s' $(seq 1 300))"
   EOF
   ```

   Below, "the test screen" means a zsh or bash pane where you have just run `sh /tmp/yubi-screen.sh`. The last line is a URL long enough to soft-wrap over several rows. `first line` is printed with three trailing spaces. Your prompt may hold Matches of its own, such as `~/proj`.

"Check the clipboard" means: paste with your desktop's paste shortcut into a text field outside herdr, on the machine you are sitting at, and compare with the expected text.

## Version

- [ ] In the plugin root, `./yubi version` prints the manifest `version` followed by `+` and the short commit, with no `.dirty`.

## Keys: kitty-capable terminal

Use a terminal with the kitty keyboard protocol (kitty, Ghostty, WezTerm or foot), herdr running locally, on the test screen.

- [ ] `prefix+f`: Hints appear over both URLs (sharing one Hint), `/tmp/yubi/build.log` and `1a2b3c4d5e6f7a8b`. Everything else is faint and the status row reads `HINT`.
- [ ] Type the Hint of `/tmp/yubi/build.log`: the overlay closes at once (no Enter), herdr shows its "copied" toast, and the clipboard holds `/tmp/yubi/build.log`.
- [ ] `prefix+f`, type a letter that starts no Hint: nothing happens. Type the first letter of a two-letter Hint: other Hints disappear and the typed letter is faint. Backspace brings them back. Esc closes the overlay at once, with no visible lag, and nothing is copied.
- [ ] `prefix+y`, type the Line Number of `second line`, Enter: the clipboard holds `second line`.
- [ ] `prefix+y`, select `first line` and `second line` (e.g. `N-M`) and press Enter: the clipboard holds `first line` and `second line` on two lines, with no trailing spaces and no trailing newline.
- [ ] `prefix+y`, select all rows of the wrapped URL and press **Ctrl-Enter**: the clipboard holds the URL as one line, with no line breaks.
- [ ] Same with **Ctrl-J**: same result.
- [ ] `prefix+y`, type `0` and press Enter: the status row shows `invalid input` and the overlay stays open. Type a number larger than the top Line Number and press Enter: `no line N`. Ctrl-U clears the input. Esc closes.
- [ ] `prefix+y` then Esc: the overlay closes at once.

## Keys: legacy terminal

Use a terminal without the kitty keyboard protocol (e.g. xterm, GNOME Terminal, macOS Terminal.app), on the test screen.

- [ ] `prefix+y`, select the wrapped URL rows, press **Ctrl-J**: the clipboard holds the URL as one line.
- [ ] Same selection, press **Ctrl-Enter**: note what happens. A normal copy (lines kept, the overlay closes) is expected, because the terminal sends a plain Enter.
- [ ] `prefix+y`, select `second line`, press **Alt-Enter**: `second line` is pasted on the shell prompt and not run.
- [ ] `prefix+y` then Esc: the overlay closes after a short delay (about 150 ms). Nothing is copied.
- [ ] `prefix+y`, press Esc and then quickly `1`: the overlay may stay open (Esc and `1` combined into Alt-1, which is ignored). Then Esc closes it.

## Clipboard

- [ ] **Local**, OSC 52: done above. The "copied" toast showed every time.
- [ ] **Inside tmux**: run herdr inside tmux with `set -g set-clipboard on`. Copy with Hint Mode and check the clipboard.
- [ ] **Over SSH**: `ssh` to another machine with yubi linked there, run herdr there, copy with Hint Mode, and check the clipboard on your local machine.
- [ ] **With `--remote`**: install or link yubi on the server, add the keybindings to the herdr config on both machines, run `herdr --remote <ssh-target>`, copy with Line Mode, and check the clipboard on your local machine.
- [ ] **`copy_command`**: put this in yubi's `config.toml`:

  ```toml
  [clipboard]
  copy_command = ["sh", "-c", "cat > /tmp/yubi-copy.txt"]
  ```

  Copy a Line, then `cat /tmp/yubi-copy.txt` shows exactly that line. There is no "copied" toast.
- [ ] Change it to `["sh", "-c", "echo no display >&2; exit 4"]`. A copy shows `copy failed: sh: exit status 4: no display`, and any key closes the overlay. Remove the `config.toml` again.

## Paste Copy

Run these at a zsh or bash prompt, which turn on bracketed paste, on the test screen.

- [ ] `prefix+f`, type the Hint of `/tmp/yubi/build.log` with an **uppercase** first letter: the status row shows the `paste` marker after the first letter of a two-letter Hint (or the paste happens at once for a one-letter Hint). The path appears on the prompt, not run. The clipboard holds it too.
- [ ] `prefix+y`, select `first line` and `second line`, **Alt-Enter**: both lines appear in the shell's input buffer as one paste and are **not** run. Clear the buffer with Ctrl-U or Ctrl-C.
- [ ] `prefix+y`, same selection, **Alt-Ctrl-J**: `first line   second line` appears as one line, not run (the first line keeps its trailing spaces in a Joined Copy).
- [ ] On a kitty-capable terminal, the same with **Alt-Ctrl-Enter**: same result.

## Layouts

- [ ] **Split**: split the tab three ways (`herdr pane split --current --direction right`, then in the new pane `herdr pane split --current --direction down`), show the test screen in one pane and focus it. `prefix+f` and `prefix+y`: the Target Pane is redrawn exactly where it was. The other panes are faint at their places, with faint rules between panes, and nothing on screen jumps. In Line Mode only the Target Pane has Line Numbers; Hint Mode on the other panes is tested in [Neighbour Panes](#neighbour-panes).
- [ ] **Split, narrow**: in a narrow split pane, print a URL that is cut off at the right edge. `prefix+f`: its Hint is right-aligned to the last visible cell, and typing it copies the full URL.
- [ ] **Zoomed**: zoom a split pane (`herdr pane zoom --current --on`). `prefix+f` and `prefix+y`: the zoomed pane is redrawn full size exactly where it was, with no other panes or rules, and a copy works. After the overlay closes the pane is still zoomed. The same holds when it closes by pressing its binding again (`prefix+f` twice), and after `prefix+f`, `prefix+y` and Esc.
- [ ] **Single pane filling the tab**: the status row covers the last row and the rightmost columns are clipped (see the README's known limits). Copies still work.

## Neighbour Panes

Split the tab three ways as in **Split** above: a left pane, a top right pane and a bottom right pane. Show the test screen in the left pane and in the bottom right pane, run `ls -d /*` in the top right pane, then focus the bottom right pane. It is the Target Pane; the left pane is Neighbour Pane `n` and the top right pane is Neighbour Pane `b` (labels go from the end of the alphabet, top to bottom, then left to right).

- [ ] `prefix+f`: Hints appear in all three panes. The left pane's Hints start with `n` (e.g. `na`) and the top right pane's with `b`, drawn with the label included. The Target Pane's Hints are one or two letters and none starts with `n` or `b`.
- [ ] The test screen's URL has one Hint in the Target Pane and one `n` Hint in the left pane.
- [ ] Type `n`: only the left pane's Hints stay, with the `n` drawn faint. Backspace brings all Hints back. Type a left pane Hint in full, e.g. `na`: the overlay closes and the clipboard holds that Match from the left pane.
- [ ] `prefix+f`, type a `b` Hint: the clipboard holds that path from the top right pane.
- [ ] `prefix+f`, type an `n` Hint with an uppercase first letter (e.g. `Na`): the `paste` marker appears after `N`, then the text is pasted on the Target Pane's prompt (the bottom right pane), not run, and not in the left pane.
- [ ] Open the same layout three times: the labels and Hints are the same every time.
- [ ] **Short alphabet**: put `hint.alphabet = "asd"` in yubi's `config.toml`. `prefix+f`: the left pane gets the label `d`, the top right pane gets no Hints and is only drawn faint, and the Target Pane's Hints use only `a` and `s`. Remove the `config.toml` again.

## Re-launch and mode switch

- [ ] `prefix+f`, then `prefix+f` again: the overlay closes.
- [ ] `prefix+y`, then `prefix+y` again: the overlay closes.
- [ ] `prefix+f`, then `prefix+y`: the overlay switches to Line Mode (`LINE` in the status row) on the same Target Pane. A copy there copies from the original pane.
- [ ] After any of these, `prefix+f` on a different pane opens yubi on that pane.

## Stale exits

- [ ] **Target closed**: in the Target Pane run `sleep 5; exit`, then press `prefix+y` within 5 seconds. When the shell exits, the overlay closes by itself with no message.
- [ ] **Resize**: open the overlay and resize the terminal window: the overlay closes by itself with no message.
- [ ] **No false resize**: open the overlay and wait a few seconds, then press the same binding again: the overlay closes (it did not close by itself, and no `cannot read target` message appears).
- [ ] **Focus leaves**: open the overlay and use herdr's key to switch to another tab: the overlay is gone when you come back.
- [ ] **Un-zoom**: in a split tab, open the overlay and use herdr's zoom toggle key: the overlay closes by itself.

## Messages and config errors

- [ ] On an empty screen (`clear`), `prefix+f` shows `nothing to copy`. Any key closes it.
- [ ] On a screen with only plain words, `prefix+f` shows `no matches`. Any key closes it.
- [ ] Put `[hint]` and `alphabet = "aa"` in yubi's `config.toml`. `prefix+f` shows `config: hint.alphabet: "a" repeats`, nothing is selectable, and any key closes it. Remove the file: the next launch works again, with no reload.
