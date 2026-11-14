---
title: Consolidate conversation mail delivery and report presentation
---

# Consolidate conversation mail delivery and report presentation

## Background

The dashboard can show a structured mailbox-history row and a separate mailbox wake card for the same delivery. They represent different observations: the mail exchange and the text delivered to the model. Their simultaneous display is not evidence of duplicate model delivery. Some child reports also appear as owner/whiteboard input because producer paths share input records.

The owner approved one ticket with two phases under (781)=a: a mockup-first, T0 asset/display-only Phase 1, followed by an optional, separately settled Phase 2 for delivery IDs and typed reports. MAIN relayed (804)=ok on 2026-11-14 at 22:42:15 UTC, approving the Phase 1 mockup and authorizing ticket finalization. This capture preserves that settled baseline; it does not authorize implementation or claim browser acceptance.

The supporting rendering inventory was a static investigation, not approval of its entire five-family consolidation proposal. The approved mockup deliberately narrows Phase 1 to mail delivery details and truthful source presentation. MAIN is separately preparing a unified chat/mailbox/whiteboard mockup extending this baseline under (811)/(812); that awaited input has not supplied an approved replacement contract here.

## Decisions

1. **Keep one ticket with two stable phases — owner-confirmed (781)=a.** Finalize only the display-only Phase 1 baseline now. Phase 2 remains optional and deferred pending its own explicit decisions and compatibility assessment. Rejected: treating all rendering families or producer/schema changes as part of Phase 1.
2. **Use the approved Phase 1 visual reference — (804)=ok.** The unchanged approved HTML is `ai-docs/tickets/assets/261115-feat-ws-dashboard-conversation-rendering-consolidation/approved-phase1-mockup.html`. The mockup's historical “not approved” annotation records its drafting state; the approval provenance in this ticket supersedes that annotation without changing the approved artifact. Synthetic names, times, case headings, explanatory side notes and the 360 px preview are review fixtures, not production copy or an approved product breakpoint.
3. **Give matched mail one primary representation.** Preserve the familiar mail arrow, peer, preview and time. When a corresponding mail row can be reliably identified from existing data, subordinate the wake's raw transport text under that row in a collapsed “Delivery details” disclosure rather than another standalone wake card. The owner-facing label may be “전달 세부”. Keep mail-body and delivery-detail expansion independent.
4. **Retain honest fallbacks.** When the matching mail row or reliable correlation evidence is missing, retain an independent compact notification with expandable delivery details. History retention, pagination or an unavailable body is not permission to discard the remaining wake detail. Rejected: blanket suppression of `wake/mail`, matching by body equality alone, timestamp proximity alone, or the presence of some nearby mail.
5. **Preserve legitimate repetition and timing distinctions.** Equal sender/body text can belong to separate messages. Keep those messages independent. Mail claim/history time is not model-consumption time; grouping must not fabricate delivery acknowledgment or change observed timestamps.
6. **Make report labels follow available provenance.** Display an already-identifiable child report distinctly from actual owner/whiteboard input. Preserve genuine human input as human input. For an inseparable legacy/mixed record whose uncertainty is supported by existing data, use a neutral source-unresolved presentation and retain the original text; do not fabricate a split. A prose marker such as `[child report]` is not proof of authorship. New producer metadata or typed-report emission belongs outside Phase 1.
7. **Phase 1 compatibility target is T0, asset/display-only.** Change presentation without changing stored records, mailbox queue/claim/acknowledgment behavior, lead-input text, producer event contracts or API semantics. No schema/protocol bump or new persisted field is authorized. A need to cross that boundary returns to MAIN before implementation.
8. **Defer the optional Phase 2 mechanisms.** Delivery IDs and typed reports are the owner-approved subjects for possible later work, not selected APIs or event/store designs. Their exact fields, correlation rules, producer changes, compatibility tier and execution scope remain unselected. Avoid T2; any T2 proposal requires explicit owner approval under the compatibility rules. This ticket does not supply an executable Phase 2 plan.

## Constraints

- Phase 1 is limited to the approved mail/transport-detail and report/source-label presentation. It does not authorize the inventory's broader human/assistant/work/system-family redesign, composer changes, tool or turn regrouping, approval-control changes, or assistant-bubble redesign.
- Preserve raw delivery diagnostics and meaningful failures as inspectable escaped text. Do not execute diagnostic text as HTML or infer identity from presentation copy.
- Preserve existing stable entry keys/cursors and separate messages. Presentation grouping must survive initial rendering, live insertion, historical insertion and reconnect without altering stored history or model input.
- Preserve disclosure state and existing conversation scroll ownership/anchoring. This ticket does not choose a second scroll owner or change the viewed-panel golden rules.
- Where current record metadata cannot support a proposed association or source distinction safely, preserve the compact/neutral fallback instead of silently widening scope to producer or storage changes. Any new product choice is returned to MAIN for its decision ID; this capture mints none.
- The approved HTML is a visual reference, not evidence that a matcher, classifier, history reconciliation or accessibility behavior already works. No browser or runtime verification was performed for ticket finalization.
- Respect the separately approved asset-split start pause and consume the landed split's move map before a new UI implementation starts. The split's overlap does not authorize source import or bypass its coordination gate.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before eventual tooling/asset edits, and `ai-docs/manuals/release-compatibility.md` for any eventual cross-version surface change. Phase 1 does not authorize shipped playbook edits.
- MAIN alone owns the (811)/(812) unified-mockup extension. This capture neither duplicates that mockup nor settles its layout, behavior, delivery model or implementation scope. If a later relayed contract conflicts with the Phase 1 baseline, report the conflict to MAIN rather than silently replacing an approved constraint.

