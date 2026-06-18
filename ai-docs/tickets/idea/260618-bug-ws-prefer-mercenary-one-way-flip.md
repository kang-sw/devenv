---
title: prefer_mercenary has no revert path within a session
related:
  260605-research-ws-native-subagent-pivot: mercenary surface and delegation-default guidance originate here
related-mental-model:
  - mcp-runtime
---

# prefer_mercenary has no revert path within a session

## Background

`ws.lead.prefer_mercenary` is a one-way set, not a toggle. `setPreferMercenary`
(`session_auth.go`) hardcodes `entry.preferMercenary = true` with no parameter,
and the handler (`server.go`) always returns `prefer_mercenary: enabled`. Once a
lead flips a session key to mercenary-primary, there is no tool to flip it back;
the only reset is minting a fresh session key (a new session entry defaults to
native) or a server-process restart (in-memory state).

Found during dogfood: a flag enabled while probing could not be turned off on the
same key when the user asked to revert. The reasonable caller expectation is that
a preference can be unset.

## Open Questions

- Should the tool accept a desired state (enable/disable) rather than being
  one-way? Or is one-way set intentional for the session lifetime?
- If one-way is intentional, the fresh-login reset path should be documented as
  the supported revert (and surfaced in the tool description / manual) rather
  than left implicit.
- Interaction with `260617-refactor-mcp-stateless-subagent-context`: if session
  state moves to a per-session file, `preferMercenary` becomes editable metadata,
  which would make a disable path trivial — decide whether to defer the fix to
  that work or add a disable path independently.
