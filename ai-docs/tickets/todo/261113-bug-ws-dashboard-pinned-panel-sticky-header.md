---
title: "Dashboard: a pinned topic's sticky header does not stick in the pinned column"
related:
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "the sticky topic head this should match; still in ready/, not merged"
  261113-feat-ws-dashboard-iframe-shell: "changes how the board is framed; may change the pinned column's embedding"
---

# Pinned panel sticky header

## Observed (2026-11-13)

Owner report (Korean, paraphrased): on the pinned panel the sticky header does not work. Leave it as a todo ticket.

On the dashboard session page (wide view), a whiteboard topic can be pinned into the right-hand pinned column (`.bpcol`, `data-pincol`). In the main board a topic's bar stays at the top while the topic scrolls; in the pinned column it does not stick.

## Expected

The pinned topic's header sticks to the top of the pinned column while its content scrolls, as it does in the board.

## Likely cause (inferred, not verified)

- Board stickiness is scoped to the whiteboard rail: `.wb-rail .topic > .tbar { position: sticky; top: var(--wb-rail-top, 0px) }` (`agents-plugin-tool/internal/wswhiteboard/assets/page.css:656`) and, for cards, `.wb-rail .bcard:not([data-max]) .bc-hd` (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css:608`). rail.js adds the `wb-rail` class only to the board host (`agents-plugin-tool/internal/wswhiteboard/assets/rail.js:22`).
- The pinned list `.bpins` lives in `.bpcol` and is its own scroll container (`dashboard.css:1479`, `overflow: auto`), outside `.wb-rail`, so the `.wb-rail`-scoped sticky rules do not match pinned topics and cards. rail.js also drives `--wb-rail-top` and the stuck shadow for the board's items only.
- Fix direction: apply the same sticky rules (and a stuck-state shadow) to `.bpins` topics and cards against `.bpins` as the scroller, either by scoping the rules to `.bpins` too or by running a rail instance over it. Confirm `.bpins` topics are not clipped by an overflow ancestor.

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md` (Implementation Conventions row for `agents-plugin-tool/`).
- `ai-docs/manuals/release-compatibility.md` (row for `agents-plugin-tool/internal/`). Expected tier T0: asset CSS/JS only, no MCP, protocol or store change.
- Board and narrow layouts must keep their current behaviour.
- Coordinate with the sticky-head ticket (a new `.tophead` wrapper) and the iframe-shell ticket so the fix targets the final markup.

## Phases

### Phase 1: Make pinned topics and cards stick in the pinned column

Reproduce, confirm the cause above, then make a pinned topic's bar (and a pinned card's header) stick to the top of `.bpcol > .bpins` while its content scrolls.

Verification: a LOOK capture of the pinned column scrolled with a topic header stuck at its top; a capture of the main board and of the narrow layout showing them unchanged.
