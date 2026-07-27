---
title: Retire the 🚧 planned-marker mechanism; pending spec contracts live in the ticket
related:
  260726-research-spec-planned-marker-management-cost: the research that decided this; carries the measurement and the function-relocation table
  260726-bug-spec-planned-marker-ready-ticket-cycle: dropped by this decision; its surviving finding is extracted to the ticket below
  260726-bug-inline-playbook-invocation-commit-ownership: the extracted finding — a real defect independent of this retirement
  260723-feat-ready-spec-address-hard-gate: owns the strength of the ready spec-address gate, which becomes the sole spec-addressing path
  260726-feat-verify-ticket-graph-advisories: supplies the advisory-not-blocking reversibility principle, and its mechanical-floor scope is what the ticket-body-read carve-out is measured against
sage-review-design: completed
sage-review-completeness: completed
---

# Retire the 🚧 planned-marker mechanism; pending spec contracts live in the ticket

## Background

Decided in `260726-research-spec-planned-marker-management-cost` after measuring
the corpus. The short version:

- **Real usage corpus-wide: 1 marker**, and it is stale — its backing ticket
  `260524-feat-ws-dashboard-workspace-root-prune-policy` is in `.done/` and
  `ai-docs/spec/ws-web-dashboard/index.md:231` still carries the callout.
- **Contract-first declarations: 1 yes / 10 no** across live tickets.
- The footprint is **six Go call sites across three files**, an embedded
  convention section, four rsrc playbooks in **both** plugin trees (8 files),
  three marker-carrying lead-bootstrap distribution files (2.7 touches five,
  counting the second lineage's template and its pinning test), a judge, and a whole
  `lead-update-spec` step — for that one stale instance. The counts size the
  argument; the sites themselves are the compiler's job, not this document's (see
  2.1).

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
   conditions, and `lead-forge-spec` mirrors it in a table row. An earlier
   revision called deleting these "a behavior change beyond marker removal";
   that is **wrong**, and the correction matters because it removes what would
   otherwise be an out-of-scope convention change. The condition's trigger is the
   presence of `🚧`, so once no `🚧` can exist it can never fire — deleting it and
   leaving it are observationally identical. It is dead prose, not a loss. See
   2.3.
3. `lead-write-spec` Invariants line 14 and step 6 accuracy checks, both phrased
   "every heading without `🚧`", need rephrasing to plain "every heading" — the
   check itself survives, only its exception clause goes.

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
    `spec-conventions.md:27`: "Do not put ticket references in the marker. Before
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
- **The advisory's predicate is `🚧` only. The check that gated this is now
  done.** `markerContext` matches `🚧`, `planned`, and `wip` case-insensitively.
  Measured over `ai-docs/spec/`: **22 matching lines across 4 files, 1 real
  marker.** The three spurious files (`documentation-system.md`,
  `mcp-tools.md`, `workflow-skills.md`) match only because they *describe* the
  mechanism in prose — a 21/22 false-positive rate, and every false positive is a
  document that will be edited by 2.5 anyway. So the advisory keys on the emoji
  alone. `markerContext` itself stays as-is for its existing callers; the advisory
  does not inherit its looseness.
- **Ticket→spec matching is by exact spec file path or anchor stem, never by
  substring or area.** Undefined matching would make Phase 1's own verification
  ambiguous: live tickets write spec references three ways (a full
  `ai-docs/spec/<file>.md` path, a bare `{#YYMMDD-slug}` anchor, and the literal
  "Target spec area: none"). Under exact-path/anchor matching, no live ticket
  references `ws-web-dashboard/index.md` and clause 1's orphaned case holds; under
  substring or area matching, roughly fifteen live dashboard tickets match and the
  same clause fails. Exact matching also has the better failure mode — a missed
  reference yields "orphaned, strip it", which a human reviews, whereas a spurious
  match yields "move it into this unrelated ticket".
- **The compat note is temporary and says so.** It exists to migrate legacy
  markers, and this repo has exactly one. Phase 1 adds an advisory branch to four
  tools; without a stated horizon the retirement trades one dead mechanism for
  four live ones. The note carries a removal condition in its own `### Result`:
  it is deleted once no supported downstream version can still be emitting markers
  — concretely, one bootstrap ratchet cycle after 2.7's v0045/v0006 ship.
- **Advisory, never blocking.** Per the reversibility principle already adopted
  in `260726-feat-verify-ticket-graph-advisories`: legacy markers are a migration
  state, not an error. The note routes; it does not fail a commit.
- **Bootstrap template prose is edited in Phase 2, both trees, no separate
  approval** (owner decision, 2026-07-26). `WORKFLOW.md:60` describes `🚧` as the
  contract-first mechanism; after this ticket that mechanism does not exist, so
  leaving the text instructs downstream to use a removed feature. That is drift
  correction on contact, not a workflow-semantics change, which is what
  AGENTS.md's ask-first clause guards. The two trees' copies **differ** and are
  hand-maintained (no generator), so this is two edits, not one plus a regen.
- **A checklist item nullified by a later version is not carried forward — at
  clause granularity, not line granularity.** Owner-stated rule. Applying it to
  `AGENTS.template.md:153`: v0024 bundles three clauses (`[!note] Constraints` →
  body prose; unscheduled gaps → Implementation Gap callout; planned ticketed
  features → `### 🚧`) and only the third is nullified. Blanket-obsoleting the
  line would drop two migrations a project pinned below v0024 still needs, so
  **strip only the third clause** and leave the rest. The established form for a
  *fully* nullified item is a `[obsoleted by vNNNN]` placeholder preserving the
  ordinal (v0002, v0007, v0008, v0012) rather than deleting the line; that form
  does not apply here because v0024 survives in part.
- **Two independent bootstrap version lineages, not one.**
  `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md` is **not** a
  mirror of the `agents-plugin` template — it carries a package-local lineage
  (v0001-v0005) and `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:196`
  asserts both the `v0005` tag and the "package-local version history" line. So
  the ratchet is **two entries with different numbers**: `agents-plugin` v0044 →
  v0045, `agents-plugin-wsflow` v0005 → v0006, plus that test's assertions. The
  wsflow lineage contains no `🚧` clause anywhere, so the v0024 clause-strip is
  `agents-plugin`-only.
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
  266) is left at its current looseness for **`specs.find`'s match scoring**
  (`spec_discovery.go:85`), which is its only surviving consumer once 2.1 rewrites
  the render paths. Do not justify the retention by "its existing callers" — 2.1
  removes those. The advisory keys on `🚧` alone regardless; see the predicate
  decision above.
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

