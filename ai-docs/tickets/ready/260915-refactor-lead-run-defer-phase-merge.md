---
title: "Move lead-run's phase-completion default from merge to continue, and guarantee impl-branch deletion on mid-ticket merge"
related:
  260910-feat-lead-run-worktree-parallel-route: overlaps lead-run Select + merge-timing; this ticket lands first and sets the phases-stack/merge-is-need-driven baseline, not edited here
  260911-feat-impl-derivation-hardening-branch-aware-select: foundation (done); its route.resolve_implement "continue" verdict is what lets consecutive phases stack on the persistent impl branch
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 856955c97db245e3
sage-review-completeness-reviewed: 856955c97db245e3
---

# Move lead-run phase-completion default from merge to continue

## Background

`lead-run` merges the impl branch on every accepted `stop: none` worker report,
regardless of whether the report is `completion: phase` or `completion: ticket`
(`agents-plugin/rsrc/lead-run/lead-run.md`, "Handle the report": the merge
instruction keys on `stop: none`, and only the post-merge branch distinguishes
phase from ticket). Combined with deterministic impl-branch naming — the branch
stem is derived purely from the ticket stem (`wskey.Derive(ticket_stem, 3)` in
`agents-plugin-tool/internal/mcp/implement_resolver.go`; every phase of one
ticket derives the SAME `impl/<root>/<stem>` branch, proven by
`TestResolveImplementSameTicketStemAcrossPhasesContinues`) — this makes
per-phase merge an actively harmful default: each phase completion merges (which
deletes the impl branch via `git branch -d`, `git_merge.go` success path) and
the next phase re-creates the same-named branch, forcing delete/recreate churn
on every phase boundary even when nothing depends on that phase landing.
Downstream this surfaces as impl-branch collision clutter on multi-phase
tickets.

The default should be "continue to the next phase on the persistent impl
branch," not "merge." `route.resolve_implement` already returns a `"continue"`
verdict when the deterministic branch still exists (landed by
`260911-feat-impl-derivation-hardening-branch-aware-select`), so the machinery
for stacking consecutive phases already exists; `lead-run` just needs to stop
forcing a merge at each boundary.

Merges do not disappear — they become need-driven, not mechanical. A ticket
still merges at completion, and a mid-ticket merge is legitimate when a landing
is needed (for example a dependent ticket blocked on this phase). All such
merges keep the existing user-approval gate; auto-merge is explicitly rejected
because a non-goal run may be working on `main`/master or another protected
branch where an unattended merge is itself a hazard.

## Decisions

- **Phase-completion default is "continue," not "merge."** On an accepted
  `stop: none` + `completion: phase` report, `lead-run` does NOT merge. It
  retains the impl branch on the assignment note as active/unmerged and ends the
  turn with a continue verdict; the next phase stacks on that branch via
  `route.resolve_implement`'s existing `"continue"` verdict. Rejected:
  auto-continuing within one lead invocation (spawning worker after worker until
  a blocking stop) — the one-worker-per-invocation invariant and the per-phase
  turn boundary are kept, so the user retains a lightweight control point at each
  phase without being handed a merge decision.
- **Merges stay user-gated; no auto-merge is introduced.** A merge (at ticket
  completion, or a deliberate mid-ticket landing) goes through the existing
  `merge_confirm` gate (`ask`/absent → user approval). Rejected: authorizing
  auto-merge (`skip`) for autonomous drain runs — a non-goal run may sit on
  `main`/master or a protected branch, so unattended integration is a hazard.
  The broader "move the control point off per-merge approval" idea is deferred
  to a separate ticket, not this local fix.
- **selector re-spawn is skipped for an active ticket.** On re-invocation,
  before rendering `ticket-selector`, `lead-run` checks `session.children` for an
  active, unmerged, phases-remaining assignment; if present it skips selection
  and dispatches that ticket's next phase directly. An invocation that names a
  ticket explicitly still takes priority. A blocked assignment (an unresolved
  user stop) is NOT auto-continued.
- **New continue verdict.** End-the-turn gains a variant for "this ticket has
  autonomously-advanceable phases remaining; re-invoke `lead-run` to continue,"
  distinct from the generic ready-queue-advanceable line and keeping
  `finished`/`complete`/`done` out. The generic ready-queue line remains for the
  no-active-ticket case.
- **Fresh worker per phase is preserved.** Each phase is still executed by a
  freshly spawned worker (`ticket-worker` runs exactly one phase per invocation,
  `ticket-worker.md` Inputs); the only change is that the lead does not merge
  between phases.
- **Mid-ticket merge guarantees impl-branch deletion.** A merge performed while
  the ticket is not yet closed must remove the impl branch, so a later same-ticket
  phase re-derives the same deterministic name cleanly. See Phase 2.

## Constraints

- Convention: `ai-docs/manuals/skill-authoring.md` (`lead-run` is a shipped
  playbook).
