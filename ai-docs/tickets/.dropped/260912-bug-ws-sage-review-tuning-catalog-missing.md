---
title: "Sage review setting is visible but cannot be tuned through the catalog"
dropped: 2026-09-12
---

# Sage review setting is visible but cannot be tuned through the catalog

## Background

During Pi dogfood, the owner explicitly requested that Sage review run automatically instead of asking at every ticket gate. `config.list` reported the resolved global value `sage_review: ask`, and `tickets.sage_gate` itself advised callers to inspect `config.list` for this setting. However, the returned tuning-knob catalog omitted `sage_review`; `lead-tune` permits writes only through catalog-declared writers, so it could not map or safely apply the requested change.

The resolved-setting surface and the advertised tuning surface therefore contradict each other: the setting is discoverable as active policy but unavailable through the only supported tuning workflow.

## Phases

### Phase 1: Expose and tune Sage review policy consistently [dropped]

Determine whether `sage_review` remains a supported public setting. If supported, expose it as a `config.list` knob with its exact value enum, scope, and `config.tune` writer contract, then teach `lead-tune` to route the standing preference without editing config files directly. If intentionally not tunable, remove the misleading `tickets.sage_gate` guidance and provide the actual supported policy surface.

Verification must cover `config.list` discovery, valid and invalid writes, resolved global behavior, no-agent visibility as appropriate, and a ticket gate changing from `ask` to automatic review execution without bypassing required Sage stages.


## Resolution (2026-09-12)

Dropped as a duplicate of develop-completed `260626-bug-sage-review-config-setter-missing`. The fix landed through `c1699e1e`, `6376480e`, and `b0248e51`, closed at `31134ca3`, and merged to develop as `3266f912`; the Pi track consumes it through the active refound sync rather than reimplementing shared ws-mcp behavior.
