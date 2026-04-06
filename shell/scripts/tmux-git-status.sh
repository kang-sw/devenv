#!/bin/bash
# tmux-git-status.sh — compact git status for tmux status bar
# Usage: tmux-git-status.sh [directory]
# Output: tmux-formatted string with branch, ahead/behind, working tree status
#   or "—" if not a git repo
# Colors match statusline.sh conventions

dir="${1:-.}"
# Skip git operations on WSL2 Windows mounts (/mnt/c/, /mnt/d/, etc.) — NTFS git is too slow
[[ "$dir" =~ ^/mnt/[a-z]/ ]] && { echo "—"; exit 0; }
cd "$dir" 2>/dev/null || { echo "—"; exit 0; }
git rev-parse --git-dir >/dev/null 2>&1 || { echo "—"; exit 0; }

branch=$(git branch --show-current 2>/dev/null)
[ -z "$branch" ] && branch=$(git rev-parse --short HEAD 2>/dev/null || echo "???")

# Branch name — colour114 (GIT_BRANCH_FG)
out="#[fg=colour114]${branch}"

# Ahead / behind upstream — ahead=114(green), behind=214(yellow)
if git rev-parse --verify "@{u}" >/dev/null 2>&1; then
  ahead=$(git rev-list --count "@{u}..HEAD" 2>/dev/null)
  behind=$(git rev-list --count "HEAD..@{u}" 2>/dev/null)
  [ "$ahead" -gt 0 ] 2>/dev/null && out+=" #[fg=colour114]↑${ahead}"
  [ "$behind" -gt 0 ] 2>/dev/null && out+=" #[fg=colour214]↓${behind}"
fi

# Line-level diff stats (added/deleted lines)
diff_stat=$(git diff --numstat 2>/dev/null | awk '{a+=$1; d+=$2} END {printf "%d %d", a+0, d+0}')
lines_added=$(echo "$diff_stat" | cut -d' ' -f1)
lines_deleted=$(echo "$diff_stat" | cut -d' ' -f2)

# File-level working tree changes
staged=$(git diff --cached --name-only 2>/dev/null | wc -l | tr -d ' ')
modified=$(git diff --name-only 2>/dev/null | wc -l | tr -d ' ')
untracked=$(git ls-files --others --exclude-standard 2>/dev/null | wc -l | tr -d ' ')

# Detail: same order & colors as statusline.sh
#   +added(114/green) -deleted(203/red) ~modified(214/yellow) ?untracked(75/blue)
#   ●staged shown separately before diff stats
detail=""
[ "$staged" -gt 0 ] 2>/dev/null && detail+="#[fg=colour114]●${staged} "
[ "$lines_added" -gt 0 ] 2>/dev/null && detail+="#[fg=colour114]+${lines_added} "
[ "$lines_deleted" -gt 0 ] 2>/dev/null && detail+="#[fg=colour203]-${lines_deleted} "
[ "$modified" -gt 0 ] 2>/dev/null && detail+="#[fg=colour214]~${modified} "
[ "$untracked" -gt 0 ] 2>/dev/null && detail+="#[fg=colour75]?${untracked} "

if [ -n "$detail" ]; then
  out+=" #[fg=#555555]· ${detail% }"
fi

echo "$out"
