# Versioning and releases

## Layout

- Each plugin lives in `<plugin-id>/` with a `herdr-plugin.toml`. The directory name equals the manifest `id` (kebab-case).
- One Go module at the repo root. Shared code goes in `internal/`.
- Users install one plugin at a time: `herdr plugin install wchoco/herdr-plugins/<plugin-id>`. Herdr shallow-clones the whole repo and runs only that plugin's `[[build]]` commands in `<plugin-id>/`, so the root `go.mod` and `internal/` are available at build time.
- Each plugin's `[[build]]` runs `go build`. Users need a Go toolchain; say so in the plugin's README.

## Versions

- Each plugin has its own SemVer version. The single source of truth is `version` in its `herdr-plugin.toml`.
- Bump the version per release, not per build.
- Builds identify themselves by version plus commit: embed the manifest with `//go:embed herdr-plugin.toml` to read `version`, and read `vcs.revision` / `vcs.modified` from `runtime/debug.ReadBuildInfo()`. Display as `0.3.0+a1b2c3d`, or `0.3.0+a1b2c3d.dirty` when modified. Do not rely on `git describe` (installs are shallow clones without tags) or on `-ldflags` (`[[build]]` commands are argv arrays, not shell).
- Set `min_herdr_version` to the herdr version the plugin was verified against at release time.

## Releases

Only tag a plugin once it has a README and has been verified working. Experimental plugins stay at `0.0.x` and untagged.

1. On a branch `chore/release-<plugin-id>-vX.Y.Z`, bump `version` (and `min_herdr_version` if needed) and open a PR.
2. Merge it.
3. Only when the user asks for the release: tag the merge commit on `main` as `<plugin-id>/vX.Y.Z`, push the tag, and create a GitHub Release with `gh release create` including release notes.

Users pin a release with `herdr plugin install wchoco/herdr-plugins/<plugin-id> --ref <plugin-id>/vX.Y.Z`.

Do not add the `herdr-plugin` GitHub topic.
