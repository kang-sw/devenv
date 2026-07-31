---
title: "playbook.render root_override resolves manifest from repository root"
---

# playbook.render root_override resolves manifest from repository root

## Observation

During Phase 2 of `260730-chore-ws-dashboard-drop-sweep`, a lead session bound
to a linked worktree rendered `plan-populator-survey` with `root_override` set
to that worktree. The call failed with:

```text
rsrc manifest missing at <worktree>/manifest.json
```

`root_override` is documented as selecting delegate include resolution and the
child-key root; it should not make the renderer look for the plugin resource
manifest in the repository worktree.

## Direction

Separate the plugin resource root from the delegate worktree root in
`playbook.render`. Add a regression test that renders a delegate playbook for a
linked worktree and confirms resource lookup still uses the installed plugin
bundle.
