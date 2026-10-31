#!/bin/sh
# Every check a change must pass. CI runs this same script.
#
# Usage: scripts/check.sh
#
# Each plugin's scripts/e2e.sh runs too when herdr and tmux are installed;
# CI has neither, so there it is skipped.
set -eu
cd "$(dirname "$0")/.."

unformatted=$(git ls-files -z '*.go' | xargs -0 gofmt -l)
if [ -n "$unformatted" ]; then
	echo "not gofmt-formatted:"
	echo "$unformatted"
	exit 1
fi
# A translation must keep its English original's shape: the same number of
# headings and checklist items.
for ja in $(git ls-files '*.ja.md'); do
	en=${ja%.ja.md}.md
	for pattern in '^#' '^ *- \[ \]'; do
		if [ "$(grep -c "$pattern" "$en")" != "$(grep -c "$pattern" "$ja")" ]; then
			echo "$ja and $en differ in lines matching '$pattern'"
			exit 1
		fi
	done
done
go vet ./...
go test ./...
for manifest in */herdr-plugin.toml; do
	(cd "$(dirname "$manifest")" && go build -o /dev/null .)
done
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

for e2e in */scripts/e2e.sh; do
	[ -e "$e2e" ] || continue
	if command -v herdr >/dev/null && command -v tmux >/dev/null; then
		echo "== $e2e"
		"$e2e"
	else
		echo "== $e2e skipped: needs herdr and tmux"
	fi
done
