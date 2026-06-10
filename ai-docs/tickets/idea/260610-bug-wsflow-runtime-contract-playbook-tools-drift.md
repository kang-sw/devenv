---
title: wsflow runtime contract omits playbook.print/playbook.render exposed by agentless capabilities
related:
  260609-feat-ws-playbook-surface-mvp: source — M1 added the playbook tools to the agentless capabilities payload
  260605-epic-ws-playbook-factory-pivot: parent direction — playbook surface rollout
---

# wsflow runtime contract omits playbook.print/playbook.render

## Background

`agents-plugin-wsflow/tests/test_wsflow_runtime_contract.py::test_runtime_contract_matches_agentless_capabilities`
fails: the runtime-computed agentless capabilities payload exposes `playbook.print`
and `playbook.render`, but the committed wsflow `runtime.json` `tools` contract does
not list them.

```
AssertionError: Items in the second set but not the first:
'playbook.render'
'playbook.print'
```

This was confirmed **pre-existing** at commit `46e4b42f` (before the M2 Phase 2
internal-procedure migration), so it is an M1 rollout gap, not a Phase 2 regression.
M1 (`260609-feat-ws-playbook-surface-mvp`) added the playbook tools to the shared
capabilities surface without reconciling the wsflow runtime contract.

## Decision needed

Two candidate resolutions; pick one before fixing:

- **Expose in wsflow**: add `playbook.print`/`playbook.render` to the wsflow
  `runtime.json` tools contract if they are intended to be callable in the wsflow
  product mode. (Note: the playbook tools are documented as "Full ws; not
  wsflow-only" — wsflow uses `prompt.render` for its delegate-prompt path — so this
  may be the wrong direction.)
- **Gate out of agentless capabilities**: if the playbook tools are full-ws-only,
  the agentless capabilities payload should not advertise them; gate them out so the
  contract and payload match. This is the more likely-correct direction given the
  full-ws-only design and the existing `prompt.render` vs `playbook.*` split.

## Notes

- Not a blocker for M2 Phase 2: the other 9 wsflow tests (forbidden-reference and
  inventory-drift checks) pass, and Phase 2 does not touch wsflow source,
  `runtime.json`, or tool registration.
- Coordinate with M3 (spawn-runtime reshape) and the wsflow convergence deferral,
  since playbook-surface exposure semantics in wsflow may shift then.
