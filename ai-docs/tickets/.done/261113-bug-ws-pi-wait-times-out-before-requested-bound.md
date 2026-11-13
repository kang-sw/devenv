---
title: "Pi wait MCP call times out before its requested wait bound"
completed: 2026-11-13
---

# Pi wait MCP call times out before its requested wait bound

## Observation

During whiteboard implementation on 2026-11-13, `pi.wait` with
`timeout_s: 300` returned `MCP request timed out after 60000ms` while the
child remained active. Its tool contract advertises a 300-second default and
up to 1800 seconds. The transport deadline appears shorter than the advertised
blocking interval; this was not investigated during the implementation.

## Desired outcome

Make the advertised wait bound reachable, or return the child's running
status within the transport deadline without presenting an MCP failure.


## Resolution (2026-11-13)

Fixed by 3eb38e6ad (piMCPTimeout ~1860s, agents-plugin-tool/.../pi.go:131). Live check 2026-11-13 (decision 474): pi.wait with timeout_s 300 against a Pi worker running `sleep 100` blocked from 13:28:28Z until the turn ended (~100s, past the former 60s cut-off) and returned the turn's final message with no MCP client timeout. Duplicate of 261113-bug-pi-wait-mcp-client-timeout-before-requested-deadline, closed together.
