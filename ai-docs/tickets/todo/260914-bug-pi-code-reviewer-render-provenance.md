---
title: "Bug: Pi rejects rendered code-reviewer playbooks for missing trusted provenance"
---

# Bug: Pi rejects rendered code-reviewer playbooks for missing trusted provenance

## Background

On the Pi adapter, `ws/playbook.render(name: "code-reviewer")` rejects the
bundled reviewer prompt with `delegated playbook lacks trusted shipped
provenance`. This blocks the ticket-worker workflow's required independent
review even when the named bundled playbook and its generated findings artifact
are supplied.

## Phases

### Phase 1: Restore bundled reviewer rendering on Pi

Trace the provenance check and make the installed/bundled code-reviewer
playbook renderable without weakening reviewer artifact scope enforcement or
accepting arbitrary prompt paths. Add regression coverage for the successful
render and the preserved rejection boundary.
