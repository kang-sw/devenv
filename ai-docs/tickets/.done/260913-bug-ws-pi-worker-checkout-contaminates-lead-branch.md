---
title: "Worker checkout can contaminate lead-owned commits"
related:
  260909-epic-ws-worker-interpreter-refoundation: constraint — lead-run/ticket-worker are the shared worker-interpreter surface this touches
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7f3a5b86ff2cb6c4
sage-review-completeness-reviewed: 7f3a5b86ff2cb6c4
completed: 2026-09-13
---

# Worker checkout can contaminate lead-owned commits

> Upstream report raised from a downstream project (ws Pi). The Pi-side stems
> and the goal-run reproduction below are that project's, not this repo's; the
> fix lands here in the shared lead-run/ticket-worker playbook surface. The
> originating downstream tickets were `feat-ws-pi-bounded-web-access-for-explore`
> (the dogfood run that exposed the shared-worktree branch leak) and
> `feat-ws-pi-recursive-worker-subtree-lifecycle` (worker lifecycle preserving
> parent completion and state boundaries).

## Background

During the Pi goal run for bounded web access, the ticket worker created and checked out its `impl/goal/...` branch in the same Git worktree used by the lead. The checkout was worktree-global, not agent-process-local. After the worker returned stop `(c)`, the lead correctly authored ticket Editions and a localized hotfix but did not first restore its goal branch, so six lead-owned commits landed after the unfinished implementation tip.

Recovery required preserving the contaminated tip, selectively transplanting only lead-owned commits to the goal branch, excluding worker implementation ancestry and review findings, and resetting the inactive impl ref to the worker's last intended checkpoint.

## Decisions

- **Root cause is branch-unawareness, not missing isolation.** When a worker checks out its `impl/...` branch in the shared worktree, the worktree-global `HEAD` stays on that branch after the worker returns. Contamination happens only because the lead then performs HEAD-relative writes without noticing which branch it is on — not because the checkout was shared. The fix is to remove the unawareness, not the sharing.
- **Mandate awareness; leave the rest to lead discretion.** On receiving a worker's terminal (stop-condition) report, before any lead-owned HEAD-relative write (ticket Edition/revision, hotfix commit, follow-up dispatch base), the lead must call `ws/git.status`, read `branch.head` and `impl_ticket`, and make an explicit stack-vs-return decision: stack on the impl branch when the write belongs to that impl ticket; check out the base branch when the write is unrelated or the ticket is fully blocked and the lead is exiting. Everything past that check is lead judgment; there is no fail-closed machinery.
- **The command is a forcing function, not the requirement.** Running `ws/git.status` and ignoring its result reproduces the same unawareness. The enforced obligation is *awareness plus the branch decision*, not the bare call.
- **Base branch is derivable, not recorded.** The return target is encoded in the impl branch name (`impl/<root>/<stem>` → `<root>`, the same convention `ws/git.merge` already uses), so a compacted lead reconstructs it from `ws/git.status` alone. Do not add `base_branch`/`base_oid`/`impl_oid` handoff fields to the assignment note.
- **Branch-explicit lead operations are already safe.** `ws/git.merge` names its source and target explicitly and is unaffected by the current checkout; the awareness step targets only HEAD-relative writes, not branch-explicit tools.
- **Reject worktree isolation as the main strategy.** Per-worker git worktrees would remove HEAD sharing but impose a worktree burden on downstream projects and on developers unfamiliar with worktrees, for a plugin that ships broadly. Do not adopt it as the default lead-run flow; a future isolated-worktree design may still be considered separately.
- **Use the shared workflow source.** Author this awareness contract on `develop` in the shared lead-run playbook text (and ticket-worker where it documents the handoff), not in host-local runtime enforcement. Regenerate the required shipped mirrors; do not allocate a worktree implicitly.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin/rsrc/ticket-worker/ticket-worker.md, agents-plugin/rsrc/worker-stop-protocol.md (shared `worker-stop-protocol` include holding the Report block and Branch section; also included by ticket-worker-escalated.md and ticket-worker-elevated.md), and their required shipped mirrors |
| scope.surface | public-interface | lead-run and ticket-worker are shared shipped workflow interfaces |
| scope.new_public_symbol | no | no new tool or exported symbol is required; the awareness step reuses the existing ws/git.status impl_ticket surface |
| scope.new_type_contract | no | the light approach records nothing new — no base_branch/base_oid/impl_oid handoff fields are added to notes or reports |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go and internal/wsrsrc tests cover shared playbook rendering and mirror integrity |
| complexity.reuse_points | confirmed | ws/git.status already surfaces branch.head + impl_ticket (state/stem/path/status); the change is playbook text over that surface |
| complexity.side_effect_risk | low | text-only playbook change; adds a read-only status check and a judgment prompt, no destructive or halting machinery |
| risk.correctness | moderate | stack-vs-return guidance must name both branches of the decision or a lead could still mis-target a HEAD-relative write |
| risk.fit | moderate | the awareness contract must stay host-neutral in shared shipped playbook text |
| risk.test | low | verified by mirror-integrity and playbook-render tests; no behavioral regression harness is required for a text contract |
| risk.security_or_contract | moderate | branch-awareness is a workflow-safety contract, but advisory (judgment-based), not fail-closed enforcement |

## Phases

### Phase 1: Add a mandatory branch-awareness step to lead-run stop handling

