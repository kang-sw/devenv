---
title: "Investigate Pi publish-worker startup after an unclaimed version bump"
---

# Investigate Pi publish-worker startup after an unclaimed version bump

## Background

During shipping of v0.46.29, local preflight completed and the user approved
publication of fa514d5681776451c043f4407694629394b61d1f. Resuming the parked
preflight delegate failed before publication because cold startup attempted to
download the still-unpublished release. This is a dogfood evidence capture,
not authority to change launcher, adapter, or shipping semantics.

## Outcome Ledger

### Verified Findings

- `ws-agent-send` to `ws-ship-preflight` first exited with code 1 and stderr
  reporting HTTP 404 for the v0.46.29 darwin-arm64 release binary.
- A `ws-execute` trusted bootstrap command built a 0.46.29-dev runtime from
  the pinned source into the ignored `agents-plugin-pi/.runtime/darwin-arm64/`
  contract-addressed destination. No tracked file or release SHA changed.
- A direct launcher `tools git.status` probe then exited 0 and returned its
  schema, confirming cold launcher compatibility for that probe.
- The execute child and another resumed publish delegate still exited with
  code 1 and blank stderr. The publish delegate's session transcript contained
  no new task turn after its preflight result; no publish command was run.
- The root lead's existing MCP channel remained operational throughout.

### Confirmed Decisions

None. The user authorized shipping, not a shipped runtime-protocol repair.

### Proposals

None selected.

### Open Questions

- What environment/runtime contract differs between a direct launcher probe
  and the Pi child bootstrap after a version bump?
- Why does the resumed child's remaining bootstrap failure omit its diagnostic
  from stderr and the persisted session transcript?
- How should a downstream shipping executor remain cold-startable while its
  selected release version is not yet published, without weakening compatibility?

### Rejected Alternatives

No release pin changes, forced pushes, premature release tags, or compatibility
bypasses were used to work around startup.
