---
title: Ticket Result editions
parent: 260513-epic-workflow-question-loop-hygiene
related-mental-model:
  - documentation-system
  - workflow-skills
  - git-workflow-tools
---

# Ticket Result editions

## Background

Ticket phase bodies are intentionally frozen after a `### Result` section is
added, but the current rule is too rigid for the implementation final gate. A
completed phase can receive user-requested tweaks before merge, and the workflow
needs an append-only way to capture those follow-up implementation passes
without editing the original Result text.

## Decisions

- Keep phase plan text frozen after the first `### Result`.
- Treat Result content as append-only rather than mutable.
- Add `#### Edition (<short-hash>) - YYYY-MM-DD` entries for later tweak passes.
- Preserve the original `### Result (<short-hash>) - YYYY-MM-DD` heading as the
  first reviewable implementation record for the phase.

## Phases

### Phase 1: Update ticket conventions and wrap-up behavior

Document Result editions and teach executor wrap-up to append an Edition when a
completed phase receives additional implementation work.

Acceptance criteria:

- Ticket conventions explain that prior Result and Edition entries are frozen.
- A first completion still adds `### Result (<short-hash>) - YYYY-MM-DD`.
- Later tweak passes append `#### Edition (<short-hash>) - YYYY-MM-DD` under the
  phase's Result area.
- `lead-implement` final gate wording no longer says completed Results block all
  follow-up tweak capture.

### Phase 2: Align tooling and detection

Update workflow tooling that detects ticket result changes so edition additions
are visible in commits and summaries.

Acceptance criteria:

- `git.commit` ticket update detection recognizes newly added Edition headings
  or otherwise reports the ticket update clearly.
- Documentation specs and mental models record the append-only Result edition
  rule.
- Existing tickets with plain Result sections remain valid.
