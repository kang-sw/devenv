---
title: "Ticket-graph advisories at verify: parent-board nudge and cross-reference integrity"
spec:
  - 260723-git-commit-ticket-verify-gate
  - 260723-tickets-verify-tool
related:
  260723-epic-ticket-write-reshape: refines its "verify = mechanical floor" boundary from intra-file to cross-file reference resolution, and adopts its must-not-forget action-time framing
  260724-bug-ws-git-commit-verify-fails-on-staged-rename: path-based emission gating is chosen specifically so this feature does not depend on staged-rename detection
sage-review-design: required
sage-review-completeness: required
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
2. **Reference drift is invisible.** An unresolvable `parent:` or `related:` stem,
   a `parent:` cycle, and a `parent:` pointing at a non-epic all pass verify
   silently. All are currently at zero on the live board — the intended steady
   state for a compile-style guard, and the reason this half of the work is
   verified synthetically rather than against the board.

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

**The amend recipe is appended by the `git.commit` response layer, not by the
check text.** `wsdoc.TicketVerify` also backs the standalone `ws/tickets.verify`
tool, where nothing has been committed and an amend instruction would be
nonsense. Check messages therefore state the remedy commit-neutrally ("Remove or
correct the entry"); only the commit path adds the amend sentence. This also
preserves `{#260723-tickets-verify-tool}`'s guarantee that "a standalone call and
the commit gate return the same verdict for identical input" — the verdict is
identical; only the commit-path presentation adds the recipe.

### Emission split (three-way)

| Output | Fires when |
|---|---|
| Integrity checks (`FIX:`/`CHECK:` reference resolution) | always |
| Board-block ACTION line (ancestor whose children have all closed) | always |
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

**Frontmatter only. No check reads a ticket body.** `parent:` is the sole
authority for the child edge set, and the epic body is not consulted at all — see
**No epic-body checks** below for why every body-reading candidate was rejected.

**Stem resolution spans two namespaces.** A `related:` or `parent:` key resolves
against ticket stems **union spec anchor stems** (`{#YYMMDD-slug}` in
`ai-docs/spec/`). Measured: of 10 `related:` keys that are not ticket stems, 4 are
spec anchors — `260513-harness-local-agent-tier-config` in three tickets and
`260505-lead-skill-namespace-surface` in one. Pointing a ticket's `related:` at a
spec anchor is an established repeated pattern, so resolving against tickets only
would emit false `FIX:` advisories against deliberate references.

The remaining 6 are genuinely unresolved (`260524-mcp-actor-setup-bootstrap` in
four tickets, `260611-bug-rsrc-manifest-regen-missed` — a truncation of the real
`260611-bug-rsrc-manifest-regen-missed-after-shipped-edit` — in one, plus one
more), and **all 6 sit in `.done`/`.dropped`**. There is therefore **no live
`FIX:` instance on any open ticket**, which is why Phase 2's durable verification
must be synthetic rather than board-derived.

**Integrity advisories are capped** at 5 per verified ticket, followed by
`... +N more`, mirroring the sibling-listing cap.

| Check | Shape | Live hits |
|---|---|---|
| `parent:` names an unresolvable stem | `FIX:` | 0 |
| `related:` key names an unresolvable stem | `FIX:` | 0 open (6 in `.done`/`.dropped`) |
| `parent:` chain contains a cycle | `FIX:` | 0 |
| `parent:` target is not an `epic` | `CHECK:` | 0 |
| Ancestor board status (`N of M open`, or all-closed) | board block | n/a |

"Unresolvable" means the stem is neither a ticket stem nor a spec anchor stem,
per the two-namespace rule above.

**Every integrity check currently has zero live hits.** That is the intended
steady state for a compile-style guard, not evidence the checks are pointless —
but it does mean the board cannot verify them, so Phase 2's verification is
entirely synthetic.

### No epic-body checks

**Epic bodies stay format-flexible and no check reads them.** Three body-reading
candidates were designed, measured, and rejected. Recording why matters, because
each looked well-grounded until measured.

**Rejected: "open child of an open epic not mentioned in the epic body."**
`tickets.template(type: "epic")` serves `## Child Tickets` with `- Planned: <child
ticket description>` as its own example line, so the section is a planning board
that may name children *before they exist* — not a mirror of the `parent:` edge
set. The template asserts no completeness obligation, so this check would invent
one. It is also the hand-maintained duplication the section does not need: for
children that already exist, `parent:` holds the edge set authoritatively, and
re-listing their stems in prose is a second copy to keep in sync. Measured 7 hits
scoped to open children of open epics (59 unscoped, dominated by `.done`
children); rejected on the reasoning, not the volume.

**Rejected: "body-listed stem carrying no `parent:` back-link."** This is the
opposite direction and initially survived — the body asserts an edge the
authoritative field does not carry, which needs no completeness and no convention
change. It dies on extraction scope. Heading-scoped to `## Child Tickets` it
yields 6 hits, but coverage then depends on the author happening to use that
heading name (2 of 7 open epics do not), which is exactly the format rigidity
being avoided. Widened to the whole body it yields 19 and becomes wrong in
principle: a stem in an epic body can be a research anchor
(`260605` names `260605-research-ws-native-subagent-pivot`), the epic's own parent
(`260616` names `260605`), an explicitly excluded item (`260624`'s
`## Dropped from workset`), or a stem the prose **declares not to be a child** —
`260723`'s body reads "**Delegation/fork reshape** is a sibling standalone, not a
child of this epic". No extraction rule can read that sentence, so the check would
contradict the document it is validating. The two-namespace rule is also required
here: `260605`'s `## Child Tickets` names four stem-shaped tokens that are spec
anchors (`260609-playbook-tools`, `260609-playbook-harness-rendering`,
`260609-rsrc-playbook-distribution`, `260619-session-key-lineage-children`).

**Rejected: "`epic` carrying `### Phase N:` headings"** (1 live hit,
`260630-epic-skill-playbook-diet`). Mechanically clean and backed by an explicit
convention, but it is a heading check on an epic body, and it addresses the wrong
target: `260630`'s defect is not that it has phase headings but that a methodology
document lives in an epic — six non-template sections, of which `### Phases` is
one. A check on the heading catches the smallest symptom. This belongs in epic
convention prose, not in verify.

**Rejected: epic template conformance** (non-template `## ` sections). Measured
perfectly bimodal — 5 of 7 open epics have exactly zero non-template sections,
the other two have 6 and 2, with nothing in between. Precise, but `260630`'s
`## Out of Scope` is synonym drift from `## Non-Scope` rather than misplaced
content, and a synonym allowance list makes the check fuzzy. Both hits are already
visible without it.

Rejected check: **`related:` symmetry.** `ticket-conventions` states "No
cross-link updates needed", so asymmetry is legal by convention.

Rejected checks: **workset structural violations.** Conventions forbid worksets
from carrying implementation phases or `parent:`-linked children. Measured across
all 437 tickets: zero violations of either. Doubly moot — the `workset` category is
itself slated for retirement now that goal-loop batching over `ready/` absorbs its
grouping role, so no new code should encode workset rules.

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
- The walk needs each ancestor's frontmatter and each ancestor's child set — the
  same graph load the integrity checks need, which is why board block and integrity
  checks are one implementation rather than two tickets.
- A verified ticket with no `parent:` produces **no `## Parent Board` section at
  all** — not an empty one. Most tickets have no parent, so an empty section would
  attach to nearly every commit.
- `TicketVerify` receives a path list, so one commit can carry several tickets
  sharing an ancestor. Ancestors are deduplicated by stem: each ancestor's block
  is emitted once per verify call, however many verified children point at it.

**Subject sets.** The board block takes walked ancestors as subject. The integrity
checks take the **verified ticket's own frontmatter** as subject and never inspect
ancestors — a dangling `related:` on an ancestor is that ancestor's problem, to be
reported when a commit touches it.

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

Sibling listing is capped at 5 rows followed by an overflow line. Because rows are
sorted by status, the cap can land mid-group and the hidden rows may span several
statuses, so the overflow line always renders per-status counts in the same sort
order rather than a single status: `... +3 more open (3 idea)`,
`... +3 more open (1 todo, 2 idea)`. Completed children are never listed.

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
    ... +3 more open (3 idea)
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
FIX:   related: `260611-bug-rsrc-manifest-regen-missed` resolves to no ticket stem
       and no spec anchor. Did you mean
       `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`?
       Correct or remove the entry.

CHECK: parent: `260619-epic-ws-layered-config-prompt-tuning` is category `epic`
       but resolves to a spec anchor, not a ticket. Confirm the intended parent.
```

On the commit path only, an advisory carrying a mechanical remedy gets the recipe
sentence appended: `Then git commit --amend --no-edit.` The standalone
`ws/tickets.verify` output omits it.

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
- `ws/git.commit` has **two entry points** wired through the same adapter — the
  MCP dispatch (`internal/mcp/server.go:1076`) and the ws-cli path
  (`cmd/ws-mcp/main.go:496` via the exported `mcp.VerifyAdapter`,
  `internal/mcp/format.go:58`). The adapter comment states that "every
  ws.git.commit entry point is gated identically"; Phase 1 must preserve that.
  This rules out "collect advisories on a second path", which would break parity
  and force re-deriving wsgit's internal staged path set
  (`expandCommitPathsForTicketMoves` plus `filterIndexDeleteSidePaths`,
  `internal/wsgit/git.go:451-490`) outside wsgit. The channel widening therefore
  happens at the `wsgit.Verifier` boundary.
- Advisories appear in text mode. Whether `format: "json"` carries them follows
  the todo re-injection precedent, which is text-mode only
  (`{#260626-git-commit-todo-reinjection}`); Phase 1 states the chosen answer
  explicitly rather than leaving it implicit.
- **Known limitation:** `internal/wsdoc/tickets.go:263` asserts
  `fm["related"].(map[string]string)`, and the hand-rolled frontmatter parser
  builds a list-shaped `related:` as `[]string`, so list-form `related:` silently
  resolves to nil and escapes the dangling-stem check. Three tickets use that
  shape, all in `.done`/`.dropped`. Either normalise the parse or state the gap;
  do not advertise a floor the check does not cover.
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
  `Findings` is empty and that `Warnings` never affect `OK`. The non-blocking
  half of the model fits, but the **carrier does not**: `VerifyFinding` is a
  single-line `{Path, Guardrail, Message}` tuple and `formatTicketVerify`
  renders it as one `WARN [guardrail] path: message` line. The `## Parent Board`
  block is multi-line, deduplicated per verify call, and not attributable to any
  single path, so it needs a distinct advisory carrier (for example
  `VerifyResult.Advisories []string` plus a matching commit-result field) rather
  than a `Warnings` entry. Routing it through `Warnings` would also emit
  `formatTicketVerify`'s "should be addressed or explicitly accepted"
  next_instruction, which is wrong for a no-action-needed ancestor note.
- `internal/wsdoc/tickets_verify_test.go` holds 19 `t.TempDir()` fixture tests —
  the established durable-verification pattern for this exact function.
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

One graph load serves both halves: walking `parent:` upward needs each ancestor's
frontmatter and child set, which is also the input the integrity checks resolve
against.

**Verification is synthetic**, following
`internal/wsdoc/tickets_verify_test.go`'s existing 19 `t.TempDir()` fixture tests.
This is not a fallback — the board cannot verify this work. Every integrity check
sits at zero live hits, and the board-block output is a function of counts that
change with every ticket-landing commit. Coverage must include all four integrity
checks, the board block in each of its four renderings (all-closed, siblings
remain, cap applied, ancestor already closed), the two-namespace stem resolution,
the ancestor dedup across a multi-ticket path list, the no-`parent:` case emitting
no section, both caps, and the `todo/`-vs-`.done/` emission gating.

Three deliberate **negative** cases, each a false positive an earlier draft of this
ticket would have emitted, are worth constructing as fixtures because each cost a
measurement to find:

| Negative case | Must emit nothing |
|---|---|
| `related:` key resolving to a spec anchor rather than a ticket | 4 live instances; a ticket-only resolver would flag deliberate references |
| An epic with zero `parent:`-linked children | `260616` is correctly scoped pre-decomposition; absence of children is not a defect |
| A stem named in an epic body without a `parent:` back-link | no body is read at all; see **No epic-body checks** |

Board numbers in **Output Format** are illustrative renderings, not assertions;
`260514` currently has 9 open children of 53 and `260605` has 4 of 31, and both
move with every landing commit.

## Out of Scope

- Changing any existing verify `Finding` into a warning, or vice versa. The
  reversibility principle motivates this ticket's new checks; retro-applying it
  to shipped blocking guardrails would change established commit-gate semantics
  and needs its own decision.
- Adding amend support to `ws/git.commit`.
- Staged-rename detection, and therefore any dependency on
  `260724-bug-ws-git-commit-verify-fails-on-staged-rename`.
- **Any check that reads a ticket body.** Epic bodies stay format-flexible by
  decision; the four rejected body-reading candidates and their measurements are
  recorded under **No epic-body checks** so they are not re-derived.
- **Epic convention prose.** The real lesson from `260630-epic-skill-playbook-diet`
  is a convention gap, not a missing check: implementation detail must be extracted
  to an implementation ticket, an epic stays a light board for one decomposed
  outcome, and unsettled deliberation about an epic belongs in a research ticket
  rather than the epic body. That is a `ticket-conventions` plus epic-template
  change, which AGENTS.md puts behind explicit approval, and it is a separate
  ticket. Supporting measurements to carry over: research is 3 of 93 children of
  open epics, only 4 of 36 research tickets carry any `parent:`, and epic template
  conformance is perfectly bimodal at 5 clean versus 2 with 6 and 2 non-template
  sections.
- **`workset` retirement.** The category is slated for removal now that goal-loop
  batching over `ready/` absorbs its grouping role. Footprint measured for whoever
  picks it up: 5 workset tickets ever (4 `.done`, 1 open), against 5 playbooks,
  11 Go files, 3 specs, the conventions doc, and the body template. This ticket
  adds no workset-aware code, and `260624-epic-pre-release-cleanup`'s mislabel is
  therefore left alone rather than being redirected toward a category that is going
  away.
- A board-health surface outside verify (for example in `project_tree`).
- `related:` symmetry enforcement, `plans:` path existence, and
  `related:`-to-`.dropped` acknowledgement, each rejected under **Check set** with
  its measurement.
