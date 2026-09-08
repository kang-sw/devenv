---
title: Complete compact direct-tool results and muted push rows while retaining native tool styling
related:
  260906-feat-ws-pi-tool-result-yaml-tui-rendering: reuse Phase 1 helper; coordinate with pending Phase 2 dispatch summaries
  260906-bug-ws-pi-yaml-result-rerender-cost: performance evidence and accepted native-cache replacement; remaining attribution is not a prerequisite
  260906-workset-ws-pi-dogfood-ux: non-hierarchical capture of overlapping push presentation requests
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: bb7857b84cbc67d3
sage-review-completeness-reviewed: bb7857b84cbc67d3
---

# Complete compact direct-tool results and muted push rows while retaining native tool styling

## Background

Owner requested these follow-ups after YAML result Phase 1: display input parameters as YAML, visually distinguish input/output backgrounds, give the direct read/run tools ten-line result previews, and make ws-agent-settled background consistent with ws-agent-report. Some bridged-tool work subsequently landed as recorded below. On 2026-09-08 the owner authorized promoting this existing ticket with ALL remaining polish scope after the required ready gates, not splitting compact pushes into a separate ticket or leaving direct read/run polish behind. Implementation starts only through a later implementation handoff. Do not implement duplicate dispatch headers owned by the YAML ticket's Phase 2.

## Scope update - 2026-09-06

The historical YAML ticket Editions record bridged `ws__*` input YAML and
logical ten-line input/output previews in `bbf9a29a`/`2c8c50a`, with owner
confirmation that lag disappeared and both displays worked. This describes
those dated records, not the current physical wrapped-row policy identified
below. A suspected JSON exception was stale pre-reload rows, resolved by the
owner. The `4d278c13` Edition records bold titles, native-theme input/output
backgrounds, 946 passing tests and installed-host review with one Minor
right-padding background notch, pending owner-live styling acceptance. Those
historical test results were not independently rerun during this promotion. Do not duplicate these
bridged changes or infer a broad registration-unification requirement.

The separate-background policy and its right-padding-notch acceptance above
are historical, superseded by the owner's 2026-09-07 uniform-background
decision. Do not restore those surfaces or carry their old acceptance gate
forward as a requirement to reproduce the notch.

Remaining scope is direct read/run result previews and compact muted push
messages with shared backgrounds, with regression verification of current
bridged styling. Keep all genuinely remaining polish in this ticket. Preserve
native caching and the scoped logical-line preview contract below, not the
historical custom ASCII/width fallback. Broader unspecified tool coverage is
not included.

## Owner-requested push presentation

The owner reports that verbatim `ws-agent-settled` and other subagent messages
currently occupy the human transcript view. These messages address the lead
agent, not the human, so their presentation should be visually subordinate to
human-facing conversation.

- Apply a collapsed preview of at most ten newline-separated logical lines to
  subagent push messages, including `ws-agent-settled` and `ws-agent-report`,
  following the accepted tool-preview policy. This is not a strict cap of ten
  wrapped terminal rows. Preserve expansion to the full message.
- Gray out the foreground of these messages, not merely their background, so
  agent-to-agent traffic reads as secondary content. Keep the existing shared
  push-background requirement; foreground dimming is an additional requirement.
- This is display-only: the lead still receives the complete original message.
  Preserve message metadata, delivery, wake/settle behavior, and existing
  interaction controls. Do not summarize or truncate the model-facing payload.
- Verify short, exactly-ten-line, and longer report/settled messages; collapsed
  versus expanded display; gray foreground across theme changes; and unchanged
  model-facing payloads. Retain the logical-line/native cached rendering
  strategy and redraw-cost checks rather than reviving custom visual-row sizing.

The original capture-only restriction is superseded by the owner's 2026-09-08
ready-promotion decision. This ticket still does not authorize starting code
changes before its gates and the subsequent implementation handoff.

## Current source checkpoint - 2026-09-08

Source inspection supersedes the historical missing-hook premise: both direct
read/run tools already register through the common `registerWsTool` renderer.
The current shared helper uses a physical wrapped-row preview, with expansion
already wired. The remaining direct-tool work is to satisfy this ticket's
explicit ten-logical-line contract through the existing seam, not add duplicate
hooks. Preserve other tools' existing behavior outside this ticket's explicit
scope; do not silently change every shared-helper caller's preview policy.

The common push renderer currently emits all body lines, has no collapsed/
expanded branch or background callback, and dims only the status line. The
remaining push behavior below is planned, not a description of current code.

## Confirmed background policy - 2026-09-08

The owner reaffirmed the September 7 decision recorded in `930c668e`,
implemented by `2741a0dd` and documented by `fc4be8df`: tool input/output
inherit Pi's uniform native parent lifecycle background; neither installs a
separate background override. Foreground distinction and existing separators
remain. The owner explicitly rejected restoring separate tool backgrounds
while approving the full genuinely remaining polish scope in this same ticket.
Common push backgrounds are a different surface and remain in scope.

## Decisions and boundaries

- **Full remaining scope, one ticket.** Direct-tool result previews cover
  `do-i-really-have-to-read-this-myself` and
  `do-i-really-have-to-run-this-myself`. Preserve their titles, arguments,
  read offset/limit semantics, command 4KB/30-second execution limits, and
  model results. Collapse human-visible text output to ten logical lines with
  an overflow/expand indication; expanded output exposes the full returned
  result, not data beyond the tool's own execution limits.
