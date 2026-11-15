# Approved compact panel header — 649A

## Authority and outcome

Owner decision 649=a, confirmed by MAIN on 2026-11-14. Implement the settled compact selected-A design from `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/03045922-01-panel-card-design-variants-a-compact.md` and sibling HTML/fragment references. This is a bounded styling implementation, not a new design/API choice. Work on `feat/compact-panel-header`, based on reviewed develop `8c0840f3`, in this full worktree.

## Scope

Primary source: `agents-plugin-tool/internal/wswhiteboard/assets/page.css`. Existing native markup is in `assets/page.tmpl`; the production maximize/minimize buttons are siblings after `.pmeta`, not nested mockup metadata. CSS-only implementation is expected. Focused assertions may be added in existing `internal/wswhiteboard/stream_test.go`, `panel_script_test.go`, `render_test.go`, `regions_test.go`, `internal/wsdashboard/cards_test.go`, or `scripts/board-headless.mjs` only as needed. Do not introduce a module, exported symbol, JS layout engine, new template wrapper or semantic behavior; return a scope blocker if styling cannot satisfy the settled contract.

## Contract

- Wide header: chevron 10px, naturally wrapping full title `minmax(0,1fr)`, right-aligned metadata and existing action controls; 8px gaps, minimum 24px row, zero padding. Remove forced metadata second row/negative margin. Place sibling controls explicitly without overlap.
- Actual history content width <=420px may put metadata on a second right-aligned row. Reuse existing responsive/container-query precedent; test narrow content inside a wide viewport, not viewport width alone.
- No ellipsis, clamp, fixed-height title clipping or absolute overlap. Time/count/reference/maximize remain in open and closed states; preserve exact-time tooltip, keyboard/native details behavior and >=24px interactive targets.
- Selected A has no enclosing card border or header divider. Shell body top gap is 6px with a quiet 1px left rule and approved 18px+12px inset. Scope body changes to `.entry-panel > .entry-body`; preserve arbitrary real renderer/iframe content and intrinsic inner spacing.
- No timestamp formatting, history selection/cache/chunk IDs/recut/order/store changes. Concurrent history-fit and relative-time units remain separate; source overlap is not a dependency, but integration must check geometry and boundary stability.

## Verification and delivery

Read all applicable repository manuals before edits. Run focused Go/style tests, build or syntax checks as appropriate, and real isolated browser checks at 1440/800/360/320 plus actual content<=420 within a wide host, long titles, collapsed/open, exact tooltip, max/Escape, body/iframe preservation and no overflow/overlap/errors. Artifact measurements are not production evidence. Exercise history boundary/cache stability across wrapping and geometry changes and compatibility with existing relative-label refresh paths without altering their algorithms. Report tested source identity and limitations honestly. Use only owned isolated fixtures/ports; no installer, host setup, live daemon/snapshot activation, release, push or integration. Request root-rendered independent review if nested provenance is rejected; no bypass. Commit one logical unit with AI Context and this plan, after source freeze/review coordination; lead owns integration. Large/high tier due compounded responsive, sibling-control, arbitrary-body and history-geometry correctness/test risks.
