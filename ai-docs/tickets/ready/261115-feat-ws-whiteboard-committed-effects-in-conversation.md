---
title: Show committed whiteboard effects in the session conversation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b9e69134e4ba8119
sage-review-completeness-reviewed: b9e69134e4ba8119
---

# Committed whiteboard effects in the session conversation

## Background

The owner wants whiteboard updates to be readable as expandable changes in the dashboard conversation, including for terminal leads whose native tool transcript is unavailable. Existing conversation tool rows retain bounded argument previews rather than complete update arguments or results. Parsing those previews cannot recover actual committed changes.

MAIN assigned owner request (777) to EXEC3 for accepted-backlog capture. The owner's selected approach is to append the actual effects of a committed whiteboard update to the calling session's dashboard transcript. This ticket records that behavior, not a general native-transcript viewer or a replacement for the whiteboard.

## Decisions

1. **Append actual effects to the calling session.** Collect typed effects of the update and append an independent whiteboard-update entry to the dashboard-owned conversation ring associated with the exact calling ws session key. Describe effects that happened, not merely parsed requested commands or current board state.
2. **Commit before presenting effects as committed.** Append only after authoritative durable board commitment. Returning successfully from the pure `Apply` callback is insufficient: the enclosing store may still refuse commitment, while derived HTML publication can fail after commitment. Do not label a refused batch as applied or a committed update as unchanged merely because publication failed.
3. **Cover hosted and terminal top-level leads.** Use the existing session-key-to-dashboard-session identity mapping. This does not require a native transcript reader, host result forwarding, or a running live adapter. Preserve top-level board authorization and existing dashboard-enabled/registration policy; ordinary agents and children gain no board authority.
4. **Show persisted effects without a live adapter.** The dashboard must expose the recorded update history for an otherwise eligible adapter-less or stale-adapter lead. Do not fabricate live status or enable controls merely to render persisted effects. Make each update expandable so the user can inspect its changes without expanding raw command text.
5. **Do not require exact turn alignment.** An independent session-ring entry is the confirmed behavior. Exact placement inside a native historical turn or immediately after a matching tool call is not required; do not block direct append on new turn/call identifiers or infer an exact join from timestamps or the JSON-RPC request ID.
6. **Keep native conversations and model input unchanged.** This is dashboard history, not mutation of Claude/Codex/Pi native conversation files and not injection of an additional model-visible user message.
7. **Compatibility target: T0.** Preserve existing tool arguments/results, ring readers and old conversation records. Establish the actual cross-version effects before readiness and implementation; a new required protocol/store shape, tracker event or T2 boundary is not authorized. Return any incompatible requirement to MAIN.
8. **Owner (801)=a: best-effort append, no store change.** MAIN relayed the confirmed answer on 2026-11-14 at 15:21:30 UTC: append to the existing ring after durable board commitment using best-effort projection. Accept the commit-to-ring crash gap and append failure; the board remains the source of truth. Do not add an outbox, new store, store-format migration or guaranteed replay/delivery contract. Preserve the existing on-disk shape. Rejected: guaranteed replay through an outbox (801=b). Do not retry the already-committed board command merely to obtain a missing conversation entry.
9. **Overnight execution priority.** MAIN relayed the owner's priority change: finish this ticket, promote it through the normal gate, hand the docs hash to MAIN for serialized integration, then implement through a worker. Static implementation may start after that handoff; tests/build/browser work still requires a separately granted heavy slot. This is not deployment authority.
10. **(805)=a, MAIN-assumed pending owner override.** MAIN explicitly authorized this assumption on 2026-11-14 at 15:35:43 UTC if the owner had not answered at finalization: omit the effects entry when a committed call has no semantic effects, including render-only calls. A sequence/generation increment alone is not a semantic effect. Preserve existing tool summaries. This is not an owner-confirmed answer. Alternative (805=b): emit an explicit “no semantic changes” entry.
11. **(806), technical, auto-confirmed from the grounding gap: existing-field carrier.** Use `Kind="whiteboard-effects"`, a readable `Caption` bounded to 200 runes, and a JSON envelope `{"v":1,"effects":[...]}` inside the existing `Text` string. Use existing `Truncated` when the projection omits detail; leave unrelated fields unset. Bound the complete valid UTF-8 envelope to the existing 64-KiB text budget before persistence; never clip serialized JSON. Decode only this exact kind and supported version, validating typed effect variants and bounded contents. Render escaped strings in expandable details, not executable HTML or markdown. Malformed, unsupported-version or unknown-variant records remain visible through a bounded escaped “details unavailable/unsupported” fallback; do not invent effects. Rejected: overloading user/reply/mail/command kinds, which would misclassify provenance or change existing meanings. A pinned old reader accepts the unchanged outer record shape and old assets omit this new kind; it need not display new effects, but must preserve existing history. This additive-kind behavior supports T0 statically; executed cross-version acceptance is still required.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Persist only actual, attributable semantic effects. Existing shortened previews, English system-event strings, equal entry timestamps and board generation counters are not an exact per-call effects contract.
- Preserve exact session isolation, existing board permissions and bounded/untrusted-content rendering. Do not execute or directly inject raw panel HTML in the conversation.
- Preserve ordinary live tool summaries, board rendering and legacy conversation behavior; this work does not create missing native transcript messages.
- Report board commitment, HTML publication and conversation projection separately. Under (801)=a, a missing conversation projection does not roll back the board, imply validation failure, or promise eventual delivery. Keep the accepted post-commit append/error/crash-gap boundary explicit; the board remains authoritative.
- Do not add a journal/outbox, new persisted fields or a store-format change to close that accepted gap. Fit typed display into the existing ring representation; return a concrete incompatible requirement to MAIN rather than silently expanding the selected contract. The report's update-ID, historical reconstruction and exact turn/call-correlation proposals are not implementation authority.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing shipped tooling or plugin content, `ai-docs/manuals/release-compatibility.md` before tooling-internal changes, and `ai-docs/manuals/ws-mcp.md` before MCP changes. Read the authoring and mirroring manuals if shipped guidance or resource mirrors change.
- Corrected owner (778) reserves daemon rebuild/restart/swap, swap smoke and profiling to MAIN. EXEC mails each eventual develop integration hash immediately; no runtime replacement, main push or version bump follows from this capture.
- The whiteboard asset split (793) is not a prerequisite for this ticket. MAIN confirmed that it had not started and did not constrain this static worker on 2026-11-14 at 15:35:43 UTC. If it starts before this work merges, follow its move-map when rebasing; do not bypass MAIN's split-start coordination.

