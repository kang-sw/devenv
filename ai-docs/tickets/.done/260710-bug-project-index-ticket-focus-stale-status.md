---
title: remove the _index.md Ticket Focus section and retire its maintenance machinery
related:
  260605-epic-ws-playbook-factory-pivot: workflow routing depends on accurate project-memory status
sage-review-completeness: completed
sage-review-design: completed
completed: 2026-07-25
---

# remove the _index.md Ticket Focus section and retire its maintenance machinery

## Background

`ai-docs/_index.md`'s `## Ticket Focus` section is a hand-maintained prose
cache of ticket status and cross-ticket narrative. It repeatedly diverges from
the filesystem-backed ticket state that `wsflow/tickets.list` and
`wsflow/project_tree` report as authoritative — advertising absent or
already-closed tickets as `ready`, and so risking an implementation route toward
a nonexistent target (originally observed 2026-07-10 with
`260702-bug-config-unset-asymmetry`).

The drift persists **despite existing maintenance machinery**: `lead-write-ticket`
writes a focus entry on every `ready/` promotion, `executor-wrapup` removes
completed tickets from the section, `lead-bootstrap` preserves it across AGENTS
regeneration, and `WORKFLOW.md` already states "only `ready/` entries are direct
implementation targets." A fresh live instance was observed 2026-07-25: a goal
run closed `260724-feat-lead-fan-out-worktree` to `.done/` while its focus entry
still read `ready` / `Implementation-ready`, requiring a hand fix during a
`main` merge.

## Decision

**Remove the `## Ticket Focus` section entirely and retire the machinery that
reads, writes, and maintains it.** Rely on filesystem-backed discovery
(`tickets.list`, `project_tree`, the status directories) plus each ticket's own
body for status and detail. Settled with the user 2026-07-25.

This resolves the ticket's previously-blocking design question — the
recurrence-prevention mechanism, left `resolution: missing` by the 2026-07-13
design review (automated guard `(a)` vs. documented manual procedure `(b)` vs.
other `(c)`) — as **`(c)`: delete the drift surface itself.** Removal is the only
option that makes the drift class *structurally impossible* rather than adding
another manual step; a guard or procedure cannot beat "no cached status exists."

Rationale:
- **The status half is pure duplication.** Ticket Focus's status assertions
  ("ready", "todo") duplicate the directory-backed source of truth, which is
  where every drift instance originates.
- **The narrative half is also a cache.** Settled decisions, blocker rationale,
  and phases already live authoritatively in each ticket body (`## Blocked`,
  phase Results, frontmatter); the section re-copies them and the copy ages.
- **The ordering signal is already redundant and already consumed.**
  Inter-ticket prerequisite ordering lives in `related:` / `parent:` frontmatter,
  and the `lead-goal-step` selection subagent already reads those to prefer a
  prerequisite before FIFO. Nothing downstream depends on Ticket Focus for
  ordering.

## Rejected Alternatives

- **(a) Automated guard** wired into `tickets.close`/`tickets.move` or MCP:
  rejected. The section is free-form prose with no structured status field, so
  detecting "which stems it claims as ready" is brittle; and a guard leaves the
  narrative-cache duplication untouched.
- **(b) Documented manual regeneration procedure**: rejected. The procedure
  already exists (the writer/cleaner hooks above plus the `WORKFLOW.md` rule) and
  still drifted this session. Re-stating a rule that already failed does not
  prevent recurrence.
- **Demote to stem-only pointer list** (keep a curated attention set, strip
  status words and duplicated prose): considered, rejected. The attention set's
  only unique value was ordering, which `related:`/`parent:` already carry; a
  pointer list is residual maintenance for marginal value.
- **Introduce a new `dep:` frontmatter field** for prerequisite ordering:
  rejected. `related:` is already `stem: rationale-prose` and `parent:` encodes
  hierarchy, and both are already read by selection. A third dependency
  vocabulary is avoidable; sharper prerequisite emphasis, if ever needed, goes in
  `related:` prose. This removal must not bundle a new-frontmatter convention.

## Phases

### Phase 1: Remove Ticket Focus and its machinery across ws, wsflow, specs, and the bundled convention

