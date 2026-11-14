---
title: "Dashboard: shared card collapse, expand and raise actions with topic-relative preview opening"
related:
  261110-feat-ws-dashboard-file-preview: "preserve existing preview reuse and sandbox; opening placement changes here"
  261112-feat-ws-dashboard-pin-column: "preserve existing pin/flow grouping"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "preserve topic ticket badges and state displays"
  261113-feat-ws-dashboard-board-reorder-hold: "interaction reference only; frozen Phase 4 is not reopened"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 7d5a6e0295ba02c2
sage-review-design-reviewed: 7d5a6e0295ba02c2
---

# Shared card actions and topic-relative preview opening

## Background

The owner requested one framework for topic, terminal and file-preview card operations: collapse/expand the whole card, raise it, and open a preview immediately above the topic from which it was opened. Folding only a topic's content leaves its history and question regions consuming space. Existing topic-specific ticket badges must remain visible and extensible rather than being lost in a generic rewrite.

Owner approvals on 2026-11-14: (660)=ok, (661)=a, (662)=a, (663)=ok. There is no approved new visual mockup; this ticket captures an interaction/structure contract and preserves existing renderer/chrome rather than treating unrelated toolbar/header mockups as approval for this work. This standalone unit does not wait for hosted-agent commonality or mention notes.

## Decisions

1. **Adopt existing topic DOM into a shared action layer.** Preserve server-rendered topic body and renderer; share collapse/expand/raise behavior with terminal and preview cards through type-specific header/body/badge/lifecycle extension points. Rejected: replacing the whole topic renderer or hiding its ticket metadata to fit a generic card. Authority: (660).
2. **Whole-card collapse.** Hide body, history and question regions, leaving title, ticket badges, state and action controls. Restore existing internal disclosure states on expansion. Rejected: content-only collapse or resetting every inner disclosure. Authority: (663).
3. **Raise icon.** Put a raise-to-top icon action in the upper-right common card controls. It moves the card to the top of its existing pinned or ordinary region, without changing pin state. Authority: the original request and (662)=a.
4. **Topic-relative opening is one-time placement.** A preview opened from a topic is placed immediately above that topic when the topic is in the ordinary region; this applies to new cards and reuse/replacement of the existing preview slot. The preview is independently movable afterward. Rejected: a persistent follow/attachment relation. Authority: (661)=a.
5. **Pin preservation wins at the boundary.** Opening an ordinary preview from a pinned topic places it at the ordinary-region head rather than silently pinning it. Existing matching-pinned-preview reuse remains pinned. Raise never changes pin state. Rejected: changing pin state merely to guarantee adjacency across regions. Authority: (662)=a.
6. **Compatibility is T0.** Reuse existing card placement/pin persistence where possible; no non-additive store/protocol change or new persistent parent/follow relation is authorized.

## Constraints

- Keep type-owned lifecycle behavior. Generic card close is not the workflow operation that closes a whiteboard topic; existing terminal disposal and preview cleanup remain their own operations.
- Preserve existing preview sandboxing, one-unpinned-slot behavior and matching pinned reuse. Files explorer's floating sheet is not automatically converted into a generic card by this ticket.
- Carry the validated originating topic through direct links, link menus and sandbox frame-link dispatch; a detached/invalid origin falls back to normal ordinary-region opening instead of borrowing an unrelated topic.
- Preserve existing source-update, pin/drag/manual placement and maximum-view behavior outside the specifically approved placement/collapse/raise changes. No persistent topic-preview relationship is introduced.
- Existing frozen board-reorder Phase 4 and header/ResizeObserver work are not reopened. If common actions cannot be integrated without changing a frozen contract, return that precise conflict to the lead rather than retrying the frozen lane.
- Applicable implementation manuals: `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md`. No skill/template/convention change is required by this interaction ticket.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Art

