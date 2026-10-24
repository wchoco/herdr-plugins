#!/bin/sh
# End-to-end check of yubi against a real herdr, through scripts/throwaway-herdr.
#
# Usage: yubi/scripts/e2e.sh
#
# It builds yubi into a scratch plugin dir, runs herdr with a client inside a
# private tmux server (`tmux -L yubi-e2e`), presses the bindings with
# `send-keys` and checks what herdr reports: panes, focus, zoom and the
# plugin log. It covers what the fake-socket tests cannot see: herdr's own
# resizes, focus and zoom restore, and re-launch through a real keybinding.
# Needs herdr, tmux, go and python3. Leaves nothing running.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
plugin_src=$(dirname "$here")
repo=$(dirname "$plugin_src")
export THROWAWAY_HERDR_DIR="$(mktemp -d "${TMPDIR:-/tmp}/yubi-e2e.XXXXXX")"
dir=$THROWAWAY_HERDR_DIR
sock=yubi-e2e.$$

H() { "$repo/scripts/throwaway-herdr" herdr "$@"; }
X() { tmux -L "$sock" "$@"; }

cleanup() {
	H server stop >/dev/null 2>&1 || true
	X kill-server 2>/dev/null || true
	rm -rf "$dir"
}
trap cleanup EXIT

failures=0
pass() { echo "ok   $1"; }
fail() {
	echo "FAIL $1"
	failures=$((failures + 1))
}

# layout <field>: one field of `herdr pane layout` for the focused pane.
layout() {
	H pane layout | python3 -c 'import json,sys; print(json.load(sys.stdin)["result"]["layout"][sys.argv[1]])' "$1"
}
pane_count() { H pane list | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["result"]["panes"]))'; }
focused() { layout focused_pane_id; }
zoomed() { layout zoomed; }
screen() { X capture-pane -p; }

# until <seconds> <command...>: retry the command until it succeeds or time runs out.
until_ok() {
	n=$(($1 * 10))
	shift
	while [ "$n" -gt 0 ]; do
		if "$@" >/dev/null 2>&1; then return 0; fi
		sleep 0.1
		n=$((n - 1))
	done
	return 1
}
# need <seconds> <what> <command...>: like until_ok, but stop the run when it fails.
need() {
	secs=$1
	what=$2
	shift 2
	until_ok "$secs" "$@" || {
		echo "setup failed: $what"
		exit 1
	}
}
is_open() { [ "$(pane_count)" -gt "$base" ]; }
is_closed() { [ "$(pane_count)" -eq "$base" ] && [ "$(focused)" = "$target" ]; }
shows() { screen | grep -q "$1"; }

# press <key>: the herdr prefix (ctrl+g), then the key.
press() {
	X send-keys -H 07
	sleep 0.2
	X send-keys "$1"
}

# --- setup -----------------------------------------------------------------

mkdir -p "$dir/cfg/herdr" "$dir/plugin" "$dir/cfg/herdr/plugins/config/wchoco.yubi"
cat >"$dir/cfg/herdr/config.toml" <<'EOF'
[keys]
prefix = "ctrl+g"

[[keys.command]]
key = "prefix+f"
type = "plugin_action"
command = "wchoco.yubi.hint"

[[keys.command]]
key = "prefix+y"
type = "plugin_action"
command = "wchoco.yubi.line"
EOF
cat >"$dir/cfg/herdr/plugins/config/wchoco.yubi/config.toml" <<EOF
[clipboard]
copy_command = ["sh", "-c", "cat > $dir/copied"]
EOF
cp "$plugin_src/herdr-plugin.toml" "$dir/plugin/"
(cd "$plugin_src" && go build -o "$dir/plugin/yubi" .)

"$repo/scripts/throwaway-herdr" tmux -L "$sock" new-session -d -x 134 -y 44 herdr
need 10 "herdr starts" H pane list
H plugin link "$dir/plugin" >/dev/null
# A first run shows herdr's welcome (Enter) and then its settings (Esc).
need 5 "herdr shows its welcome" shows continue
X send-keys Enter
need 5 "herdr shows its settings" shows 'esc close'
X send-keys Escape
need 5 "herdr closes its settings" sh -c '! tmux -L "$1" capture-pane -p | grep -q "esc close"' _ "$sock"

target=$(focused)
base=$(pane_count)
X send-keys "clear; printf 'alpha\nbeta\n'" Enter
need 5 "the shell prints the test lines" shows '│beta'

# --- toggle and mode switch ------------------------------------------------

press f
if until_ok 5 is_open && until_ok 5 shows HINT; then pass "prefix+f opens Hint Mode"; else fail "prefix+f opens Hint Mode"; fi
sleep 2
if is_open; then pass "the overlay stays open on its own"; else fail "the overlay stays open on its own"; fi
press f
if until_ok 5 is_closed; then pass "prefix+f twice closes it"; else fail "prefix+f twice closes it"; fi
sleep 1
if is_closed; then pass "no new overlay opens after the toggle"; else fail "no new overlay opens after the toggle"; fi

press f
until_ok 5 shows HINT || true
press y
if until_ok 5 shows LINE && is_open; then pass "prefix+f, prefix+y switches to Line Mode"; else fail "prefix+f, prefix+y switches to Line Mode"; fi
press y
if until_ok 5 is_closed; then pass "prefix+y on Line Mode closes it"; else fail "prefix+y on Line Mode closes it"; fi

# --- copy ------------------------------------------------------------------

press y
until_ok 5 shows LINE || true
# The Line Number is drawn over the first cells of its row: "5eta" is beta's.
number=$(screen | python3 -c '
import re, sys
for row in sys.stdin:
    for cell in row.split("│"):
        m = re.fullmatch(r"(\d+)(\S*)\s*", cell)
        if m and len(m[1]) + len(m[2]) == 4 and "beta".endswith(m[2]):
            print(m[1])
            sys.exit()')
if [ -n "$number" ]; then
	X send-keys "$number" Enter
	until_ok 5 test -s "$dir/copied" || true
fi
if [ "$(cat "$dir/copied" 2>/dev/null)" = beta ] && until_ok 5 is_closed; then
	pass "Line Mode copies a Line and closes"
else
	fail "Line Mode copies a Line and closes (line number '${number}', copied '$(cat "$dir/copied" 2>/dev/null)')"
fi

# --- zoomed tab --------------------------------------------------------------

H pane split "$target" --direction right --no-focus >/dev/null
H pane zoom "$target" --on >/dev/null
base=$(pane_count)

press f
until_ok 5 is_open || true
press f
if until_ok 5 is_closed && [ "$(zoomed)" = True ]; then pass "zoomed: prefix+f twice keeps the zoom"; else fail "zoomed: prefix+f twice keeps the zoom"; fi

press f
until_ok 5 shows HINT || true
press y
until_ok 5 shows LINE || true
X send-keys Escape
if until_ok 5 is_closed && [ "$(zoomed)" = True ]; then pass "zoomed: prefix+f, prefix+y, Esc keeps the zoom"; else fail "zoomed: prefix+f, prefix+y, Esc keeps the zoom"; fi

# --- plugin log --------------------------------------------------------------

errors=$(H plugin log list | python3 -c '
import json, sys
for l in json.load(sys.stdin)["result"]["logs"]:
    if l["status"] != "succeeded" or l["stderr"].strip():
        print(l["action_id"], l["status"], l["stderr"].strip())')
if [ -z "$errors" ]; then pass "every action succeeded with no stderr"; else fail "actions logged errors: $errors"; fi

if [ "$failures" -gt 0 ]; then
	echo "$failures check(s) failed"
	exit 1
fi
echo "all checks passed"
