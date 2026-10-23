---
name: merge
description: Gate-check a PR against this repo's rules, merge it into main with a merge commit, and sync local main.
disable-model-invocation: true
argument-hint: "[PR number]"
---

# Merge a PR

Merge one PR into `main` after it passes every **gate**. The rules behind the gates live in `docs/agents/workflow.md`; read it before step 2.

## 1. Load the PR

Target: the PR number in `$ARGUMENTS`, or else the PR for the current branch.

```sh
gh pr view <n> --json number,title,body,state,isDraft,baseRefName,headRefName,headRefOid,mergeable,statusCheckRollup,files,commits
```

## 2. Run the gates

Check every gate and collect every failure; one failure does not end the pass.

- **State**: `state` is `OPEN`, `isDraft` is false, `baseRefName` is `main`, `mergeable` is `MERGEABLE`.
- **CI**: every entry in `statusCheckRollup` concluded `SUCCESS`, `SKIPPED` or `NEUTRAL`. An empty rollup passes.
- **Messages**: the PR title and every commit message are English and in Conventional Commits form; the title and body are English. The title and body become the merge commit, so they and every commit message carry only the change itself, with no mention of Claude, AI or a generation tool.
- **Security scan**: when any file in `files` is not Markdown, the `## Security scan` section is filled in, and its scanned commit is `headRefOid`, or the section records a rescan that covers every commit after the scanned one. A PR that touches only Markdown passes this gate.

Done when every gate is marked pass or fail with the evidence that decided it.

## 3. Stop on failure

If any gate failed, report each failure with its evidence and the fix, and stop without merging. For a failed security scan, offer to run claude-security on the branch; it starts only on the user's go-ahead.

## 4. Merge

```sh
gh pr merge <n> --merge --delete-branch --match-head-commit <headRefOid>
```

`--match-head-commit` makes the merge fail if the branch moved after the gates ran; if it does, return to step 1. The repo settings build the merge commit message from the PR title and body, so pass no `--subject` or `--body`.

## 5. Sync local

```sh
git switch main
git pull --ff-only
```

Delete the local head branch if it still exists. Done when `gh pr view <n> --json state` shows `MERGED` and local `main` equals `origin/main`.

## 6. Release PRs

When `headRefName` is `chore/release-<plugin-name>-vX.Y.Z`, tell the user the release is ready to tag as `<plugin-name>/vX.Y.Z`, following `docs/agents/versioning.md`. Tag and create the GitHub Release only when the user asks for it.
