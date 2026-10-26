# Detecting Claude Code slash-command input with no configuration (bero)

Research for #49 (parent map #43, bero). Pinned to **Claude Code 2.1.310** and **herdr 0.9.5** (socket protocol 22, Claude detection manifest `2026.10.11.1`). Sources are the Claude Code docs (hooks reference, status line), herdr's docs and its bundled API schema (`herdr api schema --json`), the Claude detection manifest that herdr downloads, and observations on a live screen. For those, Claude Code ran in a throwaway herdr (`scripts/throwaway-herdr`, headless 120x40 pane) with an isolated `HOME`, a dummy API key and `ANTHROPIC_BASE_URL=http://127.0.0.1:9`, so nothing could reach a model. Input was driven with `herdr pane send-text` / `send-keys`, and the screen read back with `herdr pane read --source visible` (text and ansi).

## Short answer

- **Yes, zero-config detection works.** Claude Code draws the slash-command menu as plain screen rows, which `pane.read --source visible` returns. Neither a hook nor a setting is needed.
- **Signal: the menu is visible.** It is not "the prompt text starts with `/`". A history recall or an Esc-dismissed menu leaves `❯ /…` in the prompt with no menu, and the user is then back to typing free text.
  - Menu open: one or more rows of the shape `^\s+(❯\s)?/\S+\s{2,}\S` (or the row `  No commands match "/…"`) directly above the prompt box's top rule `─────`, and the prompt line below that rule starts with `❯ /` (U+276F, then U+00A0).
- **The span starts** when the `/` typed as the first character of an empty prompt opens the menu.
- **The span ends** when the menu disappears, which happens when:
  - Tab completes the command (it becomes `/compact ` plus a dim argument hint),
  - Enter runs the highlighted command,
  - Esc dismisses the menu (the text stays),
  - backspace deletes the `/`,
  - the text stops looking like a command name, e.g. `/tmp/x` once the second `/` is typed.
- **Latency:** in-herdr latency is well under the ~50 ms budget. Polling `pane.read` saw the menu 22–33 ms after `send_text "/"` and saw it gone 15–22 ms after backspace. herdr's push subscription `pane.output_matched` fires on the rising edge only, 32–110 ms after the key. It never fires on the falling edge.
- **Hooks are no fallback.** No Claude Code hook fires on keystrokes, while typing, or when the menu opens or closes. The earliest related hooks, `UserPromptSubmit` and `UserPromptExpansion`, fire at submit, after the span is over. The status line is not refreshed on typing either.
- **herdr does not track this itself.** Its Claude manifest has no slash-menu rule. With the menu open, the agent state stays `idle` (rule `live_prompt_box`). bero has to match the screen itself.
- **Confidence:** high that the signal exists and is fast enough on this version. Medium that it holds over time: it relies on Claude Code's undocumented TUI layout, which changes between releases (see Caveats).

## What the screen shows

Bottom of the visible screen, idle and empty prompt. The effort row sits directly above the top rule:

```
                                                                         ◐ medium · /effort
──────────────────────────────────────────────────────────────────────────────────────────
❯
──────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ auto mode on (shift+tab to cycle)
```

After typing `/`. The menu (at most 5 entries, the selected one marked `❯`) takes the effort row's place directly above the top rule:

```
  ❯ /add-dir                    Add a new working directory
    /auto-mode-setup            Teach auto mode about your environment, plus optional rule tweaks
    /autocompact                Set how full the context gets before auto-summarizing
    /background                 Send this session to the background and free the terminal
    /branch                     Create a branch of the current conversation at this point
──────────────────────────────────────────────────────────────────────────────────────────
❯ /
──────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ auto mode on (shift+tab to cycle)
```

In ANSI (dark theme), the selected row is `\e[38;5;153m` and the other rows `\e[38;5;246m`. The rules are `\e[38;5;244m`. The prompt line is `❯ /` followed by a reverse-video space (`\e[7m \e[0m`) that Claude Code draws as its cursor. Colours depend on the theme, so match on the text, not the SGR.

A long description wraps onto a continuation row indented to the description column (seen with `/clear`). A continuation row has no `/`, so a matcher that looks for at least one `/name` row in the block above the rule still works.

### Observed transitions

