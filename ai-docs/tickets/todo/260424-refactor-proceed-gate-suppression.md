---
title: Proceed prefix-stage gate suppression + write-spec judge:idea-level demotion
spec:
  - 260424-proceed-chain-gate-suppression
  - 260424-write-spec-idea-level-nonblocking
related-mental-model:
  - workflow-routing
---

# Proceed prefix-stage gate suppression + write-spec judge:idea-level demotion

## Background

Two interactive confirmation gates fire during `/proceed`'s prefix-stage chain
(`/write-spec` → `/write-ticket`), interrupting the automated pipeline:

1. **`write-spec` `judge: idea-level`** — before writing a `🚧` entry, asks the
   user "defer until a `todo/` ticket exists?" This is paradoxical in the canonical
   chain where `/write-ticket` immediately follows `/write-spec`.

2. **`write-ticket` `judge: spec-gate`** — on the CREATE path, stops and redirects
   to `/write-spec` when no spec entry covers the behavior. When `/write-spec` exits
   as a no-op ("no public behavior affected"), spec-gate still fires and breaks the
   chain before the `Ticket:` artifact is produced.

`proceed/SKILL.md` explicitly contracts that the only valid stopping points are
`/implement`'s report-and-approval gate and merge. Both gates above are undocumented
violations of that contract.

## Decisions

- **`judge: idea-level` → non-blocking reminder**: demote from a blocking ask to a
  post-write session note. The chicken-and-egg problem (spec needs a ticket, ticket
  follows spec) is resolved by writing the entry immediately and reminding rather than
  blocking. Rejected: merging write-spec + write-ticket into `/write-docs` — bloats
  non-new-feature paths (🚧 strip, accuracy edit, phase update) for a gain that only
  helps the new-planned-behavior case.

- **Gate suppression via natural-language override in args**: `/proceed` passes
  override context in the Skill invocation args when invoking prefix stages. No formal
  `--auto` flag handler is added to sub-skills — the override is a natural-language
  instruction the sub-skill model honors contextually. Unidirectional dependency:
  proceed knows about its sub-skills, not vice versa. Rejected: `--auto` flag in
  write-spec and write-ticket — adds formal handler complexity in two sub-skills for
  a concern that belongs entirely to the caller.

## Phases

### Phase 1: Demote write-spec judge:idea-level to non-blocking reminder

Modify `write-spec/SKILL.md`: change `judge: idea-level` from a blocking
confirmation ask to a non-blocking reminder. After writing the `🚧` entry, emit:
"Session reminder: a `todo/`-or-higher ticket must be created before this session
ends for this `🚧` entry to be valid per spec-conventions."

No change to `spec-conventions.md` — the rule (todo/-or-higher ticket required for
🚧 entries) stays authoritative; enforcement shifts from gate to reminder.

Success: `write-spec` writes `🚧` entries without pausing for user confirmation.

### Phase 2: Add gate-suppression context to /proceed prefix-stage invocations

Modify `proceed/SKILL.md`: add an invariant — when invoking prefix stages via the
Skill tool, include natural-language override context in the args:

- For `/write-spec`: "Chained from /proceed — write any 🚧 entries without asking
  to defer (judge: idea-level is suppressed)."
- For `/write-ticket`: "Chained from /proceed — if /write-spec already ran (even as
  a no-op), treat spec coverage as satisfied and do not stop on judge: spec-gate."

Also update the Announce step (step 3) to note that gate-suppression context is
included in prefix-stage invocations, so the announced pipeline reflects actual
behavior.

Success: `/proceed` → `/write-spec` → `/write-ticket` completes without prompting
the user for confirmation at any prefix stage.
