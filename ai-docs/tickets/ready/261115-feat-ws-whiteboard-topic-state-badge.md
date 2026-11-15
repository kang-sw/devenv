---
title: Whiteboard topic header state and live ETA badges
related:
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: prerequisite; authoritative state, implementing run, total ETA and deadline
  261115-feat-ws-whiteboard-toolbar-state-filter: prerequisite; landed additive page metadata and approved reference
blocked-by: 261115-feat-ws-whiteboard-toolbar-state-filter
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3e5dc89bc494e94b
sage-review-completeness-reviewed: 3e5dc89bc494e94b
---

# Whiteboard topic header state and live ETA badges

## Background

The state model, overdue scheduler and toolbar filter have landed, but the topic header still has no operational-state or ETA badge. The completed toolbar ticket explicitly excluded the mockup's topic badges; this follow-on does not edit or reopen its frozen phase.

Authority: MAIN relayed the owner's approved missing-header-badge request on 2026-11-15 at 01:59:59 UTC and authorized authoring, design review, readiness and subsequent Pi implementation after an approved synchronization base. Asset-split 793 remains the higher-priority work stream; this ticket precedes settings-relocate authoring.

## Decisions

- Render the approved topic-header state badge for the four existing states: `awaiting-confirmation`, `waiting`, `discussing`, and `implementing`. Use the mockup's localized state labels and compact pill/dot treatment; implementing includes the authoritative run owner. Preserve ordinary legacy absence and diagnostic authority rather than inventing a state.
- For an implementing run, show the current total ETA and elapsed time together, with a client-side timer and overdue styling matching the reference. In Korean the reference is `구현 중 · <owner>` beside `ETA <eta>분 · <elapsed>분 경과`, with `(+N)` when overdue. The reference's ETA 30 / elapsed 47 example is `(+17)`.
- MAIN confirmed the minute-boundary technical call on 2026-11-15 at 02:14:56 UTC: displayed elapsed minutes are `max(0, floor((now - startedAt) / 60000))`. Apply overdue treatment and `(+N)` only when displayed elapsed minutes strictly exceed the current total ETA; `N = displayed elapsed - ETA`. At the exact deadline and during the first fractional overdue minute, neither the overdue treatment nor suffix is shown; `(+1)` first appears one whole minute past the deadline. Rejected: immediate deadline-based overdue treatment with `(+0)` or rounded-up partial minutes, because it would diverge from the reference's strict minute-resolution rule. Scheduler eligibility at equality remains unchanged and is not badge display authority.
- `(+N)` represents overdue minutes, not the number of ETA extensions. Reuse existing authoritative page attributes for state, owner, current total ETA, start, deadline and run identity. Rejected: adding or guessing an extension-count attribute to implement this badge; that is a different quantity and is not needed for the approved display. An ETA extension changes the current total ETA/deadline without resetting run start or elapsed time.
- Preserve closed-topic lifecycle and existing toolbar filtering. A closed topic is not a new operational state, and closing/resetting must not retain a stale implementing owner/timer.
- Owner decision (874)=b, relayed by MAIN on 2026-11-15 at 02:02:47 UTC: reorder the already-landed toolbar chips in this follow-on to `All`, `discussing`, `waiting`, `implementing`, `awaiting-confirmation`, `closed`. Korean labels are `전체`, `논의 중`, `대기 중`, `진행 중`, `확인 대기`, `종료`. Apply the same order to inline and narrow popover presentations while preserving selection identity, counts and keyboard semantics. Rejected: reopening the completed toolbar ticket or retaining its prior order.
- Reuse one document-owned timer rather than adding a timer per topic; MAIN acknowledged this technical choice on 2026-11-15 at 02:03:30 UTC. It consumes existing run/start/ETA/deadline attributes and never owns producer or scheduler behavior.

## Approved Visual Reference

`ai-docs/tickets/assets/261115-feat-ws-whiteboard-topic-state-badge/834b-state-filter-toolbar.html`

This is a byte-preserved copy of the approved reference already tracked with the toolbar prerequisite. The relevant contract is the topic-header state/owner/ETA treatment in `.stb`, `.eta`, `.eta.over` and `card(t)`, including the ETA 30 / elapsed 47 overdue sample. The already-landed toolbar corrections remain authoritative except for owner (874)'s lifecycle chip order above; this ticket does not restore the reference's removed duplicate counts or change the selected filter design. Demonstration topic text and static sample times are not live data.

