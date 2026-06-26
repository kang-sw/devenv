---
title: wsflow lead-revive skill inventory drift
related:
  - 260625-feat-ws-session-state-machine
---

# wsflow lead-revive skill inventory drift

## Surprise

During the 260625 Phase 2 forge migration audit-fix required wsflow package
check, `python3 -m unittest discover agents-plugin-wsflow/tests` failed before
reaching the forge rsrc changes:

- `agents-plugin-wsflow/skills/lead-revive/` exists.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` does not list
  `lead-revive` in `EXPECTED_SKILLS`.
- `ai-docs/ref/wsflow-mirroring.md` also omits `lead-revive` from the shipped
  wsflow skill inventory.
- There is no `agents-plugin/rsrc/lead-revive/lead-revive.md` shared playbook,
  so simply adding the skill to the expected shim set would expose a second
  mismatch: the wsflow skill currently contains a package-local body instead of
  a thin `wsflow/playbook.print` shim.

## Impact

The wsflow package test suite is red on current `feature/ferrule` for an
inventory drift unrelated to the forge migration diff. This blocks using the
documented wsflow required check as a clean acceptance signal after shared rsrc
edits.

## Follow-up

Decide whether `lead-revive` should be:

- promoted to a shared rsrc playbook with full ws and wsflow thin skill shims,
  then added to the wsflow expected inventory and mirroring reference; or
- treated as a deliberate wsflow-local exception with tests and documentation
  updated to encode that exception.