Delete the `## Ticket Focus` section from `ai-docs/_index.md` and retire every
surface that reads, writes, maintains, or describes it, keeping the ws and
wsflow trees mirror-consistent. Preserve immutable migration history (the v0041
"`Ticket Queue` → `Ticket Focus`" record is a past-event log, not a live
reference) — supersede it with a new managed-template version entry rather than
rewriting it.

Surfaces to retire (each in `agents-plugin/` **and** its `agents-plugin-wsflow/`
mirror unless noted; confirm the final set with a repo-wide `Ticket Focus`
grep-sweep, since line numbers drift):

- **Section**: `ai-docs/_index.md` `## Ticket Focus` (single tree).
- **Writer**: `rsrc/lead-write-ticket/lead-write-ticket.md` steps that add a
  focus entry on `ready/` promotion (currently ~steps 106-107) — and the
  matching **On: Spec-address Check** step 5 in the rendered `lead-write-ticket`
  playbook doctrine.
- **Cleaner**: `rsrc/executor-wrapup.md` step that removes completed tickets from
  the section (currently ~line 48).
- **Reader instruction (template + generated live copies)**:
  `skills/lead-bootstrap/AGENTS.template.md` "Check `## Ticket Focus` … before
  starting implementation" line (ws ~:79, wsflow ~:80), **and its generated live
  copy in this repo's own managed root `AGENTS.md` (~:199, a template consumer at
  `<!-- Template Version: v0041 -->`)**. devenv consumes its own template, so the
  reader line must leave the managed `AGENTS.md` too — through the bootstrap
  regeneration path below, never a hand-edit (a hand-edit is re-added on the next
  bootstrap upgrade).
- **Semantics / keep-list / mental-model membership**:
  `skills/lead-bootstrap/WORKFLOW.md` (~:47, :107, :120) and its generated live
  copy `ai-docs/WORKFLOW.md`; `rsrc/lead-bootstrap/lead-bootstrap.md` keep-list
  (~:102).
- **Specs**: `ai-docs/spec/documentation-system.md` (~:117, :248) and
  `ai-docs/spec/workflow-skills.md` (~:325).
- **Bundled convention**: `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`
  ("`ai-docs/_index.md ## Ticket Focus` is the selected active attention list …")
  — Go-embedded; update the source so `ws/convention.read(name:
  "ticket-conventions")` no longer names the section. Update the workflow
  manual's Ticket System Concepts text if it names the section.
- **Managed-template propagation**: add a new `AGENTS.template.md` version entry
  (ws next after v0041; wsflow next after v0004) instructing bootstrap to drop
  the Ticket Focus reader instruction and not re-add the section, so managed
  `AGENTS.md`/`WORKFLOW.md` consumers regenerate without it. Bump the template
  version accordingly. **This repo is itself such a consumer** (currently at
  v0041): after the bump, regenerate devenv's own managed `AGENTS.md`/`WORKFLOW.md`
  through the same bootstrap upgrade path so its reader instruction is removed by
  regeneration, not by hand.

Mechanics: edit rsrc bodies through the normal rsrc path and regenerate
`rsrc/manifest.json` (ws + wsflow) plus the wsflow rsrc mirror via the
established regen tests — do not hand-edit hashes. Keep the mirror byte-contract
green.

Deferred / out of scope: no replacement pointer list, no new frontmatter field.
This phase does not change `related:`/`parent:` semantics or the goal-step
selection logic that consumes them.

Verification:
- Repo-wide `grep -ri 'ticket focus'` returns only immutable migration-history
  entries (the superseded version notes) — zero live reader/writer/cleaner/
  semantics/spec/convention references.
- `ws/convention.read(name: "ticket-conventions")` output no longer names the
  section; a fresh `lead-write-ticket` / `executor-wrapup` render carries no
  focus step.
- wsflow rsrc mirror test green (`TestWsflowRsrcMirrorUpToDate`); rsrc + skills
  manifest-up-to-date tests green; `go build ./...`, `go vet ./...`, and the
  `internal/mcp` + `cmd/ws-mcp` + `internal/wsrsrc` suites green; the wsflow
  python bundle suite green; the bootstrap/AGENTS template-version test green
  after the bump (locate it via the template-version constant the bootstrap
  suite asserts, e.g. the `Template Version: vNNNN` marker check).

### Result (367dc6c6) - 2026-07-25

