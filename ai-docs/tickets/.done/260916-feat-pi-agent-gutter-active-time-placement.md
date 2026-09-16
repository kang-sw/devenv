---
title: "Refine Pi live-agent and footer status UX"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 17336914f9e45453
sage-review-completeness-reviewed: 17336914f9e45453
completed: 2026-09-16
---

# Refine Pi live-agent and footer status UX

## Background

Live Pi dogfooding exposed two compact-status presentation improvements. The live-agent gutter separates total elapsed time from the related active-time value, while the custom footer replaces Pi's built-in footer but currently shows only the host-provided Git branch. This ticket keeps both improvements together as a focused Pi status-UX pass.

The footer must remain cheap to render. Git work belongs behind an in-memory cache and lifecycle-driven refreshes, never in the render path or synchronously on the user-input path.

## Decisions

### Live-agent duration layout

Current shape:

```text
gutter-probe | worker | running | 1m | gpt-5.6-luna (high) | 17k | active 25s | $0.33
```

Desired shape:

```text
gutter-probe | worker | running | 1m (25s) | gpt-5.6-luna (high) | 17k | $0.33
```

- Relocate the existing active-time entry from its separate trailing position; do not delete or replace that entry.
- Remove only its `active ` prefix and render the same active-time value immediately after total elapsed time, enclosed in parentheses as `<total-time> (<active-time>)`.
- Preserve the active-time display's current color and other styling after it moves.
- Keep the existing `worker` label unchanged.

### Footer Git status

- Keep the current custom-footer scope: TUI lead and fork sessions only.
- Append compact Git status immediately after the existing cwd and parenthesized branch display:

  ```text
  ~/devenv (develop) merging ↑1 ↓2 +12 -3 ~2 ?1
  ```

- Follow `shell/statusline.sh` for the compact counters and their semantic colors:
  - `↑N`: commits ahead;
  - `↓N`: commits behind;
  - `+N`: unstaged added lines;
  - `-N`: unstaged deleted lines;
  - `~N`: unstaged changed paths;
  - `?N`: untracked paths.
- Do not add a staged-only indicator.
- When no repository operation is active and every compact counter is zero, append no Git-status suffix; do not render `working tree clean`.
- Place the repository operation state directly to the right of the branch:
  - render `conflicting` in red whenever unmerged paths exist; it overrides any operation label;
  - otherwise render `merging`, `rebasing`, `cherry-picking`, or `reverting` in yellow when that operation is active.
- When width is insufficient, omit the Git-status indicators as a group while preserving cwd and branch.

### Git-status refresh and cache

- Hold Git status in an in-memory cache keyed by working directory.
- Request an asynchronous refresh after every `turn_end`, not only at `agent_settled`. Coalesce overlapping requests so only one Git query per working directory is in flight.
- While the lead is idle, request a low-frequency refresh every five minutes.
- If user input arrives when an idle refresh is due or pending, defer that idle refresh for 30 seconds. Further input restarts the full 30-second delay, and the refresh runs only if the session is still idle when the delay expires.
- Never await Git from the user-input path. Rendering reads only the cache and requests a re-render when a refresh publishes a new value.
- Bound Git execution with a short timeout. On a transient failure, retain the last successful cached value; outside a Git repository, render no Git-status indicators.

## Constraints

- The live-agent change is a presentation-only relocation of the existing active-time entry: do not change total-time or active-time accounting semantics.
- Do not move, remove, rename, or restyle any unrelated live-agent gutter field.
- Preserve all existing footer content and extension-status behavior; this ticket augments the cwd/branch area rather than replacing another field.
- Do not perform Git, filesystem, history, or registry traversal from the footer render function.
- Do not introduce high-frequency polling or filesystem watchers; refreshes are limited to the confirmed lifecycle requests and the single low-frequency idle timer.
- Keep cached rendering bounded and responsive across reload and shutdown.

## Prior Art

- `shell/statusline.sh` defines the confirmed compact Git counters and color intent. Its `+` and `-` values are unstaged line totals, while `~` and `?` are path counts; it is not a staged/conflict contract.
- `agents-plugin-pi/src/agent-footer.ts` owns the current custom footer and cached render path.
- `agents-plugin-pi/src/index.ts` owns Pi lifecycle and input-hook registration.

## Prior Decisions