- Convention: `ai-docs/manuals/wsflow-mirroring.md` (mirror Phase 1 to
  `agents-plugin-wsflow/`).
- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (the
  continue/verdict/selector-skip text must ride generic Route Facts plus
  session/assignment state, not any repo-only fact).
- Convention: `ai-docs/manuals/ws-mcp.md` (Phase 2 touches `git.merge` / the
  implement resolver under `agents-plugin-tool/internal/mcp/`).
- Reconciliation with `260910-feat-lead-run-worktree-parallel-route` (ready):
  both edit `lead-run`'s Select and merge-timing. This ticket lands first and
  establishes the "phases stack, merge is need-driven" baseline; 260910's
  Phase 2 "merge the finished branches" text is read against that baseline when
  it executes. This ticket does not edit 260910.

## Prior Art

- `260911-feat-impl-derivation-hardening-branch-aware-select` (done):
  `route.resolve_implement`'s branch-aware `"continue"` verdict for a same-ticket
  re-entry; `TestResolveImplementSameTicketStemAcrossPhasesContinues` is the
  contract Phase 1's phase-stacking relies on.
- `git_merge.go` success path: the existing `git branch -d` on any `impl/` branch
  after a successful merge — the deletion Phase 2 hardens into a guarantee.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-wsflow/ mirror, agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin-tool/internal/mcp/git_merge.go |
