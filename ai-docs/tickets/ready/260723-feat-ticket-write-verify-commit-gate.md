---
title: "ticket.verify + commit-gate mechanical backstop, then must-not-forget mutation-tool collapse"
related:
  260627-bug-write-ticket-bypasses-tickets-create: closes the exact bypass hole its prose-only fix left open
  260624-feat-tickets-template-tool-and-convention-diet: prior offload of ticket text into MCP tools; this continues by moving validation to a gate
parent: 260723-epic-ticket-write-reshape
sage-review-design: completed
sage-review-completeness: completed
---

# ticket.verify + commit-gate mechanical backstop, then must-not-forget mutation-tool collapse

## Background

The ticket-write path has three structural problems:

1. **No mechanical backstop.** Mechanical guardrails (stem format, status/dir
   consistency, frontmatter integrity, sage-posture presence/value, spec-address
   presence, phase/Result structure) are scattered across `ticket_create.go`,
   `tickets.go`, and `tickets_mutate.go`, enforced only *before* commit by
   playbook discipline. A lead that hand-edits a ticket file bypasses all of it —
   the `260627` fix for this was prose-only, adding no enforcement.
2. **Round-trip weight.** The write path makes ~8-10 MCP calls
   (`convention.read`, `template`, `checklist`×2, `create`, `sage_gate`,
   `sage_record`, `git.commit`), each with schema + response overhead; agents
   already bypass the mutation tools and edit files directly.
3. **Front-loaded procedure.** The playbook restates each action's follow-on
   obligations up front rather than surfacing them at the moment of action.

`git.commit` already runs a validation step (`validateCommitStatus`) before
`git commit -m`, but it does **zero** ticket-content validation. That slot is the
natural host for a deterministic verify gate.

**Chokepoint scope (must resolve in Phase 1).** Hosting verify inside
`ws/git.commit` covers every commit routed through that tool, but a raw
`git commit` from the shell after a hand-edit bypasses it — re-opening a
`260627`-class hole on the shell path. Before leaning on the "every path" premise,
confirm the true universal chokepoint: a git pre-commit hook that runs verify, or
a workflow mandate that all ticket-touching commits go through `ws/git.commit`.
The Drop criterion already sanctions abandoning the `git.commit` host if no
equivalent chokepoint exists.

## Decisions

- **verify = mechanical floor; sage = semantic ceiling.** `ticket.verify` checks
  only file-state-deterministic guardrails and confirms the sage *result* stamp
  is present/valid; it never judges prose quality or design soundness.
- **Commit-gate makes verify non-optional.** Hosting verify at `git.commit`'s
  validation slot means an invalid ticket cannot be *landed*, even if the agent
  skipped a voluntary verify during editing. verify is also callable directly for
  red-green feedback mid-edit.
- **Must-not-forget filter governs tool survival.** Keep a thin tool only where it
  bundles a catastrophic-to-forget follow-on; free the rest to free-edit under the
  verify floor. Seed classification matches today's hard/soft choices (ready→sage
  is hard; close→phases-resolved is soft).
- **sage stamp is lead single-writer.** This *preserves* today's property — per
  the current `sage-gate-record-tools` spec the lead-called `tickets.sage_record`
  is already the sole frontmatter writer and reviewers already return verdicts
  only. `sage.stamp` is the thin, explicitly-named lead-only replacement for
  `sage_record`'s write (writes `completed`/`blocked`, renders the `## Blocked`
  companion); the single-writer property is what keeps reviewers from racing if
  they were ever allowed to raw-edit — it is not fixing a race that exists today.

### Rejected alternatives

- *Delete all ticket mutation tools, keep only verify* — rejected: something must
  still write frontmatter / move files; deletion targets *validation*, not
  *mutation*, and high-stakes fiddly writes (sage stamp) keep a thin tool.
- *Reviewers raw-edit the sage posture* — rejected: concurrent reviewers would
  race on the same frontmatter; the lead-single-writer property (already true via
  `sage_record`) is preserved by `sage.stamp` instead.

## Phases

### Phase 1: verify() + commit-gate backstop (pure addition)

Add a deterministic `ticket.verify(paths)` that runs the mechanical guardrail set
(stem regex, status/dir consistency, frontmatter fence integrity, ready-landing
sage-posture presence/value, phase/Result **structural presence** only, close
date-field presence). Two scope boundaries, both settled by design review:

- **spec-address stays soft-warn here, not hard.** verify surfaces a
  non-blocking warning when a ready-landing ticket lacks spec addressing, matching
  today's `tickets.move` tip. Promoting it to a hard reject is deferred scope
  owned by `260723-feat-ready-spec-address-hard-gate` (blocked on the collocator);
  this ticket must not land that promotion.
- **phase/Result is presence/well-formedness only.** The append-only convention
  ("do not edit phase text after a `### Result`; append `#### Edition`") is a
  diff-level property a snapshot check cannot enforce; verify does not attempt it.

