---
title: Route git_worktree.rs's 8 direct git spawns through the git_exec seam so they
  are bounded and counted
sage-review-design: required
related:
  260726-refactor-ws-dashboard-git-fs-watch-invalidation: its Phase 1 built the
    `git_exec::capture` seam and deliberately left these 8 call sites out of scope;
    this ticket is that deferred remainder
  260724-idea-dashboard-daemon-side-git-poll-response-timeout: Phase 1 closed it "for
    the poll path" only; these worktree flows are the remaining unbounded git waits
  260726-refactor-ws-dashboard-long-uptime-leak-hardening: shares the process/handle
    lifetime axis a timeout policy for long-running git commands touches
---

# Route git_worktree.rs's direct git spawns through the git_exec seam

## Background

`260726-refactor-ws-dashboard-git-fs-watch-invalidation` Phase 1 introduced
`git_exec::capture` as the daemon's single git-spawn seam: bounded wait, kill on
timeout, concurrent output draining, expected-vs-unexpected failure logging, and
per-subcommand spawn counters exposed at `GET /api/dashboard/diag/git`.

`ws-dashboard/crates/daemon/src/git_worktree.rs` retains **8 direct
`Command::new("git")` invocations** that do not go through it. The exclusion was
deliberate and is recorded in that ticket's Phase 1 dispositions on two grounds:

- Phase 1's acceptance gate is a *poll-path* spawn rate derived from two diag
  reads on an otherwise idle daemon, which worktree add/remove flows do not
  contribute to.
- Routing them through `capture` would put `git worktree add` — legitimately
  long-running, and the one git command here that can take minutes — under a
  10 s default budget, which is exactly the observable behavior change Phase 1
  was forbidden from making.

So the remainder is a scoped follow-up, not an oversight. The consequence to
close: `totalSpawns` / `bySubcommand` undercount real git activity, and these
call sites keep the unbounded-wait behavior `260724` describes.

The current state is disclosed, not silent: the diag handler carries a comment
naming the exclusion, and the `ws-web-dashboard` spec entry
`{#260726-dashboard-git-invocation-budget-and-spawn-diagnostics}` states that the
counters exclude the worktree flows. Any change here must update both.

## Topics

### The budget is the real design question

This is not a mechanical rewrite. `capture` takes one budget per call, and the
worktree commands do not share a plausible one:

- `worktree list --porcelain`, `worktree prune`, and the preview/status reads are
  fast and belong under the normal budget.
- `worktree add` clones or checks out a working tree. On a large repository or a
  slow disk it can legitimately exceed any poll-path budget, and killing it
  midway leaves a partially created worktree plus a stale
  `.git/worktrees/<name>` administrative entry — a worse state than waiting.
- `worktree remove` deletes files and can block on filesystem or antivirus
  latency on the Windows dogfood host.

Options to weigh: a separate long-operation budget constant; a per-call budget
parameter (`capture` already takes `budget`, so this may be free); an explicitly
unbounded variant for mutation commands that still counts and still logs; or
leaving mutations unbounded and routing only the read commands.

Whichever is chosen, "counted" and "bounded" are separable here — the counters
are the part with no downside, and the budget is the part that needs a decision.

### Whether a killed worktree mutation needs cleanup

If any mutation does get a budget, decide what happens to the half-created
worktree on kill. Phase 1's seam has no notion of compensating action, and adding
one to `capture` would widen a seam whose whole value is being narrow. More
likely this belongs in `git_worktree.rs` around the call, or the mutations stay
unbounded.

### Counter shape once worktree subcommands appear

`bySubcommand` is keyed by an interned `GitSubcommand` enum. Confirm `worktree`
is already a variant (Phase 1's enum included it for
`probe_git_worktree_paths`), and decide whether `worktree add` / `remove` /
`list` need to be distinguishable, since collapsing them hides exactly the
add-vs-list cost difference that motivates this ticket.

## Non-Goals

- Changing what `git worktree add` / `remove` do, or their UX. This is about how
  the daemon invokes and observes them.
- Renaming the diag route's JSON fields. Those are fixed by the spec entry.
- Bringing `codex_app_server.rs` and `claude_cli.rs` under the seam. Those spawn
  `codex`/`claude`, not `git`, and are out of this seam's domain.
