# Where herdr plugins keep their config

Research for wchoco/herdr-plugins#18 (map: #3, yubi). Target: herdr **0.9.5**.

Sources:

- the herdr 0.9.5 docs ([Plugins], [Configuration], [Socket API], [CLI
  reference]);
- the herdr source at tag `v0.9.5` (links below);
- 22 published plugins tagged `herdr-plugin` on GitHub (listed in section 5);
- probes against a local herdr 0.9.5.

The probes ran on a separate herdr server with its own `XDG_CONFIG_HOME`,
`XDG_STATE_HOME` and `HERDR_SOCKET_PATH`, and no `HERDR_*` variables inherited
from the live session. A throwaway plugin `probe.Cfg` had one action and one
pane. Each one wrote its `HERDR_*` environment and a listing of
`HERDR_PLUGIN_CONFIG_DIR` into the state dir. Facts marked **(probe)** were
seen there. The isolated instance was stopped and deleted afterwards.

## Answer in brief

- **Location.** herdr gives each plugin its own config dir:
  `<herdr config dir>/plugins/config/<encoded id>/`. The herdr config dir is
  `$XDG_CONFIG_HOME/herdr`, else `~/.config/herdr` on Linux **and macOS**, and
  `%APPDATA%\herdr` on Windows. A parallel **state** dir lives at
  `<herdr state dir>/plugins/<encoded id>/`, where the state dir is
  `$XDG_STATE_HOME/herdr`, else `~/.local/state/herdr`.
- **Id encoding.** ASCII lowercase letters, digits, `.`, `_` and `-` stay as
  they are. Every other byte becomes `%XX`. So `probe.Cfg` becomes
  `probe.%43fg` **(probe)**. A plain lowercase id such as `wchoco.yubi` keeps
  its name.
- **`herdr plugin config-dir <id>`** prints that path. It also **creates both
  the config and the state dir, even when no plugin with that id is
  installed** **(probe)**. So it is not read-only.
- **When the dirs are created.** herdr also creates them on `plugin link`,
  `plugin install` (online or offline), and before **every** action, event
  hook, startup hook and pane launch. If a user deletes the dir, the next run
  brings it back.
- **Lifetime.** herdr never deletes, validates or syncs the dir's contents.
  `unlink` and `uninstall` both left `config.toml` and the state files in place
  **(probe)**. `update` and reinstall build a new installation dir and leave
  the config alone (source).
- **Delivery to the plugin.** The path arrives as the env var
  **`HERDR_PLUGIN_CONFIG_DIR`**. Actions, split and overlay panes, and popups
  all get it **(probe)**. Event and startup hooks get it too (source).
  **`HERDR_PLUGIN_CONTEXT_JSON` has no config-dir field** **(probe)**.
- **No herdr-side settings mechanism.**
  - herdr's `config.toml` rejects a `[plugins]` table: "unknown config section
    [plugins]; ignoring section" **(probe)**.
  - The manifest has no settings field. An unknown table such as `[settings]`
    is silently ignored: no warning, and it does not show up in plugin info
    **(probe)**.
  - Each plugin picks its own file name and format. The docs say "the plugin
    owns the file format and lifecycle".
- **No reload signal.**
  - `herdr server reload-config` runs no plugin command **(probe)**. It only
    reloads herdr's own `config.toml`, and its diagnostics list no plugin
    files.
  - None of the events a plugin can subscribe to is about config.
  - Plugins that want live updates watch the file themselves (reviewr,
    herdr-radar).
- **Convention among published plugins.** Most read
  `$HERDR_PLUGIN_CONFIG_DIR/config.toml`. A few use `config.json`, `.env` or a
  plugin-named file. Most fall back to a path they work out for themselves
  when the env var is missing. A minority ignore the herdr dir and use
  `~/.config/<plugin>/`.

## 1. The config dir

### Path

- `plugin_config_dir(id)` is `managed_plugins_dir()/config/<component>`, and
  `managed_plugins_dir()` is `config_dir()/plugins` ([plugin_paths.rs
  L5-L7, L44-L48][paths]).
