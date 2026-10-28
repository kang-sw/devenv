---
title: Delegate by default for settled bounded work, with a write-once plan brief and post-hoc review
related:
  260909-epic-ws-worker-interpreter-refoundation: closed refoundation epic whose lead-surface work this continues
  260909-research-ws-refoundation-evidence-audit: binding anchor; retired the plan/spec/mental-model document layers as write-heavy, read-light, drift-prone; this ticket revives `ai-docs/.plans/` only as a write-once record
  260911-research-golden-fixture-verification-gap: the lead-delegate playbook prose is pinned by a golden fixture
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0f604c396c79d565
sage-review-completeness-reviewed: 0f604c396c79d565
---

# Delegate by default for settled bounded work, with a write-once plan brief and post-hoc review

## Background

Leads routinely send one-line, already-settled fixes through the full ticket
path (fact population, design review, worker run). The cause is the
`lead-delegate` routing gate (`agents-plugin/rsrc/lead-delegate/lead-delegate.md`,
`## Routing`): it routes to `lead-run`, and therefore to a ticket, whenever a
change "affects public behavior, an API, protocol, schema, template, canonical
flow, or architecture" or "independent review is needed". In this repository
almost every change touches a shipped surface, so nearly every change trips the
gate. `ai-docs/manuals/skill-authoring.md` (`## Rule Lifecycle`) calls a gate
whose default is the heavier path a defect: state the cheap default and the
condition that escalates.

The gate also ignores that a project with a review track already reviews every
commit after the fact: the `lead-ship` release gate requires the range since the
review watermark to be clear.

Moving work from tickets to delegates moves the contract from a reviewed
document into the lead's brief. A later `lead-review` then has no contract to
judge against unless the brief survives. This ticket makes the brief a
write-once file that is committed with the implementation.

## Decisions

All decisions below were confirmed by the user in discussion on 2026-10-26.

