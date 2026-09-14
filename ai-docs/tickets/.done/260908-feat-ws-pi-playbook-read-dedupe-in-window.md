---
title: "Stateless in-window dedupe for `playbook.read` and `ws-skill`: a repeat read of an unchanged body returns a short pointer instead of the body"
related:
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: same motive (lead context diet); this ticket trims repeat tool output rather than the tool list
  260904-feat-ws-pi-side-thread-fork-question-surface: forks inherit the lead's history as their prefix, so the dedupe applies to a fork's replayed reads too
spec:
  - pi-adapter-runtime
  - 260909-pi-visible-playbook-read-dedupe
plans:
  phase-1: ai-docs/.plans/2026-09/09-2336-260908-feat-ws-pi-playbook-read-dedupe-in-window.md
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: ba84d64a2bb52bdc
sage-review-design-reviewed: ba84d64a2bb52bdc
completed: 2026-09-10
---

# Stateless in-window dedupe for `playbook.read` and `ws-skill`: a repeat read of an unchanged body returns a short pointer instead of the body

## Background

Skill prose tells the lead to `playbook.read(...)` at every skill entry, and
leads do re-read a playbook that is already sitting in their context. In
the local Pi session logs (all session directories, duplicates counted only
inside the same uncompacted window):

| tool | calls | same-window repeats | mean result size |
|---|---|---|---|
| `ws__playbook_read` | 14 | 4 | 8.9k chars |
| `ws__playbook_print` (older name, same family) | 51 | 13 | 10.6k chars |
| `ws-skill` | 33 | 12 | 3.0k chars |
| `ws__playbook_render` | 77 | 29 | 0.2k chars |

Roughly one read in four is a repeat of a body the model can already see,
each worth ~2k tokens that then ride in the prefix of every later turn.
`playbook.render` results are tiny and out of scope.

Owner question (2026-09-08): can the extension know the playbook is already
in the uncompacted window and answer a repeat with "you already read this
playbook"? Yes; and it can do so without any stored state, so it survives
compaction, rewind, and tree branching by construction.

## Decisions

- **Stateless detection from the session tree.** At `execute()` time for
  `ws__playbook_read` (and `ws-skill`), read
  `ctx.sessionManager.buildContextEntries()` (Pi's public construction of
  the active branch and compaction cut, honoring branch summaries). Scan the remaining
  entries for an earlier successful `toolResult` of the same tool whose
  call had the same tool-specific key: `name` plus the `context` substitution
  map for `playbook.read`, and `name` plus `args` for `ws-skill`. Compare maps
  semantically, independent of key ordering; `session_key` is ignored. Do not
  match across the two tool families. The current call's own
  `toolCallId` is excluded. No adapter-side flags, counters, or caches:
  the session tree is the only state, and it already reflects
  compaction (summarized reads are gone), rewind (the abandoned branch is
  not on the leaf→root path), and fork prefixes (the inherited reads are
  on the path).
- **Content-compared, not name-compared.** The MCP call still runs (local,
  milliseconds). The short reply is returned only when the fresh body is
  byte-identical to the earlier result's text. A playbook edited during
  dogfood therefore reaches the model on the next read with no `force`
  parameter and no schema divergence from ws-mcp.
- **The short reply.** One paragraph: the playbook is unchanged and was
  already returned N tool calls ago (naming the earlier `toolCallId`),
  plus the body's heading list (`#`/`##`/`###` lines, in order) so the
  model can re-anchor without the full text. It is a normal text result,
  not an error; the `tool_call` hook's `block` path was rejected because
  its `reason` lands as an error-flavored result.
- **Third call passes through.** Count successful same-content occurrences,
  including the second call's short pointer, rather than requiring two full
  identical text results. First call returns the full body, second returns the
  pointer, and third and later matching calls in the same visible window return
  the full body. This is the safety valve for a model that lost the thread.
  Pointer results must carry enough verifiable provenance to resolve their
  original full-body toolCallId and key from the current context alone. Only
  count a pointer when that referenced successful full result is still visible
  and byte-identical to the fresh response. Never trust a coincidentally similar
  ordinary result or a reference outside the active context. If provenance is
  missing or ambiguous, return the full body. No adapter-side history cache.
- **Scope.** `ws__playbook_read` and `ws-skill` only. `playbook.render` and
  the workflow-manual snapshot (`lead-bootstrap.ts`, already served from
  the system prompt) are untouched. ws-mcp is untouched (adapter-only,
  per the Pi-track authoring rule).