## Approved Visual Reference

- Tracked reference: [Approved Phase 1 mockup](../assets/261115-feat-ws-dashboard-conversation-rendering-consolidation/approved-phase1-mockup.html), at `ai-docs/tickets/assets/261115-feat-ws-dashboard-conversation-rendering-consolidation/approved-phase1-mockup.html`.
- Approved source artifact: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/8f5359b5-01-chat-render-781-static-mockup.html`.
- Supporting static review note: the same cache basename with `.md`.
- Approval: MAIN relay (804)=ok, 2026-11-14 22:42:15 UTC. Preserve the source HTML verbatim; local cache paths are provenance, not a substitute for the tracked reference.
- The implementation verification must compare result captures side by side against the tracked reference for all six illustrated cases, including wide/narrow layouts and collapsed/expanded disclosures. Document intentional differences attributable to real data or existing surrounding UI rather than silently redesigning the reference.

## Awaited Supporting Input

MAIN has announced a unified chat/mailbox/whiteboard mockup under (811)/(812), extending the approved 781 baseline. Its file path and complete contract have not been relayed to this capture. Record the file when MAIN supplies it; until its contract is explicitly relayed, treat it as awaited supporting input, not an approved implementation design or replacement visual reference. Do not infer either decision's scope from its number. The approved baseline asset above remains unchanged.

## Supporting Evidence and Approval Trace

- MAIN's original owner-confirmed contract, 2026-11-14 14:08:07 UTC: “Owner answered (781)=a: chat-render consolidation = ONE ticket, two phases, mockup first”; “Phase 1 = T0 asset/display-only (e.g. demote the mailbox wake card into a ‘전달 세부’ disclosure under the mail row; fix child reports rendered as owner/whiteboard input display; no store or lead-input text change)”; “Phase 2 = optional delivery IDs + typed reports (tier must be stated; avoid T2).”
- MAIN's approval relay, 2026-11-14 22:42:15 UTC: “(804)=ok (781 mockup approved, proceed to ticket finalization for phase 1)”.
- Recovery pointers: the owning lead's local transcript `/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/maraudingunpilotedsharemollusk/pi/session.jsonl:987` carries the original (781)=a relay; `:1560` explicitly records “No ticket yet”; `:2548` carries (804)=ok. The quotes above preserve the contract without requiring that local transcript for implementation.
- The full supporting inventory is `shared/docs/chat-render-kinds.md` in the project's shared artifact directory. It is historical investigation evidence; verify current source before implementation and do not promote its broader proposals into this contract.
- The mockup handoff documented six cases and explicitly excluded the broader five-family/composer/tools/approval redesign. Approval of that narrow mockup does not select a correlation algorithm or new producer metadata.
- Decisions (805) and (806) concern `261115-feat-ws-whiteboard-committed-effects-in-conversation`, not this ticket; they are not imported here.

## Phases

### Phase 1: Consolidate mail delivery details and truthful report presentation

Implement the approved display-only baseline in the existing conversation rendering paths after the asset-split start pause is cleared and separate execution is authorized. Use the tracked approved HTML as the visual reference. The expected behavior is one canonical mail representation for safely correlated records, subordinate raw delivery diagnostics, compact unmatched fallbacks, independent repeated messages, and source labels that do not misrepresent machine reports as owner input.

Keep all stored events and lead-input text unchanged. Reuse existing data and presentation substrates; no new delivery identity, metadata producer or schema is selected by this phase. Reconcile any later explicitly relayed (811)/(812) contract with MAIN before treating it as a Phase 1 change.

Verification must cover:

- A reliably matched mail/wake pair with collapsed details and with expanded mail body/delivery diagnostics; the raw text remains inspectable without another primary wake card.
- Missing history, expired body, unmatched/ambiguous records, historical pagination and reconnect, preserving the compact fallback and preventing orphaned or duplicated diagnostics as entries arrive.
- Two separate messages with identical sender/body, distinct senders with equal bodies, and returned/reclaimed history, without content-based deletion or invented delivery acknowledgment.
- An identifiable child report distinct from real human input, and an inseparable legacy/mixed example retaining its original text and honest uncertainty without prose-based provenance inference.
- Initial render and live/history insertion, disclosure-state retention, stable entry identity/cursors and existing scroll behavior.
- Side-by-side result captures against `ai-docs/tickets/assets/261115-feat-ws-dashboard-conversation-rendering-consolidation/approved-phase1-mockup.html` for the six illustrated cases, wide/narrow layouts and collapsed/expanded states; verify actual overflow, keyboard disclosure operation, focus visibility and applicable light/dark presentation.
- Evidence that mailbox queue/claim/acknowledgment behavior, producer contracts, persisted records and model-facing delivery text remain unchanged. Record any unsupported case or acceptance gap instead of claiming the static mockup proves it.

### Phase 2: Optional delivery identity and typed reports — deferred

This phase depends on Phase 1's landed Result and a later explicit settlement through MAIN. It is not executable under this capture. Before any implementation, settle the exact optional delivery-ID and typed-report scope, the data/protocol/producer contract, old-record and mixed-version behavior, the compatibility tier and the verification plan. No proposed mechanism or new decision ID is recorded here. If this phase is not pursued, leave it deferred rather than treating its absence as Phase 1 failure.
