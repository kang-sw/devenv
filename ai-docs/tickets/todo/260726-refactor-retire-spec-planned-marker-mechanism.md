---
title: Retire the 🚧 planned-marker mechanism; pending spec contracts live in the ticket
related:
  260726-research-spec-planned-marker-management-cost: the research that decided this; carries the measurement and the function-relocation table
  260726-bug-spec-planned-marker-ready-ticket-cycle: dropped by this decision; its surviving finding is extracted to the ticket below
  260726-bug-inline-playbook-invocation-commit-ownership: the extracted finding — a real defect independent of this retirement
  260723-feat-ready-spec-address-hard-gate: owns the strength of the ready spec-address gate, which becomes the sole spec-addressing path
sage-review-design: blocked
sage-review-completeness: blocked
---

# Retire the 🚧 planned-marker mechanism; pending spec contracts live in the ticket

## Background

Decided in `260726-research-spec-planned-marker-management-cost` after measuring
the corpus. The short version:

- **Real usage corpus-wide: 1 marker**, and it is stale — its backing ticket
  `260524-feat-ws-dashboard-workspace-root-prune-policy` is in `.done/` and
  `ws-web-dashboard/index.md:231` still carries the callout.
- **Contract-first declarations: 1 yes / 10 no** across live tickets.
- The footprint is **six Go call sites across three files**, an embedded
  convention section, four rsrc playbooks in **both** plugin trees (8 files),
  three lead-bootstrap distribution files, a judge, and a whole
  `lead-update-spec` step — for that one stale instance. The Go sites:
  `markerContext` is called at `spec_discovery.go:255` (via `specMarkerContexts`,
  reached from line 206) **and** `spec_discovery.go:224` (populating
  `SpecAnchorInfo.MarkerContext`); `specStats` in `project_tree.go`; and three
  render sites at `server.go:2526`, `server.go:2628`, and
  `project_tree.go:150-155`.

The decisive argument is **ownership lifetime, and it survives the adoption
confound**. Adoption was measured while the mechanism was unusable (the ordering
cycle in `260726-bug-spec-planned-marker-ready-ticket-cycle`), so "nobody used it"
is weak evidence. But this is not:

> `## Spec Impact` is ticket-side, so pending contract text **dies with its
> ticket** — drop or close it and the pending text goes too. Staleness is
> structurally impossible. `🚧` is spec-side, so it **outlives its ticket by
> default** and stays correct only if a separate reconciliation ritual is run.

That holds at any adoption rate. The one live marker is the proof: `lead-update-spec`
§5 "Strip `🚧`" exists and does exactly the right thing — extract the stem, check
`git.log`, strip when implemented — and was simply never run.

Two of the three functions `🚧` bundled were relocated to the design reviewer in
commit `2d1a731c` (reads the `## Spec Impact` target; scans `ready/` for
spec-territory conflicts). The third — forcing the author to write the contract in
the spec's own vocabulary — is **consciously given up**, not overlooked. No
instance of its value exists in the corpus; if one is demonstrated later, this
decision should be revisited rather than worked around.

**Three further losses, also conscious.** Sage review surfaced dependents that an
earlier revision of this ticket missed, so they are named here rather than
discovered mid-removal:

1. `spec-conventions.md:43-44` — the *Implementation Gap Callout*'s stated
   resolution path is "create a qualifying ticket and convert to `🚧`". Retiring
   the marker orphans that path, so Phase 2 must state what replaces it (the
   natural answer is promoting the gap into a ticket's `## Spec Impact`, but it
   has to be written, not assumed).
2. `lead-write-spec`'s `judge: split-trigger` (line 61) lists "Its own `🚧`
   markers with a distinct ticket lifecycle" as one of three spec-split
   conditions. Deleting a split condition **changes when specs split** — a
   behavior change beyond marker removal.
3. `lead-forge-spec`'s split-condition table row (line 279) mirrors the same
   condition, plus `lead-write-spec` Invariants line 14 and step 6 accuracy
   checks, both phrased "every heading without `🚧`".

## Decisions

- **`## Spec Impact` becomes the sole path for pending spec contracts.** Nothing
  planned is written into a spec document.
