# Joined Copy: a soft wrap that falls on a space, and written trailing
# spaces on an intermediate and on a last line.
clear
print -P "${PS1}echo \$MSG"
printf 'x%.0s' {1..118}
printf ' after the wrap\n'
printf 'kept   \n'
printf 'trimmed   \n'
