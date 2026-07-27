---
title: "Ticket-graph advisories at verify: parent-board nudge and cross-reference integrity"
spec:
  - 260723-git-commit-ticket-verify-gate
  - 260723-tickets-verify-tool
related:
  260723-epic-ticket-write-reshape: refines its "verify = mechanical floor" boundary from intra-file to cross-file reference resolution, and adopts its must-not-forget action-time framing
  260724-bug-ws-git-commit-verify-fails-on-staged-rename: path-based emission gating is chosen specifically so this feature does not depend on staged-rename detection
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-07-27
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

1. **No closure nudge.** The last child of an epic landing is not distinguishable
   from any other commit, so an epic that has become closable goes unnoticed.
   `260723-epic-ticket-write-reshape` is closable today and nothing surfaced it.
   Note that a naive "all children closed" trigger would *still* miss that epic:
   one child (`260723-feat-ready-spec-address-hard-gate`) sits in `idea/` and its
   Completion Criteria explicitly permit closure with that child deferred. This is
   why the ACTION line has two tiers (see **Ancestor walk**), and why the general
   deferred-child case stays out of reach — deciding it requires reading
   `## Completion Criteria`, which **No epic-body checks** rules out by decision.
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
| Board-block ACTION line (both tiers) and the ancestor-already-closed NOTE | always |
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

**Two-namespace resolution applies to `related:` only.** A `related:` key resolves
against ticket stems **union spec anchor stems** (`{#YYMMDD-slug}` in
`ai-docs/spec/`). Measured: of 10 `related:` keys that are not ticket stems, 4 are
spec anchors — `260513-harness-local-agent-tier-config` in three tickets and
`260505-lead-skill-namespace-surface` in `.dropped/260525-bug-ws-setup-cwd-plugin-cache-root`.
Pointing a ticket's `related:` at a spec anchor is an established repeated pattern,
so resolving against tickets only would emit false `FIX:` advisories against
deliberate references.

**`parent:` resolves against ticket stems only.** A parent is walked as a graph
node — the walk needs its status, its frontmatter, and its own child set, none of
which a spec anchor has. Extending the union to `parent:` would create a case with
no measured instance and no defined walk behavior, so a `parent:` naming a spec
anchor is simply an unresolvable stem and reports as `FIX:`. The walk therefore
never encounters a non-ticket node.

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
| `parent:` names a stem that is not a ticket | `FIX:` | 0 |
| `related:` key names neither a ticket nor a spec anchor | `FIX:` | 0 open (6 in `.done`/`.dropped`) |
| `parent:` chain contains a cycle | `CHECK:` | 0 |
| `parent:` target is not an `epic` | `CHECK:` | 0 |
| Ancestor board status (`N of M open`, or all-closed) | board block | n/a |

A cycle is `CHECK:` rather than `FIX:` because the remedy is not mechanical —
which edge in the cycle is the wrong one is exactly a judgment call. When the walk
hits a cycle it reports the cycle and emits **no** `## Parent Board` block for that
ticket, since ancestor status is undefined on a cyclic chain.

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
- **The ACTION line has two tiers**, both status-only:
  - *All children closed* — the plain closure nudge.
  - *All children in `todo/`/`ready/` closed, only `idea/` children remain* —
    conventions make `todo/` the accepted backlog, so an epic whose every accepted
    child has landed is in a materially different state from one still in flight.
    Measured: exactly one live instance, `260723-epic-ticket-write-reshape`, and
    zero false fires — the other four open epics with children all have accepted
    children open. This tier exists because the epic that motivated the feature
    falls in it.
- An ancestor already in `.done/`/`.dropped/` emits a note, not an action. The note
  is worded path-neutrally: it states that the parent is already closed and that
  the remedy is editing the parent's `### Result` rather than reopening it. It must
  not assert *when* the child closed, because the same block renders on an ordinary
  `todo/`-path commit where nothing closed at all.
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
`... +3 more open (1 todo, 2 idea)`.

**Closed children render only in the all-closed tier.** In the sibling listing they
are always omitted — the question there is what remains. In the all-closed tier the
closed rows *are* the evidence a reader needs to judge closure, so they render and
the closed sort order (`.done` -> `.dropped`) applies. In the `idea/`-only tier the
closed rows render for the same reason, followed by the remaining `idea/` rows.

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

  NOTE: This parent is already closed. No action needed. If its `### Result`
    should mention this work, edit that Result; do not reopen the parent.
