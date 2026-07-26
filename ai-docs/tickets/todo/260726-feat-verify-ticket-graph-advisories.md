---
title: "Ticket-graph advisories at verify: parent-board nudge and cross-reference integrity"
spec:
  - 260723-git-commit-ticket-verify-gate
  - 260723-tickets-verify-tool
related:
  260723-epic-ticket-write-reshape: refines its "verify = mechanical floor" boundary from intra-file to cross-file reference resolution, and adopts its must-not-forget action-time framing
  260724-bug-ws-git-commit-verify-fails-on-staged-rename: path-based emission gating is chosen specifically so this feature does not depend on staged-rename detection
sage-review-design: required
---

# Ticket-graph advisories at verify: parent-board nudge and cross-reference integrity

## Background

Every current `wsdoc.TicketVerify` guardrail is **intra-file**: stem pattern,
status directory, frontmatter fence, ready sage posture, close-date field,
phase/Result headings, spec-address. None reads another ticket.

Meanwhile `TicketInfo` already parses `Parent`, `Related`, `Specs`,
`SpecRemoves`, and `Plans`, but the only consumer is a display flag
(`internal/mcp/server.go:2849`, `flags = append(flags, "parent="+ticket.Parent)`).
The whole cross-file graph is parsed and then discarded.

Two consequences observed on the live board (436 tickets scanned):

1. **No closure nudge.** An epic whose children have all landed goes unnoticed.
   `260723-epic-ticket-write-reshape` satisfies its own stated Completion
   Criteria today and nothing surfaced that.
2. **Reference drift is invisible.** A `related:` entry pointing at a
   nonexistent stem, an epic body listing a child that carries no `parent:`
   back-link, and an epic with zero `parent:`-linked children all pass verify
   silently.

This ticket treats verify as a **ticket compiler**: cross-file reference
resolution is still a pure function of the file set, so it belongs to the same
deterministic floor as the intra-file checks.

## Decisions

### Host and trigger

- Host is `wsdoc.TicketVerify`, reached through `ws/git.commit`'s existing
  validation slot — not `ws/tickets.close`.
- Rationale: `tickets.close` is bypassable (agents fall back to `git mv`), while
  a `git mv` still produces a commit that routes through the verify slot. Verify
  is the stronger trigger.
- Rejected: emitting from `tickets.close`'s return only. It was the original
  design and is strictly weaker on trigger coverage.

### Severity follows reversibility, not defect gravity

- **No new `Findings`.** Every check added here is non-blocking.
- A commit is reversible (`git commit --amend`); a push is not. A gate sitting on
  a reversible action should let the work land and hand back the remedy.
- The consumer is an agent, which can act on a returned instruction. Blocking
  buys nothing that advisory prose plus a remedy does not.
- Consequence: the earlier Finding/Warning severity split for these checks was
  dropped. The distinction moves into **message shape** (below).
- Existing verify `Findings` semantics are untouched — this ticket adds no
  blocking behavior and changes no existing guardrail's severity.

### Message shapes

Two shapes, both non-blocking:

- `FIX:` — the defect is unambiguous and the remedy is mechanical. State the
  remedy and the amend recipe.
- `CHECK:` — resolving it needs judgment. State the observation and the question;
  do not prescribe.

`ws/git.commit` exposes no amend primitive (`grep amend` over the Go tree returns
zero hits), so the recipe names native `git commit --amend --no-edit`, per the
workflow manual's rule that native Git covers operations with no ws primitive.

### Emission split (three-way)

| Output | Fires when |
|---|---|
| Integrity checks (`FIX:`/`CHECK:` reference resolution) | always |
| Board-block ACTION lines (all children closed; epic with zero children) | always |
| Board-block sibling listing (`N of M open` plus the list) | only when a verified path sits under `.done/` or `.dropped/` |

Verify runs on every ticket-touching commit, far more often than a close. The
sibling listing is where the volume is: `260514-epic-ws-web-dashboard-mvp` has
53 children, so an ungated listing would attach 8+ lines to every commit
touching any of them.

