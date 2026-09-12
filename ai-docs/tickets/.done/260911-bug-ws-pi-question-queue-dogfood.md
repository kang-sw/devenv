---
title: Resolve ws-queue-question live dogfood findings
related:
  260911-feat-ws-pi-async-question-queue: feature under live acceptance
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: Phase 2 owns same-process fork-raised `/done` reconciliation and owner-to-lead handoff
  260906-workset-ws-pi-dogfood-ux: Pi dogfood UX collection
  260908-bug-ws-pi-delegated-tool-surface-unavailable: blocks fork-raised question-path live acceptance before the fork starts
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 9b50a59d15f3d8ab
sage-review-completeness-reviewed: 9b50a59d15f3d8ab
completed: 2026-09-13
---

# Resolve ws-queue-question live dogfood findings

## Background

Live Pi acceptance of the newly landed `ws-queue-question` flow confirmed its fork-less registration, `/thread` discovery, anchored answer injection, draft persistence, Korean IME input, multiline editing, per-question drafts, Tab navigation, partial multi-question submission, and blank-question retention.

Keep the remaining findings from this dogfood run together in this rolling actionable ticket rather than creating one ticket per small observation. Split later only if implementation proves that a finding has an independently meaningful contract or delivery boundary.

## Findings

1. **Weak modal hierarchy.** The `Qn/N` and answered-count header, answer editor separator, and shortcut footer do not establish enough visual hierarchy. The header and separator need clearer semantic foreground or background treatment; shortcut help should be dimmed without becoming illegible.
2. **Counter-spatial confirmation arrows.** In the final `Yes [No]` confirmation, Right from the initial right-hand `No` moves selection left to `Yes`, and Left from `Yes` moves selection right to `No`.
3. **Inaccessible long-question truncation.** Long question/context content is replaced by an `… question truncated — see /thread <id> for the full text` marker (`agents-plugin-pi/src/ask.ts#L2329-L2347`); Up/Down and PageUp/PageDown cannot reveal the omitted text. The marker uses the same foreground treatment as ordinary prose and does not clearly communicate the inaccessible boundary.
4. **Bare `/answer` enters at the newest item.** With `q6`, `q7`, and `q8` pending, bare `/answer` opened on `q8`. A sequential oldest-first queue should begin at the oldest answerable item unless the owner explicitly names an ID.
5. **One model turn per answer from one modal submission.** The owner submitted `q6` and `q7` together while the lead was working, but the two answers later popped as separate owner messages across separate lead turns. The modal's batch boundary was lost, adding avoidable model-turn cost.
6. **Explicit terminal target falls through to another question.** `/answer q6` on an already answered question displayed `Warning: ws: question q6 was already answered or withdrawn — showing the rest of the queue instead.` and then opened another pending question. The warning was not visually salient enough to prevent the owner from entering `adf`, which was submitted as the answer to `q10`. The explicit target was not honored, and fallback changed which question received the input.

## Live Acceptance Status (2026-09-11)

Passed in the current Pi session:

- fork-less single-question registration and anchored answer injection;
- Korean IME input, multiline editing, Esc/reopen draft restoration, and exact answer preservation;
- multi-question Tab/Shift+Tab navigation, independent drafts, commit-and-advance, partial submission, and blank-question retention;
- pending-question withdrawal and post-answer withdrawal no-op;
- active-edit withdrawal deferral without modal or draft loss (`q3` remained open after withdrawal returned `deferred`, then returned `EDITING-IN-PROGRESS` from commit `e0f92cef`, entry `f8dde1e8`);
- plugin reload persistence and anchored answer recovery (`q1` asked at commit `a7f5d521`, entry `ccb097ae`, returned verbatim as `PERSIST-OK`);
- fork-raised question registration and existing-fork overlay attachment (`q4`), owner answer continuation, and final `Decisions:` delivery preserving the owner's `alpha` choice; the same fork's first task also confirmed the `codex_generate_image` unavailable-tool notice after `260908-bug-ws-pi-delegated-tool-surface-unavailable` was loaded.

The residual `/done` cleanup failure from that acceptance was retested after the same-process reconciliation fix and plugin reload on 2026-09-12. Fork `e7529581-35d5-4b1c-b676-ede04d20334f` raised owner thread `q5`, retained the owner's `alpha` answer through exactly one closeout final, emitted no duplicate advisory or settle, and parked dormant. This closes the observed normal-runtime stranded-idle gap owned by `260908-feat-ws-pi-subagent-audit-window-and-owner-steering` Phase 2 without claiming its broader owner-steering/modal scope complete; crash/reload recovery remains explicitly best-effort.

Remaining live coverage for a later session:

- optional post-compaction persistence/anchor recovery; plugin reload persistence is covered above.

## Constraints

