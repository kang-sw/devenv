---
title: ServeStdio waits unbounded on in-flight handlers after stdin EOF
related:
  261008-bug-windows-mailbox-wait-outlives-pi-host: split out of its candidate direction 3
  260917-bug-windows-smoke-servestdio-exit-timing-flake: same EOF exit path
---

# ServeStdio waits unbounded on in-flight handlers after stdin EOF

## Background

Split out of 261008-bug-windows-mailbox-wait-outlives-pi-host, where it was
candidate direction 3. After stdin EOF, `ServeStdio`
(`agents-plugin-tool/internal/mcp/server.go`) waits on `wg.Wait()` for
in-flight handlers whose contexts derive from `context.Background()`. A hung
git hook or credential prompt can therefore keep `ws-mcp serve` alive
indefinitely after its host is gone. The EOF exit is also not recorded in the
lifecycle log.

The 2026-10-08 native-Windows probe did not observe this hang: after a
node-only kill, serve exited (inferred: on stdin EOF) with no lifecycle entry.
The risk is therefore code-derived, not reproduced.

## Candidate directions (unconfirmed)

- Cancel in-flight request contexts on EOF.
- A grace timeout on `wg.Wait` after EOF.
- Record an EOF lifecycle event.
