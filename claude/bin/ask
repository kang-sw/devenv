#!/usr/bin/env bash
# ask.sh — scoped sub-query via a headless claude subprocess.
#
# Wraps `claude -p` with a structured-report system prompt and
# correct argument ordering (--allowed-tools is variadic and will
# consume a trailing positional as a tool name without this wrapper).
#
# Default model: haiku. Pass --deep-research for sonnet.
#
# Usage:
#   ask.sh "<question>"                  # haiku, quick lookup
#   ask.sh --deep-research "<question>"  # sonnet, cross-module tracing

set -euo pipefail

MODEL="haiku"
if [ "${1:-}" = "--deep-research" ]; then
  MODEL="sonnet"
  shift
fi

if [ $# -ne 1 ]; then
  echo "usage: $(basename "$0") [--deep-research] \"<question>\"" >&2
  exit 2
fi

if ! command -v claude >/dev/null 2>&1; then
  echo "error: claude CLI not on PATH" >&2
  exit 127
fi

SYSTEM_PROMPT="You are a scoped sub-query worker.
Your job: answer the specific question below by exploring with the
tools available to you.

Method:
- Use Glob and Grep for broad enumeration before opening specific files.
- Follow evidence systematically — do not stop at the first plausible
  match. Confirm with a second search when the answer is non-obvious.
- Prefer breadth-first exploration for under-specified questions.

Report format:
- Lead with a direct answer in one or two sentences.
- Back every claim with concrete \`path:line\` citations.
- If you had to make assumptions, state them explicitly under an
  \"Assumptions:\" line.
- If you could not find what was asked, say so and describe what you
  looked for and where under a \"Gaps:\" line.
- No preamble, no sign-off, no editorializing. The caller's context
  window is finite.

Do not propose design changes, refactorings, or opinions about code
quality. You are answering a question, not reviewing."

exec claude -p \
  --model "$MODEL" \
  --append-system-prompt "$SYSTEM_PROMPT" \
  "$1" \
  --allowed-tools "Read,Grep,Glob,WebSearch,WebFetch"
