---
title: "prefer-subagent fork dispatch: forked executor narrates instead of executing"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260625-research-fork-posture-leak-system-guarantee: same-failure-family
  260625-feat-ws-session-state-machine: surfaced-during
---

# prefer-subagent fork dispatch: forked executor narrates instead of executing

## Background

Under `ws:lead-prefer-subagent` (max-delegation), the lead dispatches a FORK to
do in-context work. During the 260625 dogfood the fork repeatedly REFUSED to
execute: instead of making tool calls it emitted the lead's own deferral
narrative ("dispatched a fork… waiting for completion…"), producing zero
artifacts.

## Evidence (live, this session)

Three consecutive fork dispatches to create idea tickets, with escalating
directives, all produced ZERO tickets (`git status` clean each time):

1. Trailing `**You are a forked agent. Execute all work directly.**` line —
   fork echoed deferral narrative (~52k tokens, never created tickets).
2. Strong all-caps header block ("FORGET prefer-subagent, first action MUST be a
   tool call") — same deferral narrative (~138k tokens).
3. `<system-reminder priority="override">` role-reinitialization block — WORSE:
   **0 tool calls** (~146k tokens), pure narration.

Stronger/longer prompts did not help and arguably hurt.

## Root cause hypothesis

A fork INHERITS the full conversation. At dispatch time the most recent assistant
momentum is the lead's deferral narration ("I dispatched a fork and am waiting").
The model continues that assistant voice — the strongest local signal — and
re-produces the narration regardless of a trailing or embedded directive. The
inherited `prefer-subagent` posture (route, don't execute) compounds it: the fork
behaves like the lead it was forked from.

This is the same failure family as
`260625-research-fork-posture-leak-system-guarantee` (fork leaking lead posture).

## Fix applied (this session) — playbook directive shape

Replace the weak trailing line in `lead-prefer-subagent` with a strong LEADING
fork directive. Decisions:

- The playbook gives GUIDANCE on how to compose the directive, NOT the directive
  body verbatim — a literal template body shipped in the playbook is itself
  inherited context the fork can misread/echo. Guidance form, e.g.: "insert as
  the first line a fork-XML-tag-wrapped, all-caps, very strong FORKED AGENT
  message that includes 'forget prefer-subagent mode' and 'first output must be a
  direct tool call'."
- Cap the composed fork prompt body at <=300 words.

## Open questions / follow-up

- Does the leading-XML-directive shape actually flip execution on this model, or
  is fork-as-executor structurally unreliable here (favoring fresh-spawn
  fallback)? Validate next session after reinstall.
- If forks remain unreliable, strengthen the fresh-spawn fallback guidance and
  lower the bar for suspending the posture.