Rejected: a per-session `Set` of read names cleared on `session_compact`
(has to be re-derived on rewind/branch/fork anyway, and can drift from
what the model actually sees); a `tool_call` hook block (error-flavored
result); an ws-mcp-side change (host-neutral rule; and ws-mcp cannot see
Pi's window).

## Constraints

- Adapter-only, in `agents-plugin-pi/src/` (a small pure module for the
  window scan and reply rendering, wired from the ws tool `execute()` path
  in `bridge.ts` and from `ws-skill` in `lead-skills.ts`).
- The window scan must use Pi's own context construction
  (`buildContextEntries` or the equivalent public path), never a private
  re-implementation of the compaction cut.
- Confirm in a fixture whether Pi appends the assistant message carrying
  the `toolCall` before or after `execute()` runs; the current-call
  exclusion by `toolCallId` covers both orders, but the test must pin the
  real one.

## Spec Impact

Add a short subsection under the tool-exposure section of
`ai-docs/spec/pi-adapter-runtime.md` describing the in-window dedupe:
the stateless window scan, the byte-identical condition, the reply shape,
the third-call pass-through, and the two tools it covers.

## Phases

### Phase 1: Window scan, short reply, wiring, tests

Implement the pure scan (entries → prior identical results for a key),
the reply renderer (heading list + pointer), and the wiring for both
tools. Update the spec subsection.

Verification:

- Fixture-driven tests over session entry lists: first read returns the
  body; identical second read returns the pointer naming the earlier
  `toolCallId`; a changed body returns the new body; different `context`
  maps do not dedupe; a compaction entry between the reads returns the
  body; a rewind (leaf moved to an earlier entry, the earlier read on the
  abandoned branch) returns the body; a fork-style prefix containing the
  read dedupes; the third and fourth identical calls return the body even though the
  second returned a pointer; missing/forged/stale pointer provenance falls back
  to the body; different ws-skill args do not dedupe; the current
  call's own entry never counts.
- `ws-skill` covered by the same fixtures.
- Owner-run dogfood: a lead session that re-enters `lead-discuss` or
  `lead-proceed` receives the pointer and continues the procedure without
  a third read; the adapter test suite passes.

### Result (e7d7140c) - 2026-09-10

Implementation checkpoint through `ee68fa46`; owner-run dogfood remains
pending. The installed Pi SDK exposes the specified public context API.
Fixtures use its SessionManager for compaction, rewind, and fork-prefix
behavior, and the installed agent-core execution loop with a synthetic stream
to observe the finalized assistant call before tool execution. No model or
network call is required by these fixtures.

Review dispositions:

- C1 [fixed]: malformed, stale, ambiguous, and entirely missing pointer
  provenance falls back to the fresh body. The missing-envelope reproduction
  remained after relay 1 and was resolved by Critical review 3 after relay 2.
- C2 [fixed]: unknown, unreadable, or invalid skills retain their existing
  failure response and bypass dedupe; Critical review 2 resolved this finding.
- I1 [fixed], implementer report: pointers include the original ID, distance
  in tool calls, and headings.
- T1 [fixed], implementer report: production bridge and ws-skill registration
  fixtures exercise successful repeated reads; failure bypass is also covered.
  The first report overstated ws-skill coverage. The lead required the missing
  success-path fixture in relay 2 as a bounded exception to the Important
  one-relay budget, because the implementation plan explicitly required it.
  No additional Important re-review was performed.
- T2 [fixed], implementer report: malformed, forged, stale, ambiguous, and
  missing provenance are distinguished from normal successful repeats.
- K1 [fixed], separate lead finding: the entire substitution map participates
  in the key, including a nested `context.session_key`; only the outer routing
  credential is ignored. Regressions cover both cases.
- Fit review was clean. Critical review 3 was clean within its scope. Native
  thread limits rejected fresh Fit/Test reviewer spawns, so separate existing
  sessions with no implementation ownership performed those partitions;
  the Test fallback retained its selector model rather than the requested
  medium binding. No independence from the implementer was lost.

Final verification at `ee68fa46`: 111 focused tests passed. Full adapter suite:
1,428 tests, 1,298 passed, 130 pre-existing failures, zero skipped. The failures
remain 129 Linux-specific SDK path fixtures on macOS and one stale `ws-ask`
exposure expectation; `ws-ask` remains hidden. The original plan command
`node --test test/` is rejected by Node 25, so the plan now records the verified
`node --test test/*.test.ts` command. No suite-wide pass or owner acceptance is
claimed. The implementation stayed within Pi source/tests; shared ws-mcp,
playbook text, schemas, and workflow-manual behavior were not changed.

### Phase 2: Make the pointer action-oriented and remove agent-facing provenance noise

Owner dogfood confirmed that the second unchanged read suppresses the full body and preserves enough context to continue, but the pointer mixes useful retrieval cues with machine-only provenance. Retain the unchanged/freshness signal, tool-call distance, and heading list. Replace the passive `available from successful tool call` sentence with the action-oriented contract:

> The result is unchanged. Continue using the full result already present N tool calls ago; do not read it again.

Do not repeat the long original `toolCallId` in agent-facing prose when the provenance envelope already carries it. Minimize any remaining model-visible provenance payload where that can be done without weakening stateless pointer validation, original-result resolution, or the third-read safety valve. Human-facing appearance is secondary to agent signal density: the output should spend semantic tokens on reuse and continuation, not on identifiers the agent cannot dereference.

Verification:

- The unchanged second read returns the action-oriented sentence, distance, and headings without a prose `toolCallId`.
- The lead continues the loaded procedure without a third read.
- Forged, stale, ambiguous, and missing provenance still fall back to the full body.
- The third and later matching calls retain the existing full-body safety behavior.
- Focused renderer, bridge, and `ws-skill` fixtures pass.

### Result (22f7d05f) - 2026-09-10

Changed repeated unchanged reads to tell the agent to continue with the full
result already present in context and not read it again. The pointer retains
the tool-call distance and heading summary while removing the long original
`toolCallId` from agent-facing prose. Structured provenance, freshness and
ambiguity fallback validation, and the third-read full-body safety behavior
remain unchanged.

Focused renderer, bridge, and `ws-skill` verification passed 111 tests. Owner
live dogfood confirmed that the second unchanged read suppresses the duplicate
body, gives enough direction to continue without a third read, and emits the
action-oriented pointer with no agent-facing provenance noise. The owner
accepted the resulting workflow behavior.


## Resolution (2026-09-10)

Phase 2 landed in `22f7d05f`. Focused verification passed 111 tests, and owner live dogfood confirmed the action-oriented unchanged-result pointer supports continuation without a third read while preserving validation and safety behavior.
