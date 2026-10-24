# Shell session: prompts, VCS status output, a long listing, written
# trailing spaces and blank rows.
clear
print -P "${PS1}git status"
printf 'On branch main\n'
printf 'Changes not staged for commit:\n'
printf '  (use "git add <file>..." to update what will be committed)\n'
printf '\t\033[31mmodified:   yubi/main.go\033[m\n'
printf '\t\033[31mmodified:   README.md\033[m\n'
printf '\n'
printf 'Untracked files:\n'
printf '\t\033[31myubi/testdata/\033[m\n'
printf '\n'
print -P "${PS1}ls -l"
printf 'total 12\n'
printf -- '-rw-r--r-- 1 user user 1024 Oct 24 01:12 README.md\n'
printf 'drwxr-xr-x 3 user user 4096 Oct 24 01:13 \033[1;34myubi\033[0m\n'
printf -- '-rwxr-xr-x 1 user user 2048 Oct 24 01:14 \033[1;32mbuild.sh\033[0m\n'
printf 'trailing spaces   \n'
printf '\n'
printf '\n'
