---
title: ws-dashboard e2e - "create terminal and run a command" resize-frame assertion fails, blocks acceptance suite green
related:
  260722-bug-e2e-open-work-root-locator-ambiguity: unmasked this failure once the earlier locator ambiguity was fixed
related-mental-model:
  - ws-web-dashboard
---

# ws-dashboard e2e - "create terminal and run a command" resize-frame assertion fails, blocks acceptance suite green

## Background

While fixing the openWorkRoot locator ambiguity
(`260722-bug-e2e-open-work-root-locator-ambiguity`), the acceptance suite began
progressing past the previously-blocking first step. This unmasked a separate,
pre-existing failure further down the same serial test.

## Symptom

`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts:2714`, inside the
`"create terminal and run a command"` `test.step` (step starts ~line 2519),
asserts that among all frames the browser sent on the live terminal WebSocket
during the step, at least one is `{"type":"resize",...}`. That assertion
fails. The `"type":"input"` and control-byte assertions immediately above it
pass; only the resize-over-socket assertion fails. Because
`dashboard-acceptance.spec.ts` runs as a single serial test, this red-lines
the remainder of the suite.

Confirmed via baseline rerun (locator fix stashed) that this same resize-frame
assertion reproduces identically once the earlier rootPicker failure is
bypassed - i.e. it is pre-existing, not caused by the locator fix.

## Impact

Blocks `npm run test:browser` / the Playwright acceptance gate from reaching
full green. That gate is the mandatory verification for UI-facing dashboard
work (per the ws-web-dashboard domain rule), so tickets that defer to it
(which-key overlay Phase 2, the App.tsx decomposition refactor
`260722-refactor-dashboard-app-tsx-leaf-extraction`) stay verification-blocked
until the suite runs green end to end.

## Root Cause

Diagnosis complete: a real product bug in the frontend resize-forwarding
path, high confidence, deterministic 2/2 reproduction plus Playwright
network-trace evidence (captured WebSocket frame log for the step showed
input frames but no resize frame).

`ws-dashboard/frontend/src/terminalPaneBody.tsx`'s `forwardSize()` (~lines
341-394) sends `JSON.stringify({type:"resize",columns,rows})` over the socket
only `if (socket?.readyState === WebSocket.OPEN)`; otherwise it falls back to
an HTTP resize call (`liveRef.current.actions.onResize(...)`, ~386-393) and
still latches `lastForwardedSizeRef.current`. `forwardSize` is invoked only
from the debounced ResizeObserver callback `scheduleResizeForward` (250ms
debounce, ~402-412), the `paneVisible` effect (~483-510), and `terminalPrefs`
changes (~629-648) - **never** from the WebSocket `"open"` handler
(~548-562), which only updates connection status.

On initial terminal creation the first (and, within this test step, only)
`forwardSize()` call fires while the socket is still `CONNECTING`, so the
resize goes out over HTTP and latches `lastForwardedSizeRef`. When the socket
subsequently opens, nothing re-forwards, so no resize frame is ever sent over
the now-open socket even though input frames flow over it fine.

The daemon still receives the correct size via the HTTP fallback
(`ws-dashboard/crates/daemon/src/terminal.rs` HTTP `terminal_resize` handler
-> same `session.resize()`), so the actual PTY size **is** correct - only the
transport contract (the live WebSocket carries resize frames per
`terminals.ts:47-48` and the spec anchor below) is unexercised on initial
creation. The client message schema (`terminals.ts:49-51`) and daemon
deserialization (`terminal.rs:280-285`) match; neither has a defect.

## Fix Strategy (Locked)

Conservative, behavior-preserving - not an open product question:

In `terminalPaneBody.tsx`, the WebSocket `"open"` listener must trigger a
size forward over the now-open socket (e.g. call the `forwardSize` path via a
ref after marking the connection "connected"), **and** `forwardSize()`'s
early-return/no-op gate (governed by `lastForwardedSizeRef`) must be made
transport-aware so the socket-open catch-up send is not suppressed just
because `lastForwardedSizeRef` already matches from a prior HTTP-fallback
forward.

