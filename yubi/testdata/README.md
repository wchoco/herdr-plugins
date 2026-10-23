# yubi test data

## `herdr-<version>/`: real herdr captures

Each fixture `<name>` has three files, captured from herdr 0.9.5 in a single-pane tab of 120x40:

| file | command |
|---|---|
| `<name>.txt` | `herdr pane read <pane> --source visible` |
| `<name>.ansi` | `herdr pane read <pane> --source visible --format ansi` |
| `<name>.layout.json` | `herdr pane layout --pane <pane>` |

The screen content comes from `scripts/<name>.sh`, sourced by a zsh with the fixed prompt in `scripts/zshrc`:

| fixture | content |
|---|---|
| `shell` | prompts, VCS status output with colours and tabs, a long listing, written trailing spaces, blank rows |
| `cjk` | CJK and emoji wide glyphs mixed with ASCII, truecolor and 256-colour SGR, reverse video |
| `wrap` | a soft-wrapped ASCII URL and a soft-wrapped run of wide glyphs |
| `links` | Hint Mode input: coloured URLs (one printed three times), absolute, relative and home paths, a path between wide glyphs, a row with no Match |
| `joined` | a soft wrap that falls on a space, written trailing spaces on consecutive rows |

Re-capture all fixtures whenever `min_herdr_version` is raised, into a new `herdr-<version>/` directory.

### Capturing

Use a throwaway herdr server so your own session is never touched. In one terminal:

```sh
d=$(mktemp -d)
export XDG_CONFIG_HOME=$d/cfg XDG_STATE_HOME=$d/state XDG_RUNTIME_DIR=$d/run XDG_DATA_HOME=$d/data
unset HERDR_SOCKET_PATH HERDR_PANE_ID HERDR_TAB_ID HERDR_WORKSPACE_ID HERDR_ENV
herdr server
```

In a second terminal with the same environment:

```sh
yubi/testdata/herdr-0.9.5/scripts/capture.sh shell cjk wrap links joined
herdr server stop
```

A headless server sizes new panes 120x40. `capture.sh` creates one workspace per fixture, waits for the script to finish, reads the pane and closes the workspace.

### Socket captures: `ndjson/`

Each `ndjson/<name>.ndjson` holds one request line and herdr's response line, replayed by the adapter tests' fake socket. They were taken from a pane showing the `cjk` fixture, so the `pane-read-*` texts equal `cjk.txt` and `cjk.ansi`. With the throwaway server running and such a pane open:

```sh
python3 yubi/testdata/herdr-0.9.5/scripts/capture-ndjson.py <pane-id>
```

The re-launch captures (`pane-get*`, `plugin-pane-close*`) need any pane and an open yubi overlay (`herdr plugin link yubi`, then `herdr plugin action invoke wchoco.yubi.line`); the script closes the overlay:

```sh
python3 yubi/testdata/herdr-0.9.5/scripts/capture-relaunch.py <pane-id> <overlay-pane-id>
```

### Event captures: `ndjson/events-*`

Each `ndjson/events-<scenario>.ndjson` holds yubi's `events.subscribe` request line, herdr's response line and every event line pushed after one session change; `events-<scenario>.ids.json` names that scenario's workspace, tab, Target Pane, sibling and overlay. The overlay is a stand-in plugin (`scripts/probe-plugin`) whose pane only sleeps. With the throwaway server running:

```sh
herdr plugin link yubi/testdata/herdr-0.9.5/scripts/probe-plugin
python3 yubi/testdata/herdr-0.9.5/scripts/capture-events.py
```

`events-subscribe-rejected.ndjson` is a subscription naming a pane that does not exist (herdr answers `pane_not_found` and closes the connection). herdr refuses to move a pane out of a zoomed tab, so `target-moved` un-zooms the tab before moving the Target.

## `golden/`: render goldens

Text dumps of rendered frames: the cell grid, then a style map with one letter per cell. Regenerate them after an intended rendering change with:

```sh
go test ./yubi/... -update
```

and review the diff before committing.