1. **Delegate is the default for settled, bounded work.** The routing gate
   states a cheap default and the conditions that escalate, instead of an
   enumerated list of touched surfaces. Agreed sketch of the gate prose (the
   final wording follows `skill-authoring.md`):

   ```text
   Default to delegate when the outcome is settled and you can name the files
   before starting. Capture a ticket when a decision is still open, the work
   spans phases or outlives this session, or the scope cannot be named up front.
   Delegated commits on the review track are reviewed afterward through lead-review.
   ```

   Rejected: keeping the touched-surface triggers ("public behavior / API /
   template"), because they route nearly every change in a shipped-text
   repository to a ticket. Rejected: a bare "if the edit scope is narrow and
   clear" judgment line, because the reader model takes the heavier path when a
   rule leaves it unsure (`skill-authoring.md` `## Reader Model`). The existing
   "delegate discovers broader scope -> stop and return" clause stays as the
   safety valve.
2. **Independent review is satisfied after the fact.** A delegated commit that
   lands on the project's declared review track is reviewed afterward by
   `lead-review` (review sweep or release gate); this counts as the independent
   review, so "independent review is needed" no longer forces `lead-run`. When
   the project declares no review track, the lead offers `lead-review` right
   after the delegate returns. Rejected: requiring review before dispatch.
3. **The brief is a write-once plan file.** For a code-writing delegate, the
   lead's brief is written to a repo-local plan at
   `ai-docs/.plans/YYYY-MM/DD-HHMM-<stem>.md`, allocated through the existing
   `path.generate(kind: "plan")` hook (`agents-plugin-tool/internal/wsstate/generated_paths.go`,
   `generatePlanPaths`). The plan is never edited after the delegate starts.
   Results and deviations go to the commit's `## AI Context`, never back into
   the plan. Rejected: a delegate-updated plan, because a living document
   re-creates the drift the refoundation retired.
4. **Allocation happens at `delegate-implementer` render time.** Rendering
   `delegate-implementer` allocates the plan path and splices it into the
   floor body, so the step cannot be skipped. The stem comes from the render
   `context`. Rejected: a lead-side prose instruction to call
   `path.generate` separately, because the lead can omit it.
5. **Only a top-level lead's render allocates a plan.** The condition is the
   session entry `scope == roleLead && parent == ""`, the same condition the
   mailbox peer-lead stamp uses (`handleMailboxSend`). A worker's key carries a
   parent, so leaf delegates rendered under `ticket-worker-elevated` (whose
   contract is the ticket) get no plan. Rejected: allocating on every render,
   which would leave one plan per leaf under a ticket.
6. **The floor tells the delegate to commit the plan with its implementation**
   in the same commit.
7. **No exception for NEEDS FIX repairs.** A local repair that `lead-review`
   hands to `lead-delegate` also gets a plan, so the next review reads what the
   repair intended the same way.
8. **`lead-review` reads the plans as the contract.** When the reviewed range
   contains plans, the reviewer judges the corresponding changes against them;
   commits without a plan are judged from the diff and its `## AI Context`.
   Plan absence never blocks a review.

9. **Render output and the lead's write.** Render reserves an empty plan file
   and returns its path as an additional output line next to the prompt path;
   the lead writes the brief into it with ordinary file tools. Rejected:
   passing the brief text to render as a `context` string, because render
   stays an allocator and a multi-paragraph brief does not belong in a
   substitution value.
10. **Missing stem.** A top-level-lead render without a stem in `context` falls
    back to the stem `delegate`. Rejected: refusing the render, which costs a
    retry turn over a naming detail; the timestamp already makes the path
    unique.
11. **How `lead-review` finds contracts.** Contracts are the `ai-docs/.plans/`
    files added in the reviewed range. Rejected: a new commit-message trailer
    naming the plan, because the plan lands in the same commit and the diff
    already identifies it.
12. **`lead-discuss` opening line.** Reframe its first sentence ("What the user
    confirms is captured through `ws:lead-ticket`") so ticket capture no longer
    reads as the default outcome of a discussion; bounded settled work routes
    to `lead-delegate` under the new gate. Rejected: leaving `lead-discuss`
    untouched, which keeps a ticket-first framing upstream of the gate.
    Rejected: having `lead-discuss` itself ask "implement now or write a
    ticket?" once a discussion converges, because it re-creates the premature
    wrap-up offer that the "Do not offer to wrap up or persist; wait for the
    user's signal" rule exists to stop; that rule stays.
13. **Canonical flow.** This repository's `AGENTS.md` `Canonical flows`
    `Delegate:` line becomes `delegate <bounded task> -> follow-up on the same
    native agent -> review`. `lead-bootstrap`'s `AGENTS.template.md` has no
    such line, so no shipped template changes.
14. **Ready tickets still route to `lead-run`.** The new gate keeps "a ready
    ticket owns the work -> `lead-run`" beside the cheap default, because
    `lead-run`'s contract (it executes ready or named tickets) is unchanged and
    a ready ticket sent to a delegate would get no `### Result` and no close.
15. **Uncommitted plans are deleted by the lead.** When a plan never gets
    committed (the lead does not spawn, the delegate stops on the
    broader-scope safety valve, or the delegate fails), the lead deletes it;
    on the safety-valve path its brief carries into the captured ticket.
    Write-once binds committed plans only. The delegate stages only its own
    plan path. Rejected: committing abandoned plans, or a tool-side cleanup
    sweep, because an uncommitted orphan would ride a later delegate's commit
    and become a false contract under Decision 11.
16. **Follow-ups reuse the plan.** Steering the same native agent gets no new
    render or plan. Lead-directed follow-ups are recorded in the resulting
    commit's `## AI Context`, and `lead-review` judges the delegate's other
    commits in the range against the plan plus that AI Context, not as
    contract deviations. Rejected: re-rendering per follow-up, which breaks
    one plan per delegation.
17. **Output line order.** The plan-path line is appended after the
    prompt-path line, alongside the `recommended-*` lines, because the pi
    bridge (`agents-plugin-pi/src/bridge.ts`) reads the first line as the
    prompt path.
18. **Allocation root and timing.** The plan is allocated under the render's
    resolved worktree root (`root_override`, else the session root) and only
    after config and variable resolution succeed, so the delegate commits it
    from its own checkout and a failed render
    (`TestRepoScopeMalformedFileFailsLoudAtPlaybookReaders`) leaves no empty
    file. Rejected: allocating in the lead's root or before validation.
19. **Stem key.** The stem arrives as context key `plan_stem`, consumed by the
    render handler rather than declared as a template variable, so a parented
    worker render never fails variable validation over it. Rejected: a
    frontmatter-declared variable, which `buildPlaybookVars` would demand on
    every render.

## Constraints

- Shipped surfaces: read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  and `ai-docs/manuals/ws-mcp.md` before editing the paths they cover
  (`AGENTS.md` `### Implementation Conventions`).
- `ai-docs/.plans/` is revived only as a write-once record; no tool, playbook,
  or reviewer may edit an existing plan.
- The `lead-delegate` prose is pinned: update
  `agents-plugin/tests/fixtures/lead_delegate_contract.json`,
  `agents-plugin-tool/internal/mcp/lead_delegate_test.go`,
  `agents-plugin/tests/test_skill_dispatch_contracts.py`, and the
  `TestPlaybookPrintGoldenLeadDiscuss` / `TestPlaybookPrintGoldenLeadReview`
  goldens in `agents-plugin-tool/internal/mcp/playbook_tools_test.go` with the
  change, and
  regenerate the wsflow and pi rsrc mirrors and the manifests
  (`260911-research-golden-fixture-verification-gap`; the red landing was the
  tier-resolution merge `61f6351c2`, reconciled in `0c884ce4a`, while commit
  `de0c92ef4` updated `lead_delegate_contract.json` in the same commit as its
  prose change: `git show --stat de0c92ef4`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260911-refactor-lead-run-ticket-only-delegate-implementer (2026-09-11, Decisions): "New `delegate-implementer` playbook: a tier-unaware minimum floor... delegate renders `delegate-implementer` whenever the assignment writes code." — bearing: supports
- 8db45279 (2026-09-11, commit): "two Go prose-pin tests also pinned the edited body and had to move with it: `playbook_tools_test.go` ... and `lead_delegate_test.go`. The ticket under-named the test" — bearing: constrains
- 9f2dc7dd (2026-09-11, commit): "the ad-hoc lead-run route is redundant with lead-delegate's own routing gate, which already claims mechanical/hotfix/localized-internal work" — bearing: supports
- 260912-bug-lead-review-fix-relay-ticket-only-run (2026-09-12, Decisions): "For this handoff, the required follow-up `lead-review` supplies independent review. That criterion alone does not reroute an otherwise bounded local repair from `lead-delegate` to `lead-run`." — bearing: supports
- 648de81b (2026-09-12, commit): "Only the independent-review gate gains a review-originated-repair exception because the mandatory follow-up lead-review provides that verification" — bearing: supports
- 260628-feat-lead-implement-single-plan-delegation (2026-06-28, Decisions): "Plan files live under `ai-docs/.plans/YYYY-MM/DD-hhmm-<stem>.md`." — bearing: supports
- 53d158fb (2026-04-06, commit): "plans excluded from lead read scope. Plans are the core implementation axis — too heavy for lead context. Verified by one-shot plan-reviewer agents instead." — bearing: constrains
- 261023-feat-ws-shared-question-classes-and-ids (2026-10-23, Result): "`TestPlaybookPrintGoldenLeadDiscuss` gained the question rules... manifests were regenerated (WSRSRC_REGEN, WS_REGEN_WSFLOW_RSRC). The pi tree was copied by hand" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/server.go playbook.render, agents-plugin-tool/internal/wsstate/generated_paths.go, agents-plugin/rsrc/delegate-implementer, agents-plugin/rsrc/lead-delegate, agents-plugin/rsrc/lead-review, agents-plugin/rsrc/lead-discuss, wsflow and pi rsrc mirrors and manifests, AGENTS.md |
| scope.surface | public-interface | playbook.render output for delegate-implementer gains a plan path line; shipped lead-delegate, lead-review, lead-discuss, delegate-implementer playbooks |
| scope.new_public_symbol | no | no new MCP tool or exported Go symbol named; existing path.generate kind plan is reused |
| scope.new_type_contract | yes | playbook.render result gains an additional plan-path output line for top-level-lead renders of delegate-implementer |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/lead_delegate_test.go, agents-plugin/tests/fixtures/lead_delegate_contract.json, agents-plugin/tests/test_skill_dispatch_contracts.py, plus TestPlaybookPrintGoldenLeadDiscuss and TestPlaybookPrintGoldenLeadReview in agents-plugin-tool/internal/mcp/playbook_tools_test.go which the ticket does not name |
| complexity.reuse_points | confirmed | Manager.generatePlanPaths at agents-plugin-tool/internal/wsstate/generated_paths.go#L72-L93; top-level-lead condition at agents-plugin-tool/internal/mcp/mailbox_tools.go#L88 |
| complexity.side_effect_risk | moderate | render starts creating files under the worktree's ai-docs/.plans on every top-level-lead render, a new filesystem side effect of a previously pure render |
| risk.correctness | moderate | lead-vs-worker key discrimination and stem fallback decide whether a plan is allocated; wrong condition leaves orphan plans per leaf |
| risk.fit | moderate | routing-gate semantics change across three shipped playbooks and three mirror trees must stay coherent with lead-run, lead-review relay, and skill-authoring rules |
| risk.test | moderate | several exact-prose golden pins across Go and Python suites, one of them unnamed by the ticket, plus new render allocation tests |
| risk.security_or_contract | moderate | changes the playbook.render output contract and the canonical delegate flow that downstream projects consume |

## Phases

### Phase 1: Delegate-default gate, render-time plan allocation, and plan-aware review

One phase: the floor text, the allocation, and the gate prose are one
contract, and neither half is useful alone.

- `playbook.render(name: "delegate-implementer")` from a top-level lead
  (Decision 5) allocates a plan through the existing plan-path generator
  (Decisions 3, 4), using the stem from `context` or `delegate` (Decision 10),
  splices the path into the floor body, and returns it as an extra output
  line after the prompt path (Decisions 9, 17), allocating under the resolved
  worktree root only after config and variable resolution (Decision 18).
  Worker-key renders are unchanged.
- `delegate-implementer` floor: when a plan path is present, the brief lives
  there; commit it in the same commit as the implementation, never edit it,
  and stage only its own plan path; record results, deviations, and
  lead-directed follow-ups in `## AI Context` (Decisions 3, 6, 15, 16).
- `lead-delegate`: replace the routing gate with the cheap-default prose of
  Decision 1, keeping the ready-ticket route (Decision 14), the post-hoc review
  rule of Decision 2 (including the no-review-track offer), the instruction to
  write the brief into the allocated plan before spawning, passing the stem as
  `plan_stem`, and the uncommitted-plan cleanup (Decisions 15, 19). NEEDS FIX
  repairs follow the same path (Decision 7).
- `lead-review`: read plans added in the reviewed range as the contract for
  their changes, judging the same delegate's follow-up commits against the
  plan plus their AI Context; absence never blocks (Decisions 8, 11, 16).
- `lead-discuss` opening line (Decision 12) and the repository `AGENTS.md`
  canonical flow line (Decision 13).

Verification: Go tests covering the render allocation (top-level lead gets a
plan path and floor splice; a worker child key gets neither; missing stem uses
`delegate`; a failed render leaves no plan file), the pinned prose fixtures and dispatch-contract tests from
`## Constraints`, the wsflow/pi mirror tests, and a real `playbook.render` of
`delegate-implementer` in both product modes.

