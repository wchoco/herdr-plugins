# Wide glyphs (CJK, emoji) mixed with ASCII, truecolor and 256-colour SGR.
clear
print -P "${PS1}cat notes.txt"
printf '日本語のテキスト: wide glyphs\n'
printf 'mixed ASCII 漢字 and かな, then a URL https://example.com/路径\n'
printf '\033[38;2;255;136;0mtruecolor 色\033[0m and \033[38;5;45m256-colour\033[0m\n'
printf 'emoji 🎉 party\n'
printf '\033[7m全角\033[0m reverse\n'
