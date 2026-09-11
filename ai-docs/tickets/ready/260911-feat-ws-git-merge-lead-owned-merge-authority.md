---
title: "Lead-owned merge authority: a constrained ws/git.merge tool, with merge taken off the worker"
related:
  260911-research-impl-lifecycle-merge-authority-goal-loop-rehoming: context; the closed design this ticket derives from (research, stays in todo/, not a code prerequisite)
  260910-feat-lead-run-worktree-parallel-route: adjacent; the parallel route serializes all merges through the lead, which this tool makes possible
  260911-refactor-lead-run-ticket-only-delegate-implementer: adjacent; that ticket removes worker/lead surface, this one moves merge off the worker onto a lead tool
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: bbb23ea0e5e9ada7
sage-review-design-reviewed: bbb23ea0e5e9ada7
---

# Lead-owned merge authority via ws/git.merge

## Background

A dogfood session observed an autonomous fast-forward merge of an `impl/*`
branch into its base, flattening the plan/review/doc history the impl branch
carried. Ground-truth diagnosis (recorded in the research ticket) established
that the merge is **not prescribed by source**: `route.resolve_implement`
returns advisory todos only and never merges ("This tool performed no merge",
`agents-plugin-tool/internal/mcp/implement_resolver.go`). The regression is a
**playbook-prose vs installed-todo divergence** — `ticket-worker.md` tells the
worker to "Merge per the route verdict: into the goal branch on your own" with
no `--no-ff`, while the installed todos default to no-merge/opt-in. Rather than
patch the prose, this ticket removes merge from the worker entirely and routes
it through a single constrained lead tool, mirroring how `ws/git.commit`
succeeds by enforcing structure the caller cannot bypass.

## Decisions

- **Merge becomes a lead-only tool call, not worker prose.** The failure mode
  is a free-form `git merge` a playbook line invites. A tool that structurally
  cannot fast-forward removes the failure mode at the source. Rejected:
  patching `ticket-worker.md` to add `--no-ff` — leaves merge as worker-executed
  free-form git, one prose drift away from the same bug, and blocks the parallel
  route where only the lead may merge.
- **The tool always merges `--no-ff`.** Fast-forward flattening is made
  structurally impossible, not merely discouraged. A merge commit always records
  the impl branch's boundary.
- **The tool validates the target against the impl branch's encoded root.**
  `parseImplBranchRoot` already recovers the merge root from an
  `impl/<root>/<stem>` name; the tool merges into that root and refuses a
  mismatched target, so a mis-typed or stale target cannot land the branch in the
  wrong place.
- **Forbidden targets are refused outright.** Never `main` (per Branch Policy);
  the tool enforces this rather than trusting the caller.
- **Auto-delete the merged impl branch on success.** Restores the auto-delete
  that was removed earlier (`CHANGELOG.md`), closing the "impl branches
  accumulate unmerged" leak that motivated branch-aware selection.
- **`merge_confirm` re-homes to the lead.** With the worker out of merging,
  `merge_confirm` stops meaning "does the worker ask before merging" and becomes
  a lead-side signal: `skip` = the lead auto-calls `ws/git.merge`; `ask` = the
  lead surfaces the merge to the user first.
- **Merge conflicts derive out to lead-delegate.** Structurally absent in the
  serial case (the impl branch is ahead of a base it was cut from); possible only
  under the future parallel route. A conflict is a bounded fix task, which is
  lead-delegate's lane.

## Constraints

- `agents-plugin-tool/internal/mcp/` — read `ai-docs/manuals/ws-mcp.md` before
  editing; the new tool follows the `ws/git.commit` structure (explicit inputs,
  workflow-aware record, no hidden git state).
- `agents-plugin/rsrc/`, `agents-plugin/skills/` — read
  `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/shipped-surface-boundary.md`; the worker/lead prose that
  changes ships downstream and must not depend on repo-only facts. Route the merge
  through the generic tool, not a repo-specific hook.
- `agents-plugin-wsflow/` — read `ai-docs/manuals/wsflow-mirroring.md`; mirror the
  playbook prose changes into the wsflow derivative.
- New protocol tool = Ask-first surface; the tool's name, inputs, and refusal
  semantics are an API decision, not an internal refactor.
- **Shared `lead-run.md` surface (four-writer coordination).** Four `ready/`
  tickets edit `lead-run.md` and regenerate the same exact-prose goldens in
  `agents-plugin/tests/test_skill_dispatch_contracts.py` (and the wsflow mirror):
  this ticket (Handle-the-report), `260911-refactor-lead-run-ticket-only-delegate-implementer`
  (intro + Select ad-hoc removal), `260911-feat-impl-derivation-hardening-branch-aware-select`
  (Select rewrite), and `260911-bug-lead-run-goal-branch-staging-not-created` (goal
  trigger). This ticket's edit is section-disjoint (Handle-the-report) from the
  other three, so whichever order it lands, regenerate the shared goldens against
  the then-current `lead-run.md` body rather than assuming its byte layout.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go, agents-plugin-tool/internal/mcp/implement_resolver.go, agents-plugin/rsrc/ticket-worker.md, agents-plugin/rsrc/ticket-worker-escalated.md, agents-plugin/rsrc/ticket-worker-elevated.md, agents-plugin/rsrc/worker-stop-protocol.md, agents-plugin/rsrc/lead-run/lead-run.md, and their agents-plugin-wsflow mirrors |
