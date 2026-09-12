---
title: Reconcile the Pi ready-ticket dogfood and closure backlog
related:
  260906-workset-ws-pi-dogfood-ux: UX collection board whose included tickets are reconciled here without creating parentage
  260908-research-ws-pi-lifecycle-race-monitoring: recurrence ledger that remains reference material rather than ready-queue work
  260911-bug-ws-pi-question-queue-dogfood: rolling actionable dogfood report whose accepted defects remain independent implementation work
---

# Reconcile the Pi ready-ticket dogfood and closure backlog

## Background

The Pi track accumulated implementation-ready tickets whose source work had landed but whose required owner-live acceptance was never recorded, alongside tickets that still contain unimplemented phases. This research ticket is the live coordination document for reducing that backlog without treating incidental primitive use as proof that a broader ticket contract is complete.

The baseline audit on 2026-09-12 found fourteen tickets in `ready/`: eight with implementation landed and dogfood or live diagnosis remaining, six with implementation work remaining, and none safe to close immediately.

## Evidence policy

- Close a ticket only when every phase contract is implemented and resulted and every ticket-required live gate has direct evidence.
- Record a successful session interaction as implicit acceptance only for the exact exercised slice. Mere tool availability, lack of owner complaint, or successful adjacent behavior does not satisfy visual, restart, race, retention, or broader-phase checks.
- Reuse one live run across tickets when it exercises their independent acceptance contracts, but record a separate disposition for each ticket.
- Keep report and collection tickets distinct from implementation tickets. Closing an implementation ticket does not automatically close its workset or monitoring record.
- Preserve explicit non-goals: the same-process fork `/done` fix does not promise exact crash or plugin-reload reconciliation and does not complete the broader owner-steering phase.
- Append dated evidence and dispositions here as work proceeds; update the owning ticket before moving it to `.done/`.
- For multi-step owner-live UI checks, queue the checklist in the owner answer modal while the relevant state is still visible and collect the observations before changing that state. Do not ask the owner to reconstruct several visual states after the run.
- Treat an agent's narration of an owner action as unverified unless the owner explicitly confirms it or the adapter emits direct lifecycle evidence. In particular, a fork report saying the owner closed a thread is not evidence that `/done` was entered.

## Baseline ready inventory

### Implementation landed; dogfood or live diagnosis remains

- `260906-bug-ws-pi-rsrc-mirror-drift`
- `260906-feat-ws-pi-tool-and-push-tui-polish`
- `260907-bug-ws-pi-deep-explore-missing-collection-tool`
- `260907-feat-ws-pi-persistent-explore-deep-research`
- `260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner`
- `260909-feat-ws-pi-agent-count-panel-header`
- `260909-feat-ws-pi-agent-row-model-and-usage`
- `260909-feat-ws-pi-report-header-distinction`

### Implementation remains

- `260906-bug-ws-sage-stamp-leaves-blocked-section-after-autonomous-fold`
- `260906-feat-ws-config-tune-agents-tier-returns-only-the-written-harness`
- `260908-feat-ws-pi-agent-session-disk-retention`
- `260908-feat-ws-pi-claude-delegate-tool`
- `260908-feat-ws-pi-subagent-audit-window-and-owner-steering`
- `260909-feat-ws-pi-goal-stop-controls`

## Session evidence accepted at baseline

The lead directly observed the following interactions in the 2026-09-11 through 2026-09-12 Pi session. They are persisted here as the session evidence ledger; unless an owning ticket or commit is cited separately, they were not independently replayed from Git history:

- local-devenv marker repointing, source build, plugin reload, and live ws-mcp runtime identity;
- workflow state access and post-compaction `lead-revive` recovery;
- persistent agent spawn, aliasing, reports, follow-up, explicit stop, automatic dormant parking, and registry listing;
- fork readiness with a metadata-identical unavailable extension-tool stub;
- fork-less queued-question reload persistence, exact answer recovery, and active-edit withdrawal deferral;
- fork-raised owner question continuation and final decision delivery;
- same-process fork `/done` closeout with one final, exact `Decisions: alpha`, no duplicate terminal or settle, and dormant parking in live thread `q5`;
- simple Explore execution and a settled Explore follow-up;
- project-scoped Pi tier mutation and immediate restoration;
- child push delivery into the lead session;
- execute-worker approval request, approval, command execution, and final report during the guarded no-ff merge;
- ticket, spec-index, Git inspection, workflow commit, and guarded merge primitives.

This evidence is partial, not whole-ticket acceptance, for persistent/deep Explore, owner audit and steering, session retention, and all visual TUI tickets. The same session instead reproduced the full-table `config.tune agents.tier` response, separate-turn held pushes, missing one-off concrete model selection, and concurrent todo update loss.

## Cleanup sequence

### 1. Reconcile resolved and stale records

- Close `260908-research-ws-pi-ws-ask-removal` as resolved research after confirming its redesign outcome is fully represented by done ticket `260911-feat-ws-pi-async-question-queue`.
- Recheck `260910-bug-ws-execute-worker-missing-exec-tool` against the current runtime. The approve path passed live on 2026-09-12; verify deny, run-instead, stale approval ID, diagnostics, and completion before closing or narrowing the residual.
- Reuse that run for the remaining approval-control check in `260906-feat-ws-pi-tool-and-push-tui-polish` where applicable.
- Keep `260908-research-ws-pi-lifecycle-race-monitoring` as recurrence reference rather than implementation work.

