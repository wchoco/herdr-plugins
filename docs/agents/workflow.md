# Development workflow

## Branches and PRs

- Start every change from the latest `main`: `git switch main && git pull && git switch -c <type>/<short-kebab-description>`.
- `<type>` is a Conventional Commits type (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`, ...). Example: `feat/agent-state-idle`.
- Open the PR into `main` as a draft (`gh pr create --draft`), and mark it ready (`gh pr ready`) once the checks below and the security scan are done. Direct pushes to `main` are blocked by a ruleset.
- PRs are merged with a merge commit; the merge commit message is the PR title and body. Write the PR title in Conventional Commits form, since it becomes the `main` history (`git log --first-parent`).
- Fill in `.github/pull_request_template.md`.

## Security scan (claude-security)

Required for every PR that changes anything other than Markdown docs: Go code, `go.mod`/`go.sum`, `herdr-plugin.toml`, build scripts, `.github/`, and so on.

1. Open the PR as a draft. Push fixes to it freely while it is a draft, including fixes from a plugin's manual test.
2. Before marking it ready, run claude-security on the branch: either a scan of the branch's changes or a full codebase scan. Pick the effort level to fit the size of the change. A scan starts only once the user's own request accepts its time and token cost, so ask for that acknowledgment while the user is present, together with any other questions.
3. Fix each finding, or record why it is not fixed.
4. Fill in the `## Security scan` section of the PR body: scan type, effort, scanned commit SHA, and the result (no findings, or each finding and how it was handled).
5. If code is pushed after the scan, rescan before merge (a scan of only the new commits is enough) and update the section.

Do not mark such a PR ready without a scan. Scan reports are written to `CLAUDE-SECURITY-<timestamp>/`, which is self-gitignored; never commit them.

## Checks

`scripts/check.sh` runs every check: `gofmt`, a shape match of each `*.ja.md` against its English original (heading and checklist-item counts), `go vet`, `go test`, `go build` of each plugin, `govulncheck`, and each plugin's `scripts/e2e.sh` when `herdr` and `tmux` are installed. CI runs the same script (without herdr, so it skips the e2e). Run it before every push; `git config core.hooksPath .githooks` makes `git push` run it. CI complements, but does not replace, the claude-security scan.

## Live herdr

An agent runs herdr (`plugin link`, `server`, capture scripts, e2e) only through `scripts/throwaway-herdr <command>`, which strips the `HERDR_*` variables and points herdr at a private server, so the user's own session is never touched. The user runs a plugin's manual test (`<plugin-name>/docs/manual-test.md`) in their own herdr. Known herdr behaviour that its schema and docs do not tell you is in `docs/herdr.md`.
