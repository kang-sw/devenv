# Plan: Distinguish report headers within compact muted push messages — Phase 1: Add report-only header distinction

## Relevant Ticket Contract
- Give only the `ws-agent-report` header a distinct, theme-aware foreground; retain its existing status text and leave the owner-selected palette as an existing theme role rather than a fixed value.
- Keep muted payload bodies, the shared `customMessageBg` background, ten-logical-line collapse and expansion, metadata, payloads, ordering, delivery, and wake/settle behavior unchanged.
- Preserve terminal sanitization, native cached rendering, width bounds, and live theme behavior; do not add per-frame work proportional to a hidden body.
- Verify report headers against settled/question/approval/error, collapsed and expanded reports, theme changes, and 40/80/120-column rendering. The owner separately accepts report/body contrast under live light and dark themes.

## Out of Scope
- The other five push-family headers, their approval/question/error controls, and their separate live-acceptance gate.
- Changes to `agents-plugin-pi/src/spawner.ts` payload construction or send/delivery/wake/settle paths, and any body/background/layout/cache rewrite.

## Codebase Findings
- `agents-plugin-pi/src/push-render.ts#L146-L183` — `buildPushComponent` owns all six visual bands. It currently paints every head `muted`, sends bodies through the shared `createBoundedText`/`updateText` cache and sanitization seam, keeps statuses `dim`, and preserves the shared `customMessageBg` box.
- `agents-plugin-pi/src/push-render.ts#L196-L203` and `agents-plugin-pi/src/spawner.ts#L1268-L1275` — one common renderer registration covers the six named families, so report-only branching belongs at the head paint call and must not alter the registered family set.
- `agents-plugin-pi/src/tool-result-render.ts#L300-L397` — the reused body component caches sanitized source, width/expanded layout, joined display, theme styling, and fitted native rows; leaving its invocation and options unchanged preserves the no-hidden-body-work requirement.
- `agents-plugin-pi/test/push-render.test.ts#L180-L355` — injected fake TUI/theme tests already assert foreground/background token calls, collapsed/expanded recovery, and registration behavior; extend this seam rather than adding a second renderer harness.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/components/custom-message.js#L1-L57` — Pi’s native custom-message renderer uses `customMessageLabel` for its label; this is the existing semantic label role. Its built-in dark/light definitions provide dedicated purple values in `theme/dark.json` and `theme/light.json`, making it a source-backed, theme-aware report-header choice that remains visually distinct from the other families’ `muted` heads.
- `ai-docs/spec/pi-adapter-runtime.md#L1075-L1094` — the current presentation contract says all head/body/status foregrounds are subdued (`muted`/`dim`); update only this presentation statement to record the report-head exception and retained shared body/background behavior.

## Implementation Plan
1. In `agents-plugin-pi/src/push-render.ts`, select `customMessageLabel` only when the message’s registered family is `ws-agent-report`; use the existing `paint` helper for that head and retain `muted` for every other head, `muted` body/marker treatment, `dim` status, and the existing `customMessageBg` box. Do not change message parsing, `PUSH_FAMILIES`, body joining, `createBoundedText`/`updateText` options, renderer registration, or any spawner delivery/wake code.
2. In `agents-plugin-pi/test/push-render.test.ts`, extend the injected renderer tests to build report, settled, question, approval, and error/advisory-family messages through the registered renderer and assert that only the report head requests `customMessageLabel`, while the comparison heads remain `muted`; retain assertions that bodies and statuses use their existing roles. Exercise a long report collapsed and expanded at widths 40, 80, and 120, asserting the existing logical-line cap/recovery and fitted output remain intact. Mutate the same fake theme object's output between renders, then re-render unchanged content at the same width; use the existing bounded-preview probes or a focused counter in the existing fake TUI to show restyling happens without extra source sanitization/join/layout preparation. Keep this inside the current harness—no new test infrastructure.
3. In `ai-docs/spec/pi-adapter-runtime.md`, amend the pushed-message display presentation paragraph to state that `ws-agent-report` alone uses the theme’s `customMessageLabel` foreground for its head; other heads, bodies, markers, statuses, background, logical-line cap, expansion, and interaction semantics remain as specified.

## Verification Plan
- From `agents-plugin-pi/`, run `npm test` and read the complete output; the focused `test/push-render.test.ts` coverage must prove report-vs-settled/question/approval/error-family token selection, collapsed/expanded report behavior, theme replacement, 40/80/120 width bounds, and unchanged bounded-body preparation/layout behavior.
- Review the renderer and `spawner.ts` diff to confirm model-facing `content`/`details`, payload construction, family registration, delivery, wake, and settle code are untouched; run `git diff --check`.
- Owner-live check (separate from automated verification): open a report and comparison pushes in Pi under built-in light and dark themes; confirm the report header is recognizable while its muted body does not compete with owner conversation.

## Escalations
- None.
