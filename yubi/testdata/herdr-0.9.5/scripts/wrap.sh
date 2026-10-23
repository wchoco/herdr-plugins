# Soft-wrapped long lines: an ASCII URL and a run of wide glyphs that wraps
# at an odd column.
clear
print -P "${PS1}echo \$URL"
printf 'https://example.com/a/very/long/path/'
for i in $(seq 1 30); do printf 'segment%02d/' "$i"; done
printf '\n'
for i in $(seq 1 70); do printf '漢'; done
printf '\n'
