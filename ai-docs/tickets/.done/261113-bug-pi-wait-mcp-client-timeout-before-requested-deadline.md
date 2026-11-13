---
title: "Pi worker pi.wait hits the MCP client's 60-second timeout"
completed: 2026-11-13
---

# Pi worker pi.wait hits the MCP client's 60-second timeout

## Observed

During mailbox-stream implementation on 2026-11-13, a Pi worker called
`pi.wait` for its own running child with `timeout_s: 300`. The tool returned
`MCP request timed out after 60000ms`, rather than waiting for the requested
bound or returning the child's running status. The child remained running.

The advertised wait default is 300 seconds (maximum 1800), so the worker's
MCP client timeout can make an otherwise valid wait fail before its deadline.
Inspect the worker bridge's tool-call timeout and preserve the requested wait
bound without replaying a timed-out request automatically.


## Resolution (2026-11-13)

Fixed by 3eb38e6ad (piMCPTimeout ~1860s, agents-plugin-tool/.../pi.go:131). Live check 2026-11-13 (decision 474): pi.wait with timeout_s 300 against a Pi worker running `sleep 100` blocked ~100s (past the former 60s cut-off) and returned the turn's final message with no MCP client timeout. Duplicate of 261113-bug-ws-pi-wait-times-out-before-requested-bound, closed together.