## Supporting Evidence

MAIN supplied `shared/docs/wb-update-in-chat-feasibility.md` in the project's shared artifact directory. Its **Owner follow-up: append-at-apply** corrects the earlier turn-alignment framing and is the relevant feasibility evidence. This local supporting artifact is not a downstream dependency or approval of every proposal in its earlier alternatives section.

Static observations re-grounded at source baseline `2e1d9ea88730105cdb80a182bbd9a004d25fe064` using read-only `git show`; these are VERIFIED source facts, not executed acceptance:

- `wswhiteboard.Apply` parses and computes state inside the mutation callback, but durable commitment is owned by `FolderStore.UpdateAndPublish`. HTML publication follows commitment (`agents-plugin-tool/internal/wswhiteboard/store.go#L580-L675`, `agents-plugin-tool/internal/wswhiteboard/folders.go#L135-L164`, `agents-plugin-tool/internal/mcp/session_state.go#L635-L658`, `agents-plugin-tool/internal/mcp/whiteboard_tools.go#L255-L279`).
- `liveSessionDir` / `liveSessionDirOf`, `ReplyID`, `SessionDir` and `RingDir` already map a top-level session key to its dashboard ring. `recordLiveMail` / `appendLiveRing` show server-generated independent ring entries without native transcript access (`agents-plugin-tool/internal/mcp/live_snapshot.go#L319-L342`, `agents-plugin-tool/internal/mcp/live_snapshot.go#L684-L734`, `agents-plugin-tool/internal/mcp/lead_mail.go#L178-L184`).
- Current tool summaries retain previews, not complete generic tool results. Ring entries have no stable native turn/call identity; current aggregate tool-summary rows can arrive after the update commits (`agents-plugin-tool/internal/wslive/toolsummary.go#L8-L63`, `agents-plugin-tool/internal/wslive/ring.go#L63-L108`, `agents-plugin-tool/internal/wsagent/stream.go#L525-L535`).
- The current session-view path returns before constructing conversation history when no fresh adapter is present; board-only region selection also omits conversation regions. Writing a ring event alone therefore does not satisfy adapter-less visibility, although the separate history route already reads without an adapter (`agents-plugin-tool/internal/wsdashboard/pageview.go#L510-L572`, `agents-plugin-tool/internal/wsdashboard/pages.go#L64-L70`, `agents-plugin-tool/internal/wsdashboard/conversation_history_test.go#L259-L265`).
- Existing `appendLiveRing` logs and drops ring-write errors. Board commitment and ring projection are not presently one transaction (`agents-plugin-tool/internal/mcp/live_snapshot.go#L684-L698`, `agents-plugin-tool/internal/mcp/whiteboard_tools.go#L255-L301`). The report identifies that boundary; owner (801)=a subsequently selects best-effort projection and explicitly accepts the gap, without changing stores.

