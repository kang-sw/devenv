---
title: "Pi reload can retain generated skill shims for removed playbook names"
---

# Pi reload can retain generated skill shims for removed playbook names

## Background

After synchronizing `track/pi-agent` with the new develop workflow and reloading Pi, the exposed `lead-write-ticket` skill instructed the lead to call `playbook.read(name: "lead-write-ticket")`. The synchronized rsrc inventory contains the replacement `lead-ticket` playbook and no `lead-write-ticket`, so the call failed with `no such rsrc playbook`. Calling `lead-ticket` directly succeeded.

This indicates that the package-local generated skills visible after reload can remain stale relative to the synchronized rsrc/entry workflow. The runtime/rsrc byte-identity tests remained green, so the current guard does not cover this loaded generated-skill seam.

## Phases

### Phase 1: Keep reloaded Pi skill shims aligned with the shipped playbook inventory

Reproduce the stale shim after a workflow rename or removal, identify whether the source is the ignored pack-time `skills/` copy, installed Pi package cache, or reload lifecycle, and make the supported local sync/reload path regenerate or reject stale entry shims. Add a test that fails when a generated Pi skill references a nonexistent playbook name. Preserve the package topology in which generated skills are not committed, and do not restore removed playbook aliases merely to hide stale generated state.