- Target spec areas, all three required — sweep each file rather than trusting
  the line numbers, which an earlier revision under-listed:
  - `ai-docs/spec/documentation-system.md` — the contract-first `🚧` paragraph
    (~98-102) and every other marker mention (at least ~104-107, ~236-240, ~247,
    ~258).
  - `ai-docs/spec/workflow-skills.md` (~309-312, ~320-326) — documents the
    `lead-write-spec` / `lead-update-spec` marker behavior being removed.
  - `ai-docs/spec/mcp-tools.md` — the tool output contracts this ticket changes:
    ~828-829 documents `specs.list` / `specs.find` / `specs.status` exposing
    marker context, and ~807 documents `project_tree`'s spec inventory. AGENTS.md
    names this file as the MCP behavior contract, so changing those outputs
    without addressing it leaves the contract stale. Its `tickets=` /
    spec-ticket-reference documentation is a **different** mechanism and stays.
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
  (`spec_discovery.go:263`, reached from 206), which already reads spec bodies,
  filtered to `🚧`-bearing lines per the predicate decision. Do **not** build on
  `specStats` — it reads `features:` frontmatter that no spec file declares.
- **Resolve ticket → spec, never spec → ticket.** For a spec carrying a marker,
  scan live tickets (`idea/`, `todo/`, `ready/`) for references to that spec:
  `spec:` and `spec-remove:` frontmatter, and `## Spec Impact` body text. The
  marker itself carries no ticket reference and by convention never will. Match
  on the exact spec file path or on an anchor stem belonging to that file —
  never substring or area prefix; see the matching decision above.
- **Emit an advisory note** on `specs.list` and `specs.status` (which already
  render marker context), on `specs.find`'s query path (which does **not** —
  `formatSpecFind` needs the note added), and on `project_tree` (which needs new
  body-level detection). The note states that planned markers are a retired
  mechanism and gives the resolution:
  - one or more live tickets reference this spec → name them with their statuses;
    move the marker text into the ticket's `## Spec Impact` and strip the marker.
  - no live ticket references this spec → the marker is orphaned; strip it,
    keeping the described behavior as an ordinary implemented entry if it shipped.
- **Document the advisory in `ai-docs/spec/mcp-tools.md` in this phase**, at the
  `specs.*` (~828-831) and `project_tree` (~806-810) contracts. Phase 1 changes
  four documented tool outputs and claims independent value "even if Phase 2 never
  lands"; under that fallback the contracts would change and never be documented.
  2.5 then handles only what remains.
- **Record two things in this phase's `### Result`**: the retained detection
  surface (2.1 may extend but not shrink its stated retain-list), and the compat
  note's removal condition per the temporary-note decision.