Rationale: this re-sends the SAME already-fitted size the daemon already
holds - idempotent, since `session.resize()` with identical dimensions is a
no-op effect - so the daemon's resulting PTY size is unchanged and no
user-observable behavior changes. It only routes the resize over the intended
WebSocket transport as the contract requires. This ticket's scope is
behavior-preserving under expected public behavior.

## Spec Impact

No spec change. `ai-docs/spec/ws-web-dashboard/index.md` at
[Remote Terminal WebSocket Gatewaying](#remote-terminal-websocket-gatewaying)
already documents that the live socket "carries PTY output, input, resize,
ping/pong, and close frames" - that is the target contract this ticket brings
`terminalPaneBody.tsx` into compliance with. The daemon-side gatewaying, the
client message schema (`terminals.ts:49-51`), and daemon deserialization
(`terminal.rs:280-285`) are unaffected and already correct.

## Completion Boundary

The full `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` acceptance
suite reaches `spec.ts:2714` green with no new failures introduced elsewhere.

Known environment caveat: the daemon subprocess must run with
`SHELL=/bin/bash` (via `frontend/e2e/daemonHarness.ts` env passthrough) to
avoid an unrelated prompt-content mismatch at `spec.ts:2588` under a
zsh/starship ambient shell - that is an environment artifact, not part of
this ticket.

## Reporter Context

Surfaced by the implementer during
`260722-bug-e2e-open-work-root-locator-ambiguity` Phase 1 (commit 2bc160d4),
which narrowed the openWorkRoot locator and let the suite advance to this
step. Root-cause diagnosis completed 2026-07-22: deterministic 2/2
reproduction plus Playwright network-trace (WebSocket frame log) evidence
isolating the missing resize frame to `terminalPaneBody.tsx`'s
`forwardSize()`/socket-open interaction described above.

## Phases

### Phase 1: Forward terminal resize over the socket on WS open

Scope: In `ws-dashboard/frontend/src/terminalPaneBody.tsx`, wire the
WebSocket `"open"` handler (~548-562) to trigger a resize forward over the
now-open socket - e.g. call the `forwardSize()` path via a ref once the
connection is marked "connected" - and make `forwardSize()`'s existing
early-return/no-op gate (governed by `lastForwardedSizeRef`, ~341-394)
transport-aware so this socket-open catch-up send is not suppressed just
because a prior HTTP-fallback forward already latched the ref to the same
dimensions.

Constraints: Behavior-preserving. The re-sent dimensions must be identical to
what the daemon already holds via the HTTP fallback (`session.resize()` in
`ws-dashboard/crates/daemon/src/terminal.rs`), so the resulting PTY size and
all user-observable behavior are unchanged; only the transport the resize
travels over changes, to satisfy the socket contract documented at
`ai-docs/spec/ws-web-dashboard/index.md#remote-terminal-websocket-gatewaying`.
Do not touch the client message schema (`terminals.ts:49-51`) or daemon
deserialization (`terminal.rs:280-285`) - both already match and are not
implicated.

Completion boundary: `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`
reaches line 2714's resize-frame assertion green, and the full acceptance
suite run introduces no new failures elsewhere.

Verification: Run the full acceptance suite (e.g. `npx playwright test
dashboard-acceptance.spec.ts` from `ws-dashboard/frontend/`) with the daemon
subprocess launched under `SHELL=/bin/bash` (`daemonHarness.ts` env
passthrough) to avoid the unrelated zsh/starship prompt-content mismatch at
`spec.ts:2588`. Confirm the `"create terminal and run a command"` step's
input/control-byte assertions still pass and the resize-frame assertion at
`spec.ts:2714` now passes, with no regressions elsewhere in the suite.