- **The compat note must not be dropped from the retirement.** Removing the
  mechanism deletes `lead-update-spec` §5, which is downstream's only cleanup
  path; without a replacement every existing downstream marker leaks silently.
  Phase 1 is therefore independently valuable and is a hard prerequisite of
  Phase 2's removal — but note the protection is *retention*, not sequencing:
  Phase 1's note is Go code that reaches installed plugins only through the
  version bump listed in Phase 2, so a single dev-merge delivers both at once and
  no downstream project ever sees the note first. An earlier revision of this
  ticket claimed the ordering itself protected downstream. It does not; it is
  merely harmless, because Phase 2 retains the note.
- **Scan direction is ticket → spec. The reverse is impossible by construction.**
  An earlier revision decided the note could resolve a marker to its backing
  ticket "without opening any ticket file", using the marker's anchor. That is
  **false**, three independent ways:
  - `specAnchorRE` (`spec_tools.go:13`) = `\{#([0-9]{6}-[a-z0-9-]+)\}` captures a
    **spec** stem. The live marker's anchor is
    `{#260524-dashboard-workspace-root-prune-policy}`; its backing ticket is
    `260524-feat-ws-dashboard-workspace-root-prune-policy`. They differ by a
    `feat-ws-` infix and neither is derivable from the other.
  - `ticketRefRE` (`project_tree.go:15`) = `\[(\d{6}-[\w-]+/p\d+)\]` has **zero**
    matches anywhere under `ai-docs/spec/`.
  - The convention being retired forbids the premise outright.
    `spec-conventions.md:26`: "Do not put ticket references in the marker. Before
    implementation, readiness traceability lives in ticket `spec:`,
    `spec-remove:`, or `## Spec Impact`."

  So traceability is **ticket-side by explicit design**. The scan reads live
  tickets' `spec:` / `spec-remove:` frontmatter and `## Spec Impact` sections for
  references to the spec file carrying the marker. This restores the owner's
  original formulation — stale marker plus some ticket referencing that spec —
  which the anchor-based revision replaced with something cheaper that does not
  work.

  Reading `## Spec Impact` is a ticket-body read, and that is fine here:
  `260726-feat-verify-ticket-graph-advisories` excludes body reads from
  **`tickets.verify`'s mechanical floor**, not from every tool. This is a
  spec-side advisory, a different surface.
- **Reuse what genuinely exists; build the rest.** An earlier revision claimed
  "both halves of the detection already exist" and the work was pure
  prose-attachment. Half of that is true:
  - **Usable:** `markerContext` / `specMarkerContexts` (`spec_discovery.go:263`,
    reached from line 206) does real body-level marker detection and populates
    `SpecInfo.MarkerContexts`. `specs.list` renders it via `formatSpecs`
    (`server.go:2526`), and `specs.status` renders `MarkerContext`
    (`server.go:2627-2628`).
  - **Not usable:** `specStats` (`project_tree.go:169`) reads
    `fm["features"].([]string)` from spec **frontmatter**, and **zero** files
    under `ai-docs/spec/` declare a `features:` key. Its WIP counter is
    structurally 0 and `SpecInfo.TicketRefs` is always empty. The one live marker
    is a body callout it cannot see by construction. `project_tree` needs new
    detection.
  - **Incomplete:** `specs.find` routes to `formatSpecFind` whenever `query` is
    non-empty, and that path emits no marker line; only the no-query fallback
    reaches `formatSpecs`. The note must be added there, not inherited.
- **Advisory, never blocking.** Per the reversibility principle already adopted
  in `260726-feat-verify-ticket-graph-advisories`: legacy markers are a migration
  state, not an error. The note routes; it does not fail a commit.
- **`260726-bug-spec-planned-marker-ready-ticket-cycle` is dropped by this
  ticket**, since it exists to make `🚧`'s ordering satisfiable. Its one finding
  that survives — `lead-write-spec` step 7 committing unconditionally while the
  contract-first branch invokes it inline — is extracted to
  `260726-bug-inline-playbook-invocation-commit-ownership` and must not be lost
  with the drop.

## Constraints

- Do not delete the one existing marker's *content*. Its backing ticket is
  `.done/`, so the behavior it describes is implemented; the entry becomes an
  ordinary implemented spec entry with the marker stripped, not a deletion.
- `markerContext` (defined `spec_discovery.go:263`; the match condition is line
  266) matches `🚧`, `planned`, and `wip` case-insensitively, so it fires on prose
  containing the word "planned". Do not tighten it to the emoji alone without
  checking what the looser match is currently carrying — and do not let the compat
  note inherit that false-positive rate.