| Input (prompt empty at start) | Prompt line | Menu above the rule |
|---|---|---|
| `/` | `❯ /` | yes, full list |
| `/co` | `❯ /co` | yes, filtered (`/copy`, `/color`, …) |
| `/cozzzz` | `❯ /cozzzz` | `No commands match "/cozzzz"` |
| backspace back to `/co` | `❯ /co` | yes |
| backspace to empty | `❯` | no (effort row back) |
| `/co` then Esc | `❯ /co` | **no**. A second Esc after a pause did not clear the text. |
| …then type or delete a character | `❯ /c` | yes again (editing reopens it) |
| `/comp` then Tab | `❯ /compact  <optional custom summarization instructions>` | no, the command has been chosen |
| …then type `xyz` | `❯ /compact xyz` | no |
| `/hel` then Enter | (help dialog replaces the prompt box) | no |
| `hi /` | `❯ hi /` | no: only a leading `/` opens the menu |
| `/tmp/x` | `❯ /tmp/x` | no |
| Up (history recall of `/help`) | `❯ /help`, top rule reads `─── History 1/1 ───` | **no** |

So the span is "menu visible". Matching on the prompt text alone gives false positives (the history recall, the text left after Esc, `/compact xyz` while typing arguments).

## Reading it from a herdr plugin

- **Read:** `pane.read` with `source: "visible"` and `format: "text"`. Find the last two rows that are a horizontal rule (`^─{20,}` and `^─── History`) and take the prompt box between them. The menu is open when:
  - the prompt line starts with `❯` + whitespace + `/`, and
  - the contiguous non-blank rows directly above the upper rule contain a `^\s+(❯\s)?/\S+\s{2,}\S` row or a `^\s+No commands match ` row.

  Anchoring to the rows directly above the prompt box keeps transcript text that happens to list commands from matching. herdr's own manifest uses the same geometry, with regions named `prompt_box_body` and `last_non_empty_above_prompt_box`.