Rejected alternatives: resolving the marker's anchor to a ticket stem (impossible
— the anchor is a *spec* stem and `spec-conventions.md:26` forbids ticket
references in markers); hosting the check in `tickets.verify` (wrong subject —
this is spec-side, and verify's mechanical-floor role excludes it); blocking a
commit on a legacy marker (migration state, not an error).

Verification boundary:

1. With `ai-docs/spec/ws-web-dashboard/index.md`'s existing marker in place,
   `specs.find` with a non-empty query, `specs.list`, `specs.status`, and
   `project_tree` each return the advisory. Re-establish first that no live ticket
   references that spec under the exact-match rule — true at authoring time, but
   it is repo state, not an invariant — then all four report the orphaned case
   with a strip instruction.
2. A synthetic `## Spec Impact` in a `ready/` ticket naming that spec flips all
   four to the move-the-text case, naming the ticket and its status.
3. Removing that synthetic reference returns all four to the orphaned case —
   confirming the scan reads live ticket state rather than a cached value.
4. **No advisory fires for `documentation-system.md`, `mcp-tools.md`, or
   `workflow-skills.md`** — the three files the loose `markerContext` predicate
   matches on prose alone. This is the false-positive check; without it clause 1
   passes on a predicate that flags every document describing the mechanism.
5. `mcp-tools.md` documents the advisory on the `specs.*` and `project_tree`
   contracts, so Phase 1 leaves no undocumented tool-output change behind.
6. `### Result` names the retained detection surface and the note's removal
   condition.

### Result (3514973a) - 2026-07-28

Done. All four surfaces carry the compat advisory; `ai-docs/spec/mcp-tools.md`
documents it on both contracts. Commits: `3b4afa52` (implementation), `ed79d3c7`
(review cycle 1 fixes), `3514973a` (review cycle 2 fixes), `073b6325` (mental
model). All six verification clauses hold, clause 4 included.

#### Retained detection surface — 2.1 may extend this list, never shrink it

Everything the advisory is built on, so that 2.1 does not delete a live
dependency. Go reports unused locals and imports but **not** unused struct fields
or unused unexported functions, so the compiler will not catch a mistake here.

- `internal/wsdoc/legacy_marker.go` in full: the three shape regexes
  (heading, `> [!note] Planned 🚧` callout, `- 🚧 ` list), `fenceTracker`,
  `htmlCommentTracker`, `splitMarkdownIndent` / `maxMarkdownBlockIndent`,
  `yamlFrontmatterEnd`, `legacyMarkerLines`, `collectSpecImpact` /
  `specImpactSection` / the `specImpactHeading` constant, `legacyMarkerResolver`
  with its `incomplete` flag, and `advise`.
- `SpecInfo.LegacyMarkerAdvisory` and `SpecInfo.legacyMarkers` (unexported),
  `SpecAnchorStatus.LegacyMarkerAdvisory`, and `applyLegacyMarkerAdvisories`
  in `spec_discovery.go`.
- `scanLegacyMarkersUnderSpecRoot` and the repo-root threading through
  `renderSpecs` / `renderSpecDir` in `project_tree.go`.
- The four render points in `internal/mcp/server.go`: `formatSpecs`,
  `formatSpecFind` (its own append — it inherits nothing), `formatSpecStatus`,
  and the `project_tree` line.
- `specAnchorRE` (`spec_tools.go`) as the anchor extractor.
- `TicketsList` with `Statuses: idea/todo/ready` and `TicketInfo.Specs` /
  `SpecRemoves` as the frontmatter half of the resolver.

Not built on: `markerContext`, `specMarkerContexts`, `specStats`,
`SpecInfo.TicketRefs`. All four are byte-identical to the phase base, verified by
programmatic body extraction rather than by reading the diff.

#### Removal condition

The compat note is deleted once no supported downstream version can still emit
markers — concretely, one bootstrap ratchet cycle after 2.7's v0045 / v0006 ship.
Phase 2 **retains** the note; deletion belongs to a later, unwritten ticket.

#### Lead deviations from the ticket text

Two settled before implementation, both re-verified against the corpus. Both were
unsatisfiable-as-written, not preferences.

- **D1 — the predicate keys on marker shape at line start, not on the emoji
  alone.** The `## Decisions` justification claimed the three prose files match
  "only because they describe the mechanism", implying a bare `🚧` check is safe.
  `documentation-system.md` carries **six** literal `🚧`, two of which embed the
  marker shapes inside inline code mid-line. A bare check — or an unanchored
  shape check — fires on it and fails this phase's own clause 4.
- **D2 — a ticket matches via the marker's own anchor stem or the exact spec
  path, never "any anchor belonging to that file".** Phase 1's wording is
  over-broad and contradicts the ticket's own expected outcome:
  `ws-web-dashboard/index.md` carries 72 anchors and 15 live-ticket `spec:`
  entries resolve into it, so file-level matching reports 15 unrelated dashboard
  tickets for a workspace-root-prune-policy marker and inverts clause 1.

A third departure was taken during review: the orphaned branch's guidance gained
an "or as an Implementation Gap callout if it did not" clause the ticket's
verbatim string lacks. The ticket's text is a conditional with no else — an agent
finding the behavior did not ship is told to "strip it" with nothing to preserve,
against the first `## Constraints` line. 2.6 supplies the missing branch.

Two more the implementation made and the lead accepted:

- The ticket's "reuse `markerContext` / `specMarkerContexts`" instruction was
  **not** honored; a standalone predicate was written instead. Sound, for a
  reason the ticket does not state: building on `specMarkerContexts` would have
  pinned a *second* consumer onto a function 2.1 wants reduced to one, via 2.1's
  own retain-boundary.
- `mcp-tools.md` extends two existing anchored sections' prose rather than
  minting a new anchor. Correct, but not for the reason first given (Phase 2
  retains the note, it does not delete it) — the real reason is that `{#slug}`
  anchors are durable identities that tickets and mental models cross-reference,
  and minting one for a note with a stated sunset creates a reference that must
  later be broken.

#### Errors in the ticket text

- **Three drifted `server.go` offsets**: 2526 → 2536, 2627-2628 → 2637-2638,
  2518 → 2529. The ticket's own round-3 review noted it "has already carried two
  wrong line numbers"; these are three more.
- **Phase 1's match rule contradicts the ticket's `## Decisions`** — see D2.
- **The `## Decisions` predicate justification is factually wrong** — see D1.
- Phase 1 does not cover the `features:`-frontmatter marker form that 2.7
  requires v0045 to cover. The lead kept it as a third shape: it is the form
  `spec_discovery_test.go`'s existing fixture uses, and it is safe for clause 4.

#### Review

Two partitioned cycles, one under budget. Cycle 1 (correctness / fit / test)
returned 0 Critical, 6 Important, 24 Minor; **every** finding was adjudicated
fix-it, none dismissed, and a relay applied all 19 items.

Cycle 2 (correctness of the new logic / verification that each fix closed its
finding) returned 5 more Important. The important thing about cycle 2 is what it
caught: cycle 1's fix **introduced a regression** and applied its own new rule to
only one of two consumers.

- The fix added a CommonMark fence tracker to `legacyMarkerLines` but left
  `specImpactSection` on bare `TrimSpace`, and gave the tracker no unbalanced-
  fence recovery — so one unclosed fence anywhere before `## Spec Impact` hid the
  heading, dropped every reference, and reported a marker a live `ready/` ticket
  owns as "orphaned; strip it". `3b4afa52` did not have that failure.
- Cycle 1's own A3 directive, written by the lead, was wrong. Tightening the
  `## Spec Impact` heading match moved the failure from over-matching to
  under-matching, while `tickets_mutate.go`'s ready gate stayed loose — so
  `## Spec Impact and Phases` passed the ready gate as spec-addressed and then
  had its markers reported orphaned. Reverted in cycle 3; the two sites must
  agree and loose is the safe direction.
- A guard the fix added to close a finding was itself the anti-pattern the same
  cycle flagged: a `chmod 000` test that skips where chmod does not restrict. On
  a DrvFs `TMPDIR` it skipped and both mutations killing the behavior it guards
  survived with the suite green.

Three of the five cycle-2 Importants were the same shape and produced the rule
now recorded in `ai-docs/mental-model/documentation-system.md`: an advisory whose
branches differ in destructiveness must land every uncertain or degraded input on
the non-destructive branch. This phase shipped three separate paths — a read
failure, an unbalanced fence, an over-tight heading match — that each emitted a
delete instruction off an error path.

Cycle 3's relay reversed two lead directives, correctly: the proposed
permission-independent driver (a directory named `<stem>.md`) does not work
because `scanTickets` skips directories before `readTicket`, and the resolver's
ticket sort could not be made load-bearing because `advise` sorts the rendered
strings — so the dead sort was deleted rather than tested around.

Review stopped at the budget. One item ships unresolved with a stated
disposition: `TestTicketGraphUnreadableTicketDegradesToSilence` still uses the
`chmod 000` pattern. It is pre-existing (`b81bb3df`, present at the phase base,
never touched in this range) and its premise — one unreadable ticket among
readable ones — is not served by the driver this phase used. Captured as
`260728-bug-chmod-based-tests-skip-silently`.

#### Verification

`go build ./...`, `go vet ./...`, `go test ./... -count=1` green across all 12
packages; also green with `TMPDIR` on a DrvFs mount, with zero skips in the
legacy-marker suite. Every mutation that survived a prior cycle now fails, each
naming its catching test. Ticket-corpus parity: all 458 live and archived tickets
extract byte-identically under the new `specImpactSection` as under `3b4afa52`'s.
Real-repo spot check: exactly one file flagged
(`ai-docs/spec/ws-web-dashboard/index.md`, line 231), verdict **orphaned** as the
ticket expected, and nothing for `documentation-system.md`, `mcp-tools.md`, or
`workflow-skills.md`.

#### Forward to Phase 2

- `readyGateWarning` still uses a literal `"## Spec Impact"` with bare
  `TrimSpace`, while `collectSpecImpact` uses the `specImpactHeading` constant
  with the ≤3-column indent rule. They agree on prefix looseness — the property
  that matters — but differ on what "line start" means. Not a defect today; if
  2.1 or a later ticket unifies them, unify toward the safe direction.
- Phase 1 introduced no `🚧` into `ai-docs/spec/`; `grep -c 🚧 mcp-tools.md` is 0,
  so 2.5's "zero `🚧`" clause is not pre-broken.
- `ai-docs/spec/` still specifies the mechanism as live in
  `documentation-system.md:98-102,236,240` and `workflow-skills.md:857`, while
  `mcp-tools.md` now says it is being retired. That contradiction is scheduled —
  2.5 owns all three — but it exists in this repo from this commit onward, so
  Phase 1's "independently valuable even if Phase 2 never lands" claim holds only
  for the tooling, not for the spec corpus.

### Phase 2: Remove the mechanism and ratchet downstream

Requires Phase 1 landed. Run the sub-steps in the stated order — 2.7 changes
downstream-visible distribution and must not run before the in-tree removal is
green, and 2.8 is the delivery vehicle for everything above it.

**2.1 Go removal.** Remove the planned-marker reporting from the tool surfaces —
`project_tree`'s WIP/planned counting and the marker render paths in `specs.list`
and `specs.status` — rewriting them to Phase 1's advisory rather than the old
planned-count output, and drop the struct fields that fall dangling. Follow the
compiler and the test suite to the sites; do not work from a hand-copied symbol
list. One caveat: Go reports unused locals and imports but **not** unused struct
fields or unused unexported functions, so the dangling-field half needs `grep` or
`staticcheck` — a field orphaned by removing a render path still builds green.

Two boundaries, because they are judgment and the compiler cannot supply them:

- **Retain whatever Phase 1's advisory is built on.** Phase 1's `### Result`
  names the retained surface; 2.1 may extend that list, never shrink it. The
  body-level marker detection Phase 1 reuses is not a removal target.
- **`SpecInfo.TicketRefs` is not a marker field and is not touched.**
  `specTicketRefs` reads `ticket:`, `tickets:`, `feature:`, and `features:`
  frontmatter — a separate mechanism, which the marker convention forbids markers
  from carrying. It backs `ticketsFromSpecRefs` and therefore the
  `references.trace` tool, renders as the `tickets=` flag, and is documented in
  `mcp-tools.md`. An earlier revision listed it among the dangling marker fields
  and had a verification clause demanding its removal; both were wrong. Its
  emptiness on *this* corpus is a corpus fact, not grounds to delete a documented
  capability.

Deliberately no symbol-and-line enumeration here. Two review rounds spent their
critical finding on this sub-step, and the second one — deleting `TicketRefs` —
is a build break the compiler reports in seconds. Prose has no verifier for a
symbol graph, and this ticket has already carried two wrong line numbers. The
ticket owns what is retired and what is off-limits; the compiler owns where.

**2.2 Embedded conventions.** Remove `spec-conventions.md`'s `## 🚧 Markers`
section and examples. Rewrite the *Implementation Gap Callout*'s resolution path
(`spec-conventions.md:43-44`), which currently says "convert to `🚧`": the
replacement is to create the qualifying ticket and carry the gap's contract text
in that ticket's `## Spec Impact`, removing the callout at implementation
closeout. The callout itself survives — only its exit path changes.

`ticket-conventions.md:29` says `lead-write-ticket` "invokes `lead-write-spec`
only for contract-first planned spec entries". After 2.4 removes that branch the
answer is **never** — `lead-write-ticket` does not invoke `lead-write-spec` at
all, and spec addressing runs through `## Spec Impact`. State that once and apply
it to all three sites carrying the clause: `ticket-conventions.md:29-30`,
`documentation-system.md:247`, and `workflow-skills.md:322-323` (the latter two
are 2.5's).

**2.3 rsrc playbooks, both trees.** Sweep each file for *every* `🚧` occurrence
rather than working from the line list below — an earlier revision named one of
nine sites in `lead-forge-spec` and treated the enumeration as complete.
`lead-write-spec` (contract-first branch, the `> [!note] Planned 🚧` template, the
`Planned marker:` output line, the unsatisfiable "Session reminder", the split
condition in `judge: split-trigger`, Invariants line 14, step 6's accuracy
checks); `lead-update-spec` (§5 Strip `🚧` entirely); `lead-forge-spec` (the
split-condition table row plus the "Implemented or planned?" branch, the
heading-marker placement step, the `🚧 Planned:` count line, and the two
strip/review closeout bullets — its forge-time analogue of `lead-update-spec` §5).

**`fresh-reader-audit:58` is out of scope — the same class as 2.1's
`TicketRefs`.** Under `## What not to flag` it reads "Issues already present in
text marked explicitly as `TODO` or `🚧`". Paired with `TODO`, that is a generic
work-in-progress annotation on arbitrary audited text, with no dependence on the
spec convention. An earlier revision listed the file for removal and clause 2
mandated a marker-free tree, which together would delete an audit exclusion and
start flagging issues inside text its author explicitly marked as unfinished.
This is the one site where reducing the enumeration would have been wrong: it is
prose with no compiler and no test behind it, so nothing else catches the mistake.

**Delete the `🚧` split condition; do not invent a replacement.** An earlier
revision called this "a behavior change beyond marker removal" and instructed
stating a replacement rule. Both were wrong: the condition's trigger is *the
presence of `🚧` markers*, so once no `🚧` can exist the condition can never fire.
Deleting it and leaving it in place are observationally identical, which makes
deletion dead-prose removal rather than a convention change. Authoring a new split
condition would be a convention change and is **not** in scope.

**2.4 Judge.** Remove `judge: contract-first-spec` from `lead-write-ticket` and
`lead-write-spec`, and the Spec-address Check branch that invokes it. Coordinate
with `260726-bug-inline-playbook-invocation-commit-ownership`, whose only known
instance is that branch — see its landing-order decision.

**2.5 Repo spec corpus.** The sub-step an earlier revision omitted entirely,
leaving verification clause "`ai-docs/spec/` contains zero `🚧`" unreachable from
any stated work.

**This sub-step's sweep predicate is not the advisory's.** The advisory keys on
`🚧` alone because it acts on live markers. This sub-step edits *prose describing
the mechanism*, which is mostly emoji-free — `workflow-skills.md` contains
**zero** `🚧` yet documents the behavior throughout, so a `🚧` sweep finds nothing
there and clause 5 passes trivially. Sweep for `🚧` **or** "planned marker /
planned entry / planned stem / strips planned" in spec-mechanism context. Exclude
`## Planned References` (`documentation-system.md:146-147`,
`workflow-skills.md:354-355`) — that is the live workset reference mechanism, a
different thing the loose predicate hits.

- `ai-docs/spec/documentation-system.md` — the contract-first paragraph
  (98-102), and every other marker mention in the file (at least 104-107, 236-240,
  247, 258).
- `ai-docs/spec/workflow-skills.md` — the `lead-write-spec` / `lead-update-spec`
  marker behavior 2.3 removes: 309-312, 320-326, **818-819** ("strips planned
  markers when implementation lands"), and **997** ("associates planned stems with
  active tickets"), which sits under anchor
  `{#260707-forge-spec-autoproceed-classification-2}` and is therefore an
  anchored-entry amendment, not free prose.
- `ai-docs/spec/mcp-tools.md` — the `specs.*` and `project_tree` output contracts
  changed by Phase 1 and 2.1 (~807, ~829), to the extent Phase 1 has not already
  documented its own advisory. Do **not** remove the `tickets=` /
  ticket-references documentation; see 2.1's out-of-scope note.

**2.6 The one live marker.** `ai-docs/spec/ws-web-dashboard/index.md:231` is a
`> [!note] Planned 🚧 {#260524-dashboard-workspace-root-prune-policy}` **callout**,
not a `## 🚧` heading, and `spec-conventions.md` defines no "implemented callout"
form — so "strip the marker" does not by itself name a target shape. Required:

- Determine whether the described behavior actually shipped. Its ticket is in
  `.done/`, but `ai-docs/mental-model/ws-web-dashboard.md:64` and `:147` still
  describe the policy as planned/unimplemented. **Check the code; do not assume
  either source.** An earlier revision of this ticket asserted it shipped.
- If shipped: convert to an ordinary implemented entry. If not: it is an
  Implementation Gap, so use that callout form with the 2.2 resolution path.
- Either way the anchor `{#260524-dashboard-workspace-root-prune-policy}` must
  survive — the mental model cross-references it twice.

**2.7 Bootstrap ratchet, both lineages.** Edits across two hand-maintained trees —
see the two-lineage decision above before touching anything:

- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`: add **v0045**, move
  the tag at line 191 from v0044 to v0045, and strip the `planned ticketed
  features -> ### 🚧 <Feature Name>` clause from **v0024** at line 153, leaving
  its other two clauses intact.
- v0045's text must cover **both** marker forms: `## 🚧` headings and
  `> [!note] Planned 🚧` callouts in spec bodies, *and* the `- 🚧 <name> [stem/pN]`
  entries in spec `features:` frontmatter that `specStats` used to parse (fixture:
  `project_tree_test.go:16`). `AGENTS.template.md:151` v0022 still tells
  downstream projects to rebuild `features:` frontmatter, so a project on v0022
  can hold that form while 2.1 deletes the only tool that surfaced it. Resolution
  per marker: move the pending text into the owning ticket's `## Spec Impact` when
  a live ticket references that spec; otherwise keep it as an implemented entry if
  it shipped, or an Implementation Gap callout if not.
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`: add the
  equivalent as **v0006** and move the tag at line 163 from v0005 to v0006. No
  clause strip — this lineage never carried a `🚧` item.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:196`: update the
  `v0005` tag assertion to v0006. The test pins the lineage on purpose; changing
  the template without it fails the bundle suite.
- The contract-first `🚧` paragraph in **three** `WORKFLOW.md` files, all
  different and none generated: `agents-plugin/skills/lead-bootstrap/`(:60),
  `agents-plugin-wsflow/skills/lead-bootstrap/`(:60), and this repo's own
  `ai-docs/WORKFLOW.md`(:63), which AGENTS.md names as the workflow-shape guide.

AGENTS.md classifies migration-checklist semantics as always-ask; the owner
approved the checklist item and the `WORKFLOW.md` prose edit specifically
(2026-07-26), not a general license to edit template prose.

**2.8 Regenerate and bump.** **Three** regens, not two — the third is a distinct
flag on purpose: `WSRSRC_REGEN=1` (rsrc manifest), `WS_REGEN_WSFLOW_RSRC=1`
(wsflow mirror), and `WSRSRC_REGEN_SKILLS=1` (`agents-plugin/skills/manifest.json`,
which carries SHA-256 entries for `lead-bootstrap/AGENTS.template.md` and
`lead-bootstrap/WORKFLOW.md` — exactly the files 2.7 edits; `TestSkillsManifestDriftIsVisible`
fails on a stale manifest). Then a plugin version bump through
`bump-ws-version.sh`, without which the embedded convention change never reaches
installed plugins.

**2.9 Ticket drop.** Confirm `260726-bug-inline-playbook-invocation-commit-ownership`
exists and carries the extracted finding, then `git mv`
`260726-bug-spec-planned-marker-ready-ticket-cycle` to
`ai-docs/tickets/.dropped/` with a `## Resolution` recording that its premise was
retired.

Rejected alternatives: leaving the convention text in place as documentation of a
retired mechanism (invites re-adoption); keeping `judge: contract-first-spec` as a
no-op (a judge that always answers the same way is dead prose); authoring a
replacement spec-split condition (a convention change, out of scope — see 2.3);
removing `SpecInfo.TicketRefs` along with the marker fields (a different
mechanism backing `references.trace` — see 2.1).

Verification boundary, one clause per sub-step (2.2 and 2.3 share one):

1. **2.1** — `project_tree`, `specs.list`, and `specs.status` report no planned
   or WIP counts and no marker context beyond Phase 1's advisory; `references.trace`
   still resolves spec→ticket references and `specs.list` still renders `tickets=`;
   `go test ./...` passes.
2. **2.2/2.3** — no `🚧` remains in the conventions or in either plugin tree's
   playbooks **except `fresh-reader-audit:58`, which must still exclude
   `TODO`/`🚧`-marked text from flagging**; the Implementation Gap Callout states
   the `## Spec Impact` resolution path; both split-condition sites are deleted
   with no replacement condition added; wsflow bundle tests pass.
3. **2.4** — a fresh `lead-write-ticket` run on a spec-touching ticket reaches
   `ready/` through `## Spec Impact` with no contract-first branch offered.
4. **2.5** — `documentation-system.md`, `workflow-skills.md`, and `mcp-tools.md`
   describe no marker mechanism; `mcp-tools.md` still documents `tickets=` /
   spec ticket references.
5. **2.6** — `ai-docs/spec/` contains zero `🚧`; the anchor
   `{#260524-dashboard-workspace-root-prune-policy}` still resolves and both
   `ws-web-dashboard.md` mental-model cross-references still land; the entry's
   form matches the implemented-vs-gap finding, recorded in `### Result`.
6. **2.7** — `agents-plugin`'s checklist carries v0045 with the tag at v0045 and
   v0024 reduced to its two surviving clauses; `agents-plugin-wsflow`'s carries
   v0006 with the tag at v0006; `test_wsflow_skill_bundle.py` passes against the
   updated assertion; none of the three `WORKFLOW.md` files mentions `🚧`. A
   project pinned at v0044 (resp. v0005) applies the new item cleanly; a project
   pinned below v0024 still receives v0024's two surviving clauses; and v0045
   gives an instruction for a `features:`-frontmatter marker, not only body forms.
7. **2.8** — all three regens are idempotent (no diff on a second run),
   `TestSkillsManifestDriftIsVisible` passes, and the version bump touches only
   the script's edition points.
8. **2.9** — the cycle ticket is in `.dropped/` with a `## Resolution`, and the
   commit-ownership ticket still exists and still carries the extracted finding.

### Result (e89fbcf5) - 2026-07-28

All nine sub-steps landed. 26 commits from the survey plan; three partitioned
review cycles (correctness, fit, test), each followed by a fix relay.

**Verification boundary, clause by clause.** All eight are satisfied. Two could
not be evidenced from this corpus alone and are recorded honestly rather than
worked around:

- Clause 1's `specs.list` still renders `tickets=` half is not demonstrable on
  the live corpus, because no spec here declares ticket frontmatter — a corpus
  fact this ticket itself calls out in 2.1. Confirmed instead on a fixture:
  `ai-docs/spec/demo.md - Demo [anchors=1 tickets=260504-ticket-demo]`.
- Clause 3's "a fresh `lead-write-ticket` run reaches `ready/` with no
  contract-first branch offered" is a live-procedure clause with no automated
  evidence. It is reasoned from the branch's absence
  (`grep -rn contract-first-spec` over both plugin trees and `ai-docs/spec/`
  returns zero), not from an executed run.

Clause 5's "recorded in `### Result`" sub-clause is this paragraph: 2.6's
conversion was decided by reading the Rust, as the clause required. The shipped
rule for `active_work_root_count` is **availability only** —
`WorkspaceBuilder::push` computes `enabled = available && active` for the view
but increments the counter under `if available` alone (`discovery.rs:186-203`,
asserted by `routes.rs:881-925`). The callout had claimed activation *and*
availability. The spec now matches the code, and "disabled or" was dropped
because only `recoveryNeeded` is emitted.

**Two lead deviations, both settled before implementation.** D1: the marker
predicate keys on shape at line start rather than the emoji alone, because
`documentation-system.md` carried six literal markers in prose and a bare check
fails this phase's own clause 4. D2: ticket-to-spec matching is scoped to the
marker's own anchor plus the exact spec path, because the wider wording names 15
unrelated dashboard tickets and inverts its own expected orphaned outcome. Both
were unsatisfiable as written, not preferences.

**Lead-directed scope extension.** `internal/wsdoc/tickets_template.go:65` was a
fourth site carrying the `Contract-first spec: yes|no` clause that the ticket's
"apply it to all three sites" wording missed. Removed under this ticket's
authority: leaving it would have violated the rejected-alternatives list, which
forbids keeping convention text for a retired mechanism because it invites
re-adoption.

**What the review cycles cost and bought.** Cycle 1 returned 6 Important and 12
Minor; cycle 2 returned 3 Important, two of them found independently by two
partitions; cycle 3 returned 1 Important, also found independently by two
partitions. Nothing was dismissed without a stated reason.

The durable finding is that **the same defect class was introduced twice by this
ticket's own fixes**. Cycle 1's directive scoped `lead-forge-spec` step 5's
iteration to implemented items, which silently defeated step 4's promise that
step 5 collects every ambiguity marker for the wrap-up. Cycle 2's repair of that
made an in-session record the wrap-up's *only* source and forbade reading the
on-disk markers — which made a resumed session emit a confident
`Ambiguous classifications: 0` instead of an incomplete list. Both are the
advisory-asymmetry rule recorded in Phase 1: an uncertain input landed on the
branch that asserts absence. The second occurrence sharpened it into a corollary
now in `ai-docs/mental-model/documentation-system.md` — replacing a recoverable
source with a better but ephemeral one, and forbidding the old source, converts
undescribed recovery into prescribed-against recovery.

**Three lead directives were proven wrong by measurement and correctly
overridden.** The cycle-1 assertion `strings.Contains(listText, "marker: ")`
fails on the *unmutated* tree, because Phase 1's retained advisory renders
`legacy-marker: ` and contains it as a substring — on the exact fixture the
directive named. The cycle-2 preference for guarding at the end-to-end
`server_test.go:1133` fixture would have been half-vacuous, because that
fixture's only anchor carries no marker token and `omitempty` drops the field;
proven by dumping the real JSON-RPC payload. The cycle-3 directive to compare
`strings.ToLower(key)` against a snake_case literal does not catch a *deleted*
`json:"-"` tag, because Go emits `MarkerContexts`, which lowercases to
`markercontexts`.

**What ships unguarded**, measured rather than assumed:

1. `rsrc/` playbook prose is guarded for *consistency*, not content. A partial
   revert fails four hash checks; a clean revert plus regen is fully green. This
   covers the `lead-update-spec` §5 deletion and renumber, `lead-write-spec`, and
   the `spec-conventions.md` retirement text. Two exceptions now carry targeted
   content guards — the Implementation Gap carve-out across the three
   hand-maintained `WORKFLOW.md` (`internal/wsrsrc/workflow_guide_test.go`) and
   the `lead-forge-spec` ambiguity contract (`57c743d3`) — both added because the
   sentence carries safety-critical force and had been broken more than once.
2. The two `lead-bootstrap/AGENTS.template.md` copies can silently disagree;
   reverting a fix in the wsflow copy alone leaves both suites green.
   `test_wsflow_skill_bundle.py:229-230` *requires* the two files to differ, so a
   naive equality guard needs a substitution table. Captured in
   `260728-research-parallel-workflow-guide-divergence`.
3. Everything in the three `WORKFLOW.md` except the one guarded sentence.

**Guarded and mutation-proven dead:** the retired `formatSpecs` marker line at
any indentation including tabs, the `formatSpecStatus` ` # ` suffix, both
`json:"-"` tags independently at the wire level under both the reverted-tag and
deleted-tag shapes, Phase 1's advisory across all three render paths plus its
truncation bound, and the carve-out in each of the three `WORKFLOW.md`
separately.

**Observed during closeout, no action taken:** `ws/convention.read` served
pre-retirement convention text this session, because the running MCP binary
predates this branch. The source files at HEAD contain none of it. This is the
plugin-version propagation surface and it resolves with the merge-time bump.

**Tickets created or moved:** `260728-chore-spec-mcp-server-repair-unspecified`,
`260728-chore-ws-tree-skill-pointer-guard`,
`260728-bug-chmod-based-tests-skip-silently`,
`260728-research-presentation-artifact-convention-policy`,
`260728-research-parallel-workflow-guide-divergence` created;
`260726-bug-spec-planned-marker-ready-ticket-cycle` dropped with a
`## Resolution`; evidence appended to `260723-research-reviewer-worktree-isolation`
and a resolution note to `260726-research-spec-planned-marker-management-cost`.

**Forward, for `260726-bug-inline-playbook-invocation-commit-ownership`:** 2.4
deleted `lead-write-ticket.md:106`'s inline invocation while `lead-discuss.md:62`
survives, so that ticket's Phase 1 survey overstates Category A by one site and
must be re-run. Separately, deleting `lead-write-spec`'s former step 6 renumbered
its unconditional commit from step 7 to step 6, and `lead-update-spec`'s from
step 7 to step 6; the two Phase 1 instruction bullets were corrected in place and
the ticket's `## Landing-order inversion` section enumerates what was left as
historical record.


## Reviewed (2026-07-26, round 3)

Design **concern**, completeness **pass** — first round with no critical and no
finding needing an owner decision. All six design and six completeness findings
were folded in before promotion.

Both reviewers independently assessed the 2.1 reduction and reached the same
conclusion: it removed duplication, not recoverable-only information. The
symbol entry points it had enumerated were already stated with `file:line`
elsewhere in the ticket, and what it delegated was only which lines those symbols
occupy — twice gotten wrong. Design added that round 2's list was net-negative,
since its `TicketRefs` entry was itself the error.

Findings fixed in this pass:

| # | Lens | Title | Severity |
|---|------|-------|----------|
| 1 | design | 2.5's `workflow-skills.md` coverage incomplete and its sweep predicate unstated — the file has **zero** `🚧`, so a marker sweep finds nothing while clause 5 passes trivially; `:818-819` and `:997` were outside the stated ranges | important |
| 2 | design | 2.3 + clause 2 ordered deletion of `fresh-reader-audit:58`, a generic `TODO`/`🚧` audit exclusion unrelated to this mechanism — same class as round 2's `TicketRefs`, and the one such site with no compiler behind it | important |
| 3 | design | Phase 1 changes four documented tool contracts but had no spec step, while claiming value "even if Phase 2 never lands" | important |
| 4 | design | `ticket-conventions.md:29`'s post-removal behavior unanswered across three mirrored sites | minor |
| 5 | design | `markerContext`'s retention rationale ("for its existing callers") is undercut by 2.1, which removes them; the real surviving consumer is `specs.find` match scoring | minor |
| 6 | design / completeness | Damaged sentence — the **Advisory, never blocking** decision lost its head during an earlier insertion | minor |
| 7 | completeness | "Follow the compiler" under-delivers: Go does not report unused struct fields or unexported functions | minor |
| 8 | completeness | `260726-feat-verify-ticket-graph-advisories` load-bearing twice but absent from `related:` | minor |
| 9 | completeness | "one clause per sub-step" header vs 8 clauses for 9 sub-steps | minor |
| 10 | completeness | Background retained one unprefixed spec path | minor |

Accepted without change: Phase 2 still bundles nine sub-steps under one
completion boundary (round-1 completeness finding 1). Completeness round 3 judged
this mitigated rather than resolved — the sub-steps are ordered with a stated
rationale, each has a named acceptance clause, and the Go/prose/distribution work
is separable without new decisions — and explicitly not worth blocking a third
time.

## Blocked (2026-07-26, round 2)

Round 1's ten design findings: 7 resolved, 3 partial (5, 6, 10 — the named
instances were fixed without re-running the sweep that finds their siblings).
Round 1's four completeness findings: 2 resolved, 2 partial. The rewrite then
introduced new defects; both reviewers returned **block**.

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| N1 | `SpecInfo.TicketRefs` is not a marker field; 2.1 ordered deletion of unrelated documented behavior | critical | VERIFIED TRUE. `specTicketRefs` (`spec_discovery.go:235`) reads `ticket:`/`tickets:`/`feature:`/`features:` frontmatter, backs `ticketsFromSpecRefs` (`references.go:81`) and thus `references.trace`, renders as `tickets=` (`server.go:2518`), documented at `mcp-tools.md:829`. Addressed: 2.1 now carries an explicit out-of-scope clause and the verification clause was inverted to require the capability still works. |
| N2 | 2.1's removal list contradicted Phase 1's retained surface | important | The "except what Phase 1 retains:" colon-list read as the retain set and was the removal set. Addressed: 2.1 split into explicit Retain / Remove / Out-of-scope lists. |
| N3 | Detection predicate left undecided; stated verification could not pass under the mandated one | important | VERIFIED: `grep -riE "planned\|wip\|🚧" ai-docs/spec/` = 22 lines, 4 files, 1 real marker. Addressed: predicate decision fixes the advisory to `🚧` alone; Phase 1 clause 4 added as the false-positive check. |
| N4 | Ticket→spec match rule undefined, and clause 1's expected result depended on it | important | Addressed: matching decision fixes exact spec path or anchor stem, with the failure-mode rationale. |
| N5 | 2.7 omitted the skills-manifest regen, a CI gate on exactly the files the ratchet edits | important | VERIFIED: `WSRSRC_REGEN_SKILLS=1` is a third distinct flag; `manifest.json` hashes `lead-bootstrap/AGENTS.template.md` and `WORKFLOW.md`. Addressed in 2.8. |
| N6 | A third `WORKFLOW.md` carries the same paragraph and was out of scope | important | VERIFIED: `ai-docs/WORKFLOW.md:63`. Addressed in 2.7, plus `ticket-conventions.md:29` in 2.2. |
| N7 | 2.5's conversion target undefined and its "already implemented" premise contested | important | The live entry is a callout, not a heading; `ws-web-dashboard.md:64`/`:147` still call the policy planned. Addressed: promoted to its own sub-step 2.6 requiring a code check and anchor preservation. |
| N8 | v0045 did not cover the `features:`-frontmatter marker form | minor | Addressed in 2.7; v0022 still tells downstream to rebuild `features:` frontmatter. |
| N9 | New permanent advisory surface across four tools, no sunset | minor | Addressed: the compat note now carries a removal condition. |

### Completeness Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | No sub-step performed the `## Spec Impact` spec updates | important | VERIFIED: 2.2 covered only the embedded conventions dir, so clause "`ai-docs/spec/` contains zero `🚧`" was unreachable. Addressed: new sub-step 2.5. |
| 2 | 2.1's "except" clause parsed two ways | important | Same as N2. |
| 3 | Spec-split replacement rule required but never decided; Background contradicted 2.3 | important (resolution: missing) | Resolved **against** the reviewer's framing: the condition's trigger is `🚧` presence, so with no `🚧` it can never fire and deleting it is observationally identical. Dead-prose removal, not a convention change — no owner decision needed. Background loss #2 corrected. |
| 4 | `markerContext` false-positive constraint had no acceptance criterion | important | Same as N3. |
| 5 | Phase 2's ordering rationale cited the wrong sub-step numbers | minor | Off-by-one left by the earlier split. Corrected. |
| 6 | "one clause per sub-step" stated but not delivered | minor | Was 7 clauses for 8 sub-steps; now 8 for 9. |
| 7 | `ws-web-dashboard/index.md` cited without a directory prefix | minor | Corrected to `ai-docs/spec/ws-web-dashboard/index.md`. |

## Blocked (2026-07-26, round 1)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Phase 1's core resolution step rests on a false premise — markers do not carry ticket stems | critical | VERIFIED TRUE. specAnchorRE captures a SPEC stem; the live marker's anchor {#260524-dashboard-workspace-root-prune-policy} differs from its ticket 260524-feat-ws-dashboard-workspace-root-prune-policy and is not derivable. spec-conventions.md:26 explicitly forbids ticket refs in markers and states traceability lives ticket-side. Fix: invert the scan direction to ticket->spec, reversing the No ticket-body reads decision. |
| 2 | specStats is dead code on this corpus, so the second half of the detection does not exist either | critical | VERIFIED TRUE. Zero spec files declare features: frontmatter, so the WIP counter is structurally 0 and TicketRefs always empty. project_tree needs new detection; the No new parser framing is wrong for that surface. |
| 3 | specs.find does not print marker: lines on its primary path | important | formatSpecFind handles non-empty query and emits no marker line; only the no-query fallback reaches formatSpecs. specs.status is an unmentioned third marker surface. |
| 4 | Go footprint is undercounted; 2 Go call sites is wrong | important | Six sites across three files, including spec_discovery.go:224 and server.go:2628 which Phase 2 would leave dangling. |
| 5 | Spec Impact names only documentation-system.md, but the caller-visible change lands in mcp-tools.md | important | Add mcp-tools.md as a target; it documents the specs.* and project_tree output contracts being changed. |
| 6 | Phase 2's removal list misses semantic dependents that would be silently orphaned | important | spec-conventions.md:43-44 implementation-gap resolution path, lead-write-spec judge:split-trigger and Invariants/step 6, lead-forge-spec split-condition row. These are additional conscious losses not currently listed. |
| 7 | Downstream template prose is touched but only the migration-checklist item was approved | important | RESOLVED by owner 2026-07-26: WORKFLOW.md is edited in Phase 2 without separate approval (drift correction, not a semantics change), and v0024's marker clause is stripped at clause granularity under the stated rule that a later version's nullification does not carry forward. Investigating this also surfaced that wsflow runs a separate v0001-v0005 lineage with a pinning test, so the ratchet sub-step is several edits across two lineages, not one. |
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
