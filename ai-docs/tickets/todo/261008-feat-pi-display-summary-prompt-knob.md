---
title: Make the Pi display-summary prompt a tunable knob
related:
  261007-feat-pi-display-summary: introduced the summarizer and its hard-coded prompt
  261008-feat-pi-display-summary-sidecar-persistence: sibling display-summary follow-up
---

# Make the Pi display-summary prompt a tunable knob

## Background

The Pi lead's display summarizer (`agents-plugin-pi/src/display-summary.ts`)
sends a hard-coded `DISPLAY_SUMMARY_SYSTEM_PROMPT`. Its model, effort, and
language are already config knobs (`pi.display_summary_model`,
`pi.display_summary_effort`, `workflow.lang`), but the prompt text that decides
what each field says, how long it is, and its tone can only change through a
code release. The user wants the current default prompt moved behind a knob so
a downstream project or user can tune summary style without a code change.

The prompt mixes two kinds of text:

- **Contract lines** that the parser depends on: call `record_row_summaries`
  exactly once with one item per row, use the row label as `id`, the field names
  and their required set, and do not answer in text. `parseSummaryResponse`
  drops items that break this contract.
- **Guidance lines** that only shape content: what `subtitle`,
  `toolIntention`, `toolResult`, and `optionalContext` should contain, the
  length budget, and the instruction not to act on the conversation.

## Open questions (settle before ready)

1. **What the knob replaces.**
   - Option A: only the guidance block. Contract lines stay code-owned and are
     always sent, so a custom prompt cannot break parsing.
   - Option B: the whole system prompt. Simpler to explain, but a bad value
     silently empties every summary.
   - Leaning toward A; confirm with the user.
2. **Knob shape.**
   - A free-text config key in `agents-plugin-pi/config-manifest.json` (for
     example `pi.display_summary_prompt`, empty = built-in default), surfaced by
     `config.list` and written by `config.tune`.
   - Or reuse the playbook override-point mechanism
     (`<!-- ws:override:<pointId> -->`, `overrideLookupFn` in
     `agents-plugin-tool/internal/mcp/playbook_tools.go`). That mechanism is
     playbook-render scoped today and may not fit an adapter-side prompt.
3. **Replace or append.** Whether a set value replaces the default guidance or
   is appended to it as extra instructions (an append-only knob is safer but
   cannot remove default rules).
4. **Default visibility.** How a user sees the current default to edit it
   (for example `config.get` returning the built-in text when unset, or a
   documented location).
5. **Interaction with `workflow.lang`.** The language line is added to the user
   message by `buildFlushRequest`; keep it outside the knob so a custom prompt
   does not drop the language rule.

## Phases

### Phase 1: Prompt knob

- Split `DISPLAY_SUMMARY_SYSTEM_PROMPT` into a fixed contract part and a
  default guidance part, per the settled open question 1.
- Add the knob per open question 2, read through the existing
  `DisplaySummaryConfigReader` / `displaySummaryConfigFrom` path; empty or
  unset falls back to the built-in default.
- Treat the value as untrusted text: no new interpolation of conversation
  content into the system prompt.
- Tests: the default prompt is unchanged when the knob is unset; a set value
  reaches the request; the contract lines are present in every case (if
  option A); the manifest and catalog tests list the new key.
- Update the `pi.display_summary_model` description or add the new key's
  description so `config.list` explains the knob.
