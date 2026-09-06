---
title: YAML tool inputs, distinct input/output backgrounds, and consistent compact result and push rows
related:
  260906-feat-ws-pi-tool-result-yaml-tui-rendering: reuse Phase 1 helper; coordinate with pending Phase 2 dispatch summaries
  260906-bug-ws-pi-yaml-result-rerender-cost: investigate responsiveness before adding rendering work
---

# YAML tool inputs, distinct input/output backgrounds, and consistent compact result and push rows

## Background

Owner requested these follow-ups after YAML result Phase 1: display input parameters as YAML, visually distinguish input/output backgrounds, give the direct read/run tools ten-line result previews, and make ws-agent-settled background consistent with ws-agent-report. Captured only; no implementation started. Keep this idea separate until its overlap with the existing YAML ticket's Phase 2 is reconciled; do not implement duplicate dispatch headers.

## Scope update - 2026-09-06

Bridged `ws__*` input YAML and logical ten-line input/output previews landed in
`bbf9a29a`/`2c8c50a`; owner confirmed lag disappeared and both displays worked.
A suspected JSON exception was stale pre-reload rows, resolved by the owner.
Bold titles and native-theme input/output backgrounds subsequently landed in
`4d278c13`; 946 tests and installed-host review passed with one Minor right-padding
background notch, pending owner-live styling acceptance. Do not duplicate these
bridged changes or infer a broad registration-unification requirement.

Remaining scope is direct read/run previews and shared push backgrounds, plus
any explicitly requested broader tool coverage. Preserve the logical-line/native
cached strategy, not the historical custom ASCII/width fallback described below.
The following findings record the original capture state.

## Findings and boundaries

- Bridge registrations currently have renderResult but no renderCall. Pi supports streaming argument rendering through renderCall and context.argsComplete. The observed raw JSON input presentation was not reproduced: inspected Pi defined-tool fallback renders only the tool name; generic missing-definition fallback renders JSON. Confirm the actual affected rows before broadening scope.
- Default call/result components share a parent Box/state background. Independent full-region backgrounds can use renderShell: "self", but then pending/success/error framing must be preserved explicitly. Theme palette, exact styling and target tool coverage are not settled; this is a feasibility finding, not a chosen final design.
- `do-i-really-have-to-read-this-myself` and `do-i-really-have-to-run-this-myself` are registered in `src/execute-gateway.ts` without result hooks. Reuse cycle-free `src/tool-result-render.ts` for collapsed preview and expanded full display. Do not change model payloads, read offset/limit semantics, or the command's existing 4KB/30-second execution limits. Preview row count follows the accepted best-effort policy; width loading failure uses the safe ASCII/logical-line fallback, not unsafe overwide rows.
- Both ws-agent-report and ws-agent-settled are registered through the same renderer in `src/push-render.ts`. Its Box currently lacks a background callback. Adding the theme customMessageBg through the shared Box seam could unify all push families without changing messages/details or lifecycle.
- The visual discrepancy might involve asynchronous renderer registration in `src/index.ts`: a message before registration can use Pi's background-bearing default renderer. This is a hypothesis, not a reproduced race. Do not conflate presentation cleanup with lifecycle changes.

## Phases

### Phase 1: Consistent display-only tool and push presentation

Resolve precise tool coverage and theme/state framing before implementation. Implement the accepted input YAML, distinct input/output regions, direct read/run preview, and shared push background requirements while reusing common helpers. Preserve partial/malformed argument tolerance, model content, error state, existing dispatch behavior, and expansion. Do not add resolved-model plumbing here without reconciling the pending YAML Phase 2 contract.

Verify input streaming, narrow terminals, collapsed/expanded read/run output, error/pending backgrounds, and both report/settled background callbacks. Include redraw-cost regression checks informed by the performance ticket and owner-live TUI acceptance. No further code work was approved in the capture session.