- Embedded convention text (`agents-plugin-tool/internal/wsdoc/conventions/`) is
  `go:embed`'d, so the removal only reaches installed plugins through a version
  bump.
- **The surviving path's hardening is unscheduled, and that is accepted with
  eyes open.** This ticket makes `## Spec Impact` plus the ready spec-address gate
  the only path for pending contracts, but
  `260723-feat-ready-spec-address-hard-gate` — which owns that gate's strength —
  sits in `idea/`, unaccepted (`260726-feat-verify-ticket-graph-advisories`
  independently records the same fact). Retiring the redundant path while the
  survivor is unhardened is a net weakening. It is accepted because the retired
  path had one stale instance and therefore contributed no real coverage to
  weaken, but the gate ticket should be promoted rather than left in `idea/`. It
  is **not** a prerequisite: blocking on it would hold the retirement behind an
  unscheduled research-adjacent ticket.

## Spec Impact

- Target spec areas, both required:
  - `ai-docs/spec/documentation-system.md` — the contract-first `🚧` paragraph
    (lines ~98-102) and the spec-index reconciliation description (~236-240).
  - `ai-docs/spec/mcp-tools.md` — the tool output contracts this ticket changes:
    ~828-829 documents `specs.list` / `specs.find` / `specs.status` exposing
    marker context, and ~807 documents `project_tree`'s spec inventory. AGENTS.md
    names this file as the MCP behavior contract, so changing those outputs
    without addressing it leaves the contract stale.
- Expected caller-visible change: specs no longer carry planned entries; pending
  contracts are read from `ready/` tickets' `## Spec Impact`. `specs.find`,
  `specs.list`, `specs.status`, and `project_tree` gain an advisory note when a
  legacy marker is present, reporting which live tickets reference that spec — or
  that none do — and the resolution. `lead-update-spec` loses its Strip step;
  `judge: contract-first-spec` is removed.
- Contract-first spec: no — and deliberately so. Writing a `🚧` entry to describe
  the retirement of `🚧` is exactly the mechanism this ticket removes.

## Phases

### Phase 1: Legacy-marker compat note

Hard prerequisite of Phase 2, and independently valuable — this repository has a
stale marker right now, so the note is correct behavior even if Phase 2 never
lands.

- **Detect markers** by reusing `markerContext` / `specMarkerContexts`
  (`spec_discovery.go:263`, reached from 206), which already reads spec bodies.
  Do **not** build on `specStats` — it reads `features:` frontmatter that no spec
  file declares.
- **Resolve ticket → spec, never spec → ticket.** For a spec carrying a marker,
  scan live tickets (`idea/`, `todo/`, `ready/`) for references to that spec:
  `spec:` and `spec-remove:` frontmatter, and `## Spec Impact` body text. The
  marker itself carries no ticket reference and by convention never will.
- **Emit an advisory note** on `specs.list` and `specs.status` (which already
  render marker context), on `specs.find`'s query path (which does **not** —
  `formatSpecFind` needs the note added), and on `project_tree` (which needs new
  body-level detection). The note states that planned markers are a retired
  mechanism and gives the resolution:
  - one or more live tickets reference this spec → name them with their statuses;
    move the marker text into the ticket's `## Spec Impact` and strip the marker.
  - no live ticket references this spec → the marker is orphaned; strip it,
    keeping the described behavior as an ordinary implemented entry if it shipped.
- **Name the retained surface explicitly in this phase's `### Result`**, since
  Phase 2's Go removal is scoped as "everything except what Phase 1 retains" and
  that boundary is otherwise undefined.

