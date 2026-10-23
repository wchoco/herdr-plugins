# herdr-plugins

## Development

- Never commit to `main` directly. Branch from the latest `main` as `<type>/<short-kebab-description>` and open a PR into `main`. See `docs/agents/workflow.md`.
- Every PR that changes anything other than Markdown docs must have a claude-security scan before it is opened. See `docs/agents/workflow.md`.
- Plugins are versioned per plugin with SemVer and tagged `<plugin-name>/vX.Y.Z` on release. See `docs/agents/versioning.md`.
- Language: Go. One `go.mod` at the repo root; each plugin lives in `<plugin-name>/` with its own `herdr-plugin.toml`, whose `id` is `wchoco.<plugin-name>`.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues (wchoco/herdr-plugins) via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Uses the default five triage labels (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