```

Only `idea/` children remain — the second ACTION tier:

```
Parent [1]: 260723-epic-ticket-write-reshape [todo] - 2 of 3 closed, 1 idea/ remaining
    .done   | 260723-feat-ticket-write-verify-commit-gate
    .done   | 260723-feat-ticket-system-concept-doc  (just now)
    idea    | 260723-feat-ready-spec-address-hard-gate

  ACTION: Every accepted child has landed; only idea/ children remain. Check
    whether this epic can be closed - read its `## Completion Criteria`, which
    may permit closure with the remaining children deferred.
```

Multi-level chain:

```
Parent [1]: 260624-epic-pre-release-cleanup [todo] - all 3 child tickets closed
    .done   | 260622-bug-wsflow-launcher-coldload-divergence
    .done   | 260622-chore-windows-shipping-hardening
    .done   | 260624-feat-prefer-mercenary-hide-option  (just now)

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

CHECK: parent: `260619-epic-ws-layered-config-prompt-tuning` resolves to a ticket
       whose category is `refactor`, not `epic`. A parent must be an epic; confirm
       the intended parent.
```

On the commit path only, an advisory carrying a mechanical remedy gets the recipe
sentence appended: `Then git commit --amend --no-edit.` The standalone
`ws/tickets.verify` output omits it.

The standalone tool **does** render the `## Parent Board` block and the integrity
advisories — same verdict, same advisories, minus the amend sentence. That is what
makes it usable for a mid-edit board check, and it is the only difference the
identical-verdict guarantee of `{#260723-tickets-verify-tool}` permits.

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
- Cross-file checks resolve against frontmatter and status directories only.
- **A graph-load failure must degrade to silence, never to a commit veto.**
  `verifyAdapter` returns `TicketVerify`'s Go error straight to `wsgit.Verifier`,
  which vetoes the commit, and today that error path is caller-input-only
  (`internal/wsdoc/tickets_verify.go:41`, "paths requires at least one path").
  Phase 2 adds a whole-board load plus a spec-anchor scan, either of which can fail
  on a malformed file unrelated to the commit. Such a failure drops the advisories
  and lets the commit proceed; it never becomes an error return, or the
  non-blocking invariant would be violated by the very code meant to honor it.
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
without importing `internal/wsdoc` into `wsgit`.

**Advisories must arrive inside `wsgit.CommitResult`** (`internal/wsgit/git.go:434`),
which means the `Verifier` boundary carries them. Both entry points render through
an argument-free formatter — `formatGitCommit(result)` at
`internal/mcp/server.go:1093` and `mcp.FormatGitCommit(result)` at
`cmd/ws-mcp/main.go:509` — so anything collected outside the result struct is
silently lost on the CLI path. Collecting advisories on a second path is therefore
not an available option.

Verification boundary: an existing warning class (`unresolved-phases` on close,
or `spec-address` on a `ready/` ticket) appears in `ws/git.commit`'s response,
the commit still lands, and `OK` is unchanged. No new check is added in this
phase.

### Result (7e0c35f5) - 2026-07-27

Code range `35239ebc..7e0c35f5`; spec `36f04c6a`, mental model `c5a65070`.

`wsgit.Verifier` is now `func(root string, paths []string) ([]string, error)`,
and `wsgit.CommitResult` carries `Advisories []string`. `verifyAdapter` and the
exported `VerifyAdapter` format `wsdoc.VerifyResult.Warnings` into
`WARN [guardrail] path: message` lines and return them as advisories, returning a
non-nil error only for hard `Findings`. Both `ws/git.commit` entry points stay
gated identically, since the advisories ride the result struct that both
argument-free formatters already receive.

**Answers the phase left open.** JSON mode carries no advisories: the field is
tagged `json:"-"`, so both emission sites omit it through plain
`encoding/json.Marshal` with no per-call-site branch — matching the todo
re-injection precedent. Trailer order is commit output -> advisories -> todo
re-injection -> session-key tip, keeping the tip last per
`{#260708-git-commit-session-key-tip}`. On a veto the advisories are discarded
deliberately, pinned by `TestCommitVetoDiscardsAdvisoriesAlongsideError`.