### 2. Run one consolidated visual TUI acceptance session

Exercise the remaining contracts for tool/push approval controls, owner-wait attention, agent-count placement, worker/fork model and usage rows, narrow-width waiting cues, and report-header distinction under light and dark themes. Disable attention animation and reload once. Close each independent ticket only when its own evidence passes.

Candidates:

- `260906-feat-ws-pi-tool-and-push-tui-polish`
- `260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner`
- `260909-feat-ws-pi-agent-count-panel-header`
- `260909-feat-ws-pi-agent-row-model-and-usage`
- `260909-feat-ws-pi-report-header-distinction`

### 3. Run one consolidated persistent and deep Explore session

Verify simple settle and follow-up, deep read-only small collection and synthesis, stop/resume, full process restart, and retained role/model/effort. Capture the actual deep-child launch environment and diagnose any remaining trigger before closing either ticket.

Candidates:

- `260907-feat-ws-pi-persistent-explore-deep-research`
- `260907-bug-ws-pi-deep-explore-missing-collection-tool`

### 4. Establish runtime identity before mirror diagnosis

Complete `260911-chore-ws-pi-track-sync-to-refound`, then run the normal release path without the local marker as owned by `260903-research-ws-pi-adapter-npm-distribution`. Reproduce the `lead-review` mirror symptom only under a recorded package and runtime identity, then close or reroute `260906-bug-ws-pi-rsrc-mirror-drift` from exact evidence.

### 5. Continue multi-phase Pi tickets without premature closure

- Verify the owner-audit Phase 1 live gate, then implement the broader steering, ownership, and modal Phase 2 of `260908-feat-ws-pi-subagent-audit-window-and-owner-steering`.
- Verify actual child session homes, then implement cleanup and TTL phases in `260908-feat-ws-pi-agent-session-disk-retention`.
- Verify Claude audit and consult on a real ticket, then continue rewrite and resume/design-review phases in `260908-feat-ws-pi-claude-delegate-tool`.

### 6. Route shared ws-mcp implementations through `develop`

Implement `260906-bug-ws-sage-stamp-leaves-blocked-section-after-autonomous-fold` and `260906-feat-ws-config-tune-agents-tier-returns-only-the-written-harness` on `develop`, then bring released behavior to the Pi track through the established sync direction. Do not author those shared changes on the Pi track.

### 7. Resolve remaining dogfood reports and blocked work

Advance `260911-bug-ws-pi-question-queue-dogfood` from `idea/` through the required ticket gate before implementing its six accepted findings; keep generic held-push batching in `260911-feat-ws-pi-held-push-batch-delivery`. Keep `260909-feat-ws-pi-goal-stop-controls` blocked until Pi exposes the required admission or cancellation seam. Close `260906-workset-ws-pi-dogfood-ux` only after the owner ends collection and every included request has an explicit disposition.

## Live disposition ledger

| Date | Ticket | Evidence or action | Disposition |
| --- | --- | --- | --- |
| 2026-09-12 | `260908-research-ws-pi-ws-ask-removal` | The redesign decision was implemented by done ticket `260911-feat-ws-pi-async-question-queue`; q1/q3/q4/q5 supplied live queue and attachment evidence. | Closed as resolved research; remaining actionable findings stay in the question-queue dogfood ticket. |
| 2026-09-12 | `260908-feat-ws-pi-subagent-audit-window-and-owner-steering` | Live `q5` `/done` produced one final with `Decisions: alpha` and parked the fork dormant. | Same-process `/done` slice accepted; broader Phase 2 remains open. |
| 2026-09-12 | `260910-bug-ws-execute-worker-missing-exec-tool` | `ccb370af` restored exact-extension RPC-child loading. Live approve, denial, run-instead, stale-ID rejection, completion, diagnostics, and owner-visible approval-card checks passed. | Closed after the full functional and visual matrix passed. |
| 2026-09-12 | `260906-feat-ws-pi-tool-and-push-tui-polish` | The execute-worker approval card completed the only check left after the 2026-09-09 direct-tool, push, theme, control, and responsiveness acceptance. | Phase 1 owner-live gate completed; close the ticket while retaining spacing and bordered-error observations in the UX workset. |
| 2026-09-12 | `260907-feat-ws-pi-persistent-explore-deep-research` | Multiple simple Explore children settled; a settled child accepted a follow-up. A deep-research discovery also returned. | Partial evidence only; exact deep-child profile and stop/restart matrix remain. |
| 2026-09-12 | `260906-feat-ws-config-tune-agents-tier-returns-only-the-written-harness` | Temporary Pi small-tier writes returned the complete multi-harness alias table. | Existing defect reproduced; implementation remains. |

## Current next action

Repeat sequence step 2 using in-modal, state-local checklists. The first attempt explicitly confirmed only gutter behavior at 120/80/40 columns; its retrospective checklist and the fork's unsupported `/done` narration are not acceptance evidence. Recreate pending question and approval states, collect their checks in the answer modal before resolving them, then separately collect report/theme and attention-toggle checks.