- Keep `No` as the confirmation's initial safe default.
- Use bounded spatial confirmation navigation: Left moves toward the visually left option, Right moves toward the visually right option, and input at an outer edge does not wrap.
- Use Pi theme semantics rather than fixed colors so hierarchy remains usable across light, dark, and custom themes.
- Replace destructive question/context truncation in this modal type with a bounded, scrollable question region. Keep the answer editor and shortcut footer visible while making the full original question reachable. Any remaining overflow cue must be visually distinct from ordinary prose.
- Answer commands are target-strict:
  - bare `/answer` starts at the oldest answerable lead-ask item;
  - `/answer qN` opens exactly `qN` when it is answerable;
  - `/answer qN` for an answered or withdrawn question reports that state and returns without opening another modal;
  - `/answer qN` for an unknown ID reports an error and returns without opening another modal.
  `Ctrl+Shift+A` behavior is not decided by this finding.
- Answers accepted by one lead-ask modal submission return in one injected lead follow-up and consume one lead turn. Preserve every included question's full question, context, verbatim answer, ask-time commit hash, and entry anchor in deterministic queue order; exclude blank questions that remain pending.
- Preserve fork-less registration, draft and withdrawal semantics, and the separate fork-raised question path. Do not generalize batching to unrelated user input or child-message families without separately deciding their ordering and wake behavior.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/ask.ts and agents-plugin-pi/test/ask.test.ts |
| scope.surface | public-interface | owner-facing /answer command and ws-queue-question flow in agents-plugin-pi/src/ask.ts#L2619-L2668 |
| scope.new_public_symbol | no | the phase changes existing commands and modal behavior; no new public symbol is specified |
| scope.new_type_contract | no | the phase specifies behavioral changes to existing queue records and delivery, not a new type or signature |
| scope.test_surface | existing | agents-plugin-pi/test/ask.test.ts covers LeadAskQueueComponent, delivery, and queue helpers |
| complexity.reuse_points | confirmed | LeadAskQueueComponent, collectLeadAskQueue, and deliverQueuedAnswer exist in agents-plugin-pi/src/ask.ts#L2027-L2451 |
| complexity.side_effect_risk | moderate | changing queue selection and follow-up delivery affects existing owner-command and lead-turn behavior |
| risk.correctness | high | target-strict selection, directional confirmation, scrolling, and atomic multi-answer delivery must not misroute owner input |
| risk.fit | moderate | the phase must preserve fork-less registration, draft/withdrawal semantics, and the separate fork-raised path |
| risk.test | high | existing unit coverage does not settle live theme hierarchy, terminal key handling, or busy-lead batch delivery |
| risk.security_or_contract | moderate | /answer target selection and the number of injected lead follow-ups are owner-visible queue contracts |

## Phases

### Phase 1: Close accepted question-queue dogfood gaps

Resolve the accepted findings while preserving the behaviors that passed live acceptance.

Verify confirmation positions and both arrow keys, including edge no-ops; long questions at constrained viewport heights with complete scroll reach and fixed editor/footer visibility; oldest-first bare `/answer`, exact explicit-ID targeting, and terminal/unknown explicit-ID refusal without fallback; semantic hierarchy in a live Pi theme; and single-turn delivery of multiple answers submitted together while the lead is busy. Retest the already-passing single answer, Korean multiline draft, partial multi-question submission, blank retention, and per-question provenance paths for regression.

### Result (64c7588) - 2026-09-13

Closed the accepted dogfood gaps in the lead-ask modal and command path. Confirmation now keeps `No` as the initial right-hand safe default with bounded spatial Left/Right movement; question/context prose uses a bounded PageUp/PageDown viewport that keeps every original character reachable while the answer editor and dimmed shortcut footer remain fixed; Pi's semantic accent, border-accent, dim, and warning theme roles establish the header, answer separator, help, and overflow hierarchy. Bare `/answer` selects the oldest answerable lead question, explicit IDs are exact and terminal/unknown IDs return without fallback, and the existing `Ctrl+Shift+A` fallback behavior remains unchanged.

One modal submission now aggregates every nonblank answer into one ordered lead follow-up while preserving each question's context, original question, answer, ask-time commit, and entry anchor; blank entries remain pending. Focused queue regression coverage passed 65/65, including Korean multiline draft isolation, partial submission, target selection, fixed viewport reach, one-row overflow reach, spatial edge no-ops, semantic painters, and the production modal-close batch callback. `git diff --check` passed. The full Pi suite passed 1,756 tests with 2 expected skips and retained 6 fork-lifecycle allowlist failures that reproduce from an archive of the unchanged base commit; they are unrelated to this phase. A standalone TypeScript compile check was unavailable because this package does not install the TypeScript compiler.

Partitioned round-one review found one Important one-row reachability defect, two Important coverage gaps, and one Minor duplicate type alias. Commit `64c7588` fixed all four; round-two correctness and test verification reported no remaining findings.



## Resolution (2026-09-13)

Implemented the accepted Pi question-queue dogfood fixes in `a31fd0a` and closed round-one review findings in `64c7588`. Focused queue coverage passes; the remaining full-suite fork-lifecycle allowlist failures reproduce from the unchanged base.