- `agents-plugin-tool/internal/wsdashboard/assets/cards.js`: generic cards, placement/pin/drag/maximize and existing card state (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L161-L215`, `#L337-L365`, `#L395-L417`, `#L428-L549`, `#L595-L697`, `#L753-L781`, `#L867-L891`). Topic decoration currently adds pin/maximize/back, while generic card headers own fold and type badges; the shared raise and topic whole-card collapse are new work.
- `agents-plugin-tool/internal/wsdashboard/assets/preview.js` and `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`: preview opening and origin/event dispatch. The existing preview slot replaces/reuses ordinary content via `card.bump()` and reuses a matching pinned card without bumping (`preview.js#L218-L265`); direct links and link menus currently pass no originating topic (`dashboard.js#L3092-L3103`, `#L3150-L3179`). Frame-link dispatch validates the source frame and document key, then creates a detached link for the menu (`dashboard.js#L3900-L3911`, `#L3958-L3971`, `#L4066-L4096`); origin must come from that validated frame, not message-supplied topic identity. Preview sandboxing and type labels remain kind-owned (`preview.js#L60-L69`, `#L111-L115`, `#L156-L168`); terminal state badges and close/disposal remain terminal-owned (`agents-plugin-tool/internal/wsdashboard/assets/terminal.js#L278-L282`, `#L392-L430`).
- `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`: server-owned topic DOM and topic-specific header/badges (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L38-L43`, `#L105-L108`). The `.tophead > .tbar` contains title/ticket/state controls, while checks, document, history and question regions are separate descendants; collapsing only `.tdoc` cannot collapse the whole card (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L36-L43`, `#L52-L52`).
- Topic morph/update integration is in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L800-L928`, not a dashboard `assets/stream.js` (that path does not exist). `agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L354-L405` owns history capture/restore, including inner fold states; retain these owners and the existing renderer. General keyed disclosure restoration is in `dashboard.js#L195-L215`. The common maximum-view lifecycle already supports cards, topics and panels without moving their DOM (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L595-L697`).

## Prior Decisions

- 01088661 (2026-11-14, commit): "Shared, not separate: the sheet lives in cards.js maximize() (one scrim, one Escape handler, one `maxed`)" — bearing: constrains
- 541fe405 (2026-11-14, commit): "Retain target-level dropdown/menu/search Escape consumption and the document-level sheet Escape handler; dismiss the Files sheet before an existing preview/card reveal" — bearing: constrains
- 8f24cec5 (2026-11-14, commit): "Saved entry IDs replace cumulative 640px snapping so later geometry can grow the clip without changing its membership." — bearing: constrains
- 0325a7aa (2026-11-14, commit): "panel fold state now persists in localStorage ws.dashboard.panelshut (capped LRU 200, key <rid>/<data-key>)." — bearing: constrains
- 701cf55a (2026-11-12, commit): "The page decides \"outside\" from markup the server rendered (file link, an absolute path, no path under the root), and the daemon decides on the click whether the path is really linked" — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-11, Result): "Unpinning a preview card makes it the slot and closes the other unpinned preview card." — bearing: constrains
- 5695371f (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: constrains
- 702e2f9d (2026-11-13, commit): "Structure is built server-side (EntryPage.Segments) rather than by client JS wrapping, because the dashboard's frameGraft needs matching DOM shapes along a frame's path." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/preview.js, assets/dashboard.js; agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl and assets/stream.js are renderer/history preservation boundaries |
| scope.surface | cross-module | window.WsCards action/placement layer, window.WsPreview.open callers, and server-rendered wswhiteboard topic DOM |
| scope.new_public_symbol | unknown | Type-specific extension points and origin plumbing are authorized, but no new exported JS name is specified; no Go/MCP export is requested |
| scope.new_type_contract | yes | Shared topic/header/body/badge/lifecycle extension contract and originating-topic context across preview entry paths; concrete signatures remain implementation details |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/cards_test.go#L74-L100, preview_outside_test.go#L64-L100, whiteboard_bridge_test.go; scripts/board-headless.mjs can host focused browser regressions; no new test file required by the phase |
| complexity.reuse_points | confirmed | cards.js order/headTime, topic decoration, persist, Card.fold/bump, maximize and move; preview.js open/slot/pinned matching; dashboard.js morphBoard/docFrame/frameLink; wswhiteboard/assets/stream.js capture/restore were read |
| complexity.side_effect_risk | high | Shared placement and collapse touch persistent view state, live topic morphing, focus and iframe/terminal sizing across pin/flow and maximum-view transitions |
| risk.correctness | high | One-time adjacency must cover slot replacement and pinned reuse without changing pin state or losing inner fold state on updates |
| risk.fit | moderate | Existing topic DOM and type-owned badges/lifecycles must be adopted into WsCards, not rebuilt; frozen lanes remain outside this unit |
| risk.test | high | Node placement tests cannot establish real hidden-body geometry, focus, iframe continuity or narrow/wide maximum-view behavior; browser verification is still future work |
| risk.security_or_contract | moderate | Origin propagation crosses validated sandbox-frame dispatch and detached link menus; keep docFrame/document-key validation and preview capability/sandbox boundaries unchanged |

## Phases

### Phase 1: Unify card actions and implement approved opening placement

Implement shared action behavior and a topic adapter without replacing the existing topic renderer. Wire collapse/expand/raise consistently across topic, terminal and preview cards. Carry the originating topic through preview-opening entry paths and apply the approved one-time placement and pin-boundary rules.

Verification:

- Browser coverage at wide and narrow widths with topic, terminal and preview cards, including a topic with ticket badges, open questions, history and individually expanded entries.
- Collapse visibly removes all body/history/question regions; expand restores each inner disclosure state; title/badges/state/actions remain usable. Verify keyboard focus and accessible action labels.
- New ordinary preview, reused ordinary slot and replaced preview slot open immediately above an ordinary originating topic. Afterward independent drag and topic updates do not create persistent following.
- Pinned-origin fallback goes to the ordinary-region head without changing pin state. Reusing a matching pinned preview preserves its pin; raise moves only within the current region. Include empty regions and maximum-view transitions.
- Cover direct link, context/menu and sandbox-frame link opening plus invalid/disappeared origin fallback. Preserve URL/file sandbox boundaries.
- Verify reload behavior using the existing placement persistence contract, and verify normal live topic updates without resetting card/inner disclosure state or duplicating event handlers.
- Run affected existing dashboard/card/preview tests and new focused browser regressions. Do not use a frozen-lane retry or live daemon replacement as acceptance. Keep source verification separate from actual runtime delivery.