- 260915-bug-ws-pi-widget-context-value-removed (2026-09-15, Phase 1): "Kept a dedicated unlabeled live-row formatter rather than changing formatContextTokens so /audit's existing ctx Nk and ctx ? contract remains unchanged." — bearing: constrains
- 260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets (2026-09-15, Result): "lastActivityAt rendered per row as active Xs/Xm/XhYYm" — bearing: supports
- 260913-bug-ws-pi-cost-footer-cpu-saturation (2026-09-13, Result): "The synchronous 250 ms descendant scan and render-time history traversal were removed because both ran on Pi's main thread and reproduced sustained CPU saturation." — bearing: constrains
- 260912-feat-ws-pi-custom-footer-cost-telemetry (2026-09-13, Result): "The built-in footer cannot be extended field-by-field, so the adapter owns a complete replacement while leaving the independent belowEditor agent widget untouched." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts#L372-L393, agents-plugin-pi/src/agent-footer.ts#L429-L491, agents-plugin-pi/src/index.ts#L462-L470 |
| scope.surface | public-interface | owner-visible live-agent panel and custom footer |
| scope.new_public_symbol | no | no externally callable symbol is specified |
| scope.new_type_contract | yes | internal Git-status cache record and refresh lifecycle |
| scope.test_surface | existing | agents-plugin-pi/test/agent-widget.test.ts and agents-plugin-pi/test/agent-footer.test.ts |
| complexity.reuse_points | confirmed | formatCompactDuration, footer controller lifecycle, and shell/statusline.sh counters |
| complexity.side_effect_risk | moderate | asynchronous Git subprocesses and an idle timer require lifecycle cleanup |
| risk.correctness | moderate | displayed counters and operation precedence must match repository state |
| risk.fit | high | footer replacement must retain existing content, width behavior, and O(1) render path |
| risk.test | high | cache coalescing, deferred idle refresh, failure retention, width, and lifecycle need coverage |
| risk.security_or_contract | moderate | Git execution and owner-visible repository-state display must fail closed outside repositories |

## Phases

### Phase 1: Relocate the live-agent active-time entry

Update the Pi live-agent gutter to produce the confirmed duration layout. Preserve the existing active-time value, accounting, and style; only its position and `active ` prefix change.

Verification:

- Add focused plain and themed renderer cases with distinct total and active durations. Assert the exact `<total-time> (<active-time>)` order, absence of a trailing `active ` field, and unchanged surrounding label, state, model/effort, context, and cost fields.
- Assert that the moved active-time value retains its existing style and that established narrow-width behavior remains bounded.
- Run the focused agent-widget tests and the full Pi TypeScript suite.

### Result (da578281) - 2026-09-16

- Relocated the existing active-duration value beside total elapsed time as `1m (25s)`, preserving `syntaxNumber` styling and all time-accounting semantics. The worker label, state, model/effort, context, and cost fields remain unchanged.
- Decision: activity now belongs to the duration field even when the remaining telemetry group does not fit; styling is applied only after plain-text truncation, retaining protected owner cues and inspection hints.
- Verification: `cd agents-plugin-pi && npm test -- test/agent-widget.test.ts` passed all 45 tests; `npm test` passed 1,573 tests with 2 opt-in skips and no failures. Added distinct-duration plain/themed cases and exhaustive width bounds; updated the exact telemetry-fit boundary for the shorter layout.
- Independent correctness, fit, and test reviews were all clean. No unresolved findings.
- Deferred: Phase 2 footer Git-status work remains unimplemented and the ticket stays ready.

### Phase 2: Add cached Git status to the custom footer

Add the confirmed counters and repository-operation state beside the existing cwd/branch display. Populate a per-working-directory cache through asynchronous `turn_end` refreshes and the debounced five-minute idle refresh, preserving an O(1) cache-only render path and non-blocking input handling.

Verification:

- Cover the confirmed counter semantics, operation labels and colors, `conflicting` precedence, clean and non-repository omission, exact placement, and grouped narrow-width fallback.
- With controlled lifecycle events and fake time, prove refresh requests after every `turn_end`, per-working-directory single-flight coalescing, the five-minute idle interval, resettable 30-second post-input deferral, the still-idle check, timeout behavior, stale-on-error retention, and timer/process cleanup on reload and shutdown.
- Assert that cached footer renders execute no Git, filesystem, history, or registry traversal and that publishing a changed cache value requests a re-render without blocking input handling.
- Run the focused footer and lifecycle tests and the full Pi TypeScript suite.

### Result (bdcee0fd) - 2026-09-16

- Added compact, semantically colored Git counters and repository-operation labels immediately after the branch and before the existing session name. Conflicts override operation labels; clean and non-repository snapshots produce no suffix. Narrow layouts drop all Git indicators together.
- Added an eight-working-directory in-memory cache with per-directory single-flight requests, asynchronous `turn_end` refreshes, a five-minute idle timer, and resettable 30-second input deferral. Rendering reads cached spans only; changed snapshots request a re-render. Reload/shutdown abort pending queries and suppress late publication.
- Decisions: no startup query or filesystem watcher was added; only turn boundaries and the idle timer request snapshots. Git uses a two-second whole-query deadline and subprocess timeout; transient failures preserve the last successful value. NUL-delimited porcelain v2 and numstat preserve path-count versus unstaged-line semantics, including staged-only omission.
- Verification: `cd agents-plugin-pi && npm test -- --test-reporter=spec test/agent-footer.test.ts test/footer-git-status.test.ts` passed all 26 tests; `npm test -- --test-reporter=dot` passed the full Pi suite after the final test change. Real Git fixtures cover repository states and subprocess cancellation/timeouts; controlled time covers refresh/debounce lifecycle behavior.
- Independent correctness and fit reviews were clean. The test review's Important request for explicit render-time filesystem/registry guards was addressed in e5e47a96 and verified clean in round 2. A test-only inherited-Map mock restoration failure was diagnosed and fixed with removable own-property guards. No unresolved findings.
- Omitted: interactive Pi smoke testing; automated rendering, real-Git, lifecycle, and full-suite verification were used.
