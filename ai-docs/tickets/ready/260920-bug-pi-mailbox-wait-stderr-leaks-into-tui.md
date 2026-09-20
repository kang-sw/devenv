---
title: pi mailbox-wait subprocess stderr leaks into the TUI
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3d1d9c0a1a19c3f0
sage-review-completeness-reviewed: 3d1d9c0a1a19c3f0
---

# pi mailbox-wait subprocess stderr leaks into the TUI

## Background

While a pi lead waits on ws mailbox, a warning from the `ws-mcp mailbox wait`
child process reaches the terminal the pi TUI owns and corrupts the render.
Observed line:

```
[ws-mailbox] ws-mcp mailbox wait: warning: named inbox devenv@machine is not currently owned by this --session-key; falling back to a reply-id-only wait
```

The warning is benign (an expected fallback when the named inbox is not owned by
this session key → reply-id-only wait). The defect is that it reaches the TUI's
terminal at all and breaks the render.

**Located cause.**

- The Go CLI writes the warning to **stderr**, which is correct: the
  `os.Stderr` write in the `mailbox wait` command (search
  `agents-plugin-tool/cmd/ws-mcp/mailbox.go` for `falling back to a
  reply-id-only wait`). This stream choice stays.
- pi's waiter spawns the child with `stdio: ["ignore", "ignore", "pipe"]` and
  pipes its stderr, then routes it through a default sink that writes straight
  to the shared TTY: ``const stderr = options.onStderr ?? ((line: string) =>
  console.error(`[ws-mailbox] ${line}`))`` (agents-plugin-pi/src/mailbox-waiter.ts#L269;
  quote corrected from the ticket's original paraphrase — the source uses a typed
  parameter and a template literal, not string concatenation; search
  `agents-plugin-pi/src/mailbox-waiter.ts` for `onStderr` and the `child.stderr`
  data handler). Under the pi-tui, `console.error` writing to the raw terminal
  corrupts the render; the `[ws-mailbox]` prefix in the observed line is this
  default sink's output.

The `onStderr` hook exists precisely so a caller can inject a TUI-safe sink; the
TUI arming path supplies none, so it falls back to `console.error`.

## Constraints

- pi-native only (`agents-plugin-pi` stdio handling); a host adapter, so no
  shipped-surface boundary obligation.
- The Go warning's stream choice (stderr) is correct and is **not** changed by
  this ticket; the fix is purely on the pi side rendering a child's stderr.
- A behavior change carries a test change: the waiter's stderr routing under the
  TUI is the contract to pin.

## Prior Decisions

- 260913-bug-ws-pi-ownership-maintenance-errors-flood-tui (2026-09-13, Decisions): "No ownership observer or maintenance path may write directly to stdout, stderr, or console.* in normal mode." — bearing: constrains
- 260907-feat-ws-pi-local-devenv-ws-mcp-build-bootstrap (2026-09-07, commit cb0a89b7): "Pi's TUI owns the terminal at session-start time, so inheriting stdio would corrupt the TUI's rendering." — bearing: supports
- 260914-feat-ws-pi-mailbox-native-steer-push (2026-09-15, commit 7f0a0d35): "The two side-effecting edges (wait subprocess, recv drain) are injected as runWait/drainMail so the arrival->push detection path is unit-testable with a fake waiter and no live mailbox." — bearing: supports
- 260917-feat-ws-pi-mailbox-waiter-slug-wake (2026-09-18, commit a8a181fc): "the CLI's existing owner-gate degrade already keeps a wrong/unreachable slug safe by construction (reply-id-only fallback with a stderr warning)" — bearing: supports
- 260913-feat-cross-session-mailbox-wake (2026-09-13, ticket Result 588dda97): "emits a one-time stderr warning when an explicit --slug cannot currently be reached (no presence yet, or owned by a different session)" — bearing: supports
- 260919-feat-mailbox-arm-wait-harness-include (2026-09-19, ticket Decisions): "arm-the-wait.pi.md is a short overlay stating the adapter already arms the waiter and pushes mail, so the lead launches nothing." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/mailbox-waiter.ts, agents-plugin-pi/src/index.ts (arm site), agents-plugin-pi/test/mailbox-waiter.test.ts |
| scope.surface | internal | no exported symbol's public contract changes; SubprocessWaitOptions.onStderr and MailboxWaiterDeps.onError already exist as injection points |
| scope.new_public_symbol | unknown | depends on an undecided design choice between changing the default sink in mailbox-waiter.ts vs. the arm site in index.ts supplying one |
| scope.new_type_contract | no | none — onStderr/onError fields already exist on SubprocessWaitOptions/MailboxWaiterDeps |
| scope.test_surface | existing | agents-plugin-pi/test/mailbox-waiter.test.ts |
| complexity.reuse_points | confirmed | SubprocessWaitOptions.onStderr injection point, documented "Diagnostic sink for the child's stderr and spawn failures" (agents-plugin-pi/src/mailbox-waiter.ts#L241-L242) |
| complexity.side_effect_risk | low | createSubprocessWait's only caller is the index.ts TUI arm site (grep confirms no other callers in agents-plugin-pi), so a default-sink change is scoped to this one path |
| risk.correctness | low | pure diagnostic-routing change; no exit-code mapping or control-flow paths touched |
| risk.fit | low | consistent with the existing injectable onStderr/onError seam and with the 260913 precedent banning normal-mode stdout/stderr/console writes |
| risk.test | low | the existing fake-driven test harness (scriptedWait pattern) already exercises this seam; asserting the default sink is not console.error is straightforward |
| risk.security_or_contract | low | no security surface; no contract change if the fix stays within the existing onStderr/onError injection points |

## Phases

### Phase 1: Route the wait child's stderr off the TUI terminal

**Intended behavior.** When the waiter runs under the pi TUI, the `mailbox wait`
child's stderr is routed to a diagnostic sink that never writes to the TUI's
terminal (the pi host log / debug channel, or swallow-with-log), instead of the
`console.error` default. Either the arming caller supplies a TUI-safe `onStderr`,
or the default sink is changed so it is never `console.error` in a TUI context.
The benign named-inbox fallback warning no longer corrupts the render.

**Deferred scope.** Quieting the Go-side warning for the expected unowned-inbox
fallback is out of scope (shared surface; an optional follow-up). Only pi-side
stderr routing changes here.

**Verification boundary.** A test asserts the waiter's stderr sink under the TUI
does not reach `console.error` / the raw TTY and that the child's stderr instead
lands on the injected diagnostic sink; existing waiter tests stay green; the
non-TUI path (if any) is unaffected.
