---
title: "Pi RPC child bootstrap failures lose the actionable error diagnostic"
related:
  260921-feat-ws-impl-branch-identity-resolver: dogfood discovery during independent review dispatch
---

# Pi RPC child bootstrap failures lose the actionable error diagnostic

## Background

During reviewer startup on 2026-09-21, ws-agent-spawn returned `Agent process exited (code=1 signal=null). Stderr:` with nothing after the colon. No child transcript was created. Both correctness and fit launches failed the same way.

The actual error was a version mismatch: the rebuilt local runtime reported `0.46.15-dev`, while the Pi bridge required exact `0.46.15`. Running Pi in RPC mode with the child-role environment cleared allowed its `extension_ui_request` notification to reach stdout, revealing the mismatch. Rebuilding the ignored runtime with `-ldflags '-X main.version=0.46.15'` allowed reviewer startup. This followed a separate, correctly surfaced launcher incompatibility after a runtime tool-inventory change.

`bootstrapOrFailLoud` in `agents-plugin-pi/src/index.ts` calls `ui.notify` and immediately exits for RPC children; the useful notification apparently does not reach the parent's surfaced failure. Investigate delivery/flush and error forwarding rather than assuming every blank-stderr exit has this root cause.

## Phases

### Phase 1: Preserve child bootstrap diagnostics

Ensure a failed child bootstrap returns an actionable message to ws-agent-spawn, including failures reported through UI notifications rather than stderr. Add a regression exercising immediate child exit after bootstrap failure; preserve fail-closed behavior without leaving a tool-less child running.