On `develop`, update the shared lead-run playbook and the shared `worker-stop-protocol` include (`agents-plugin/rsrc/worker-stop-protocol.md`, which holds the Report block and Branch section and is also pulled into `ticket-worker.md`, `ticket-worker-elevated.md`, and `ticket-worker-escalated.md`) so that, on a worker's terminal stop-condition report, the lead must call `ws/git.status` and read `branch.head`/`impl_ticket` before any HEAD-relative lead-owned write (ticket Edition/revision, hotfix commit, follow-up dispatch base), then make an explicit stack-vs-return decision stated in the playbook text: stack on the impl branch when the write belongs to that impl ticket; check out the derived base branch (`impl/<root>/<stem>` → `<root>`) when the write is unrelated or the ticket is fully blocked and the lead is exiting. Do not add base/impl handoff fields to the assignment note, do not add fail-closed refusal machinery, and do not allocate a worktree. Regenerate the required shipped mirrors.

Verify: the awareness step is present in the shared source and its shipped mirrors (mirror-integrity); the stack-vs-return guidance names both branches of the decision; the base-branch derivation matches the `ws/git.merge` `impl/<root>/<stem>` convention; and `ws/git.merge`'s existing branch-explicit behavior is documented as unaffected by the current checkout. No behavioral regression harness is required because the change is playbook text; the 2026-09-13 six-commit contamination is cited as the motivating example, not a coded test.

### Result (b8a6f827) - 2026-09-13

Landed on `impl/goal/develop/amber-quill-fern/body-crazy-carol`:

- `agents-plugin/rsrc/lead-run/lead-run.md` "Handle the report": before any
  lead-owned `HEAD`-relative write (ticket Edition/revision, hotfix commit,
  follow-up dispatch base), mandates `ws/git.status` and reading
  `branch.head`, `impl_ticket`, and the working-tree state, then an explicit
  stack-vs-return decision — stack on the impl branch when the write belongs
  to that impl ticket, or check out the derived base branch (everything
  between `impl/` and the last `/`, the same convention `ws/git.merge` already
  uses) when the write is unrelated or the ticket is fully blocked and the
  lead is exiting. A dirty working tree at that point is the lead's own
  judgment call (commit or stash first), not fail-closed refusal.
  `ws/git.merge`'s branch-explicit behavior is documented as unaffected.
- `agents-plugin/rsrc/worker-stop-protocol.md` "Branch" section: documents the
  handoff from the worker's side — the terminal report does not restore the
  checkout, and checking branch state before the next write is the lead's
  responsibility, not the worker's. This include reaches `ticket-worker.md`,
  `ticket-worker-elevated.md`, and `ticket-worker-escalated.md` unchanged.
- Regenerated `agents-plugin/rsrc/manifest.json` and the byte-identical
  `agents-plugin-wsflow/rsrc/` mirror (`lead-run.md`, `worker-stop-protocol.md`,
  `manifest.json`) per `wsflow-mirroring.md`'s after-edit checklist.
- Added a presence pin for the new obligation to the existing
  `test_workers_report_and_lead_owns_impl_merge` in
  `agents-plugin/tests/test_skill_dispatch_contracts.py`, alongside its
  sibling pins for this same file.

Decisions taken during round-1 review fixes (`ae474ee8` -> `b8a6f827`):

- Reworded the base-branch derivation from a literal `impl/<root>/<stem>` ->
  `<root>` notation to "everything between `impl/` and the last `/`" — the
  original notation reads as a single path segment, but every impl branch this
  repo actually produces has a multi-segment root (e.g.
  `impl/goal/develop/<slug>/<stem>` -> `goal/develop/<slug>`); reused this same
  file's existing PARENT-derivation phrasing for the identical rule instead of
  adding a second notation.
- Added a one-clause dirty-working-tree judgment prompt to the checkout arm:
  checking out through uncommitted changes is a second contamination vector
  the original text left silent. Advisory only (commit or stash), no new
  fail-closed machinery, consistent with the ticket's "leave the rest to lead
  discretion" decision.
- Reordered so "Otherwise act by stop letter" stays adjacent to the
  protocol-mismatch clause it originally negated, rather than being separated
  by the new paragraph.

Verification:

- `go test ./internal/wsrsrc/... ./internal/mcp/... -count=1` (agents-plugin-tool): PASS
- `python3 -m unittest discover agents-plugin/tests`: PASS (68 tests, includes the new pin)
- `python3 -m unittest discover agents-plugin-wsflow/tests`: PASS (11 tests)
- `go build ./... && go vet ./...` (agents-plugin-tool): clean
- Two review rounds (partitioned: correctness, fit): round 1 raised 3 Important
  + 1 Minor (correctness/fit); all fixed in `b8a6f827`. Round 2 (both
  partitions): clean, no findings.

Unresolved observations (round-2, out of scope, not fixed):

- A rootless `impl/<stem>` branch shape (no merge root) exists in the
  implementation and has no stated fallback in the new derivation sentence,
  mirroring a pre-existing gap in the PARENT sentence it was modeled on.
- The exact wording of the derivation phrase and the dirty-tree judgment
  sentence is not independently pinned by the new test assertion, so a future
  edit could drift that specific wording without failing a test.
- `branch.head`/`impl_ticket` are exact response field names; "the
  working-tree state" is prose describing the `clean`/`changed_files` fields,
  not a literal field name.