| scope.surface | public-interface | new ws/git.merge MCP tool; no existing git.merge case in the server.go dispatch table (checked server.go:899-979, 3215-3264, 3616) |
| scope.new_public_symbol | yes | ws/git.merge tool name and schema |
| scope.new_type_contract | yes | new tool input/output schema modeled on git.commit's (server.go:945-979, 3264) |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/implement_resolver_test.go and server_test.go already cover branch-plan and git.commit logic; agents-plugin/tests/test_skill_dispatch_contracts.py and agents-plugin-wsflow/tests/ cover worker-prose dispatch |
| complexity.reuse_points | confirmed | parseImplBranchRoot (implement_resolver.go:831), the ws/git.commit tool structure (server.go:945-979), and the wsgit client (implement_resolver.go:598) |
| complexity.side_effect_risk | high | the tool performs git merge --no-ff and deletes a branch on success; worker-stop-protocol.md:16-20 treats any merge into a parent branch as a stop/veto point |
| risk.correctness | moderate | new merge, conflict-advisory, and target-root-validation logic with no existing precedent in this package |
| risk.fit | moderate | cross-cutting change across the mcp tool, worker/lead prose, and the wsflow mirror, but the ticket's own Decisions section already resolves the design |
| risk.test | moderate | Phase 1 names five concrete test cases; Phase 2 verification leans on existing golden/fixture suites, an area a sibling ticket (260911-research-golden-fixture-verification-gap) flags as gap-prone |
| risk.security_or_contract | high | the ticket's own Constraints call the new tool an Ask-first surface and "an API decision, not an internal refactor" |

## Phases

### Phase 1: The ws/git.merge tool

Add a constrained `ws/git.merge` MCP tool modeled on `ws/git.commit`:

- Always `--no-ff`.
- Derive the merge target from the current `impl/<root>/<stem>` branch via
  `parseImplBranchRoot`; accept an explicit target only when it equals the
  encoded root; refuse a mismatch.
- Refuse forbidden targets unconditionally (never `main`).
- Preserve the workflow merge record and the repo's commit rules.
- On a clean merge, delete the merged impl branch.
- On conflict, do not attempt resolution: return an advisory result that names
  lead-delegate as the fix path, leaving the working tree in a state the lead can
  hand off.

Ship the tool's schema and behavior; do not yet change any playbook. Verify with
Go tests covering: no-ff always, target-root validation (match accepts, mismatch
refuses), forbidden-target refusal, auto-delete on success, conflict advisory.

### Phase 2: Take merge off the worker and re-home the lifecycle

Depends on Phase 1 (the tool must exist before the worker's merge is removed).

- Reverse the worker's merge ownership: `ticket-worker*.md` and
  `worker-stop-protocol.md` no longer instruct the worker to merge, including the
  `impl → goal` self-merge under `merge_confirm=skip`. The worker's terminal is
  the report; merging is the lead's.
- Re-home `merge_confirm` per the Decisions: `skip` = lead auto-calls
  `ws/git.merge`; `ask` = lead surfaces first.
- Add the merge step to `lead-run`'s **Handle the report**: on a close-on-impl
  report, the lead calls `ws/git.merge` under the re-homed `merge_confirm` gate
  rather than just advancing the note and ending the turn. The lead identifies the
  worker's impl branch from the assignment note / report (the tool accepts an
  explicit target only when it equals the encoded root, so the caller supplies the
  branch name it already holds; it need not be checked out on it).
- Leave the goal-branch terminal merge (`lead-run.md` "Terminal: `ready/` empty on
  a goal branch") as its existing raw `git merge --no-ff` into PARENT. `ws/git.merge`
  derives its target from an `impl/<root>/<stem>` name via `parseImplBranchRoot` and
  structurally cannot serve a `goal/*`→PARENT merge, so the two merge mechanisms
  coexist by design; state that boundary so the surviving raw merge is not read as an
  oversight. (Unifying the goal-terminal merge is out of scope, tracked with the
  parallel/fan-out route in 260910.)
- Make `implementCloseMergeReviewNudge` a real warning at `tickets.close` when an
  impl branch is left unmerged (keep it a nudge, but stop it failing silently).
- Mirror the shipped-surface prose changes into `agents-plugin-wsflow/`.

Verify: the dispatch-contract and any worker-prose golden/fixture suites in
`agents-plugin/tests/` and the wsflow package tests pass against the edited
prose; run the full suite touching every edited shipped file (guard against the
golden-fixture verification gap). This edit regenerates the shared `lead-run.md`
exact-prose goldens; see the shared-surface coordination note in `## Constraints`.