Host it at `wsgit.Client.Commit` after
staging and before `git commit -m`, as a sibling of `validateCommitStatus`, so
every ticket-touching commit is gated; keep it callable standalone for mid-edit
feedback. This phase is purely additive — existing tools stay — and immediately
closes the `260627` direct-file-edit bypass hole. Returns actionable prose on
failure (which guardrail, which file, what to fix), with a bounded
retry/escalation contract so the red-green loop cannot thrash indefinitely.

**Single source of truth.** verify is the one home for these mechanical rules;
where a residual mutation tool (e.g. the ready-move sage-posture check) still
enforces the same rule, it delegates to verify rather than duplicating logic —
otherwise the phase re-creates the scatter the epic set out to remove.

**Escalation terminal behavior.** The bounded retry/escalation contract must
never let an invalid commit through — "bounded" means the red-green loop stops
and requires a human/lead fix, not an override that lands a bad ticket. The
non-bypassable guarantee wins over loop convenience.

**Acceptance check:** each hard guardrail fires on a deliberately invalid ticket
fixture (bad stem, wrong status dir, missing ready sage-posture, malformed
frontmatter, malformed phase/Result headings); a commit staging such a ticket is
blocked at the gate; the spec-address case emits a *warning*, not a block; and a
standalone `ticket.verify(paths)` call returns the same verdict as the gate for
identical input (call-site parity). Go test coverage for these cases passes.

### Result (9744429) - 2026-07-23

`wsdoc.TicketVerify(root, paths)` ships seven hard guardrails (stem, status-dir
against the canonical five dirs, file-exists, frontmatter-fence integrity,
ready-landing sage-review posture, close-date field, phase/Result heading
well-formedness) plus a soft spec-address warning, exposed as a standalone
`tickets.verify` MCP tool + CLI subcommand and wired as a commit-time gate on
`wsgit.Client.Commit`. Spec addressing recorded under `260723-tickets-verify-tool`
and `260723-git-commit-ticket-verify-gate` (commit `44e530a`).

- **Import boundary preserved.** The gate uses a `Verifier func(root, paths)
  error` seam on `wsgit.Client` (mirroring the existing `Runner` field) so
  `internal/wsdoc` never imports `internal/wsgit`, honoring
  `{#260720-wsdoc-commit-boundary}`; MCP dispatch and the CLI each construct the
  client with a `verifyAdapter`.
- **Single source of truth.** A pure `readyPostureProblems` predicate extracted
  in `tickets_mutate.go` is shared by both the ready-move check and verify. Its
  signature deviates from the plan's illustrative `(fm, stem)` form — it takes
  resolved posture values + required-ness so a hand-authored ready ticket with a
  genuinely unset required stage reports "unset" rather than being misread as
  not-applicable; caught by `TicketVerifyReadySagePostureGuardrail/missing`
  before shipping.
- spec-address is soft-warn only (never fails `OK`); phase/Result is
  presence/well-formedness only (append-only not attempted); the gate is
  non-overridable (an invalid staged ticket blocks the commit, `HEAD` unmoved).