| scope.surface | public-interface | Phase 1 changes lead-run.md's user-facing merge/continue behavior; Phase 2 changes route.resolve_implement's branch-plan verdict (implement_resolver.go#L879-L897) and git.merge's cleanup-failure handling (git_merge.go#L304-L308), both existing MCP tool contracts |
| scope.new_public_symbol | no | no new MCP tool or exported symbol named; behavior changes to existing lead-run.md, route.resolve_implement, and git.merge |
| scope.new_type_contract | unknown | Phase 2's "clean/reset it or surface a clear resolution" and "fail-loud" wording leave the exact new field/verdict/error shape undecided |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go, agents-plugin-tool/internal/mcp/git_merge_test.go, agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py all already exist |
| complexity.reuse_points | confirmed | route.resolve_implement's existing "continue" action (implement_resolver.go#L940-L943) and observeImplementBranch's already-computed obs.TargetExists field (implement_resolver.go#L605-L609, currently unused on the create path) are the reused primitives |
| complexity.side_effect_risk | high | Phase 2's create-path "clean/reset" of a leftover impl branch can discard unmerged commits if implemented without the existing AheadOfMergeRoot check (implement_resolver.go#L944-L951) |
| risk.correctness | high | changes the default merge timing for every multi-phase ticket lead-run drives system-wide, and a wrong leftover-branch reset can lose work |
| risk.fit | moderate | reuses landed primitives (260911's continue verdict, session.children) but must reconcile with 260910 (ready) editing the same Select/merge-timing sections, per this ticket's own Constraints |
| risk.test | moderate | verification spans three existing suites (Go table tests, skill-dispatch-contract tests, wsflow mirror-drift tests) with no single suite covering the whole change |
| risk.security_or_contract | moderate | changes lead-run's default merge-timing contract (an observable workflow-behavior change under AGENTS.md's Approval Protocol) while explicitly preserving the existing merge_confirm approval gate |

## Phases

### Phase 1: Move lead-run phase-completion default to continue

Edit `lead-run`'s "Handle the report," "Select," and "End the turn":

- Remove the mechanical per-phase merge: on `stop: none` + `completion: phase`,
  do not merge; retain the impl branch on the assignment note as active/unmerged
  and continue. Merge only on `completion: ticket`, or a deliberate mid-ticket
  landing the lead performs when a dependent needs it — both through the existing
  user-approval `merge_confirm` gate.
- Assignment-note lifecycle: the note moves through explicit states —
  `dispatched` → `active` (impl branch retained, unmerged, phases remain) →
  `merged` (on ticket completion) or `blocked` (on an unresolved user stop).
  Select's continue-detection keys on the `active` state; `blocked` is not
  auto-continued and `merged` ends the ticket. (Exact marker/field names are an
  autonomous implementation choice within these states.)
- Select: before rendering `ticket-selector`, check `session.children` for an
  active/unmerged/phases-remaining assignment and, if present, skip selection and
  Spawn the next phase on that branch. Explicit named-ticket invocation still
  wins; a blocked assignment is not auto-continued.
- End the turn: add the continue verdict variant (autonomously-advanceable
  phases remain → re-invoke `lead-run`), keeping the `finished`/`complete`/`done`
  ban intact.
- Mirror to `agents-plugin-wsflow` per `wsflow-mirroring.md`.

Verification: playbook/skill-shim tests that (a) phase completion does not merge
and retains the branch, (b) ticket completion still merges through the user
gate, (c) re-invocation with an active advanceable assignment skips the selector
and continues the next phase, (d) the continue verdict is emitted and a
single-phase ticket is unchanged (one merge at its only completion). Mirror-drift
test in `agents-plugin-wsflow`.

### Result (260dd93) - 2026-09-15

Landed the phase-completion default change in `lead-run`. On `stop: none` +
`completion: phase` the lead no longer merges: it marks the assignment note
`active` (impl branch retained, unmerged, phases remain), leaves the ticket
active, and ends with the new continue verdict; the next phase stacks on the
deterministic impl branch via `route.resolve_implement`'s existing `continue`
verdict. Merge stays user-gated (`merge_confirm` skip/ask) and fires only at
`completion: ticket` or a deliberate, need-driven mid-ticket landing — no
auto-merge added. Select gained active-assignment continue-detection
(`session.children`, scope `control`) that skips `ticket-selector`; named-ticket
invocation still wins and `blocked` is not auto-continued. Note lifecycle:
`dispatched` → `active` → `merged`/`blocked`.

- Edited `agents-plugin/rsrc/lead-run/lead-run.md` (Select, Spawn step 4 note
  lifecycle, Handle the report, End the turn).
- Mirrored byte-identically to `agents-plugin-wsflow/rsrc/` and
  `agents-plugin-pi/rsrc/` (three mirror consumers, not two — `agents-plugin-pi`
  hand-mirrors `agents-plugin/rsrc` under `TestPiMirrorUpToDate`); manifests
  regenerated for all three.
- Tests: added `test_run_defers_phase_merge_and_continues_active_assignment`
  (agents-plugin) covering (a)–(d); updated the stale phase-wording assertions in
  `test_workers_report_and_lead_owns_impl_merge` (agents-plugin),
  `test_wsflow_run_and_stop_protocol_carry_merge_obligation_text` (wsflow), and
  the rendered-policy `TestPlaybookPrintLeadRunWorkerTierPolicy` (Go, ws+wsflow).

Verification: `python3 -m unittest discover agents-plugin/tests` (70 ok);
`python3 -m unittest discover agents-plugin-wsflow/tests` (12 ok);
`go test ./... -count=1` in `agents-plugin-tool` (14 packages ok, incl.
`TestPiMirrorUpToDate`, `TestWsflowRsrcMirrorUpToDate`, manifest guards).

Review: partitioned correctness/fit/test — round 1 all `clean` with one Minor
each (Select continue-path wording; thrice-restated `active`-state gloss;
duplicate assertions across two Python methods). Fixed the first two (commit
f820319f); kept the duplicate assertions as distinct contracts. Round 2 verifier
`clean`, no remaining findings; one cosmetic long-line observation reflowed
(commit 260dd93).

Decisions taken (all recorded in commit `## AI Context`, none escalated): kept
the existing `merge_confirm: skip` goal-run auto-merge path (decision 2 rejects
*adding* auto-merge, not removing the existing goal gate); encoded note states
as freeform note text per the ticket's "exact markers are an autonomous choice";
resynced the third (`agents-plugin-pi`) mirror the ticket's Constraints did not
name.

### Phase 2: Guarantee impl-branch deletion on mid-ticket merge

Make impl-branch deletion a guarantee so a same-ticket phase re-entered after a
mid-ticket merge never collides with a leftover branch:

- A successful impl-branch merge removes the branch (the `git_merge.go` success
  path already does this). Harden the failure mode so a cleanup failure is not
  silently swallowed into an advisory that leaves an orphan the next phase
  cannot see. Because the merge itself succeeded, do NOT convert it to a
  hard/fatal error (that would misreport a successful merge) — surface it as a
  loud, visible non-fatal diagnostic (mirroring `lead-run`'s existing
  release-target-acknowledgement surfacing), with the create-path leftover
  detection below as the actual recovery.
- The create-path branch resolution must detect an existing target impl branch
  (`implement_resolver.go` create path currently checks only an ancestor
  directory/file ref conflict via `obs.MergeRootRefConflict`, not exact target
  existence, even though `obs.TargetExists` is already computed — so a leftover
  branch is invisible there) and resolve it under the existing
  `AheadOfMergeRoot` guard, never with a raw `git switch -c` error reaching the
  worker:
  - Leftover branch with **no commits ahead of merge-root** (the failed-`-d`
    remnant of a just-succeeded merge — already fully landed): clean it by
    delete-then-recreate — `git branch -d` the fully-landed leftover (the safe
    delete succeeds precisely because it has no unique commits), then proceed
    with the normal create — rather than resetting the ref in place. This is
    what delivers clutter-free re-entry.
  - Leftover branch **ahead of merge-root** (un-landed unique work — a genuine
    same-name collision, not a cleanup remnant): surface a stop; never discard
    the un-landed commits.
- Absorbs the dropped idea `260915-bug-merge-cleanup-fail-orphans-impl-branch`.

Verification: Go table tests under `agents-plugin-tool/internal/mcp/` for the
create path detecting a leftover impl branch, and for a merge cleanup failure
being surfaced rather than leaving a silent orphan.
