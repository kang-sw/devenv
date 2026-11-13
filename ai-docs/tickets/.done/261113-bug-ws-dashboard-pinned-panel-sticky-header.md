---
title: "Dashboard: a pinned topic's sticky header does not stick in the pinned column"
blocked-by:
  - 261112-feat-ws-whiteboard-topic-tickets-sticky-head
  - 261113-feat-ws-dashboard-iframe-shell
related:
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "the sticky topic head this should match; still in ready/, not merged"
  261113-feat-ws-dashboard-iframe-shell: "changes how the board is framed; may change the pinned column's embedding"
completed: 2026-11-13
---

# Pinned panel sticky header

## Observed (2026-11-13)

Owner report (Korean, paraphrased): on the pinned panel the sticky header does not work. Leave it as a todo ticket.

On the dashboard session page (wide view), a whiteboard topic can be pinned into the right-hand pinned column (`.bpcol`, `data-pincol`). In the main board a topic's bar stays at the top while the topic scrolls; in the pinned column it does not stick.

## Expected

The pinned topic's header sticks to the top of the pinned column while its content scrolls, as it does in the board.

Both pin layouts are in scope (owner board screenshot, relayed by MAIN
2026-11-13): the pinned column (`.bpcol`) and the pin area atop the board
column in narrower layouts. Both are the same movable `#bpins` element
(3e922446), so the fix and its verification cover each layout's scroller.

## Likely cause (inferred, not verified)

- Board stickiness is scoped to the whiteboard rail: `.wb-rail .topic > .tbar { position: sticky; top: var(--wb-rail-top, 0px) }` (`agents-plugin-tool/internal/wswhiteboard/assets/page.css:656`) and, for cards, `.wb-rail .bcard:not([data-max]) .bc-hd` (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css:608`). rail.js adds the `wb-rail` class only to the board host (`agents-plugin-tool/internal/wswhiteboard/assets/rail.js:22`).
- The pinned list `.bpins` lives in `.bpcol` and is its own scroll container (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css#L1482-L1482`, `overflow: auto`), outside `.wb-rail`, so the `.wb-rail`-scoped sticky rules do not match pinned topics and cards. rail.js also drives `--wb-rail-top` and the stuck shadow for the board's items only.
- Fix direction: apply the same sticky rules (and a stuck-state shadow) to `.bpins` topics and cards against `.bpins` as the scroller, by scoping the rules to `.bpins` too (see Decisions). Confirm `.bpins` topics are not clipped by an overflow ancestor.

## Decisions

- Fix by CSS scoping only: the sticky rules and the stuck-state shadow also match `.bpins` topics and cards, with `.bpins` as the scroller (announced default, EXEC 2026-11-13). Rejected: a rail instance over the pinned column, already rejected by `261112-feat-ws-dashboard-pin-column` decision 451 ("the pinned column has no rail") and commit 9c7cae82.
- Lands after `261112-feat-ws-whiteboard-topic-tickets-sticky-head` and `261113-feat-ws-dashboard-iframe-shell` (`blocked-by:`), because the fix targets their final markup (`.tophead`, shell framing); landing order set by the MAIN lead 2026-11-13.

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md` (Implementation Conventions row for `agents-plugin-tool/`).
- `ai-docs/manuals/release-compatibility.md` (row for `agents-plugin-tool/internal/`). Expected tier T0: asset CSS/JS only, no MCP, protocol or store change.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Board and narrow layouts must keep their current behaviour.
- Coordinate with the sticky-head ticket (a new `.tophead` wrapper; no `tophead` in the tree under `agents-plugin-tool/internal` yet, pending 261112-feat-ws-whiteboard-topic-tickets-sticky-head) and the iframe-shell ticket so the fix targets the final markup.

## Prior Decisions

- 261112-feat-ws-dashboard-pin-column (2026-11-12, Decisions 451): "The topic rail (wswhiteboard `rail.js` capsules ...) keeps railing the board column only; the pinned column has no rail. Rejected: a second rail instance for the pinned column" — bearing: contradiction-candidate
- 9c7cae82 (2026-11-12, commit): "Rejected: server-side serving of pinned topics (breaks the T0 asset-only scope); a second topic rail in the pinned column." — bearing: constrains
- 3e922446 (2026-11-12, commit): "The pinned column lives at the .dbody level (sibling of the rail and the grid), as in the approved mockup, so the topic rail bound to #bcol never sees pinned topics." — bearing: constrains
- 3e922446 (2026-11-12, commit): "One movable pin list element (#bpins) instead of two reconcilers: a layout switch moves that one element" — bearing: constrains
- 9034387e (2026-11-11, commit): ".bc-in now clips rather than hides its overflow so it is no scroll container for the sticky header." — bearing: supports
- d9b292c8 (2026-11-12, commit): "the topic note follows the sticky header as a floating capsule while traversing long topics, and SEND closes every open note." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.css, agents-plugin-tool/internal/wswhiteboard/assets/page.css; rail.js only if a rail instance is chosen |
| scope.surface | internal | asset CSS/JS only; no exported symbol, MCP, or store change |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | wsdashboard/terminal_page_test.go and wswhiteboard/rail_offsets_test.go, rail_motion_test.go reference bpins/bpcol or wb-rail; the LOOK capture is manual |
| complexity.reuse_points | confirmed | .wb-rail sticky rules at page.css#L656-L656 and dashboard.css#L608-L608, stuck shadow page.css#L677-L677 |
| complexity.side_effect_risk | moderate | rules scoped to .wb-rail also serve the board and narrow layouts (pins sit in .r-board below 1600px); the rail-instance option adds a second scroll listener |
| risk.correctness | moderate | cause is inferred, not verified; the sticky head markup (.tophead) is not landed (pending 261112-feat-ws-whiteboard-topic-tickets-sticky-head) |
| risk.fit | high | the ticket's fix direction (a rail instance over .bpins) reverses recorded pin-column Decision 451 and the 9c7cae82 rejection; markup also moves with the iframe shell |
| risk.test | moderate | verification is a manual LOOK capture; automated tests only pin CSS text |
| risk.security_or_contract | low | asset CSS/JS only, tier T0 stated in Constraints; no MCP, protocol, or store surface |

## Phases

### Phase 1: Make pinned topics and cards stick in the pinned column

Reproduce, confirm the cause above, then make a pinned topic's bar (and a pinned card's header) stick to the top of `.bpcol > .bpins` while its content scrolls.

Verification: a LOOK capture of the pinned column scrolled with a topic header stuck at its top; a capture of the main board and of the narrow layout showing them unchanged.


## Resolution (2026-11-13)

Fixed by the MAIN lead's ticketless delegate on fix/wb-pin-empty-sticky, commit 8c6b1f5b7, merged into develop at 05447f96e. `.bpcol > .bpins` topic bars and card headers stick. The above-board `#bpins` already sat inside `.wb-rail`, so the existing rail rules cover it; scoping to bare `#bpins` would have overridden `--wb-rail-top`. This covers both pin layouts. Not done: the pin column has no stuck-state shadow, because rail.js sets `.wb-stuck` for the board only; this is cosmetic and left to UX post-swap QA. The interaction with `.tophead` from 261112-feat-ws-whiteboard-topic-tickets-sticky-head is re-checked when that branch merges.
