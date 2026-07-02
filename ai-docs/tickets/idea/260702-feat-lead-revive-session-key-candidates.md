---
title: lead-revive should offer in-memory session-key candidates on lost key
sage-review: blocked
---

# lead-revive should offer in-memory session-key candidates on lost key

## Context

Found during a v0.31.1 dogfooding pass. Session state (agenda, todo,
session-tree) is bound per session key, not per working root: an agenda set
under one key was invisible when reloading `workflow_manual` under a second
key minted for the same root. If a lead loses its session key (e.g. after a
compaction that drops the summary line), re-minting via `ferrule` silently
returns a clean, empty session — no error, no hint that other state exists for
that root. The lost key strands all of its agenda/todo/session-tree state with
no recovery path short of guessing or grepping transcripts.

This is a reasonable-expectation surprise: a caller who loses a key expects
either an error/warning or some way to reconnect to prior state for the same
root, not silent success on an empty session.

## Suggestion

In `lead-revive` (or the underlying MCP primitive it calls), if the MCP
process still holds transient in-memory session keys, surface a few of them as
recovery candidates. This requires no persistent-storage access, so it cannot
leak state across process restarts.

Constraints:
- **Same-root scoping is required.** Candidates must be restricted to session
  keys bound to the caller's current working root — never a global,
  cross-root list. Even though this is single-process, in-memory-only state
  (no persistent storage involved), a process can still hold keys for
  multiple unrelated working roots; without root scoping, candidate exposure
  would leak keys across roots that happen to share the process, not just
  across restarts.
- **Schema/description obfuscation, same posture as `ferrule`.** Per
  `260702-bug-lead-manual-sections-thin`, `ferrule`'s terse public schema is
  deliberate: it assumes only a lead agent should ever invoke it, never a
  subagent, so the schema stays uninformative and the real discipline lives
  in the lead-gated `workflow_manual` output. `lead-revive`'s own MCP tool
  schema/description should follow the same posture: assume it is attended to
  only by a lead agent whose own session has just been compacted, never by a
  subagent and never by a lead agent outside that post-compaction moment.
  Keep the public schema terse and non-descriptive of the recovery procedure;
  document the actual candidate-key recovery procedure only in the lead-gated
  `workflow_manual` output, alongside the ferrule discipline from
  `260702-bug-lead-manual-sections-thin`.

This is a complement to, not a replacement for, the existing "preserve the key
verbatim in the compaction summary" discipline — that remains the primary
path; this is a fallback when the summary path fails.

## Spec Impact

Target: `ai-docs/spec/workflow-skills.md`. Caller-visible change: `lead-revive`
can surface same-root in-memory session-key candidates on lost key, with the
recovery procedure documented only in the lead-gated `workflow_manual` output
(ferrule-style schema obfuscation), not in the public tool schema.
Contract-first spec: no.

## Blocked (2026-07-02)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Storage-model premise is factually wrong | critical | missing |
| 2 | No root-to-keys index exists; ticket doesn't scope how to build one | important | autonomous |
| 3 | No candidate selection/ordering/cap policy, and no expiry despite unbounded key retention | important | missing |
| 4 | Interaction with existing FAIL-LOUD no-restore path is unaddressed | important | missing |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | No phase sections | important |
| 2 | No verification expectations | important |
| 3 | "Same-root" scoping mechanism unspecified | important |
| 4 | Candidate surfacing mechanism unspecified | minor |

Design reviewer finding requires resolution before this ticket can proceed:
the ticket's core premise ("transient in-memory session keys," "requires no
persistent-storage access, so it cannot leak state across process restarts")
is factually wrong. The actual implementation
(`agents-plugin-tool/internal/mcp/session_auth.go:71-80`) stores one JSON
file per session key on disk under `<cache-root>/keys/<key>.json` with no
eviction — state already survives process restarts today, the opposite of
the ticket's stated security invariant. This changes the leak-surface
analysis (unbounded, cross-restart, on-disk candidate records rather than
process-lifetime-bounded in-memory ones) and must be re-authored before
sage review can pass. The reviewer also flagged unresolved candidate
selection/ordering/cap policy and an unaddressed interaction with the
existing FAIL-LOUD no-restore branch in `workflow_manual.go` that this
feature must slot into without reintroducing the leak that branch was
hardened against.