- **Deviation:** added `tickets.verify` to both `runtime.json` trees and
  `runtimeCapabilityCommandNames()` (outside the plan's file list) — required by
  the launcher-contract test, a mechanical same-pattern addition matching sibling
  `tickets.*` tools.

Verification: `go build ./...`, `go vet ./...`, `go test ./...` all clean (12
packages, 0 failures). Review (partitioned): fit clean; test clean + 3 minor
coverage-gap notes; correctness's lone "critical" was a **false alarm** — a
review subagent left an uncommitted `if false` edit disabling the stem guardrail
in the shared working tree, which a sibling reviewer then ran tests against. The
committed tip `9744429` has the correct `ticketStemRE` check and passes the stem
tests; the stray edit was discarded and the tree re-verified green.

Deferred (minor, none blocking Phase 2): (1) an empty ticket file in
`todo/`/`idea/` passes verify (documented choice); (2) phase/Result match could
over-fire on `### Results summary` (low likelihood; convention uses the exact
form); (3) three narrow-branch test gaps (missing-opening-fence,
`sageReviewStageError` default branch, `.dropped` happy-path). The shared-worktree
reviewer-contamination hazard is captured separately as an `idea/` ticket.

### Phase 2: must-not-forget mutation-tool collapse + action-time obligation prose

With the verify floor in place, collapse the mutation surface. Produce the
**complete per-tool keep/collapse disposition** by applying the must-not-forget
filter tool-by-tool (not only the seed cases below); the seed classification is
ready→sage hard, close→phases soft. Concretely:

- Rename `tickets.create` → an attention-salient name (e.g. `create_empty` /
  `create_template`) whose return prose states it yields a *valid empty skeleton +
  initial posture*, not a full mutation orchestrator.
- Free `tickets.close` to free-edit + soft verify-warn on unresolved phases (no
  hard block) — matches the flexible end of the filter.
- Keep the ready-move hard enforcement (sage posture) — the catastrophic end.
- Add the thin lead-only `sage.stamp` tool; reviewers stop writing frontmatter.
- Replace front-loaded playbook procedure with **action-time obligation prose**:
  each tool/gate return surfaces the must-know follow-on for that action.

**Acceptance check:** the renamed create tool's return prose states the
"valid empty skeleton + initial posture" caveat; `tickets.close` on a ticket with
unresolved phases soft-warns without blocking; a ready move still hard-blocks on
missing/blocked sage posture; `sage.stamp` writes the posture + `## Blocked`
companion and is exercised only via the lead path (reviewers no longer write
frontmatter); and the write-ticket path's front-loaded procedure prose is
measurably reduced (net token count on the base path drops). Go tests cover the
close soft-warn and ready hard-block cases.

### Result (fcb96383) - 2026-07-23

Collapsed the ticket mutation surface behind the Phase 1 verify floor. Complete
per-tool disposition applied via the must-not-forget filter:

- `tickets.create` → renamed **`tickets.create_empty`** (MCP tool; CLI
  `create-empty`), kept thin with its unchanged design-review hard block; the
  schema description and `formatTicketCreate` return prose both state the "valid
  empty skeleton + initial posture, not a full mutation orchestrator" caveat.
- `tickets.sage_record` → renamed **`tickets.sage_stamp`** (kept in the
  `tickets.*` family, reusing `wsdoc.SageRecord`'s `(stem, stage, verdicts[])`
  contract verbatim) and newly gated **lead-only** via `isLeadOnlyTool` — the
  pre-rename tool had no such gate; reviewers never call it. This is the thin
  lead-only replacement named `sage.stamp` in the plan text.
- `tickets.close` → **freed** to free-edit + soft warn: new
  `ticketUnresolvedPhaseWarning` predicate, wired as a non-blocking tip in
  `TicketsClose` and a soft `unresolved-phases` warning in `TicketVerify`'s
  `.done`/`.dropped` branch; never a hard block.
- `tickets.move` ready-landing sage-posture stays **hard**, single-sourced
  through `readyPostureProblems` (delegates to the verify floor, no duplication).
- `tickets.verify` (Phase 1) gains the same soft unresolved-phases warning plus
  `next_instruction` lines on FAIL/WARN; `tickets.sage_gate` unchanged.

Front-loaded playbook procedure replaced with **action-time obligation prose**:
`formatTicketCreate`/`ticketMutateNextInstruction`/`formatTicketVerify` emit
`next_instruction:` only when a real follow-on exists; `lead-write-ticket.md`
dropped 205→203 lines / 2069→2006 words. Rename rippled across all caller
surfaces (MCP dispatch/schema/allowlist/`isLeadOnlyTool`, CLI subcommand + usage
+ `runtimeCapabilityCommandNames()`, both `runtime.json` trees, rsrc prose +
regenerated manifests in both plugin trees). Spec (`mcp-tools`) and mental-model
(`mcp-runtime`) updated on contact; ticket-number-keyed spec anchors deliberately
not renamed.

- **Naming deviation from plan.** The plan wrote `sage.stamp` (new `sage.*`
  namespace); implemented as `tickets.sage_stamp` in the existing `tickets.*`
  family per explicit caller decision, reusing `SageRecord`'s contract verbatim
  rather than authoring a new write. `tickets.create` renamed to `create_empty`
  (the plan's `create_empty`/`create_template` option A).

Commits `f3748b09..5e7d9511` (5) + review-fix `fcb96383`; version bumped
0.35.1→0.35.2. Review (partitioned): correctness clean; fit clean (2 minor — a
stale pre-rename comment, fixed in `fcb96383`; and this closeout); test found one
critical — the `create_empty` caveat prose had no asserting test — closed in
`fcb96383`, which also added the missing `ticketMutateNextInstruction` /
`formatTicketVerify` next_instruction branch tests. `go build`/`go vet`/`go test
./...` all green (12 packages).

By-design notes carried forward: a `.dropped` ticket with unfinished phases
always soft-warns (dropping implies unfinished — intended); the `### Result`
prefix also matches `### Results…` (suppression-only, low likelihood). The
shared-worktree reviewer-contamination hazard from Phase 1 remains tracked in
`idea/260723-research-reviewer-worktree-isolation`.

## Spec Impact

- Target spec area: `mcp-tools` — new `ticket.verify` contract, the commit-gate
  behavior, the `tickets.create` rename, the freed `tickets.close` semantics, and
  the new `sage.stamp` tool with reviewers-return-prose-only.
- Expected caller-visible change: playbooks call a smaller mutation surface + a
  verify gate; direct file edits are now caught at commit; sage posture is written
  by a lead-only stamp.
- Contract-first spec: no — the exact tool signatures and prose are still
  design-level and will be refined during implementation; the ticket phases carry
  the behavioral intent and the spec update follows as closeout.
