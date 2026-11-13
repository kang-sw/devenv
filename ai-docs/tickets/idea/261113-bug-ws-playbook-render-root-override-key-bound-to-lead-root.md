---
title: "playbook.render with root_override splices a child key bound to the lead root"
---

# playbook.render with root_override splices a child key bound to the lead root

## Background

Dogfood 2026-11-13: the lead acquired a pooled worktree
(`worktree.acquire`, worker_key `dandruff-eggplant-satiable`, path
`.ws-worktrees/devenv/freebase-prominent-pronto`), then called
`playbook.render(name: ticket-worker, root_override: <that worktree>)` and
passed the rendered prompt to `pi.spawn(cwd: <that worktree>)`. The rendered
body carried a render-minted child key (`babied-alkalize-drizzly`) that the
Pi worker found delegate-scoped at the lead's root
(`D:\Workspace\Repos\devenv`): `git.status` with it reported `develop`, not
the worktree branch. The worker stopped (`stop: b`) before any write. It also
found `workflow_manual` and `route.resolve_implement` absent from a Pi
worker's tool list, while the ticket-worker playbook requires them.

## Idea

- Bind the render-minted key to `root_override` when it is given (the tool
  description says root_override drives child-key binding), or let render
  take the acquired worker_key so the body and the worktree lease agree.
- Decide whether a Pi worker gets the playbook-required tools or the
  playbook has a Pi path without them.

## Open

- Whether the mis-binding is in render or in how the Pi bridge resolves a
  key from cwd.