The source citations above are pinned to `2e1d9ea88730105cdb80a182bbd9a004d25fe064`, not the sparse checkout or a served binary. Re-ground them if the implementation candidate changes.

## Prior Decisions

- 1c35f85a (2026-11-15, commit): "MAIN made777 the overnight priority and confirmed801a." — bearing: supports
- 0586a428 (2026-11-14, commit): "in shell mode the rail lives in the shell document" — bearing: constrains
- f331efeb (2026-11-13, commit): "Hosted sessions use the actual harness PID and stopped flag, with PID liveness watched by the existing stream refresh; terminal sessions use the registration heartbeat as requested." — bearing: constrains
- 972766ab (2026-11-13, commit): "Commit exact bytes and append offsets under one folder lock; replay interrupted writes idempotently before any consumer reads authority." — bearing: constrains
- 9ee5b99e (2026-11-08, commit): "Migration is lazy on the first write, never on read" — bearing: constrains
- 14605071 (2026-11-06, Result): "Failure isolation. Write errors are logged only, as debug events dashboard.snapshot_write_error and dashboard.ring_write_error." — bearing: supports
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, Decisions): "Dashboard key off writes nothing." — bearing: constrains
- 3e99f523 (2026-11-04, commit): "a failed render now deletes only the file it wrote." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/store.go, internal/wswhiteboard/folders.go, internal/mcp/whiteboard_tools.go, internal/mcp/session_state.go, internal/mcp/live_snapshot.go, internal/wslive/ring.go, internal/wsdashboard/pageview.go, internal/wsdashboard/pages.go; all internal/ paths are package-relative. |
| scope.surface | cross-module | Typed semantic effects cross wswhiteboard, MCP commitment, wslive ring and dashboard rendering; existing MCP arguments/results must remain unchanged. |
| scope.new_public_symbol | unknown | No new tool/route is planned; the exported Go effect or commitment API, if any, has not been selected. |
| scope.new_type_contract | yes | Phase 1 requires typed actual effects; Outcome currently carries only IDs, Withdrawn and Info (agents-plugin-tool/internal/wswhiteboard/store.go#L303-L315). Decision 11 selects a versioned envelope inside existing RingEntry.Text, without changing outer persisted fields. |
| scope.test_surface | existing | Existing agents-plugin-tool/internal/mcp/whiteboard_publication_test.go, internal/wslive/ring_test.go, internal/wswhiteboard/store_test.go, internal/wswhiteboard/folders_test.go, internal/wsdashboard/conversation_tools_test.go and internal/wsdashboard/conversation_history_test.go; static inventory only. |
| complexity.reuse_points | confirmed | Read Apply, FolderStore.UpdateAndPublish, liveSessionDir, recordLiveMail, appendLiveRing, AppendRing and conversation/region rendering at the pinned source baseline. |
| complexity.side_effect_risk | moderate | Adds a best-effort cross-store projection after board commitment and changes adapter-less rendering; no model input, replay or board rollback is authorized. |
| risk.correctness | high | Capture must distinguish refused callbacks, durable intent/recovery, post-commit publication failures, implicit effects and semantic no-ops (agents-plugin-tool/internal/wswhiteboard/folders.go#L30-L33, agents-plugin-tool/internal/wswhiteboard/folders.go#L135-L164). |
| risk.fit | moderate | Existing Kind/Caption/Text/Truncated fields support decision 11 without changing outer persisted shape; old readers accept the entry and old assets omit unknown kinds (agents-plugin-tool/internal/wslive/ring.go#L63-L108, agents-plugin-tool/internal/wslive/ring.go#L319-L336, agents-plugin-tool/internal/wsdashboard/pageview.go#L900-L924, agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L293). Static feasibility only. |
| risk.test | high | Phase 1 spans failure seams, concurrent writers, hosted/terminal identity and stale/no-adapter browser display; no tests or runtime acceptance were executed in this static pass. |
| risk.security_or_contract | high | Session authority and untrusted panel HTML must remain isolated; T0 has pinned source support for decision 11, but executed cross-version acceptance remains unverified (agents-plugin-tool/internal/mcp/server.go#L755-L756, ai-docs/manuals/release-compatibility.md#L62-L79). |

## Phases

### Phase 1: Append and display committed whiteboard effects

Implement the confirmed session-scoped effects path at the authoritative whiteboard mutation boundary and the dashboard view that displays its persisted entries. Carry typed actual effects through the existing session identity path; do not reconstruct them from truncated previews, parse human-readable system events, or run updates again for display. Preserve hosted/terminal coverage and show the saved entries without requiring a live adapter. Keep expandable display bounded and safely escaped, separate from model input, and compatible with legacy records and existing authorization. Use the selected best-effort post-commit append without an outbox or store-shape change; do not turn a missing projection into board-command replay. Use decision 11's existing-field carrier and safe fallback without expanding outer persisted fields. Record actual T0 evidence, the accepted crash gap and the distinct board-commit, HTML-publication and conversation-projection outcomes.

Verification: use isolated fixtures for accepted changes, validation refusal, resolved/minted identities, multiple commands and topics, semantic no-change and render-only cases that omit effects entries under MAIN-assumed (805)=a while preserving tool summaries, publication failure after commitment, append failure without board rollback/retry, the accepted commit-to-ring crash gap without guaranteed replay, unchanged store shape, and concurrent ring writers. Pin unchanged outer stored keys/types and old-reader acceptance/old-renderer omission of the additive kind. Cover valid UTF-8 and size boundaries, malformed/unsupported-version/unknown-variant visible fallback, and complete typed semantic-effect coverage. Check correct-session delivery for hosted Claude/Pi and terminal leads, including no-adapter and stale-adapter views, disabled-dashboard/unauthorized cases, old ring records and existing tool-summary behavior. Exercise expandable display in narrow and wide layouts with hostile content. Record exact commands, candidate/served pins, outputs and limitations; distinguish source evidence, executed acceptance and durability guarantees. After eventual develop integration, mail MAIN the merge hash; MAIN alone owns subsequent runtime replacement.
