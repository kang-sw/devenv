---
title: lead-revive should offer in-memory session-key candidates on lost key
related:
  260702-bug-lead-manual-sections-thin: shares the ferrule-style schema-obfuscation posture this ticket follows
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

This is new stateful capability, not a bugfix: the system today is fully
stateless per call (session keys persist on disk under
`agents-plugin-tool/internal/mcp/session_auth.go`, one JSON file per key, but
nothing about "the most recently active key" is tracked anywhere). Introduce
exactly one new piece of **process-lifetime, non-persisted, in-memory-only
scalar** — the "last-minted session key" — holding the single most recent key
returned by `ferrule`.

Design, fully settled:

- **Single scalar, unconditional overwrite.** Every `ferrule` call overwrites
  this scalar with its newly minted key. No history, no list, no candidate
  cap or ordering policy — there is only ever one value. It is lost on
  process restart (in-memory only, never written to disk). This deliberately
  replaces the earlier "enumerate several in-memory candidates from a
  registry" framing, which depended on a registry that doesn't exist.
- **`lead-revive`-only exposure.** The scalar is read/considered only from the
  `lead-revive` code path — never surfaced by `ferrule` itself or any other
  tool. This keeps `ferrule`'s existing contract intact (a fresh `ferrule`
  call still mints a clean, empty session with no leakage of prior state) and
  lets `lead-revive` distinguish "genuine post-compaction recovery attempt"
  from "fresh session start," since only the former path ever consults it.
- **Same-root guard, no index needed.** Each persisted session-key JSON file
  already records its bound working root
  (`agents-plugin-tool/internal/mcp/session_auth.go`). When `lead-revive`
  considers the last-minted-key candidate, it reads that candidate's
  persisted root field and compares it against the caller's current/declared
  working root. On mismatch, discard the candidate and fall back to today's
  existing recovery path unchanged. Because there is only ever one candidate
  to check, no root-to-keys index is needed.
- **Auto-adopt on match.** When the same-root guard passes, `lead-revive`
  auto-adopts the candidate rather than surfacing it for separate
  confirmation. Rationale: a wrong auto-adopt is bounded to "same root,
  different concurrent lead's live session" — not a cross-repo leak, and no
  worse than today's alternative of silently starting an empty fresh session.
  Requiring manual confirmation on every recovery adds friction for a case
  that is usually correct.
- **Accepted limitation (out of scope for this ticket).** In a same-root,
  multiple-concurrent-lead scenario (e.g., two terminals both working the
  same repo), the scalar reflects whichever lead most recently called
  `ferrule`, so `lead-revive` could hand a lead a different concurrent lead's
  live session state instead of its own. This is accepted for a lightweight
  recovery backstop and is not solved here.
- **Complement, not replacement.** The primary recovery path remains
  "preserve the session key verbatim across compaction"; this is only a
  fallback when that path fails.
- **Schema/description obfuscation, same posture as `ferrule`.** Per
  `260702-bug-lead-manual-sections-thin`, `ferrule`'s terse public schema is
  deliberate: it assumes only a lead agent should ever invoke it, never a
  subagent, so the schema stays uninformative and the real discipline lives
  in the lead-gated `workflow_manual` output. `lead-revive`'s own MCP tool
  schema/description should follow the same posture: assume it is attended to
  only by a lead agent whose own session has just been compacted, never by a
  subagent and never by a lead agent outside that post-compaction moment.
  Keep the public schema terse and non-descriptive of the recovery procedure;
  document the actual mechanism only in the lead-gated `workflow_manual`
  output, alongside the ferrule discipline from
  `260702-bug-lead-manual-sections-thin`.

## Phases

### Phase 1: last-minted-key scalar + same-root auto-adopt

Implement the in-memory last-minted-key scalar (set on every `ferrule` call,
process-lifetime only, never persisted), read it only from the `lead-revive`
path, compare its persisted root against the caller's current root, discard
on mismatch (fall back to the existing no-restore path unchanged), and
auto-adopt on match. Keep `lead-revive`'s public tool schema/description
terse per the obfuscation posture above; document the mechanism only in the
lead-gated `workflow_manual` output.

Verification expectations:
- A fresh `ferrule` call's own contract is unchanged: it still returns a
  clean, empty session regardless of the scalar's contents (regression check
  against the existing empty-session guarantee).
- `lead-revive` with a same-root last-minted-key candidate present
  auto-adopts it and restores that session's agenda/todo/session-tree state.
- `lead-revive` with a candidate bound to a different working root discards
  it and falls back to the existing no-restore (FAIL-LOUD) path unchanged.
- A process restart clears the scalar; no on-disk trace of "last-minted key"
  is added by this feature (the existing per-key JSON persistence in
  `session_auth.go` is unrelated and unchanged).

## Spec Impact

Target: `ai-docs/spec/workflow-skills.md`. Caller-visible change: `lead-revive`
auto-adopts a same-root, most-recently-minted session key as a recovery
candidate when the caller's key is lost, tracked via a single in-memory,
non-persisted, process-lifetime scalar (not a registry or index), with the
recovery mechanism documented only in the lead-gated `workflow_manual` output
(ferrule-style schema obfuscation), not in the public tool schema.
Contract-first spec: no.

## Blocked (2026-07-02)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | "lead-revive code path" does not exist as a distinct server-side surface | critical | missing |
| 2 | Two "lost key" entry points in `workflow_manual` are not disambiguated | critical | missing |
| 3 | Second mint call site not covered by "every ferrule call" framing | important | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Missing `## Background` heading (uses `## Context` instead) | minor |
| 2 | Missing `related:` frontmatter link to referenced ticket | important |

Design reviewer finding requires resolution before this ticket can proceed:
`lead-revive` is a client-side-only skill
(`agents-plugin/skills/lead-revive/SKILL.md`) with no dedicated Go MCP tool
handler — it just calls `workflow_manual(session_key: <recovered-or-sentinel>)`.
There is no separate "lead-revive schema" to obfuscate, and no single
unambiguous branch to hook the last-minted-key check into. `workflow_manual`
(`agents-plugin-tool/internal/mcp/workflow_manual.go`) has two structurally
different "lost key" branches: (1) a syntactically valid but unresolvable key
hits a FAIL-LOUD branch with no manual body, and (2) the reserved
`obsidian-latch` sentinel with no root hits a FRESH-no-root branch that
renders the full manual — the actual path `lead-revive`'s own skill text
drives when the key is truly lost. The ticket does not say which branch (or
both) should consult the last-minted-key scalar, nor how "the caller's
current working root" is obtained in the sentinel/no-root case for the
same-root guard to compare against. It must also account for a second mint
call site (`workflow_manual.go`'s FRESH-with-root branch calls
`s.sessions.mint(...)` directly, bypassing `ferrule`/`handleLeadLogin`) that
the "every ferrule call" framing misses. This needs a scope decision — new
dedicated MCP tool vs. hooking into `workflow_manual`'s existing dispatch and
shared schema — before re-authoring.
