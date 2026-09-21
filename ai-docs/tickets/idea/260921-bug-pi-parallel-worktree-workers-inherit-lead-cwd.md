---
title: "Pi parallel worktree workers inherit the lead cwd and can mutate its checkout"
related:
  260920-feat-pi-agent-spawn-cwd-override: provides the missing per-spawn placement primitive
---

# Pi parallel worktree workers inherit the lead cwd and can mutate its checkout

## Background

During an approved `lead-run` parallel batch, each ticket received an acquired worktree and a worker prompt rendered with that worktree as `root_override`. The spawned Pi agents nevertheless inherited the lead process cwd. Before editing, one worker observed that its ws session key was bound to the acquired worktree while native Git commands still ran in the lead checkout; another worker's route renamed the lead checkout's `develop` branch to an implementation branch, removing the local `develop` ref and blocking peer route preflight.

This makes the current parallel route's apparent worktree isolation unsafe until native child cwd placement is explicit. The related `cwd_override` ticket supplies the missing spawn primitive, but the run playbook must also pass the acquired worktree path and fail safely when the runtime lacks that capability.

## Phases

### Phase 1: Bind parallel ticket workers to their acquired worktrees

After `cwd_override` is available, make the parallel run route pass each acquired worktree path to `ws-agent-spawn`. Add a capability-safe stop or serial fallback for runtimes that cannot set the child cwd. Verification must prove that worker-native Git and filesystem operations run inside the acquired worktree and cannot rename or modify the lead checkout's branch.