- **Shared push presentation.** Apply the compact, muted treatment through the
  common push renderer, including report and settled messages. Retain family
  and agent identity, status information, error/approval/question meaning, and
  existing controls. Use theme-aware shared background and subdued foreground,
  not fixed colors or a second lifecycle policy. Expansion must recover every
  hidden part of the original human-visible message. The existing workset's
  older report-only background restriction is superseded by the later owner
  request for common push backgrounds, reaffirmed on 2026-09-08.
- **Native cached rendering remains the baseline.** Reuse the cycle-free
  `src/tool-result-render.ts` helper where applicable and native cached text
  layout. Select previews by newline-separated logical lines before layout;
  do not revive custom grapheme/visual-row counting or the obsolete ASCII
  fallback. Unavailable native helpers must leave a safe standard Pi display
  rather than break tool registration or headless operation. Preserve raw,
  malformed, error and non-text result semantics; do not force unsupported
  results through YAML conversion.
- **Existing bridge styling is retained, not reimplemented.** Keep bold names,
  the uniform native parent lifecycle background for both input/output slots,
  current foreground distinction and separators, streaming/partial argument
  tolerance, and standard pending/error framing. Do not install child background
  overrides, restore the historical separated surfaces, replace the parent shell,
  or widen registration solely on the historical unconfirmed renderer-registration
  timing hypothesis. Verify current styling for regressions rather than treating
  the superseded September 6 styling acceptance as still outstanding.
- **Sibling ownership.** YAML Phase 2 owns dispatch-tool input summaries and
  resolved-model lines. This ticket adds neither those hooks nor model-resolution
  plumbing and does not depend on that unfinished phase or its tier prerequisite.
  The common renderer and expansion seam already exist; its current physical
  preview is distinguished from this ticket's direct-tool logical-line requirement
  above. The performance ticket's owner-accepted replacement comparison is not
  an outstanding prerequisite.
- **Display only.** Preserve complete model-facing message content and tool
  results, structured metadata, asynchronous delivery, wake/settle ordering,
  report deduplication, fan-in semantics and existing interaction controls.
  Async explore continues delivering its answer through the settled message.
  The archived race tickets remain authoritative for delivery behavior; the
  monitoring consolidation is not permission to alter it. Mirror-drift remains
  a separate unresolved runtime/package issue, not a reason to silently change
  mirrored resources in this presentation ticket.

## Spec Impact

`ai-docs/spec/pi-adapter-runtime.md`: extend the tool-display contract with
logical-line previews and full expansion for the two direct read/run tools;
extend the pushed-message presentation contract with ten-logical-line collapsed
previews, full expansion, theme-aware common backgrounds and muted foreground.
Document that this changes human rendering only, preserving original content,
metadata, delivery, error/interaction semantics and headless behavior. Retain
the current bridged YAML contract: uniform native parent lifecycle background,
no separate input/output background overrides, existing foreground distinction,
separators and native-cache/fallback behavior. Dispatch summaries and
resolved-model display remain YAML Phase 2's
scope. This single phase is addressed by this Spec Impact section; no new
runtime or delivery contract is proposed.

## Phases

### Phase 1: Complete the remaining display-only tool and push polish

Implement the remaining direct read/run previews and shared compact/muted push
presentation under the contracts above. Reuse the accepted native cached
strategy and preserve the bridged input/output styling already delivered.
Keep the whole remaining polish slice together; no source changes are part of
ticket promotion itself.

Automated verification:

- Both direct tools: short, exactly-ten-line and longer text; collapsed preview,
  overflow indication and expanded full returned result; empty, error and
  unsupported/non-text shapes; unchanged execute content, metadata, offset/limit
  and command limit semantics.
- Pushes: short, ten-line and long report/settled bodies, plus the other families
  registered by the shared renderer; collapsed/full display, discoverable
  expansion, preserved identity/status and approval/question/error controls.
  Assert complete original model-facing payloads and metadata remain unchanged.
- Native themes and layout: foreground dimming and common push background in
  light/dark themes and after theme changes; preserved tool pending/error states,
  streaming/partial arguments, uniform tool backgrounds, foreground distinction
  and separators without child background overrides; narrow widths,
  Unicode and terminal-control safety, including safe standard-display fallback
  when native helpers are unavailable. Ten logical lines are not ten wrapped rows.
- Performance: repeated unchanged renders reuse preparation/layout caches;
  content, width, expansion and theme changes invalidate only the necessary work.
  Verify the two direct tools through their production registration factories
  and ensure unrelated shared-helper callers retain their preview policy.
  Exercise a long transcript with large collapsed outputs; do not claim numeric
  latency improvement or a specific historical root cause from unit tests alone.
- Run the Pi adapter suite and installed-host rendering checks. Headless imports
  and operation remain unaffected; no static pi-tui dependency is introduced.

Owner-live acceptance after implementation: in a fresh/reloaded real Pi TUI,
confirm direct read/run preview and expansion, compact subdued report/settled
messages with matching backgrounds and usable controls, and retained bridged
YAML input/output styling across themes with the uniform native parent background,
foreground distinction and separators intact. Do not revive the superseded
separate-background acceptance gate. Repeat a long-session responsiveness
comparison and report observations separately from
automated test evidence. This live acceptance remains pending at ready landing.

## Resolved review decision (2026-09-08)

The owner settled the background-policy gap: retain the September 7 uniform
native parent lifecycle background and do not restore separated input/output
backgrounds. The ticket is corrected above; the following first-round review
is retained as history, not an active blocker. Fresh design and completeness
reviews both passed after the owner-confirmed correction.

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Owner decision required: retain September 7 uniform native tool backgrounds or explicitly reverse it; September 6 distinct-background record is superseded by owner decision in 930c668e, implementation 2741a0dd and spec fc4be8df | critical | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
