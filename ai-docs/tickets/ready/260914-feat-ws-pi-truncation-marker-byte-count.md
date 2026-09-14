---
title: "Show total byte size in the collapsed preview truncation marker"
related:
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 94a17e4d9792c763
sage-review-completeness-reviewed: 94a17e4d9792c763
---

# Show total byte size in the "..." truncation marker

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | single-file | agents-plugin-pi/src/tool-result-render.ts (only edit target; web-fetch.ts is cited as Buffer.byteLength precedent, execute-gateway.ts/spawner.ts only as the other-marker follow-ups Decisions leaves untouched) |
| scope.surface | internal | truncatedMarker/physicalPreviewLayout are non-exported functions (agents-plugin-pi/src/tool-result-render.ts#L141-L145,L152-L166) |
| scope.new_public_symbol | no | none required; change is local to truncatedMarker/physicalPreviewLayout |
| scope.new_type_contract | no | Decisions confines this to the single visible marker; unifying the three markers (a shared PreviewFormat field) is explicitly a follow-up, not this ticket |
| scope.test_surface | existing | agents-plugin-pi/test/tool-row-render.test.ts#L233-L241 exercises PREVIEW_ROWS/truncatedMarker cap |
| complexity.reuse_points | confirmed | Buffer.byteLength pattern (agents-plugin-pi/src/web-fetch.ts#L260 extractedBytes) computes the total-source byte size directly |
| complexity.side_effect_risk | moderate | touches physicalPreviewLayout's shared width/wrap layout math used by every collapsed and expanded tool preview |
| risk.correctness | low | N is `Buffer.byteLength` of the full source text (a total, not a remainder), so no multibyte cut-point matching is needed; the marker text must not shift existing row-budget layout |
| risk.fit | low | reuses the repo's existing Buffer.byteLength precedent, no new pattern introduced |
| risk.test | moderate | existing test only asserts row-cap behavior, not marker text/byte-count wording |
| risk.security_or_contract | low | internal TUI display formatting only, no external contract or security surface |

## Background

When the TUI collapses a tool-call/result preview it shows a "..." marker with
no indication of how large the content is. The owner wants that marker to report
the **total** byte size of the content (the bytes already shown plus the hidden
ones) as "N bytes".

## Evidence

- The visible "..." is `truncatedMarker()` (`tool-result-render.ts`), appended
  by `physicalPreviewLayout()` when a collapsed preview exceeds
  `PREVIEW_ROWS = 10` wrapped/logical lines (expand via Ctrl+O). It is
  currently row/line-based and does **not** compute bytes, so surfacing a byte
  count needs a `Buffer.byteLength` on the source text at the truncation point.
- Precedent that computing the datum is cheap: `web-fetch.ts` already computes
  `extractedBytes = Buffer.byteLength(...)` and returns it on the tool result.
- Two other truncation markers already have byte counts in scope but do not
  surface them: `CAP_OUTPUT_DROP_HINT` (`execute-gateway.ts`, one-liner
  4096-byte cap) and `PROMPT_TRUNCATION_MARKER` (`spawner.ts`,
  `PROMPT_STORAGE_CAP_BYTES = 4096`).

## Decisions

- **Scope to the visible `truncatedMarker()` only.** This ticket changes the
  collapsed tool-call/result preview marker (`tool-result-render.ts`) only. The
  other two markers (`CAP_OUTPUT_DROP_HINT`, `PROMPT_TRUNCATION_MARKER`) keep
  their current text; unifying all three is a follow-up, not this ticket.
- **Marker wording: `...[N bytes total]`.** `N` is the byte size of the whole
  content (`Buffer.byteLength` of the full source text), i.e. it counts the
  bytes already shown plus the hidden ones — the total, not just the hidden
  remainder. Compute it on the source text at the truncation point (the marker
  is row/line-based today and does not compute bytes).

## Phases

### Phase 1: Total-byte annotation on the collapsed preview marker

Compute `Buffer.byteLength(sourceText)` for the full pre-truncation source at the
point `truncatedMarker()`/`physicalPreviewLayout()` decides to collapse
(`tool-result-render.ts`), and render the marker as `...[N bytes total]`. Change
only this marker; leave `CAP_OUTPUT_DROP_HINT` and `PROMPT_TRUNCATION_MARKER`
untouched. Preserve the existing `PREVIEW_ROWS` row-budget/wrap layout — the
byte annotation must not shift the collapse threshold or the expanded view.

Verification: extend `agents-plugin-pi/test/tool-row-render.test.ts` to assert
the marker text carries the correct total byte count (including a multibyte case
so `Buffer.byteLength` vs. string length is exercised) and that the row-cap
behavior is unchanged.

## Open Questions

- None blocking.
