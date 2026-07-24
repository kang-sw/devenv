---
title: Daemon has no bounded timeout around git Command invocations, so a wedged git child can hang a request unboundedly on the server side
related:
  260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge: parent bug; its Phase 2 bounded only the CLIENT-side fetch (AbortController + ~8000ms timeout) for the activity badge and sibling pollers, and deliberately left this daemon-side bound out of scope
related-mental-model:
  - ws-web-dashboard
---

# Daemon-side response timeout for git poll invocations

## Background

The parent bug `260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge`
Phase 2 added a client-side `AbortController` fetch timeout so a stalled daemon
response transitions the activity badge (and the git-toolbar / resource
pollers) to their error phase and retries, instead of hanging in `loading`
forever. That fix is entirely client-side.

The daemon side remains unbounded. Confirmed during the parent investigation:

- There is no `tower_http` request-timeout layer anywhere in
  `ws-dashboard/crates/daemon`.
- None of the `Command::new("git")...output()` call sites in
  `git_toolbar.rs`, `git_worktree.rs`, `discovery.rs`, or
  `work_root_activity.rs` wrap the git child process in
  `tokio::time::timeout` / `wait_timeout` / kill logic. (Only `terminal.rs`
  and `servers.rs` have kill/timeout logic, for PTY/SSH processes, unrelated
  to git.)

So a wedged or blocked `git` child process (e.g. stuck on a lock, a slow
filesystem, or an unresponsive worktree) can hold a daemon request open
indefinitely. The client now gives up after its timeout, but the daemon
worker stays parked, which can also contribute to `spawn_blocking`
thread-pool pressure under repeated retries.

## Idea

Bound the daemon side too, so a wedged git invocation degrades to a bounded
error response rather than an unbounded hang. Candidate approaches (not yet
decided):

- A `tower_http` timeout layer on the relevant routes, or
- A bounded `tokio::time::timeout` (with child kill on expiry) around each git
  `Command` invocation on the poll path, returning a clear failure result.

Scope, timeout budget, which call sites to cover first (poll-path reads vs.
mutating routes), and whether killed git children need cleanup handling are
all open and belong to a later implementation-ready pass.