**Deviation from Prior Art, accepted.** Prior Art suggested a
`wsdoc.VerifyResult.Advisories` field alongside the commit-result field. Phase 1
adds only the commit-result half, because nothing populates a wsdoc-side field
until Phase 2. The fit review confirmed this does not force a Phase 2 reshape:
the `[]string` carrier is pre-formatted and unattributed, which is exactly what
the multi-line, per-call-deduplicated `## Parent Board` block needs, and the
renderer splits each advisory on newlines and indents every line so a multi-line
block renders correctly on arrival.

Verification: `go build ./...` clean and `go test ./... -count=1` green across
all 12 packages. The verification boundary is proved end-to-end by
`TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories`, which drives
the real MCP dispatch over a `.done` ticket fixture carrying an
`unresolved-phases` warning and asserts all three clauses — warning surfaced,
commit landed, `OK` unchanged — with a non-vacuous JSON-mode absence assertion
(the same fixture is shown to warn on the text path first). The correctness
reviewer additionally confirmed the CLI entry point by live e2e.

Review: correctness, fit, and test partitions all returned clean with zero
Critical/Important findings. Six minor findings; four were fixed in `7e0c35f5`
(stale `Verifier` doc comment, multi-line advisory indentation, explicit
veto-discard intent plus its test, and an empty-`Advisories` assertion on the
verifier-skipped path). Two were rejected: a dedicated `wsgit`-level threading
test, made redundant by the veto-discard test's stub verifier, and the spec
qualification, which belonged to the documentation pass and landed as
`36f04c6a`.

Spec: `{#260723-git-commit-ticket-verify-gate}` gained sub-anchor
`{#260727-git-commit-verify-advisories}` for the advisory channel, its
text-mode-only boundary, and entry-point parity.

Not carried forward: Phase 2's checks, the `## Parent Board` block, and any
`wsdoc`-side advisory field remain unimplemented, as scoped.

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
checks, the board block in each of its five renderings (all closed, `idea/`-only
remaining, siblings remain, cap applied, ancestor already closed), `related:`
two-namespace resolution against a `parent:` that resolves to tickets only, the
ancestor dedup across a multi-ticket path list, the no-`parent:` case emitting no
section, cycle detection suppressing the board block, a graph-load failure
degrading to silence with the commit still landing, both caps, and the
`todo/`-vs-`.done/` emission gating.

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

### Result (b81bb3df) - 2026-07-27

Code range `0ef10104..b81bb3df` (6 commits); mental models `b635b7f8`. Spec text
landed inside the code range rather than as a separate closeout commit.

New `internal/wsdoc/tickets_graph.go` runs one whole-board graph load — composed
from the existing `scanTickets` and `scanSpecs` rather than new scanners — and
serves both the `parent:` ancestor walk and the four integrity checks from it.

**Carrier deviates from Prior Art, deliberately.** Prior Art suggested
`VerifyResult.Advisories []string`; the landed carrier is
`VerifyAdvisory{Kind, Text}` with `Kind` in `{fix, check, board}`. With
`[]string` the commit layer would have to re-parse the `FIX:` prefix to decide
amend-recipe eligibility, which makes the settled Output Format load-bearing for
behavior. `Kind` keeps format and behavior independent.

The `related:` known limitation was **normalised, not documented**: `relatedEntries`
handles map, list, and bare-string forms and runs list items through `cleanScalar`,
so a trailing `# comment` no longer survives. `TicketInfo.Related`'s type and JSON
tag are unchanged. Chosen over stating the gap because `TicketInfo.Related` had no
non-test consumers, making the helper near-zero-risk.

**Deliberate omissions from the settled Output Format, each with a disposition:**

- The `(unchanged by this close; shown for chain context)` line under a depth-2
  ancestor is **not emitted**. The row-level `(epic, closable - see above)`
  parenthetical already carries the chain-context signal, so the line is
  redundant rather than missing. Note the reasoning that does *not* apply: an
  earlier argument that the block "also renders on an ordinary `todo/`-path
  commit" is false, because the sibling-listing tier is gated to
  `.done`/`.dropped` paths.
- `Did you mean <near-stem>?` is **not implemented**. No Decisions line specifies
  a matching rule, so any fuzzy heuristic would be unspecified behavior.
- The tier-1 ACTION text uses the shorter generic form. The longer example's
  trailing clause is a fact about `260723`'s own `## Completion Criteria`, which
  **No epic-body checks** forbids establishing.

