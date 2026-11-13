---
title: "Pi worker's pi.wait MCP request times out before its requested wait budget"
---

# Pi worker's pi.wait MCP request times out before its requested wait budget

## Background

During the dashboard spawn-tree ticket's independent review on 2026-11-13,
a Pi worker called `pi.wait` for its reviewer with `timeout_s: 1000`. The tool
returned `MCP request timed out after 60000ms`, rather than waiting for the
requested budget or returning the documented running state. The reviewer was
not stopped. The timeout may belong to the Pi MCP bridge/client rather than
the daemon; root cause is not yet established.

## Intended investigation

- Identify which layer applies the 60-second request deadline to long `pi.wait`
  calls, and whether the default 300-second wait has the same mismatch.
- Preserve asynchronous completion delivery; never stop or respawn a healthy
  child merely because its wait request timed out.
- Add coverage at the layer that owns the deadline before choosing a fix.