Gating is on the **path's status directory**, not on staged-rename detection.
Accepted imprecision: appending an `#### Edition` to an already-closed ticket has
a `.done/` path and will emit the sibling listing spuriously. Accepted because
editing a closed ticket is rare and the output is non-blocking. Rejected
alternative: detect a staged rename into `.done/` — rejected to avoid coupling
with `260724-bug-ws-git-commit-verify-fails-on-staged-rename`.

### Check set

Resolved against `parent:` frontmatter, never the epic body's prose list.

| Check | Shape |
|---|---|
| `parent:` names a nonexistent stem | `FIX:` |
| `related:` key names a nonexistent stem | `FIX:` |
| `parent:` chain contains a cycle | `FIX:` |
| `parent:` target is not an `epic` | `CHECK:` |
| `epic` with zero `parent:`-linked children | `CHECK:` |
| Open child of an open epic not mentioned in the epic body | `CHECK:` |
| Stem listed in the epic body carrying no `parent:` back-link | `CHECK:` |
| Ancestor board status (`N of M open`, or all-closed) | board block |

**Closed children are excluded from both mention checks.** Unscoped, the
"child not mentioned in the body" check yields 59 hits dominated by `.done`
children (32/53 on `260514`, 20/31 on `260605`); an epic body is not expected to
list completed children, and `_index.md` pruning doctrine puts completed work in
Git history. Scoped to open children of open epics it yields 7.

Rejected check: **`related:` symmetry.** `ticket-conventions` states "No
cross-link updates needed", so asymmetry is legal by convention.

Rejected check: **`plans:` path existence.** Measured 3 dangling paths plus 2
literal `null` placeholders, all in `.done/` tickets. Historical residue with no
attention value. (An initial scan reported 26; that count conflated `plans:`
with `skeletons:`, which share the `phase-N:` key shape.)

Non-goal: **`related:` pointing at a `.dropped` ticket** (8 hits). Most already
acknowledge the drop in their relationship note, so a check would mostly restate
what the note says.

### Ancestor walk

- Walk `parent:` upward from each verified ticket, unbounded depth, with a cycle
  guard. Current board maximum depth is 2.
- An ancestor already in `.done/`/`.dropped/` emits a note, not an action: the
  child closed after its parent, and the remedy is editing the parent's
  `### Result`, never reopening it.
- The walk loads each ancestor's child set and body, which is exactly the data
  the epic-shape and mention-divergence checks need. This is why all of these
  checks belong to one implementation rather than separate tickets: separating
  them would implement the same traversal three times.
- A verified ticket with no `parent:` produces **no `## Parent Board` section at
  all** — not an empty one. Most tickets have no parent, so an empty section would
  attach to nearly every commit.
- `TicketVerify` receives a path list, so one commit can carry several tickets
  sharing an ancestor. Ancestors are deduplicated by stem: each ancestor's block
  is emitted once per verify call, however many verified children point at it.

## Output Format

Settled. ASCII only. Appended to the verify/commit advisory output.

Child rows put **status first, padded to 8**, then `|`, then the stem, then an
optional parenthetical. Status is a closed set with a maximum width of 8
(`.dropped`), so padding is fixed-cost; the stem is 30-60 characters and varies,
so it goes last and needs no padding. Status is also the field being scanned
("what is left"), so it takes the fixed column. No bullet markers: indentation
already groups the rows.

Row sort order is fixed so the status column reads monotonically: open as
`ready` -> `todo` -> `idea`; closed as `.done` -> `.dropped`.

Sibling listing is capped at 5 rows followed by `... +N more open (<status>/)`.
Completed children are never listed.

Ancestors are labelled `Parent [N]:` by depth.

All children closed:

```
## Parent Board

Parent [1]: 260723-epic-ticket-write-reshape [todo] - all 3 child tickets closed
    .done   | 260723-feat-ticket-write-verify-commit-gate
    .done   | 260723-feat-ticket-system-concept-doc
    .done   | 260723-feat-ready-spec-address-hard-gate  (just now)

  ACTION: Check whether this epic can be closed. Read its `## Completion
    Criteria` first - "all children closed" is not itself the closure test,
    and this epic's criteria explicitly allow closure with deferred children.

  No further ancestors.
```