**Non-obvious contracts a later change must not undo.** `byStem` keeps the
most-open copy of a duplicated stem so an abnormal board degrades toward "still
open" instead of producing a false closure nudge — but the integrity subject
resolves through a separate `byPath`/`verifiedInfo` lookup, so checks always read
the verified file's own frontmatter. Collapsing those two lookups into one was an
actual regression during review. The `No further ancestors.` claim derives from
the ancestor's own frontmatter (`chainEndsAt`) rather than a call-scoped flag,
which makes it correct under ancestor deduplication for free; the earlier
call-global flag leaked across tickets in a multi-path verify.

Review: two cycles. Cycle 1 returned one Important — the integrity cap was applied
per verify call rather than per verified ticket, contradicting **Check set**, and
the divergence had already been written into the spec. Both were corrected. Cycle 2
returned clean on all three partitions with both `won't fix` items accepted.
Pinning the closed-inclusive overflow wording also exposed a real defect: counts
were emitted in a fixed global status order that contradicted the rows above them,
and now follow the hidden rows' own order.

Verification: `go build ./...` clean, full `go test ./... -count=1` green across
all 12 packages, 39 graph subtests. All three cycle-2 code fixes were
mutation-checked, each new test failing only for its own target. The whole-board
sweep reproduces this ticket's measured baseline exactly — zero `FIX:`/`CHECK:` on
any open ticket and 6 under `.done`/`.dropped` — across three separate runs
spanning the fix cycles, so no false positive was introduced.

Forward: `fix`/`check` advisories carry no subject path, so on a commit staging
several tickets a reader cannot tell which ticket an advisory belongs to, and with
the now-per-ticket cap a single response can carry several unattributed
`... +N more` lines. Rejected here as scope expansion — the settled Output Format
shows no path prefix — and left for whoever revisits that format.

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
- **Epic convention prose — partly landed already; do not redo it.** The real
  lesson from `260630-epic-skill-playbook-diet` is a convention gap, not a missing
  check. Two of the three rules landed directly in `ticket-conventions.md` at
  **`120e2b25`**: implementation detail moves to an implementation child ticket,
  and deliberation that outgrows a settled decision line moves to a `research`
  ticket. No ticket was opened for that half; it is recoverable from that commit.

  Two pieces remain:
  - The third rule — an epic stays a light board for one decomposed outcome — is
    meaning rather than operation, so it belongs in the workflow manual's
    **Ticket System Concepts**, not in the conventions doc. Unwritten. The manual
    is loaded once per session, so it has a wider blast radius than the conventions
    doc and was deliberately held back.
  - `120e2b25` is shipped as of `dea5e6c4` (`0.36.13`). The conventions doc is
    `go:embed`'d into `ws-mcp` and plugin-cache invalidation keys on the version
    string, so the bump — not the content commit — is what made the two rules
    reachable by installed plugins.

  Measurements not to re-derive: research is 3 of 93 children of open epics and only
  4 of 36 research tickets carry any `parent:` — both normal, since a research
  ticket is primarily a home for discussion with nowhere else to go and only
  epic-scale discussion earns the link. Epic template conformance is perfectly
  bimodal: 5 open epics with zero non-template sections, 2 with 6 and 2.

  **Epics need no new staging vocabulary** (`milestone`, `step`, `### Ticket N:`),
  and the reason is positive rather than absence of demand: the demand exists and
  the item line already absorbs it. `260605-epic-ws-playbook-factory-pivot` groups
  its children into milestones as parentheticals on each `## Child Tickets` line
  (`(ready, M1 — playbook surface MVP)`, `(done .done/, M2 — ...)`), and
  `260723-epic-ticket-write-reshape` expresses sequencing the same way ("blocked on
  the collocator child"). Ordering and grouping are edge annotations on the item
  line, not containers. Keeping items as flat bullets rather than `###` headings is
  load-bearing: a heading invites body content beneath it, which is how
  implementation work ended up in `260630`'s body.

  A generated `## Child Tickets` list was also considered and rejected. Notes are
  effectively universal (17 of 20 entries on `260514`, 4 of 4 on `260723`) and the
  stem is the anchor the note hangs from, so generating the stem half cannot
  preserve the other half; the only removable duplication would be bare-stem lists,
  which barely occur. A tool that rewrites a doc section is also the pattern this
  repo is backing out of — see `260725-idea-retire-ticket-focus-root-regen` and
  `260710-bug-project-index-ticket-focus-stale-status`.
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