- `config_dir()` is `$XDG_CONFIG_HOME/herdr`, else the platform default
  ([config/io.rs L30-L35][io-dirs]):
  - Unix, including macOS: `$HOME/.config/herdr`. It is **not**
    `~/Library/Application Support`.
  - Windows: `%APPDATA%\herdr`, then `%USERPROFILE%\AppData\Roaming\herdr`,
    then `$HOME/.config/herdr`.
  - A debug build uses `herdr-dev` instead of `herdr`.
- `HERDR_CONFIG_PATH` moves herdr's own `config.toml`, but **not** the plugin
  dirs ([config/io.rs L195-L200][io-path]). The dirs are not session-scoped
  either: a named session uses the same plugin config dir, just as plugins are
  "global to the current user" ([Plugins], "Install and link").
- State: `state_dir()/plugins/<component>`, where `state_dir()` is
  `$XDG_STATE_HOME/herdr`, else `~/.local/state/herdr` on Unix and
  `%LOCALAPPDATA%\herdr` on Windows ([plugin_paths.rs L50-L54][paths],
  [config/io.rs L37-L42][io-dirs]).
- `<component>` is `plugin_config_path_component(id)`:
  - Bytes in `[a-z0-9._-]` stay as they are. Every other byte becomes `%XX`.
  - A trailing `.` becomes `%2E`.
  - A Windows-reserved stem gets a `%` prefix.
  - A name over 120 characters is cut short and given a hash suffix.

  See [plugin_paths.rs L94-L121][paths].

### Creation

- `ensure_plugin_user_dirs` creates the config dir and the state dir
  ([plugin_paths.rs L56-L75][paths]). These call it:
  - `herdr plugin config-dir <id>`: it calls it and then prints the path
    ([cli/plugin.rs L91-L104][cli-config-dir]). It does not check that the
    plugin is installed. The probe ran it for an unregistered id, and both dirs
    appeared.
  - `plugin.link` and the offline link/install path ([plugins/mod.rs
    L79][link], [cli/plugin.rs L1143][cli-offline]).
  - Every runtime command spawn (actions, events, startup) ([runtime.rs
    L34][runtime]) and every plugin pane or popup launch ([panes.rs
    L258][panes]). If this fails, the launch fails with
    `plugin_user_dir_create_failed`.
- **Legacy seeding.** When the config dir does not exist yet, herdr looks for
  an older layout and copies the first one it finds into the new dir (a copy,
  not a move). There are two older layouts: `plugins/<raw id>/` and
  `plugins/<managed component>/` ([plugin_paths.rs L62-L92][paths]). The probe
  confirmed it: a `.env` placed in `plugins/probe.legacy/` showed up in
  `plugins/config/probe.legacy/`, and the original stayed where it was.
- Added in herdr **0.7.0**: "Added `herdr plugin config-dir <id>` and automatic
  plugin config/state directory creation so plugin setup docs can point users
  at a stable config path" ([CHANGELOG]).

### Survival

- From [Plugins], "Install and link": "Existing plugin config and state remain
  in place", and "Config and state are preserved."
- `plugin uninstall` is an unlink plus an installation-dir cleanup. The only
  `remove_dir_all` calls in the plugin code target managed installation dirs
  ([plugin_installations.rs L215-L220][installations]), never the config or
  state dir.
- `plugin update` and reinstall build into a new
  `plugins/github-installations/<id>/<pid>-<n>` dir and leave the old one in
  place until it is cleaned up ([plugin_paths.rs L22-L42][paths]).
- **(probe)**: after `plugin unlink probe.Cfg`, then re-linking, then
  `plugin uninstall probe.Cfg`, both `config/probe.%43fg/config.toml` and the
  state dir's files were still there. (The probe used a linked plugin. GitHub
  `update` was checked in the source only.)
- `herdr plugin list` and the `plugin install` output print the config path,
  for example `config: …/plugins/config/probe.%43fg` **(probe)**,
  [cli/plugin.rs L436-L438][cli-install].

## 2. How the plugin learns the path