Verification must capture the result and compare it side by side with the relevant light/dark reference headers, explicitly recording real data, corrected toolbar and responsive-width differences. Do not substitute source resemblance or passing selector assertions for that visual comparison.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` for tooling assets/code; read `ai-docs/manuals/ws-mcp.md` only if MCP paths actually need touching.
- Compatibility: T0 display/assets consuming existing additive page metadata. No new state enum, persistent-store shape, producer command, protocol or scheduler/nudge behavior is authorized. A non-additive requirement stops for MAIN disposition.
- Owner text is data, not markup. Preserve escaped rendering and absent/invalid/diagnostic metadata without invented timestamps, owners or implementing runs.
- Keep the single document timer bounded across board refresh, closed-history loading, frame adoption and topic replacement; avoid retaining removed nodes or accumulating per-topic timers. Timer updates must not reset the run, alter history, send nudges or require provider/network activity.
- Preserve header actions, title/recap readability, card/frame identity, pin order, scrolling and keyboard interaction. State/overdue meaning must remain textual rather than color-only, without disruptive periodic announcements.
- Verification is offline and isolated with cached dependencies and fresh owned fixtures. Never use the shared daemon or port 29417, provider calls, downloads or owner installation. MAIN alone owns develop integration and shared-daemon build/restart/swap/smoke. No outer kill/timeout around Go tests; use their own timeout and retain actual exits and cleanup evidence.
- Do not import the unmerged asset-split branch, change its immutable pins/cache artifacts or repair the unrelated rail draft-restore fixture. Disclose known unrelated failures rather than weakening tests or claiming a clean full suite.

## Prior Decisions

- 261115-feat-ws-whiteboard-toolbar-state-filter (2026-11-15, Result (01a121f70)): "Authoritative optional `TopicView.State`/`StateDiagnostic` and escaped topic data attributes for state, run owner, cumulative ETA, start/deadline and run ID." — bearing: supports
- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-15, Result (0203cff95)): "Scoped cumulative `wb:eta` preserves original start, run identity, owner and initial estimate, validates strictly increasing totals and records every extension reason/count." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | INFERRED: header markup/styles, client timer and regressions span agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl, assets/page.css, client scripts and existing tests; exact implementation files remain worker-owned. |
| scope.surface | cross-module | VERIFIED at HEAD bd316e89a via git show: whiteboard topic header is shared markup (agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L42-L46); dashboard refresh/history invokes board bindings (agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L785-L791, #L1513-L1519). |
| scope.new_public_symbol | no | INFERRED from Decisions/Constraints: consume existing page attributes only; no new producer command, protocol or exported model symbol is required. VERIFIED existing four enums and TopicRun: agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L44. |
| scope.new_type_contract | no | VERIFIED existing state/owner/total-ETA/start/deadline/run and diagnostic attributes: agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L46; INFERRED display-only work needs no new metadata contract. MAIN's delegated no-new-attribute choice is authority, not a claim established by source inspection. |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wswhiteboard/topic_state_view_test.go#L23-L134 covers four-state metadata, owner escaping, cumulative ETA, absence/diagnostics and closed windows; topic_header_test.go and agents-plugin-tool/internal/wsdashboard/board_state_filter_test.go exist in HEAD. New badge/timer coverage is still required. |
| complexity.reuse_points | confirmed | VERIFIED: read TopicView projection (agents-plugin-tool/internal/wswhiteboard/view.go#L199-L204, #L600-L607), existing attributes (assets/page.tmpl#L46), single window timer precedent (assets/ago.js#L14-L24), cumulative extension (topic_eta.go#L48-L64), and dashboard refresh hooks cited above. |
| complexity.side_effect_risk | moderate | INFERRED: periodic DOM updates and refresh/replacement reconciliation can retain nodes or disturb shared header geometry; the current ago timer is singleton, not a badge implementation (agents-plugin-tool/internal/wswhiteboard/assets/ago.js#L14-L24). No producer/store mutation is authorized. |
| risk.correctness | high | INFERRED: run replacement, fractional-minute boundaries and ETA extensions require controlled-time reconciliation tests. MAIN confirmed floored/clamped elapsed and strict displayed elapsed > ETA; VERIFIED reference card(t) uses elapsed > ETA while scheduler eligibility independently includes equality (agents-plugin-tool/internal/wswhiteboard/board_notice.go#L151-L161). Extensions preserve run/start (topic_eta.go#L48-L64). |
| risk.fit | moderate | INFERRED: use existing header, board bindings and authoritative attributes without inventing an extension-count carrier; reference card(t) computes elapsed minus ETA, not extensions (ai-docs/tickets/assets/261115-feat-ws-whiteboard-topic-state-badge/834b-state-filter-toolbar.html#L205-L211). |
| risk.test | high | INFERRED: existing metadata tests do not prove ticking, detached-node cleanup, responsive badge geometry or side-by-side visual fidelity. VERIFIED reference CSS/sample/card renderer: ai-docs/tickets/assets/261115-feat-ws-whiteboard-topic-state-badge/834b-state-filter-toolbar.html#L111-L116, #L157-L172, #L205-L211. Byte equality with the prerequisite reference verified by cmp exit 0; SHA-256 ca9cf8fb239343f6a7e7993962671aa89c3ce30aeb612bf5a672e22e0f36019c. |
| risk.security_or_contract | moderate | INFERRED: owner text must stay escaped during client updates and absent/diagnostic authority must not become fabricated runs. VERIFIED escaping/absence/reset assertions: agents-plugin-tool/internal/wswhiteboard/topic_state_view_test.go#L49-L90, #L96-L134. T0 consuming existing attributes leaves producer/store/protocol unchanged by explicit constraint. |

## Phases

### Phase 1: Render authoritative topic state and live ETA badges

Implement the approved state and implementing-owner/ETA badges in the topic header using the existing page metadata. Initialize elapsed/overdue presentation immediately, update it with one document-owned timer, and reconcile it with refreshed state/run/ETA metadata without resetting elapsed time or retaining stale badges after transitions/close. Reorder both toolbar presentations to the lifecycle order confirmed by (874), retaining existing filter identity and behavior.

Verification:

- Cover all four state labels, implementing owner escaping, legacy absence, diagnostics, malformed/missing timestamp metadata, state transitions and close/reset. Include the run identity and start remaining unchanged across total-ETA extension, with the displayed ETA/deadline updating.
- Use controlled time to verify initialization, elapsed progression and the confirmed floor/clamp rule: before start displays zero for otherwise valid metadata; at start displays zero; at deadline minus 1ms, exactly deadline and deadline plus 59,999ms there is no overdue treatment or suffix; at deadline plus 60,000ms the display gains `(+1)` and overdue treatment. Check subsequent minute increments and ETA extensions that clear overdue presentation without resetting elapsed/start. Verify removal/reinitialization after replacement or run change. Verify that repeated mount/refresh does not multiply timers or retain detached topic nodes; no scheduler/nudge mutation may result.
- Verify ordinary/live and bounded saved-history boards, streamed updates, closed-history loading, pinned topics and retained frame identity. Keep existing topic/header actions and state-filter eligibility/count values unchanged; update order-sensitive expectations to the approved lifecycle order.
- Assert the exact chip order and Korean labels in inline and popover forms. Exercise adjacent/wrapping arrow keys, Home/End, persisted selection and responsive focus transfer at 540/539px by state identity, not old position. Grounding: `agents-plugin-tool/internal/wsdashboard/assets/cards.js` arrays, keyboard handlers and responsive focus transfer; `board_state_filter_test.go` count assertions after transitions/history loading.
- Capture Korean light/dark normal, overdue and each non-implementing state at approximately 800px and narrow/mobile widths. Assert no clipped essential state/owner/ETA text or obstructed controls, and compare result captures side by side with the preserved reference headers, recording approved differences.
- Run focused Go/JavaScript and relevant affected view/header/filter/refresh regressions without weakening their assertions. Retain exact commands, complete stdout/stderr, actual exit codes, omissions and owned-process/fixture cleanup outcome. Obtain independent implementation review before recording the phase result or handing the branch to MAIN.
