#!/usr/bin/env bash
# ask.sh — scoped sub-query via a headless claude subprocess.
#
# Used by marathon teammates (planner, implementer, worker) for
# exploration beyond their direct Read/Grep/Glob tools.
#
# Default model: haiku. Pass --deep-research for sonnet.
#
# Why this script exists:
#   1. The raw `claude -p` command has an argument-ordering trap.
#      --allowed-tools is variadic (<tools...>) and will consume a
#      trailing positional as a tool name, leaving no prompt. Putting
#      the prompt before --allowed-tools avoids this.
#   2. A scoped system prompt is pre-loaded so sub-query behavior
#      matches the native Explore agent's systematic, structured-
#      report style. Callers get a consistent contract regardless
#      of the question shape.
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

SYSTEM_PROMPT="You are a scoped sub-query worker dispatched from a marathon teammate.
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