- [Plugins], "Commands and environment": runtime commands get
  `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR`,
  `HERDR_PLUGIN_STATE_DIR` and `HERDR_PLUGIN_CONTEXT_JSON`, plus any available
  workspace, tab and pane ids. The env is built by `plugin_path_env`
  ([plugins/env.rs L15-L30][env]).
- **(probe)** `HERDR_PLUGIN_CONFIG_DIR` was set to the same absolute path in
  each of these:
  - an action run by `plugin action invoke`;
  - a `split` pane;
  - an `overlay` pane;
  - a `popup`. A popup has no `HERDR_PANE_ID`, but it still gets the config
    dir.
- The working directory is `HERDR_PLUGIN_ROOT` (the installed or linked
  checkout). **Don't keep config there**: GitHub installs are managed checkouts
  that get replaced on update ([Plugins]).
- `HERDR_PLUGIN_CONTEXT_JSON` ([schema/plugins.rs
  L364-L395][context]) carries only invocation data:
  - workspace, tab and focused-pane ids, labels and cwd;
  - worktree;
  - focused pane agent and status;
  - `selected_text`, `invocation_source`, `correlation_id`;
  - `clicked_url`, `link_handler_id`.

  It has **no config or state path** **(probe)**.
- Build commands get **no** runtime plugin env ([Plugins], "Build commands").
- **Outside herdr** (running the binary by hand, or in tests) the env var is
  unset. `herdr plugin config-dir <id>` gives the same path, but as noted it
  creates the dirs as a side effect.

## 3. Settings mechanisms in herdr itself: none

- **herdr `config.toml`.** The known top-level keys are `advanced`,
  `experimental`, `keys`, `onboarding`, `remote`, `server`, `session`,
  `terminal`, `theme`, `ui`, `update` and `worktrees` ([config/io.rs
  L7-L20][io-keys]). An unknown table gets a diagnostic and is dropped. The
  probe got this from `herdr config check` and `server reload-config`:
  `unknown config section [plugins]; ignoring section` (exit 1 /
  `"status":"partial"`).
- **Manifest.** `RawPluginManifest` has only these fields: `id`, `name`,
  `version`, `min_herdr_version`, `description`, `platforms`, `build`,
  `startup`, `actions`, `events`, `panes` and `link_handlers` ([manifest.rs
  L11-L34][manifest]). It is not `deny_unknown_fields`, so serde drops extra
  keys without a word. **(probe)** A `[settings]` table in the manifest linked
  without a warning and did not appear in the `plugin link` response.
- **Docs.** "Herdr creates those directories … but it does not validate, sync,
  or delete their contents. The plugin owns the file format and lifecycle"
  ([Plugins], "Commands and environment"). They suggest "user-editable config
  such as `.env` files" as the kind of thing to put there.
- The only config-like inputs herdr gives a plugin are its env vars and the
  argv in the manifest. A pane entrypoint's `command` is fixed. A
  `plugin.pane.open` request can set placement, size and `env` (see the
  overlay research), but that is a per-call override, not user config.

## 4. Reload signals: none

