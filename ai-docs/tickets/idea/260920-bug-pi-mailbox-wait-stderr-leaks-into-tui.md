---
title: pi mailbox-wait subprocess stderr leaks into the TUI
---

# pi mailbox-wait subprocess stderr leaks into the TUI

## Symptom

While a pi lead is waiting on ws mailbox, a warning from the `ws-mcp mailbox
wait` child process leaks onto the terminal the pi TUI owns and corrupts the
render. Observed line:

```
[ws-mailbox] ws-mcp mailbox wait: warning: named inbox devenv@machine is not currently owned by this --session-key; falling back to a reply-id-only wait
```

The warning itself is benign (an expected fallback when the named inbox is not
owned by this session key → reply-id-only wait). The bug is that it reaches the
TUI's terminal at all.

## Located cause

- The Go CLI writes the warning to **stderr**, which is correct:
  `agents-plugin-tool/cmd/ws-mcp/mailbox.go:92`
  (`fmt.Fprintf(os.Stderr, "ws-mcp mailbox wait: warning: ...")`).
- pi's waiter spawns the child with `stdio: ["ignore", "ignore", "pipe"]` and
  pipes the child's stderr (`agents-plugin-pi/src/mailbox-waiter.ts:276`), then
  routes it through a default sink that writes straight to the shared TTY:
  `const stderr = options.onStderr ?? ((line) => console.error("[ws-mailbox] " + line))`
  (`mailbox-waiter.ts:269`, consumed at `:293-295`). Under the pi-tui,
  `console.error` writing to the raw terminal corrupts the render. The
  `[ws-mailbox]` prefix in the observed line is this default sink's output.

The `onStderr` hook exists precisely so a caller can inject a TUI-safe sink; the
TUI arming path is not supplying one, so it falls back to `console.error`.

## Fix direction (to settle at promotion)

Route the wait child's stderr to a diagnostic sink that never touches the TUI's
terminal — the pi host log / debug-events channel, or swallow-with-log — rather
than `console.error`, whenever the waiter runs under the TUI. Either have the
arming caller pass a TUI-safe `onStderr`, or change the default sink so it is
never `console.error` in a TUI context. Secondary (shared-surface, optional):
consider quieting the Go warning for the expected unowned-inbox fallback.

## Scope

pi-native only (`agents-plugin-pi` stdio handling); a host adapter, so no
shipped-surface boundary obligation. The Go warning's stream choice (stderr) is
correct and stays; this is purely pi rendering a child's stderr onto the TUI.