Removed the `## Ticket Focus` section from `ai-docs/_index.md` and retired every
live reader/writer/cleaner/semantics/spec/convention surface across the ws and
wsflow trees: the `lead-write-ticket` promotion-writer step, the
`executor-wrapup` cleaner step, the `lead-bootstrap` keep-list entry, both
`AGENTS.template.md` reader lines, both `WORKFLOW.md` semantics/keep-list/routing
mentions, the Go-embedded `ticket-conventions.md` clause, and the
`documentation-system.md` / `workflow-skills.md` spec prose — replaced with
filesystem-discovery wording (active attention is read from the status
directories via `tickets.list` / `project_tree`, no cached section). rsrc bodies
were edited canonically and the wsflow rsrc mirror plus both `manifest.json`
regenerated via `WS_REGEN_MANIFEST=1` / `WS_REGEN_WSFLOW_RSRC=1`; the mirror
byte-contract stayed green.

Verification: `go build ./...`, `go vet ./...`, and the full `agents-plugin-tool`
`go test ./...` (12 packages) are green; wsflow pytest is 9/9 green (with the
corrected `v0004`→`v0005` lineage assertion); `TestWsflowRsrcMirrorUpToDate` and
the rsrc/skills manifest-up-to-date tests are green; `diff -rq agents-plugin/rsrc
agents-plugin-wsflow/rsrc` is byte-identical. Partitioned review (correctness /
fit / test) is clean: 0 Critical, 0 Important, 4 Minor — all documentation-clarity
or deferred-step notes, no code fix warranted.

Deviations:
- Managed-template version was bumped to `v0044` (ws) / `v0005` (wsflow), not the
  plan's assumed `v0042`: `v0042`/`v0043` had already landed on `main` from
  unrelated work, so the new entry was appended after `v0043` without disturbing
  the immutable `v0041` / `v0004` migration records.
- The mandated full-tree manifest regen incidentally corrected pre-existing
  `lead-goal-step/SKILL.md` hash drift in `agents-plugin/skills/manifest.json`
  (stale since `b8cc7024`, unrelated to this ticket); the manifest now matches the
  file.

Deferred (post-reinstall regeneration): this repo's own managed root `AGENTS.md`
(~:199) and `ai-docs/WORKFLOW.md` (:47, :107, :120) still carry the retired
reader / semantics lines. Per this phase's own contract they must be cleared by
bootstrap *regeneration*, never a hand-edit — and a correct regen needs the
rebuilt plugin carrying the new `v0044` template (the installed binary still
ships `v0043`, so `ws/convention.read` and any bootstrap run today would
reintroduce the removed text). The regen therefore lands after this work ships
and devenv re-bootstraps; captured as follow-up idea ticket
`260725-idea-retire-ticket-focus-root-regen`. Until then a repo-wide `grep -ri
'ticket focus'` returns exactly: the immutable `v0041` / `v0004` migration
bullets, the new `v0044` / `v0005` entries that name the section they retire,
`CHANGELOG.md`, the `260713` dogfood ticket and this ticket's own body, two
historical `.plans/` docs, and the two deferred-regen root files above.

## Spec Impact

- `ai-docs/spec/documentation-system.md` — remove the `## Ticket Focus`
  description from the `_index.md` structure/lifecycle prose; state that active
  attention is discovered from the status directories via `tickets.list` /
  `project_tree`, not a cached section.
- `ai-docs/spec/workflow-skills.md` — remove the "`Ticket Focus` entries are
  maintained …" clause; drop any skill-behavior contract that writes or cleans
  the section (the `lead-write-ticket` promotion step and `executor-wrapup`
  cleanup step).

No new spec anchor is introduced; this is contract *removal*, so `## Spec Impact`
(not a contract-first `spec:` entry) addresses the phase.

## Constraints

- Keep the ws and wsflow trees mirror-consistent; the mirror byte-contract test
  is the gate.
- Do not rewrite immutable migration history; supersede it with a new
  template-version entry.
- Introduce no new frontmatter field and no replacement section; the removal is
  the whole change.
- This retires a managed-template behavior that regenerates downstream projects'
  `AGENTS.md`/`WORKFLOW.md`; that downstream-facing change was approved in
  discussion (2026-07-25) before this promotion.
- All AI-authored ticket/doc/commit content stays English.