Rejected alternatives: resolving the marker's anchor to a ticket stem (impossible
— the anchor is a *spec* stem and `spec-conventions.md:26` forbids ticket
references in markers); hosting the check in `tickets.verify` (wrong subject —
this is spec-side, and verify's mechanical-floor role excludes it); blocking a
commit on a legacy marker (migration state, not an error).

Verification boundary:

1. With `ws-web-dashboard/index.md`'s existing marker in place, `specs.find` with
   a non-empty query, `specs.list`, `specs.status`, and `project_tree` each return
   the advisory. No live ticket references that spec, so all four report the
   orphaned case with a strip instruction.
2. A synthetic `## Spec Impact` in a `ready/` ticket naming that spec flips all
   four to the move-the-text case, naming the ticket and its status.
3. Removing that synthetic reference returns all four to the orphaned case —
   confirming the scan reads live ticket state rather than a cached value.
4. The retained detection surface is named in `### Result`.

### Phase 2: Remove the mechanism and ratchet downstream

Requires Phase 1 landed. Run the sub-steps in the stated order — 2.4 changes
downstream-visible distribution and must not run before the in-tree removal is
green, and 2.6 is the delivery vehicle for everything above it.

**2.1 Go removal.** Remove every marker site *except* what Phase 1's `### Result`
names as retained: `markerContext` (`spec_discovery.go:263`) and both its callers
(`specMarkerContexts` at 255, reached from 206; `specAnchorsInText` at 224,
populating `SpecAnchorInfo.MarkerContext`); `specStats`' WIP branch
(`project_tree.go:169`); the render sites at `server.go:2526`, `server.go:2628`,
and `project_tree.go:150-155`; and the dangling struct fields
(`SpecInfo.MarkerContexts`, `SpecAnchorInfo.MarkerContext`,
`SpecInfo.TicketRefs`) once unreferenced. Update the test call sites at
`spec_discovery_test.go:33` and in `project_tree_test.go`.

**2.2 Embedded conventions.** Remove `spec-conventions.md`'s `## 🚧 Markers`
section and examples. Rewrite `spec-conventions.md:43-44` — the *Implementation
Gap Callout*'s resolution path currently says "convert to `🚧`" and must name its
replacement instead. Check `ticket-conventions.md:30` for dependent text.

**2.3 rsrc playbooks, both trees.** `lead-write-spec` (contract-first branch, the
`> [!note] Planned 🚧` template, the `Planned marker:` output line at 120, the
unsatisfiable "Session reminder" at 51, the `🚧` split condition in
`judge: split-trigger` at 61, Invariants line 14, and step 6's accuracy checks);
`lead-update-spec` (§5 Strip `🚧` entirely); `lead-forge-spec` (split-condition
table row at 279); `fresh-reader-audit`. Removing the split conditions changes
when specs split — state the replacement rule rather than deleting the row.

**2.4 Judge.** Remove `judge: contract-first-spec` from `lead-write-ticket` and
`lead-write-spec`, and the Spec-address Check branch that invokes it. Coordinate
with `260726-bug-inline-playbook-invocation-commit-ownership`, whose only known
instance is that branch — see its landing-order decision.

**2.5 Local data.** Strip the marker from `ws-web-dashboard/index.md:231`, keeping
the entry as an ordinary implemented spec entry.

**2.6 Bootstrap ratchet.** The migration checklist is the `- vNNNN:` list in
`agents-plugin/skills/lead-bootstrap/AGENTS.template.md`; current version is
**v0044** (tag at line 191), so this adds **v0045** and moves the tag. AGENTS.md
classifies migration-checklist semantics as always-ask; the owner approved this
specific item, not a general license. **Blocked on the `## Open Questions` item
below** for `WORKFLOW.md:60` and `AGENTS.template.md:153`.

**2.7 Regenerate and bump.** Both rsrc regens (`WSRSRC_REGEN=1`,
`WS_REGEN_WSFLOW_RSRC=1`) and a plugin version bump through `bump-ws-version.sh`,
without which the embedded convention change never reaches installed plugins.

**2.8 Ticket drop.** Confirm `260726-bug-inline-playbook-invocation-commit-ownership`
exists and carries the extracted finding, then `git mv`
`260726-bug-spec-planned-marker-ready-ticket-cycle` to
`ai-docs/tickets/.dropped/` with a `## Resolution` recording that its premise was
retired.

Rejected alternatives: leaving the convention text in place as documentation of a
retired mechanism (invites re-adoption); keeping `judge: contract-first-spec` as a
no-op (a judge that always answers the same way is dead prose); deleting the spec
split conditions without a replacement rule (silently changes when specs split).

Verification boundary, one clause per sub-step:

1. **2.1** — no `🚧`, `MarkerContexts`, or `TicketRefs` reference remains in Go
   outside Phase 1's retained surface; `go test ./...` passes.
2. **2.2/2.3** — no `🚧` remains in the conventions or in either plugin tree's
   playbooks; the Implementation Gap Callout and both split-condition sites state
   a replacement rather than a deletion; wsflow bundle tests pass.
3. **2.4** — a fresh `lead-write-ticket` run on a spec-touching ticket reaches
   `ready/` through `## Spec Impact` with no contract-first branch offered.
4. **2.5** — `ai-docs/spec/` contains zero `🚧`.
5. **2.6** — the checklist carries a v0045 entry and the tag reads v0045; the
   migration applies cleanly to a project pinned at v0044.
6. **2.7** — both regens are idempotent (no diff on a second run) and the version
   bump touches only the script's edition points.
7. **2.8** — the cycle ticket is in `.dropped/` with a `## Resolution`, and the
   commit-ownership ticket still exists and still carries the extracted finding.

## Open Questions

**Bootstrap template prose — unresolved, blocks 2.6.**
`agents-plugin/skills/lead-bootstrap/WORKFLOW.md:60` and its
`agents-plugin-wsflow` mirror describe `🚧` as the contract-first mechanism, and
`AGENTS.template.md:153` carries a historical **v0024** checklist line referencing
it. Editing template prose that ships to downstream projects is ask-first under
AGENTS.md, and the owner approved only the migration-checklist item, explicitly
"not a general license". Unresolved:

(a) Does `WORKFLOW.md:60` get edited in Phase 2, or does it need its own approval?
(b) Is the historical `AGENTS.template.md:153` v0024 line edited, or preserved as
a version record of what that migration said at the time?

Do not implement 2.6 until both are answered.

## Blocked (2026-07-26)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Phase 1's core resolution step rests on a false premise — markers do not carry ticket stems | critical | VERIFIED TRUE. specAnchorRE captures a SPEC stem; the live marker's anchor {#260524-dashboard-workspace-root-prune-policy} differs from its ticket 260524-feat-ws-dashboard-workspace-root-prune-policy and is not derivable. spec-conventions.md:26 explicitly forbids ticket refs in markers and states traceability lives ticket-side. Fix: invert the scan direction to ticket->spec, reversing the No ticket-body reads decision. |
| 2 | specStats is dead code on this corpus, so the second half of the detection does not exist either | critical | VERIFIED TRUE. Zero spec files declare features: frontmatter, so the WIP counter is structurally 0 and TicketRefs always empty. project_tree needs new detection; the No new parser framing is wrong for that surface. |
| 3 | specs.find does not print marker: lines on its primary path | important | formatSpecFind handles non-empty query and emits no marker line; only the no-query fallback reaches formatSpecs. specs.status is an unmentioned third marker surface. |
| 4 | Go footprint is undercounted; 2 Go call sites is wrong | important | Six sites across three files, including spec_discovery.go:224 and server.go:2628 which Phase 2 would leave dangling. |
| 5 | Spec Impact names only documentation-system.md, but the caller-visible change lands in mcp-tools.md | important | Add mcp-tools.md as a target; it documents the specs.* and project_tree output contracts being changed. |
| 6 | Phase 2's removal list misses semantic dependents that would be silently orphaned | important | spec-conventions.md:43-44 implementation-gap resolution path, lead-write-spec judge:split-trigger and Invariants/step 6, lead-forge-spec split-condition row. These are additional conscious losses not currently listed. |
| 7 | Downstream template prose is touched but only the migration-checklist item was approved | important | lead-bootstrap WORKFLOW.md and AGENTS.template.md:153 carry marker text; editing template prose is ask-first. Needs an explicit owner decision. |
| 8 | The sole surviving spec-addressing path depends on a gate ticket that is only in idea/ | important | 260723-feat-ready-spec-address-hard-gate is unscheduled; the ticket must weigh retiring the redundant path while the survivor's hardening is not accepted. |
| 9 | Phase ordering does not deliver the protection it claims | minor | The version bump is in Phase 2, so a single dev-merge delivers both phases simultaneously and the ordering is inert. Harmless but overstated. |
| 10 | Line-number pointers are imprecise | minor | specStats defined at 169 not 174; markerContext at 263 not 266. |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Phase 2 bundles eight heterogeneous workstreams under one completion boundary | important |
| 2 | retaining whatever Phase 1's note needs leaves the Go removal boundary undefined | minor |
| 3 | Bootstrap migration item lacks a concrete target version and location | minor |
| 4 | Ticket-drop sub-item has no stated destination or acceptance check | minor |
