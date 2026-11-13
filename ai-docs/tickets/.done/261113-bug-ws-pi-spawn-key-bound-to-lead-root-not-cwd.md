---
title: "Pi spawn session key is bound to the lead root, not its worktree cwd"
completed: 2026-11-13
---

# Pi spawn session key is bound to the lead root, not its worktree cwd

## Background

Peer InspectTGV lead report, 2026-11-13 (wsflow 0.49.0): rendering
`ticket-worker-elevated` with a pooled worktree as `root_override` supplied a
child key bound to that worktree: `git.status` showed its implementation
branch and dirty files. Spawning Pi with the same worktree as `cwd` instead
forced a harness key bound to the lead root; `git.status` showed the root's
branch and dirty files. The worker could not use the rendered key because its
harness requires its own key for every ws call. This makes ws git/ticket tools
unusable for pooled-worktree Pi workers and risks committing to the root;
native git in the worker's cwd was the workaround.

## Idea

Bind the Pi key to the worktree when `pi.spawn` uses a cwd inside it, or let
the harness honor the rendered prompt's key.

## Open

- What compatibility tier applies at promotion?


## Resolution (2026-11-13)

Fixed by merge dbb98b78b (ea625b763): pi.spawn resolves cwd's Git worktree and binds the child key there; nested spawns follow the same rule. Compatibility tier T0. Not yet checked live on Windows (needs the next daemon rerun).
