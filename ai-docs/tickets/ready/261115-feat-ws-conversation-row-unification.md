---
title: Unify conversation row frames and remove saved-change diagnostics
related:
  261115-feat-ws-dashboard-conversation-rendering-consolidation: related; retain primary update cards and effect data
  261115-feat-ws-dashboard-user-message-envelope: parallel; user-entry branch belongs to EXEC4
  261115-feat-ws-composer-inline-skill-and-topic-capsules: landing order; its head was delivered first
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 6e903006766d43b9
sage-review-design-reviewed: 6e903006766d43b9
---

# Unify conversation row frames and remove saved-change diagnostics

## Background

MAIN assigned this owner-settled convo-readability change on 2026-11-15 after the composer-ux head. The owner wants one restrained row frame rather than separate mail cards and tool pills, and explicitly approved removing the saved-change diagnostic disclosure. This is presentation-only work, not another mail-correlation or envelope implementation.

## Decisions

- Four row kinds share one frame: mail notification, peer mail, tool group and lead reply. Use a left icon column (mail, arrow, wrench, lead avatar respectively), middle content, right time column and a single far-right disclosure chevron where expandable. Keep real template data; expanded mockup text is placeholder.
- Remove the mail-notification border and tool-group pill outline. Row body text is 13px sans-serif; only tool names are monospace. Use one gray-default chip style with green peer-lead variant.
- **(1020)=b:** peer mail remains one row. Clicking the row body expands its details. Remove the separate `▶ 전달 세부` line, including on mail-notification rows, without hiding the real diagnostic text or breaking peer links, keyboard activation or keyed disclosure state.
- **(1021)=b:** token numbers remain visible; tool-group duration moves to the right time column.
- **(1026)=a:** retain the lead reply bubble, aligned to the frame's content column, and align its `↳ 리드 턴` footer to the same x position.
- **(1019), amended by MAIN at 07:26:17Z and 07:33:54Z:** retain committed-whiteboard update cards, topic headers/links and agent log lines. Within each topic, collapse changed-item rows into one wrapping row of clickable ID chips such as `(1031) (1032) (1033)`, each using the exact board-item target of today's `보드에서 답변` link. Remove the redundant per-item label/link rows. Remove the `저장된 변경 세부` disclosure containing per-field before/after diagnostics, plus now-dead render/i18n support; the owner explicitly approved this deletion. Keep stored effects and every MCP/API surface unchanged.
- Check MAIN's screenshot observation that an update answering 1029–1034 also displayed unrelated board-reorder-hold (518) and exec-decisions (520, 521, 523, 524). Verify the saved effect and current projection rather than assuming those items were untouched. Fix a confirmed bounded presentation leak with a focused regression; report an unresolved cause or producer/store/API change to MAIN for Claude routing rather than expanding this ticket.
- Fold the queued `TestWhiteboardEffectsRoutesReadSavedHistoryWithoutAnAdapter` contract repair into this change: assert panel recap is present and raw panel payload is absent. Do not restore raw payload rendering to satisfy the stale test.
- Compatibility: T0 display-only; no producer, persisted-effect, MCP/API, queue/claim/acknowledgment or envelope contract change.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Preserve EXEC4's landed mail-wake pairing contract (`b0efca7fa`, merged as `c51439783`): unique matched wake keeps its Key/At/raw Text; the paired mail remains a suppressed keyed entry. Keep both keys, the hidden root's `data-entry`/`data-mail-wake="suppressed"`, matched root's `data-entry`/`data-mail-wake="matched"`, and exposure of `MailDeliveryText` through the remaining disclosure. Preserve the mergeEntries attribute-sensitive replacement and entry-cache flags (agents-plugin-tool/internal/wsdashboard/conversation_mail_wake.go#L65-L84; assets/dashboard.tmpl#L298-L318, assets/dashboard-conversation.js#L17-L43 and entry_cache.go#L41-L58 under agents-plugin-tool/internal/wsdashboard/).
- EXEC4 confirmed its envelope worker edits only the user-entry branch. Leave that branch, `wsinbox.Entry.Text`, `splitUserEntries`, inbox offsets and per-envelope-block time behavior untouched; do not wait for or duplicate that parallel implementation. Preserve mail/tool/lead row ownership at integration.
- Read every matching repository Implementation Conventions manual before product edits. Use scoped relevant tests and full wsi18n, including `TestEveryEnglishKeyIsUsed` after deleting template keys. One bounded logic review only if interaction/state logic changes; pure style/expectation work gets none. MAIN alone merges and swaps; owner-screen comparison follows integration. No full suites, screenshot matrices, browser-proof gate or shared-runtime actions.
- The composer head `eb0ef4d48328bc1ee7d28a93374677c97ebf454e` was delivered to MAIN before this capture. MAIN's 07:36:05Z allocation requires the separate `TestBoardFramesMountAtEveryInsert` fix-forward head before this implementation starts. Search-agent removal (1018) now runs in parallel on disjoint tracker/backend files; do not absorb it here or edit its locale keys.
- Source boundary for the 07:33:54Z amendment: WhiteboardCard creates item rows only from decoded `item` effects, grouped by effect topic and deduplicated by kind/ID; it does not enumerate current board items (agents-plugin-tool/internal/wsdashboard/pageview.go#L488-L493; conversation_presentation.go#L68-L142 under the same dashboard directory). Chips must reuse today's validated `item-N` target (positive IDs through 999999), topic context and existing `data-wb-jump` link behavior, not mint targets for currently untargetable rows (conversation_presentation.go#L110-L114; assets/dashboard.tmpl#L304-L304; assets/dashboard-conversation.js#L143-L165 under that directory). The saved v1 effect retains kind/topic/ID/field/before/after, sufficient to check whether 518/520/521/523/524 were projected from that record, but is capped at 128 effects/64 KiB with 4096-byte values and a Truncated flag; omitted effects cannot be reconstructed from it (agents-plugin-tool/internal/wswhiteboard/effects.go#L9-L22; agents-plugin-tool/internal/wslive/whiteboard_effects.go#L16-L64). Current producer diff compares item fields around commands, not just the IDs mentioned in an answer (agents-plugin-tool/internal/wswhiteboard/effects.go#L68-L111; store.go#L728-L743 under that whiteboard directory). No saved effect for MAIN's screenshot was supplied or accessed in this pass: request that exact ring record, its Truncated flag and served revision from MAIN before attributing a leak; screenshot content alone establishes neither untouched items nor the producing source. Unknown cause or producer/store/API work returns to MAIN/Claude.

## Visual References

Approved references preserved under `ai-docs/tickets/assets/261115-feat-ws-conversation-row-unification/`:

- [after.html](../assets/261115-feat-ws-conversation-row-unification/after.html) — preferred text reference.
- [after.png](../assets/261115-feat-ws-conversation-row-unification/after.png), [after-expanded.png](../assets/261115-feat-ws-conversation-row-unification/after-expanded.png), [compare.png](../assets/261115-feat-ws-conversation-row-unification/compare.png).

Compare the implemented frame to the approved reference through the owner's on-screen post-merge check under MAIN's explicit UI-fast allocation; no pre-head screenshot matrix. The mockup contains example content and placeholder diagnostics, not replacement production fields.

## Prior Decisions

- d98dc221 (2026-11-15, commit): "Client merges previously ignored all existing keys. Replace only association-state transitions" — bearing: constrains
- d1109b20 (2026-11-15, commit): "Keep injected text, MCP arguments/routes/semantics and on-disk data unchanged (T0 display-only)." — bearing: constrains
- 261115-feat-ws-dashboard-conversation-rendering-consolidation (2026-11-15, commit): "Panel payloads never enter supported chat cards or change-detail disclosures. Panel recaps are single-line text" — bearing: constrains
- 261115-feat-ws-whiteboard-committed-effects-in-conversation (2026-11-15, commit): "Saved-history mode has entries and board regions, no composer/live inbox/activity regions, and cannot overwrite saved composer drafts." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl, dashboard-conversation.css, dashboard-conversation.js; conversation_presentation.go, conversation_history_test.go and adjacent conversation tests; agents-plugin-tool/internal/wsi18n/tables/*.json. |
| scope.surface | internal | Existing dashboard templates/assets and render helpers only; no producer, MCP/API, inbox or envelope changes authorized. |
| scope.new_public_symbol | no | None; reuse existing row, disclosure and mail-association owners. |
| scope.new_type_contract | no | None; presentation-only chip grouping and detail deletion preserve stored effects and existing entry/link contracts. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/conversation_mail_wake_test.go, conversation_mail_wake_paging_test.go, conversation_navigation_test.go, conversation_history_test.go, whiteboard_effects_test.go; agents-plugin-tool/internal/wsi18n/keys_test.go#L103-L160. No tests run for population. |
| complexity.reuse_points | confirmed | Read linkMailWakes/MailDeliveryText in agents-plugin-tool/internal/wsdashboard/conversation_mail_wake.go#L19-L84, cache flags in entry_cache.go#L41-L58, attribute-sensitive merge in assets/dashboard-conversation.js#L17-L43, existing row templates in assets/dashboard.tmpl#L298-L318. Pairing is already in HEAD via c51439783; do not reimplement. |
| complexity.side_effect_risk | moderate | Changing nested disclosures can lose keyed state, keyboard activation or peer-link behavior; hidden/matched roots also drive history reconciliation. |
| risk.correctness | moderate | Preserve mail identity/raw details and per-topic chip targets. Current WhiteboardCard cannot create item IDs absent from decoded item effects (agents-plugin-tool/internal/wsdashboard/conversation_presentation.go#L68-L142); screenshot cause remains unverified without MAIN's exact saved record/served revision. Saved-history recap-only repair remains required (conversation_history_test.go#L271-L305; whiteboard_effects_test.go#L16-L21 under that dashboard directory). |
| risk.fit | moderate | Four conversation rows reuse existing bubble/footer/tool owners; amendment replaces only per-topic item rows with one wrapping chip row, preserving topic headers, logs and panel recaps. Existing item-N/topic links and missing-target fallback are reusable (agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L304-L304; assets/dashboard-conversation.js#L113-L165 under that dashboard directory). User-entry/envelope branch remains excluded. |
| risk.test | moderate | Existing contracts cover pairing/history/cache, but new body-click disclosure and alignment need focused assertions; owner post-merge screen comparison is allocated instead of a screenshot gate. Eleven locale tables exist under agents-plugin-tool/internal/wsi18n/tables/. |
| risk.security_or_contract | moderate | Preserve escaped mail/diagnostics and peer links; delete only saved-change detail UI, not supported recap cards, unsupported-carrier fallback or stored effects (agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L303-L309; conversation_presentation.go#L144-L154). T0 has no changed writer or persisted representation. |

## Phases

### Phase 1: Shared row frame and minimal disclosures

Implement the settled frame, typography, chips, time placement and single body-click disclosure using existing conversation template/asset owners. Retain the lead bubble/footer and all real message, token, tool, peer and delivery fields. Replace each topic's changed-item rows with one wrapping row of ID chips using the existing board-item targets; preserve topic headers and agent log lines. Verify the reported unrelated-item leakage against saved effects and projection, fixing only a confirmed bounded presentation bug. Remove the saved-change details UI and genuinely dead render/i18n support only; retain whiteboard cards, panel recaps and stored data. Update focused contracts, including the saved-history recap-only test, to prove the intended UI rather than restore removed diagnostics.

Verification covers the four row kinds, collapsed/expanded behavior and keyboard/link handling, visible tokens and moved duration, lead/footer alignment, matched/suppressed/unmatched mail identity and raw delivery details, and preserved update-card topic headers/logs/recaps apart from the approved item-row grouping and diagnostics removal. Assert one wrapping ID-chip group per topic with unchanged item-link targets, unchanged topic headers/log lines, and no unrelated projected items when the saved effect does not contain them. Assert saved-change disclosure is absent, recap is present, raw panel payload is absent, and affected locale keys have real remaining uses or are removed from all eleven tables. Run relevant scoped offline tests and full wsi18n; record known baseline failures separately. A lead-owned current-develop sync before head preserves EXEC4 pairing and any later envelope work; rerun only integration-affected checks. Return a clean committed head; MAIN owns integration and owner-screen acceptance.

## Producer delta amendment — 2026-11-15 (design gate pending)

MAIN's 07:42:34Z instruction supersedes the earlier unknown-cause hold only for item 1043. Claude verified the exact saved record was not truncated and identified archive maintenance as the source of unrelated item-presence effects. This amendment authorizes a narrow T0 producer correction **only after MAIN's single elevated delta-design gate passes**. The UI work may finish independently; do not implement this producer delta before that gate. Autofold/new-ticket authoring remains outside this ticket.

### Consumer evidence

- `agents-plugin-tool/internal/wswhiteboard/store.go:728-743` records command diffs, then coalesces created history, evicts settled archive items, prunes labels and records a final diff. `effects.go:104-111` emits item `presence: present -> none` when a compared item disappeared. These maintenance removals therefore enter the saved conversation projection today.
- The only production `Outcome.Effects` consumer found is the post-commit recorder in `agents-plugin-tool/internal/mcp/whiteboard_tools.go:278-284`, through `whiteboard_effects.go:11-38` into the existing bounded v1 ring envelope. This is best-effort display data, not board replay. The decoder keeps both presence directions valid (`internal/wslive/whiteboard_effects.go:164-172`); no Kind/Field change is needed.
- Board removal does not consume effects: `internal/wsdashboard/board_topics.go:107-118` sends authoritative topic order/changed HTML and `assets/dashboard-board.js:449-467` reconstructs membership from those snapshots. Tracker refresh uses its revision event (`internal/wsdashboard/sse.go:492-496`, `assets/dashboard-stream.js:235-239`), not saved effects. No inspected live-board, client-removal or tracker path relies on eviction-presence effects.
- `internal/wswhiteboard/update_summaries.go:193-218` independently computes its own net diff; its item-presence classification counts additions (`:255-283`). Keep this summary path unchanged.
- One existing test explicitly expects archive-maintenance effects: `internal/wswhiteboard/effects_test.go:100-108` applies an empty command to an over-cap fixture and expects ID 1's removal. Under this approved delta its expectation changes to no command effects **while still asserting that eviction occurs**. Preserve the same test's withdrawal, reopening and adoption coverage.

### Intended implementation and compatibility

Move the final command-effect comparison to after `coalesceCreated` but before `evictArchive`/`pruneLabels` in `internal/wswhiteboard/store.go`. Preserve per-command effects, implicit command mutations (including close-induced withdrawals and adoption), command order, coalescing, archive cap/settlement order, label pruning, persisted board state and existing post-commit recorder chain. Do not filter by literal command IDs and do not suppress presence removals in the shared `semanticEffects` helper. A clarifying comment may name the command/maintenance boundary in `effects.go`; its closed Kind/Field variants remain unchanged.

T0: both versions write/read the same v1 effect envelope and closed fields (`internal/wslive/whiteboard_effects.go:33-64,83-100,164-172`). The new producer omits maintenance-only display transitions; old readers can decode every remaining transition. The authoritative board writer still persists the same capped working state (`internal/wswhiteboard/store.go:570-595,622-623`), so reopening either version loses no board data. No MCP argument/route, store shape, API or envelope change is authorized.

### Delta test and verification contract

- Seed exactly `ArchiveCap` settled unrelated items plus open target(s); answer the target(s) so settlement crosses the cap. Assert effects include the actual target status/answer transitions and contain no untouched archive IDs.
- Assert oldest settled items are removed, newly settled targets remain, and the settled count stays capped. Exercise the existing persistence/reload fixture to prove the resulting authoritative archive state is unchanged.
- Update the explicit empty-command eviction expectation above: no command effects, actual archive eviction retained. Keep coalescing and implicit command-effect regressions intact.
- Verify saved-envelope decode and conversation chip projection contain only command-caused IDs; the Phase 1 projection test already proves absent saved IDs are never invented.
- Run focused offline archive/effect/coalescing tests and affected projection tests only; retain the full wsi18n run for the UI deletion. Reuse isolated HOME/config/runtime fixtures and warm Go caches. No full suites, browser/screenshot gate, providers/downloads or shared-runtime action.

Implementation is not started. MAIN owns delta-design acceptance, the single current-develop integration checkpoint and final merge/swap; owner-screen acceptance remains post-merge.
