#!/bin/sh
# Capture `pane read --source visible` fixtures from a running herdr server.
# Run it against a throwaway server (see ../../README.md), never your own
# session: it creates and closes one workspace per fixture.
#
# Usage: capture.sh <fixture>...   e.g. capture.sh shell cjk wrap
set -eu

here=$(cd "$(dirname "$0")" && pwd)
out=$(dirname "$here")
zdot=$(mktemp -d)
trap 'rm -rf "$zdot"' EXIT
cp "$here/zshrc" "$zdot/.zshrc"

for name in "$@"; do
	created=$(SHELL=$(command -v zsh) herdr workspace create --no-focus --label "yubi-$name" \
		--env ZDOTDIR="$zdot" --env YUBI_FIXTURE="$here/$name.sh")
	pane=$(printf '%s' "$created" | sed -n 's/.*"root_pane":{[^}]*"pane_id":"\([^"]*\)".*/\1/p')
	ws=${pane%%:*}
	sleep 2
	herdr pane read "$pane" --source visible >"$out/$name.txt"
	herdr pane read "$pane" --source visible --format ansi >"$out/$name.ansi"
	herdr pane layout --pane "$pane" >"$out/$name.layout.json"
	herdr workspace close "$ws" >/dev/null
done
