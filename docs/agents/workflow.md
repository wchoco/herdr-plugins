# Development workflow

## Branches and PRs

- Start every change from the latest `main`: `git switch main && git pull && git switch -c <type>/<short-kebab-description>`.
- `<type>` is a Conventional Commits type (`feat`, `fix`, `docs`, `refactor`, `test`, `chore`, ...). Example: `feat/agent-state-idle`.
- Open a PR into `main`. Direct pushes to `main` are blocked by a ruleset.
- PRs are merged with a merge commit; the merge commit message is the PR title and body. Write the PR title in Conventional Commits form, since it becomes the `main` history (`git log --first-parent`).
- Fill in `.github/pull_request_template.md`.

## Security scan (claude-security)

Required for every PR that changes anything other than Markdown docs: Go code, `go.mod`/`go.sum`, `herdr-plugin.toml`, build scripts, `.github/`, and so on.

1. Before opening the PR, run claude-security on the branch: either a scan of the branch's changes or a full codebase scan. Pick the effort level to fit the size of the change.
2. Fix each finding, or record why it is not fixed.
3. Fill in the `## Security scan` section of the PR body: scan type, effort, scanned commit SHA, and the result (no findings, or each finding and how it was handled).
4. If code is pushed after the scan, rescan before merge (a scan of only the new commits is enough) and update the section.

Do not open such a PR without a scan. Scan reports are written to `CLAUDE-SECURITY-<timestamp>/`, which is self-gitignored; never commit them.

## CI

GitHub Actions runs `gofmt` check, `go vet`, `go test`, `go build`, and `govulncheck` using the Go version from `go.mod`. CI complements, but does not replace, the claude-security scan.