Siblings remain (no ACTION line - not closable):

```
Parent [1]: 260605-epic-ws-playbook-factory-pivot [todo] - 3 of 31 child tickets still open
    todo    | 260624-epic-pre-release-cleanup  (epic)
    todo    | 260703-chore-windows-branch-pinned-acceptance
    idea    | 260626-research-ws-todo-stack-nesting-model
```

Cap applied:

```
Parent [1]: 260514-epic-ws-web-dashboard-mvp [todo] - 8 of 53 child tickets still open
    todo    | 260525-feat-ws-dashboard-server-scoped-operation-forwarding
    todo    | 260525-feat-ws-dashboard-document-polishing-backlog
    todo    | 260525-feat-ws-dashboard-workroot-polishing-backlog
    todo    | 260620-feat-ws-dashboard-agent-client-activity-sources
    idea    | 260523-feat-ws-dashboard-main-session-activity-source
    ... +3 more open (idea/)
```

Ancestor already closed:

```
Parent [1]: 260503-epic-agents-plugin-skill-porting [.done] - parent already closed

  NOTE: This ticket closed after its parent. No action needed. If the parent's
    `### Result` should mention this work, edit the parent's Result; do not reopen.
```

Multi-level chain:

```
Parent [1]: 260624-epic-pre-release-cleanup [todo] - all 7 child tickets closed

  ACTION: Check whether this epic can be closed. Read its `## Completion
    Criteria` first.

Parent [2]: 260605-epic-ws-playbook-factory-pivot [todo] - 3 of 31 child tickets still open
  (unchanged by this close; shown for chain context)
    todo    | 260703-chore-windows-branch-pinned-acceptance
    idea    | 260626-research-ws-todo-stack-nesting-model
    todo    | 260624-epic-pre-release-cleanup  (epic, closable - see above)
```

Integrity advisories:

```
FIX:   dangling related: `260513-harness-local-agent-tier-config` does not exist
       (and `harness` is not a valid category). Remove or correct the entry, then
       `git commit --amend --no-edit`.

CHECK: 260624-epic-pre-release-cleanup has no parent:-linked children while its
       body reads "Workset grouping cleanup items". Either link the intended
       children, or this board is a workset - note that changing category
       requires a new stem.
