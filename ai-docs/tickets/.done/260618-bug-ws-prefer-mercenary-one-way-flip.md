---
title: prefer_mercenary has no revert path within a session
related:
  260605-research-ws-native-subagent-pivot: mercenary surface and delegation-default guidance originate here
  260619-epic-ws-layered-config-prompt-tuning: superseded — prefer_mercenary becomes a session-scope item in this epic's layered config (child-1), gaining desired-state get/set that resolves this one-way-flip bug; close when that child lands
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

## Resolution (c65326bd) - 2026-06-19

Resolved by `260619-feat-ws-layered-config-scope-substrate` Phase 2 (impl
`090e69f3`, fix-cycle `c65326bd`, landed on the epic via merge `cd0f06b8`). The
desired-state answer was chosen: `ws.lead.prefer_mercenary` now accepts an
optional `enabled` boolean (default `true` for backward-compatible legacy call
shape), and `enabled:false` disables it on the same session key. The value rides
the per-key session `Overrides` overlay through the layered config resolver, so
`playbook.render` guidance follows BOTH transitions. The one-way `setPreferMercenary`
and the in-memory `preferMercenary` field are removed. The fresh-login reset path
is no longer the only revert. Open questions are answered: desired-state, not
one-way; the per-session-file interaction (260617) made the disable path the
clean implementation, exactly as anticipated.
