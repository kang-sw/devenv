---
title: Dashboard git-status poll takes the real index lock every 5s, risking stale .git/index.lock under cross-mount access
spec:
  - 260524-ws-dashboard-git-aware-workroot-toolbar
related:
  260711-idea-dashboard-git-status-polling-index-lock-contention: earlier ticket recording the same near-term decision (owner, 2026-07-11) this ticket implements, plus the accepted-but-unscheduled long-term notify-crate watch direction (see that ticket's Decisions) which is out of scope here
sage-review-design: completed
sage-review-completeness: completed
---

# Dashboard git-status poll takes the real index lock every 5s, risking stale .git/index.lock under cross-mount access

## Background

Dogfooding report (translated from Korean): "On the Windows side, git keeps
leaving a lockfile (something like `git index.lock`) behind — I think it's
from some git-status polling — and it's making the repo hard to actually use
(git commands start failing/hanging because a stale lock is present)."

Environment fact the report didn't spell out: this repo's actual runtime is
WSL2, not native Windows. "Windows side" most likely means a Windows-native
tool (Windows git client, or an editor/IDE talking to the same working tree
over the `\\wsl$` / drvfs interop mount) touching the repo concurrently with
the ws-dashboard daemon, which runs on the WSL/Linux side and polls Git state
for the dashboard UI.

### Investigation findings

1. **Confirmed poller and interval.** The dashboard frontend polls the
   selected, visible WorkRoot's Git status every 5 seconds:
   `ws-dashboard/frontend/src/gitToolbar.ts:164` —
   `startGitRefreshScheduler(refresh, { intervalMs = 5000, ... })`, wired via
   `window.setInterval` from `ws-dashboard/frontend/src/App.tsx:4193-4203`.
   This hits `GET /api/dashboard/work-roots/{work_root_id}/git/status`
   (`ws-dashboard/crates/daemon/src/router.rs:138-140`), which is documented
   in `ai-docs/spec/ws-web-dashboard/index.md` under
   `260524-ws-dashboard-git-aware-workroot-toolbar` as "polls conservatively
   only for the selected visible WorkRoot."

2. **The poll takes the real index lock.** The `git_status` handler
   (`ws-dashboard/crates/daemon/src/git_toolbar.rs:134-148`) calls
   `status_for_path`, which calls `changes_for_path`
   (`ws-dashboard/crates/daemon/src/git_toolbar.rs:440-474`). That function
   runs `git status --porcelain=v1 --untracked-files=all` via `git_text`
   (`git_toolbar.rs:536-547`, `Command::new("git")...output()`), with **no**
   `--no-optional-locks` flag and no `GIT_OPTIONAL_LOCKS=0` env override. By
   default `git status` opportunistically refreshes and rewrites the on-disk
   index stat cache, which requires taking `.git/index.lock`
   (create-lock → write → rename/unlink). This is the only `git status`
   call-site in the daemon; every other git invocation on the poll path
   (`git diff --numstat`, `git rev-list --left-right --count`,
   `git branch --show-current`, `git rev-parse`, `git for-each-ref`) is
   read-only plumbing that does not touch the index or its lock.

3. **No daemon-side kill/timeout wraps any git invocation.** Grepped every
   `Command::new("git")` call-site across `ws-dashboard/crates/daemon/src`
   (`git_toolbar.rs`, `git_worktree.rs`, `discovery.rs`, `work_root_activity.rs`)
   — all are plain synchronous `.output()` calls with no
   `tokio::time::timeout`, `wait_timeout`, `select!`, or `.kill()` nearby.
   The only `.kill()`/`select!`/`timeout` usages in the daemon wrap PTY
   (`terminal.rs`) and linked-server SSH child processes (`servers.rs`), not
   git. **The daemon itself never forcibly aborts a git subprocess.**

### Root-cause hypothesis (not confirmed by live reproduction)

Given (1) and (2), the daemon polls a real, lock-taking `git status` every 5
seconds for as long as a WorkRoot pane is open and visible. Ordinary lock
contention between two processes calling `git status` around the same time
normally self-resolves (the loser gets "Unable to create '.git/index.lock':
File exists.", the next 5s poll retries cleanly) — that alone would not
explain a lock that stays stuck.

Because the actual runtime is WSL2 and the report names "the Windows side,"
the most likely second actor is a Windows-native process (a Windows git
client, or an editor/IDE's git integration) reaching the same working tree
either through the WSL↔Windows interop mount (`\\wsl$\...` from Windows,
9p protocol) or through a Windows-side clone under `/mnt/c/...` accessed by
both a Windows tool and this WSL daemon (drvfs). Git's lockfile protocol
relies on POSIX-atomic `O_CREAT|O_EXCL` create, followed by `rename()`/
`unlink()` to finalize or clean up. 9p and drvfs have a documented history of
incomplete or non-atomic emulation of these primitives across the VM
boundary. A create/rename/unlink sequence that is atomic on native ext4 but
not atomic (or not visible promptly to the other side) across that mount
boundary is a plausible mechanism for a lock surviving after its writer
process has already exited — turning ordinary contention (self-resolving)
into staleness (not self-resolving) — but this is not confirmed by live
reproduction; item (3) rules out the daemon's own subprocess handling as the
direct cause. If the frontend has more than one visible WorkRoot pane open
onto the same repository or its linked worktrees, or the user's own Windows
tool independently polls git status on a similar cadence, the contention
window recurs roughly every few seconds indefinitely, which is consistent
with the report describing an ongoing, hard-to-use state rather than a single
one-off collision.

## Phases

### Phase 1: Stop the poll from taking the index lock

Change the daemon's `changes_for_path` `git status` invocation
(`ws-dashboard/crates/daemon/src/git_toolbar.rs:455`) to pass
`--no-optional-locks` (preferred: as a leading git option,
`git --no-optional-locks status --porcelain=v1 --untracked-files=all`, since
`--no-optional-locks` is a top-level git option, not a `status` subcommand
flag). This makes the 5-second poll a pure read that never attempts to
create `.git/index.lock`, removing the dashboard daemon itself as a
contender in any lock race — regardless of whether the cross-mount hypothesis
above is the exact mechanism. `git_text`'s other plumbing calls already avoid
the lock and need no change.

Verification: with `--no-optional-locks`, `git status --porcelain=v1
--untracked-files=all` output is unchanged for the case this daemon parses
(untracked/modified path listing); only the opportunistic index-refresh
side-effect is skipped. Confirm via a daemon test that `changes_for_path`'s
parsed `GitChangeSummary` is unaffected on a fixture repo, and (manually or
via a test harness) confirm no `.git/index.lock` is created while
`git_status` runs concurrently with an external `git status` holding the
lock — the daemon's call should return promptly rather than blocking or
erroring due to lock contention it no longer participates in.

Deferred to a follow-up ticket if still needed after Phase 1 lands: defensive
stale-lock detection/cleanup (e.g., age-based `.git/index.lock` reap before a
daemon-initiated *mutating* git command such as `switch`/`push`/`pull` that
still legitimately needs the lock) is not part of this phase — Phase 1 only
removes the daemon's own polling contribution to lock contention. If reports
of stale locks persist after this ships, that indicates the second actor
(external tool) is leaving the lock stale on its own, which is a distinct,
separately scoped investigation (not fixable from this daemon's side beyond
"don't add to the contention").

Also out of scope here, already recorded as the accepted target architecture
in `260711-idea-dashboard-git-status-polling-index-lock-contention` (owner,
2026-07-11) but not yet scheduled: replacing fixed-interval polling with a
`notify`-crate-based watch on `.git/index`/`.git/HEAD`/`.git/refs/**`,
refreshing only on change. That direction does not supersede Phase 1 here —
a watch-triggered refresh would still want `--no-optional-locks` on its own
status call, and Windows `ReadDirectoryChangesW` watchers can surface
spurious events during git's own lock-rename sequence, requiring
debouncing regardless.

## Constraints

- Do not change the `WorkRootGitStatus` JSON response shape or the polling
  cadence/visibility gating described in
  `ai-docs/spec/ws-web-dashboard/index.md`'s
  `260524-ws-dashboard-git-aware-workroot-toolbar` section — this is an
  internal git-invocation flag change only, not a caller-visible contract
  change.
- Do not add `--no-optional-locks` to the explicit user-triggered mutating
  routes (`git switch`, `git switch -c`, `git fetch`, `git push`,
  `git pull --ff-only`) — those legitimately need the index/ref locks and are
  not on a recurring poll path.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md`,
`260524-ws-dashboard-git-aware-workroot-toolbar`. The existing text already
documents "polls conservatively only for the selected visible WorkRoot";
Phase 1 adds one clarifying sentence noting the status poll reads without
taking the repository's index lock, as a closeout doc update alongside the
code change. Contract-first spec: no — this is a bug-fix behavioral
clarification of already-documented polling behavior, not a new externally
consumed contract; the HTTP response shape and polling cadence are unchanged.