```

## Constraints

- `wsgit.Verifier` is `func(root string, paths []string) error`, which can carry
  a veto but not advisory text. `internal/mcp/server.go:2692` `verifyAdapter`
  returns `nil` whenever `result.OK`, discarding `Warnings` entirely. Phase 1
  must open that channel before Phase 2 has anywhere to write.
- `wsgit` must not import `internal/wsdoc` (`{#260720-wsdoc-commit-boundary}`).
  Any channel widening preserves that boundary — the adapter shape exists for
  this reason.
- The `Verifier` hook runs after staging, before the commit lands, so at hook
  time there is nothing to amend. Advisories therefore ride `ws/git.commit`'s
  **response**, after the commit exists, which is what makes the amend recipe
  valid.
- Cross-file checks resolve against frontmatter only. Prose lists are compared
  against frontmatter but are never the source of truth.
- All emitted text is ASCII.

## Spec Impact

Phase 1 needs no new spec text: `{#260723-git-commit-ticket-verify-gate}` already
states that a soft warning "does not block the commit; it lands with the warning
surfaced". The implementation does not do that, so Phase 1 closes a
spec/implementation divergence against text that already exists.

Phase 2 adds caller-visible output that no anchor covers yet: the `## Parent Board`
block, the `FIX:`/`CHECK:` advisory shapes, the check set, and the
`.done`/`.dropped` path gating. Target spec area is
`{#260723-tickets-verify-tool}` (verify's guardrail set) with a cross-reference
from `{#260723-git-commit-ticket-verify-gate}` (what the commit response carries).

Contract-first spec: no. The settled output format lives in this ticket's
**Output Format** section, so pre-writing it into the spec would restate the
phase rather than stabilise anything. Spec text lands at implementation closeout.

## Prior Art

- `internal/wsdoc/tickets_verify.go:131` `addWarning("unresolved-phases", ...)`
  is the existing soft-warning precedent, including its `internal/mcp/server.go:2679`
  tip mapping.
- `internal/wsdoc/tickets_verify.go:19-26` documents that `OK` is true only when
  `Findings` is empty and that `Warnings` never affect `OK` — the two-severity
  model already fits, so no new result field is required.
- `internal/mcp/server.go:2708` `formatTicketVerify` already renders warnings for
  the standalone `tickets.verify` tool; Phase 1 needs the equivalent on the
  commit path, not a new renderer concept.

## Phases

### Phase 1: Surface verify warnings through the git.commit response

Open the advisory channel that Phase 2 writes into.

Today `Warnings` are visible only when `ws/tickets.verify` is called directly.
Through `ws/git.commit` they are silently discarded, so the pre-existing
`spec-address` and `unresolved-phases` warnings never reach the caller at commit
time.

This is a **spec/implementation divergence**, not a missing feature:
`{#260723-git-commit-ticket-verify-gate}` already specifies that a soft warning
"does not block the commit; it lands with the warning surfaced". Phase 1 makes the
implementation match. Its independent value is that two already-shipped soft
guardrails start reaching the caller at the moment they matter.

Widen the verify-to-commit channel so `ws/git.commit`'s response carries warning
text alongside the commit result, without letting warnings affect `OK` and
without importing `internal/wsdoc` into `wsgit`. Whether the `Verifier` signature
grows an advisory return or `git.commit` collects advisories on a second path is
an implementation choice; either is acceptable if the boundary and the
non-blocking semantics hold.

Verification boundary: an existing warning class (`unresolved-phases` on close,
or `spec-address` on a `ready/` ticket) appears in `ws/git.commit`'s response,
the commit still lands, and `OK` is unchanged. No new check is added in this
phase.

### Phase 2: Ticket-graph advisories

Add the ancestor walk, the board block, and the cross-reference integrity checks
per **Decisions** and **Output Format**, emitted through Phase 1's channel.

One traversal serves all of it: walking `parent:` upward loads each ancestor's
child set and body, which is the input for the board block, the epic-shape check,
and both mention-divergence directions.

Verification uses live board defects rather than synthetic fixtures. All three
are present as of 2026-07-26 and are deliberately left unfixed for this purpose:

| Fixture | Expected output |
|---|---|
| `260611-research-ws-per-role-delegation-tuning-config` has `related: 260513-harness-local-agent-tier-config`, which does not exist | `FIX:` |
| `260723-research-spec-collocator-subagent` is listed in `260723-epic-ticket-write-reshape`'s `## Child Tickets` but carries no `parent:` | `CHECK:` |
| `260624-epic-pre-release-cleanup` is an `epic` with zero `parent:`-linked children, listing 7 items via `related:`, whose body opens with "Workset grouping cleanup items" | `CHECK:` |

Additional expectations to verify: `260630-epic-skill-playbook-diet` reports its
one open child `260702-research-destructive-dedup-methodology` as unmentioned in
the body; the 42 closed children of `260514-epic-ws-web-dashboard-mvp` produce no
mention findings; and a commit touching a `todo/` ticket emits no sibling listing
while a commit landing a ticket in `.done/` does.

Fix the three fixtures after the checks are confirmed to catch them.

## Out of Scope

- Changing any existing verify `Finding` into a warning, or vice versa. The
  reversibility principle motivates this ticket's new checks; retro-applying it
  to shipped blocking guardrails would change established commit-gate semantics
  and needs its own decision.
- Adding amend support to `ws/git.commit`.
- Staged-rename detection, and therefore any dependency on
  `260724-bug-ws-git-commit-verify-fails-on-staged-rename`.
- Relabelling `260624-epic-pre-release-cleanup` from `epic` to `workset`. Category
  lives in the stem, so conventions require a new ticket plus a `.dropped/` move.
  This ticket only surfaces the inconsistency.
- A board-health surface outside verify (for example in `project_tree`).
- `related:` symmetry enforcement, `plans:` path existence, and
  `related:`-to-`.dropped` acknowledgement, each rejected under **Check set**
  with its measurement.
