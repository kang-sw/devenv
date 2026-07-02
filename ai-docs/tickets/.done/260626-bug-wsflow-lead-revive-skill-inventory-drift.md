---
title: wsflow lead-revive skill inventory drift
related:
  - 260625-feat-ws-session-state-machine
completed: 2026-06-29
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

## Resolution

Resolved via the second option: `lead-revive` is a deliberate inline-body wsflow
skill, not a thin `playbook.print` shim, because revive bootstraps the workflow
primitives after compaction and cannot depend on the playbook-print path it
restores.

- Code/test half (remote update, commit `74ffb966`): `lead-revive` added to
  `EXPECTED_SKILLS` and to a new `EXPECTED_INLINE_SKILLS = {"lead-revive"}` set
  that exempts it from the thin-shim, shared-stem, and full-counterpart checks.
  `python3 -m unittest discover agents-plugin-wsflow/tests` is green (8 tests).
- Doc half: `ai-docs/ref/wsflow-mirroring.md` now lists `lead-revive` under
  shipped Included skills and documents the inline-body exception under wsflow
  Skill Rules.

The original red-suite claim no longer reproduces; the inventory drift is
encoded as an intentional, documented exception.
