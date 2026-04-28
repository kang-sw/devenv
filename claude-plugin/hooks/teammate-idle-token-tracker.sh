#!/usr/bin/env bash
# TeammateIdle hook: track per-teammate token usage as % of 150K soft limit
# Writes to .claude/usage/<team-name>.md
set -euo pipefail

SOFT_LIMIT=150000
INPUT=$(cat)

TEAMMATE_NAME=$(echo "$INPUT" | jq -r '.teammate_name')
TEAM_NAME=$(echo "$INPUT" | jq -r '.team_name')
TRANSCRIPT_PATH=$(echo "$INPUT" | jq -r '.transcript_path')

# Derive subagents dir: transcript is <project>/<session-id>.jsonl
# subagents are at <project>/<session-id>/subagents/
SUBAGENTS_DIR="${TRANSCRIPT_PATH%.jsonl}/subagents"

if [ ! -d "$SUBAGENTS_DIR" ]; then
  exit 0
fi

# Find the most recently modified transcript matching this teammate name
AGENT_TRANSCRIPT=""
LATEST_MTIME=0
for meta in "$SUBAGENTS_DIR"/*.meta.json; do
  [ -f "$meta" ] || continue
  AGENT_TYPE=$(jq -r '.agentType // ""' "$meta" 2>/dev/null)
  if [ "$AGENT_TYPE" = "$TEAMMATE_NAME" ]; then
    CANDIDATE="${meta%.meta.json}.jsonl"
    if [ -f "$CANDIDATE" ]; then
      MTIME=$(stat -f %m "$CANDIDATE" 2>/dev/null || stat -c %Y "$CANDIDATE" 2>/dev/null || echo 0)
      if [ "$MTIME" -gt "$LATEST_MTIME" ]; then
        LATEST_MTIME=$MTIME
        AGENT_TRANSCRIPT="$CANDIDATE"
      fi
    fi
  fi
done

if [ -z "$AGENT_TRANSCRIPT" ]; then
  exit 0
fi

# Context window size = last assistant turn's full input
# (input_tokens + cache_creation + cache_read = total context sent to model)
TOTAL_TOKENS=$(jq -s '
  [.[] | select(.type=="assistant")] | last | .message.usage |
  ((.input_tokens // 0) + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0))
' "$AGENT_TRANSCRIPT")

# Calculate percentage
PCT=$(( TOTAL_TOKENS * 100 / SOFT_LIMIT ))

# Write to usage file
USAGE_DIR="$HOME/.claude/usage"
mkdir -p "$USAGE_DIR"
USAGE_FILE="$USAGE_DIR/$TEAM_NAME.md"

# Create file if missing
if [ ! -f "$USAGE_FILE" ]; then
  echo "# Token Usage: $TEAM_NAME" > "$USAGE_FILE"
  echo '```json' >> "$USAGE_FILE"
  echo '{}' >> "$USAGE_FILE"
  echo '```' >> "$USAGE_FILE"
fi

# Update JSON block in-place
EXISTING_JSON=$(sed -n '/^```json$/,/^```$/p' "$USAGE_FILE" | sed '1d;$d')
NEW_JSON=$(echo "$EXISTING_JSON" | jq --arg name "$TEAMMATE_NAME" --arg val "${PCT}%/150K" '. + {("@" + $name): $val}')

{
  echo "# Token Usage: $TEAM_NAME"
  echo '```json'
  echo "$NEW_JSON"
  echo '```'
} > "$USAGE_FILE"

exit 0
