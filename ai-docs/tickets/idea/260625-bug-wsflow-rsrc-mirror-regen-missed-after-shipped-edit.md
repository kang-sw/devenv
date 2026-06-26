---
title: wsflow rsrc mirror regen missed after shipped rsrc edits
---

# wsflow rsrc mirror regen missed after shipped rsrc edits

## Problem

During `260625-feat-lead-tune-schema-backed-knob-catalog` implementation,
regenerating `agents-plugin-wsflow/rsrc/` from canonical `agents-plugin/rsrc/`
pulled in several pre-existing canonical rsrc changes outside the touched
`lead-tune` playbook. That means the generated wsflow rsrc mirror had already
drifted before this feature edit.

The current change regenerates the mirror and makes the worktree consistent
again, but the process gap remains: a shipped rsrc edit can update
`agents-plugin/rsrc/` and `agents-plugin/rsrc/manifest.json` while forgetting
the byte-identical wsflow mirror.

## Possible Follow-Ups

- Extend the existing rsrc generated-artifact guard idea from
  `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit` to require the
  wsflow mirror to be staged and current when `agents-plugin/rsrc/**` changes.
- Add a `ws/git.commit` guard or pre-commit check that runs the mirror drift
  predicate when canonical rsrc files are staged.
- Make shipped-rsrc edit guidance name both required generated artifacts:
  canonical `manifest.json` and `agents-plugin-wsflow/rsrc/`.