- `herdr server reload-config` (and the UI's "reload config") re-reads herdr's
  own config only ([Configuration], "Reload config"). **(probe)** After
  writing `config.toml` into the plugin dir and running `reload-config`, the
  plugin's command log still had just the one earlier action run. No plugin
  command was started.
- Startup hooks don't run again on config reload ([Plugins], "Startup hooks").
- None of the event types a plugin can subscribe to is about config. The full
  list is `workspace.*`, `tab.*`, `pane.*`, `layout.updated` and
  `worktree.*` ([Socket API], "Event subscriptions"). No config event exists in
  the schema.
- What follows for yubi: each run is a short-lived process (one action, then
  one overlay pane). If it reads its config **at the start of each run**,
  every run picks up the latest file, and no reload signal is needed. Plugins
  with long-lived processes watch the file themselves:
  - persiyanov/herdr-reviewr: "Saving it applies at once".
  - hhdebb/herdr-radar: `fs.watch` on herdr's own `config.toml`.

## 5. What published plugins do

Sampled at each repo's default-branch HEAD on 2026-10-23.

| Plugin | File in `HERDR_PLUGIN_CONFIG_DIR` | Fallback when the env var is unset | Notes |
| --- | --- | --- | --- |
| persiyanov/herdr-reviewr | `config.toml` | `herdr plugin config-dir` | Whole-file validation: an unknown key or bad value is an error shown in the pane. Applies on save. CLI flags (`--theme`) can override. |
| cloudmanic/herdr-plus | `config.toml` | `$XDG_CONFIG_HOME/herdr-plus`, then `~/.config/herdr-plus` | Comment: herdr "keeps it across uninstall/upgrade". |
| dio16/herdr-auto-update | `config.toml` | `herdr plugin config-dir <crate name>` | |
| iurysza/termscope | `config.toml` | none (defaults) | Python `tomllib`. Ignores invalid values. |
| smarzban/herdr-file-viewer | `config.toml` | n/a | Ships `config.example.toml`. Tells users to run `herdr plugin config-dir`. |
| andrewchng/herdr-sessionizer | `config.toml` | hard-coded `~/.config/herdr/plugins/config/sessionizer` | |
| thanhdat77/herdr-navigator | (own file) | hard-coded `~/.config/herdr/plugins/config/herdr-navigator` | Migrates from an older plugin id. Reads herdr's `[theme]` for colours. |
| senna-lang/herdr-agent-usage | seeded config | hard-coded `~/.config/herdr/plugins/config/usagebar` | |
| yuk1ty/herdr-spreader | YAML layouts | `$XDG_CONFIG_HOME`, then `~/.config` | |
| hhdebb/herdr-radar | `config.toml` | n/a | `[colors]` table. Can drive herdr's `[theme] name`. Watches herdr's `config.toml`. |
| jeffarese/herdr-bar | `config.json` | none (defaults) | "Unknown keys are ignored, and a broken file falls back to defaults." |
| ogulcancelik/herdr-browser (herdr author) | `browser.json` | `$HERDR_BROWSER_CONFIG` overrides; else `<root>/.herdr-browser/config.json` | Invalid values fall back to defaults field by field. |
| ogulcancelik/herdr-plugin-examples `agent-telegram-notify` | `.env` | `<plugin root>/.env` | The pattern the docs suggest. |
| kryptamine/herdr-auto-title | **not used** | `~/.config/herdr-auto-title/config.env` | Rejects the herdr dir on purpose: it "exists only when the server starts the plugin", so a hand-run binary would look somewhere else. |
| ChmaraX/herdr-nvim | **not used** | `~/.config/herdr-nvim/config.toml` | |
| dcolinmorgan/herdr-remote | **not used** | `~/.config/herdr-remote/config.env` | |
| qu8n/herdr-automatic-rename | **not used** | `~/.config/herdr-automatic-rename/config.sh` | |

Also fetched, with no plugin config file of their own: eliasstravik/herdr-projects
(edits herdr's `config.toml` with the user's consent),
alexarthurs/herdr-sidebar (reads herdr's `[ui] copy_on_select`),
plannotator/herdr-annotate, speardragon/herdr-plugin-manager and
robbyrussell/herdr-ohmyzsh.

Patterns:

- **`$HERDR_PLUGIN_CONFIG_DIR/config.toml` is the de facto convention.** 7 of
  the 13 plugins that use the dir read `config.toml` there. herdr itself is
  TOML-configured, so users already know the format.
- **Fallbacks differ.** Some plugins hard-code
  `~/.config/herdr/plugins/config/<id>`, which is wrong on Windows, with
  `XDG_CONFIG_HOME` set, or for an id that needs encoding. Others call
  `herdr plugin config-dir`, or use defaults only. Calling the CLI is the only
  fallback that matches herdr exactly.
- **Validation splits two ways.** Strict plugins (reviewr) report an unknown
  key or bad value. Lenient plugins (herdr-bar, herdr-browser, termscope) fall
  back to defaults.
- **Theming.** Some plugins read herdr's own `config.toml` `[theme] name` /
  `[theme.custom]` to match colours (herdr-navigator, herdr-radar). They work
  out its path from `HERDR_CONFIG_PATH`, then `XDG_CONFIG_HOME`, then the
  platform default, because herdr passes no theme to plugins.

## Points for "Config schema and theming" (#22)

- Read **`$HERDR_PLUGIN_CONFIG_DIR/config.toml`**. yubi's plugin id is not
  fixed yet. If it is all lowercase, say `wchoco.yubi`, the path is
  `~/.config/herdr/plugins/config/wchoco.yubi/config.toml` on Linux and macOS.
  An uppercase letter in the id would be `%XX`-encoded in the path. Document
  `herdr plugin config-dir <id>` as the way to find it.
- Read it **once per action and pane launch**. No reload signal is needed,
  because every yubi run is a new process. Both the action and the overlay pane
  get the env var, so the pane can read the config itself without the action
  passing it along.
- A missing file or a missing env var means built-in defaults. Don't hard-code
  `~/.config/herdr/plugins/config/...` as a fallback.
  - For dev runs: if a fallback is needed at all, use an explicit
    `--config`/env override.
  - Avoid calling `herdr plugin config-dir` from inside yubi, because it
    creates dirs as a side effect.
- Decide strict vs lenient validation. The overlay already has a status row
  (#13). There, a bad key or regex could show an error, rather than yubi
  silently using defaults (reviewr is the strict example).
- The file belongs entirely to yubi. herdr gives no schema, migration or
  `[plugins.yubi]` table. If the schema might change, a `version` key or
  tolerance for unknown keys is yubi's own job.
- The settings that have to go in this file come from earlier tickets:
  - custom patterns, and disabling built-ins by name (#10);
  - the hint alphabet (#10);
  - the `copy_command` override (#11);
  - the Joined Copy separator (#8);
  - optional overlay styling (#9 and #13).
- Theming: herdr passes plugins no colours or theme. Options:
  1. Use plain SGR (dim, reverse) only, so the overlay picks up the terminal's
     palette. This matches #9.
  2. Let the yubi config set colours.
  3. Read herdr's `[theme]` from its `config.toml` the way herdr-navigator
     does. This is fragile: path resolution, `[theme.custom]` and remote
     clients all get in the way.
- Keybindings live in herdr's `config.toml` (`[[keys.command]] type =
  "plugin_action"`), not in yubi's file. The README should show that snippet.

## Unknowns

- GitHub `plugin install`/`update` was not run in the probe. That config
  survives them comes from the source and the docs only.
- Windows paths (`%APPDATA%\herdr\plugins\config\...`) come from the source
  only. Not probed.
- Remote attach (`herdr --remote`) and SSH machines: plugins run on the
  server, so `HERDR_PLUGIN_CONFIG_DIR` should point at the server's dir. This
  follows from the docs, but was not probed.
- Future herdr versions may add plugin settings or a config-changed event.
  Nothing in the 0.9.5 docs or CHANGELOG hints at one.

[Plugins]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/plugins.mdx
[Configuration]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/configuration.mdx
[Socket API]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/socket-api.mdx
[CLI reference]: https://raw.githubusercontent.com/herdrdev/herdr/v0.9.5/docs/next/website/src/content/docs/cli-reference.mdx
[CHANGELOG]: https://github.com/herdrdev/herdr/blob/v0.9.5/CHANGELOG.md
[paths]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/plugin_paths.rs
[io-dirs]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/config/io.rs#L30-L42
[io-path]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/config/io.rs#L195-L200
[io-keys]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/config/io.rs#L7-L20
[cli-config-dir]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/cli/plugin.rs#L91-L104
[cli-install]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/cli/plugin.rs#L425-L440
[cli-offline]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/cli/plugin.rs#L1143
[link]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/mod.rs#L79-L81
[runtime]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/runtime.rs#L34-L35
[panes]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/panes.rs#L258-L260
[env]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/env.rs#L15-L30
[context]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/api/schema/plugins.rs#L364-L395
[manifest]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/app/api/plugins/manifest.rs#L11-L34
[installations]: https://github.com/herdrdev/herdr/blob/v0.9.5/src/plugin_installations.rs#L215-L220