- **Rising edge (push):** herdr has a `pane.output_matched` subscription (`type`, `pane_id`, `source`, `match: {type: substring|regex, value}`). With the regex `(?m)^\s+❯\s/\S+` on `visible` it pushed one event each time the menu appeared, carrying `matched_line`. It did not fire again while the menu stayed open (moving from `/` to `/c` changed the rows, but no event came). Matching appears to be per line: a multi-line regex that spanned the menu and the prompt never matched. No event marks the match going away.
- **Falling edge:** the schema has a `pane_output_changed` event, but `events.subscribe` takes no subscription for it (`Subscription` lists only `pane.output_matched` for output). So, while a span is open, bero has to poll `pane.read` (one request per connection, see `docs/herdr.md`). Polling every 10–20 ms during a span found the close within 15–22 ms. Another option is a second `output_matched` subscription on something that shows only when the menu is gone. The effort row (`· /effort`) is one candidate, but it is not always there (a `Ctrl+Y to paste deleted text` hint took its place once), so it is weaker.
- **Only for Claude panes:** gate on `pane.get` → `agent == "claude"` (herdr's manifest detection fills this in with no integration installed), so other TUIs that draw `❯ /` never trigger.

Timings measured (5 trials each, local socket, polling every 5 ms):

| | after `send_text "/"` | after backspace |
|---|---|---|
| menu state seen by polling `pane.read` | 22–33 ms | 15–22 ms |
| `pane.output_matched` event | 32–110 ms (first trial slowest) | (no event) |

These cover only key → Claude Code redraw → herdr. The path to the Windows IME Agent is extra (other #43 tickets).

## Why hooks and settings cannot replace this

From the Claude Code hooks reference (`https://code.claude.com/docs/en/hooks`), the hook events are SessionStart, Setup, UserPromptSubmit, UserPromptExpansion, PreToolUse, PermissionRequest, PermissionDenied, PostToolUse, PostToolUseFailure, PostToolBatch, Notification, MessageDisplay, SubagentStart, SubagentStop, TaskCreated, TaskCompleted, Stop, StopFailure, TeammateIdle, InstructionsLoaded, ConfigChange, CwdChanged, DirectoryAdded, FileChanged, WorktreeCreate, WorktreeRemove, PreCompact, PostCompact, PreModelSwitch, PostModelSwitch, Elicitation, ElicitationResult and SessionEnd. None of them reacts to editing the prompt.

- `UserPromptSubmit`: "Runs when a prompt is submitted, before Claude processes it." For a skill or custom command it receives the line as typed (`/deploy staging`). It fires at submit, which is the *end* of the span at the earliest.
- `UserPromptExpansion`: "Runs when a user-typed command expands into a prompt before reaching Claude", with `expansion_type: "slash_command"`, `command_name`, `command_args`. It is also submit-time, and the docs describe it only for prompt-type commands (skills, custom commands, MCP prompts). They do not say it fires for built-ins such as `/clear`.
- Status line (`https://code.claude.com/docs/en/statusline`, "When the status line updates"): it runs on session start, a new assistant message, `/compact`, a permission-mode change, a vim-mode toggle, a `command` change, `refreshInterval`, and rate-limit/prompt-cache expiry. Updates are debounced at 300 ms. The input JSON carries no prompt text and no menu state, and the status line "temporarily hides during certain UI interactions".

So app-side config could at best mark the end of a span (submit), never the start, and only for some commands. The screen is the only usable source for the start. herdr's own Claude integration (`herdr integration install claude`) installs only a `SessionStart` hook that reports session identity. herdr's docs say "Claude Code state comes from Herdr's screen manifest detection" (`integrations.mdx`, herdr v0.9.5).

## Caveats

- **Undocumented UI.** The glyphs (`❯` U+276F + U+00A0, `─` rules), the menu's place (above the prompt box, in the effort row's slot) and the row shape are not a public contract. herdr's Claude manifest has dated rules for exactly this reason ("Braille covers <= 2.1.227; half-circles are the 2.1.228 busy spinner"). bero should keep its matcher in data, versioned like herdr's manifest, and fail closed: when the prompt box is not recognised, no Trigger.
- **Not observed:**
  - the menu while Claude is working (sending a prompt was out of scope),
  - Claude Code's vim editor mode (`/` in NORMAL mode),
  - a light theme, very narrow panes, and multi-line prompts,
  - a client-attached herdr. Panes were headless, but `pane.read` returns the server-side screen either way.
- **The IME eats `/` first.** With a Japanese IME On, the `/` key often goes into composition (MS-IME and Google IME default to `・`), so Claude Code never receives `/` and the menu never opens. bero can only react after a `/` reaches Claude Code, so this Trigger helps mainly when the IME passes `/` through directly or the user typed it half-width. Unverified here (Linux only). It belongs to the Windows IME side of #43.
- **Esc semantics.** After Esc the text `/co` stays but the menu is gone. With the "menu visible" signal the span ends there, and typing again reopens the menu and restarts it. That matches "cancelled" in #49, but note that editing restarts the Trigger.
- **Typing the arguments.** After Tab or a space completes a command, the user may keep typing arguments (`/compact xyz`) with the IME still Off, because the Trigger ended and Restore put back the Saved State. If the spec wants the span to cover the arguments, the signal changes to "prompt starts with `/<known command> `" and picks up the history-recall false positive. That is a spec decision, not a detection limit.
- **Polling cost.** Detection needs a cheap rising-edge watch on the focused pane only (`output_matched`, or a slow poll), plus fast polling only while a span is open.

## Sources

- Claude Code hooks reference: https://code.claude.com/docs/en/hooks (event table; UserPromptSubmit and UserPromptExpansion sections)
- Claude Code status line: https://code.claude.com/docs/en/statusline ("When the status line updates")
- herdr docs index: https://herdr.dev/llms.txt. Agents page (`agents.mdx`) and Integrations page (`integrations.mdx`) at v0.9.5: `https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/{agents,integrations}.mdx`
- herdr Claude detection manifest `2026.10.11.1`, downloaded by herdr to `$XDG_STATE_HOME/herdr/agent-detection/remote/claude.toml`. Rules `live_prompt_box` (region `prompt_box_body`) and `background_agents_working` (region `last_non_empty_above_prompt_box`). No slash-menu rule.
- `herdr api schema --json` (0.9.5): `request.$defs.Subscription` (`pane.output_matched`), `OutputMatch`, `ReadSource`, `event.$defs.EventData` (`pane_output_changed`)
- `herdr agent explain w1:p1` with the menu open: `state: idle`, `rule: live_prompt_box`, `evidence: "❯\u{a0}/co\n"`
- Live observations: Claude Code 2.1.310 in a throwaway herdr 0.9.5, as described at the top
