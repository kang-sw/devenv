---
title: ws-agent-list shows each agent's model, and a revived orphan keeps its last-report time
related:
  260905-feat-ws-pi-agent-alias-park-and-registry-cap: owns the ws-agent-list shape and the dormant sidecar
parent: 260605-epic-ws-playbook-factory-pivot
spec:
  - pi-adapter-runtime
---

# ws-agent-list shows each agent's model, and a revived orphan keeps its last-report time

## Background

Two fidelity gaps surfaced in the 2026-09-05 acceptance run of the Pi
adapter:

- **D4.** The run asked the lead to compare the model of a default
  `ws-execute` worker against one spawned with `complex:true`. `ws-agent-list`
  exposes no model field, so the lead could not tell them apart even though
  every record already carries `modelBase` and `modelEffort` (the sidecar
  persists both). The only signal the lead saw was the unset-catalog advisory
  mentioning parent-model inheritance.
- **H.** After `/reload`, the restored registry entries had lost the
  `last_report_at` value they showed before. The sidecar writes
  `lastReportAt` for the orphan roll-call, but `rehydrateOrphanRecord`
  rebuilds the record with an empty `reportLog`, so the listing derives no
  last-report time for a revived agent.

## Direction

- Add `model` (base plus effort when set) to each `ws-agent-list` entry;
  omit it when the record inherited the parent's model with no resolved
  base, and say so in the field description.
- Carry the sidecar's `lastReportAt` into the rehydrated record so the
  listing keeps showing it (a single synthetic `reportLog` entry, or a
  dedicated optional field read by the listing).
- Amend the alias/park/cap entries in `pi-adapter-runtime` for both fields.
