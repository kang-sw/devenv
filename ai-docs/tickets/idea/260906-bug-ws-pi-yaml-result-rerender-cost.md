---
title: Investigate severe Pi TUI slowdown and repeated YAML result rendering cost
related:
  260906-feat-ws-pi-tool-result-yaml-tui-rendering: Phase 1 introduced the renderer under investigation
---

# Investigate severe Pi TUI slowdown and repeated YAML result rendering cost

## Background

Owner reported input/output responsiveness around 0.25 FPS in the long dogfood session on 2026-09-06. The session was stopped for ticket-only capture; no performance fix was authorized or implemented. Source HEAD was `8ce0c92`, with YAML renderer implementation `f34548f4` retained on `impl/track/pi-agent/plop-ozone-abide`, not merged to `track/pi-agent`.

Direct observations: `ps` showed interactive `pi` PID 50506 at 102–105.9% CPU, parent login zsh, about 449 MB RSS initially. `sample 50506 3 10 -file /dev/stdout` showed 250/257 main-thread samples under timer callbacks, with 238 under ArrayPrototypeFlatMap and visible SegmentIteratorPrototypeNext work. JavaScript function names were unresolved, so this does not prove the renderer was the active call site. Samples were inspected in conversation; no durable profile file was saved. PID is historical, not a future target.

Source inspection of `src/tool-result-render.ts` found `createToolResultComponent.render` recomputes `renderResultRows`; that renders/serializes text, wraps all logical lines via flatMap, and only then selects ten collapsed rows. `wrapDisplayLine` segments graphemes and repeatedly measures candidate row strings. There is no rendered-row cache and `invalidate` is empty. This is a plausible amplification of redraw cost, not a confirmed sole cause. Do not claim a specific asymptotic bound without measurement.

Owner believes Pi's O(n) terminal responsiveness is a known upstream issue; this was not independently verified. Distinguish upstream transcript traversal/render cost from adapter-added work, other extensions, and terminal compositor cost.

## Temporary rollback and comparison

After initial capture, the owner reopened the same session without extensions
and reported normal responsiveness. They also reported that the slowdown
surged after reload introduced the YAML changes. This strengthens the
extension/YAML hypothesis; it does not isolate this extension from all other
extensions or prove a particular renderer call site.

The owner then explicitly requested a temporary YAML rollback for comparison.
The adapter source, dependency manifests, tests, and implemented YAML spec
passage are restored to the pre-YAML baseline `b5df503a`; push-wake fixes and
follow-up tickets are retained. Historical renderer findings above refer to
`f34548f4`, not the temporarily reverted working tree. Reopen the same session
with the ws extension enabled after rollback to isolate the YAML contribution.
Owner-live rollback comparison is pending; do not claim restored performance
from source equivalence or unit tests alone. No caching fix is implemented.

## Phases

### Phase 1: Attribute and reduce avoidable redraw work

Reproduce on a long session with large collapsed tool outputs and measure redraw/input latency and CPU. Attribute cost using a JavaScript CPU profile or controlled renderer comparison before selecting a fix. Investigate caching by content/result state and width, avoiding repeated serialization/wrapping, with correct expansion/theme/invalidation behavior. These are candidate strategies, not an approved algorithm.

Preserve model payloads, safe per-row output, terminal-control sanitization, YAML conversion, and the owner-approved logical-line ASCII fallback. Exact ten-visual-row budgeting is best-effort; do not repeat the previous scope expansion into a second Unicode implementation. No unrelated TUI redesign in this slice.

Verification should include repeated unchanged renders, width/expansion/content changes, and a long-session latency comparison, not just correctness tests. Document residual upstream cost separately. Owner-live observation remains required before claiming responsiveness restored.
