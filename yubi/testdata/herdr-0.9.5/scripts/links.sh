# Hint Mode input: coloured URLs (one printed three times), absolute,
# relative and home paths, a path next to wide glyphs, and a row with no
# Match.
clear
print -P "${PS1}git push"
printf 'remote: Create a pull request:\n'
printf 'remote:   \033[4;34mhttps://github.com/wchoco/herdr-plugins/pull/new/feat\033[0m\n'
printf 'To github.com:wchoco/herdr-plugins.git\n'
printf 'see https://example.com/docs and https://example.com/docs again\n'
printf 'nothing to hint on this row\n'
print -P "${PS1}go build ./..."
printf '\033[1m./yubi/internal/core/hint/hint.go:12:3\033[0m: undefined: x\n'
printf 'log written to /tmp/yubi/build.log and ~/proj/out.txt\n'
printf '日本語 /var/log/syslog 漢字\n'
printf 'docs again: https://example.com/docs\n'
