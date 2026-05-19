---
title: Workflow question loop hygiene
related-mental-model:
  - workflow-skills
  - documentation-system
  - git-workflow-tools
completed: 2026-05-19
---

# Workflow question loop hygiene

## Scope

Improve the recurring discussion-to-implementation loop so conversational
checkpoints, ticket refresh, ticket result updates, and tool output defaults
match how the workflow is used during active design sessions.

This epic covers:

- a short, frequently callable spoken-question skill for blocker checks
  style design checks;
- a `lead-proceed` freshness gate that captures settled discussion into an
  existing related ticket before implementation starts;
- ticket Result edition support for post-implementation tweak loops;
- backlog capture for remaining human-readable tool output defaults.

## Non-Scope

- Do not implement the human-readable output work as part of the initial
  workflow-skill cleanup.
- Do not use this epic as an implementation target. Child tickets own concrete
  phases and ready promotion.
- Do not make `lead-proceed` inspect source code or broad documentation for the
  freshness gate; it should compare conversation state and ticket artifacts.

## Child Tickets

- `260513-feat-is-finished-yet-workflow-check` - done; added the frequent
  spoken question skill, now named `lead-check-blockers`, and the proceed
  ticket-refresh gate.
- `260513-feat-ticket-result-editions` - done; completed ticket Result
  sections receive append-only edition entries during tweak loops.
- `260513-feat-human-readable-tool-output` - done; added readable defaults for
  remaining MCP and CLI workflow tools.
- `260513-feat-tolerant-doc-find-queries` - done; added tolerant documentation
  lookup behavior for broad find queries and convention aliases.
- `260519-feat-proceed-implementation-dispatch` - done; added proceed-side
  implementation dispatch precheck and removed stale skeleton-routing text.

## Cross-Child Decisions

- Frequent checkpoint skills should stay as short as `lead-verify-discussion`:
  state when to use the skill, what evidence to consider, and what to report.
- The spoken-question skill uses `lead-check-blockers`; it should stay verb-like
  and oral rather than status-like.
- The skill should separate user-blocking design questions from autonomous code
  hygiene, implementation detail, and cleanup work.
- `lead-proceed` should fill missing ticket intent from the active conversation
  only, following the same conversation-capture spirit as `lead-write-ticket`.
- `lead-proceed` may choose implementation dispatch from conversation and
  ticket artifacts, but it must not inspect source or invoke implementation
  primitives directly.
- Ticket Result sections remain append-only after completion; tweak loops add
  edition entries rather than editing prior result text.

## Completion Criteria

- Done: all child tickets are implemented or intentionally dropped/deferred.
- Dropped: the workflow keeps the current manual discussion capture and Result
  freeze behavior.
- Deferred: human-readable tool output may remain a separate backlog item after
  workflow-skill and ticket-result behavior ships.
