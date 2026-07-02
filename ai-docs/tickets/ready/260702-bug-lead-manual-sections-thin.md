---
title: lead-only workflow_manual sections are empty/thin, missing ferrule discipline
sage-review: completed
---

# lead-only workflow_manual sections are empty/thin, missing ferrule discipline

## Context

Found during a v0.31.1 dogfooding pass. `ferrule`'s schema description
("Reserved workflow primitive. See wsflow:workflow-manual before use.") is
deliberately terse — this is intentional capability-gating that keeps
subagents from self-minting session keys. An earlier draft of this finding
proposed filling out ferrule's own schema description, but that was
explicitly withdrawn: the stub is correct as-is, and documenting ferrule
discipline there would leak the procedure to subagents who can see the
schema. The correct place for that discipline is the lead-gated
`workflow_manual` output, which subagents never see.

That is exactly where the documentation is missing:

- In the rendered `workflow_manual`, the `### User preferences` section body
  is entirely empty.
- The `### Session setup` section has odd spacing (blank lines before the
  body) and states "call ferrule once per working root" but never states the
  consequence of calling it a second time for the same root — that a second
  call mints a new session identity with empty state, stranding any prior
  agenda/todo/session-tree state bound to the earlier key.

This gap is not cosmetic: a legitimate lead fell into the redundant-mint trap
precisely because the one authorized documentation channel for this
discipline never states the consequence.

## Suggestion

Fill `Session setup` with the full ferrule discipline: reuse the existing
session key across the working session; a second `ferrule` call for the same
root mints a new identity with empty state, stranding prior agenda/todo state;
preserve the key verbatim across compaction. Also fix the section's spacing
and fill in (or otherwise repair) the empty `User preferences` section body.

## Spec Impact

Target: `ai-docs/spec/mcp-tools.md`. Caller-visible change: `workflow_manual`'s
rendered `Session setup` and `User preferences` sections gain the ferrule
reuse-discipline and are no longer empty/thin. Contract-first spec: no.
