#!/bin/bash
# Marathon bootstrap — mechanical setup before MCP calls
# Usage: bash ~/.claude/skills/marathon/bootstrap.sh
# Output: JSON with branch, team, original_branch, active_docs
set -euo pipefail

DATETIME=$(date +%Y-%m-%d-%H%M)
BRANCH="marathon/$DATETIME"
TEAM="marathon-$DATETIME"
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

# If already on a marathon branch, resume instead of creating
if [[ "$CURRENT_BRANCH" == marathon/* ]]; then
  BRANCH="$CURRENT_BRANCH"
  DATETIME="${CURRENT_BRANCH#marathon/}"
  TEAM="marathon-$DATETIME"
  # Find the branch that marathon was forked from
  MERGE_BASE=$(git merge-base main "$CURRENT_BRANCH" 2>/dev/null || true)
  if [ -n "$MERGE_BASE" ]; then
    # Check if merge-base is the tip of a known branch
    ORIGINAL=$(git branch --contains "$MERGE_BASE" --format='%(refname:short)' \
      | grep -v '^marathon/' | head -1 || echo "main")
  else
    ORIGINAL="main"
  fi
else
  ORIGINAL="$CURRENT_BRANCH"
  git checkout -b "$BRANCH"
fi

# Initialize token usage file
mkdir -p ~/.claude/usage
printf '# Token Usage: %s\n```json\n{}\n```\n' "$TEAM" \
  > ~/.claude/usage/"$TEAM".md

# Collect active docs
if [ -f ai-docs/list-active.sh ]; then
  ACTIVE_DOCS=$(bash ai-docs/list-active.sh 2>/dev/null || true)
else
  ACTIVE_DOCS=$(find ai-docs -type f -name '*.md' 2>/dev/null | sort || true)
fi

# Output JSON
jq -n \
  --arg branch "$BRANCH" \
  --arg team "$TEAM" \
  --arg original "$ORIGINAL" \
  --arg docs "$ACTIVE_DOCS" \
  '{branch: $branch, team: $team, original_branch: $original, active_docs: $docs}'
