---
title: Dashboard git-diff poll takes .git/index.lock (agent lock errors) and activity badge has no timeout (stuck "loading" forever)
related:
  260714-bug-git-status-poll-index-lock-staleness: prior fix hardened the sibling git-status call but wrongly asserted `git diff --numstat` was lock-free plumbing and left it unguarded — this ticket corrects that residual
sage-review-design: completed
sage-review-completeness: completed
---

# Dashboard git-diff poll takes .git/index.lock (agent lock errors) and activity badge has no timeout (stuck "loading" forever)

## Symptom

Observed during Windows dogfooding:

1. Working agents intermittently hit "index.lock exists / another git
   process seems to be running" errors during their own `git` operations, in
   the same worktree the dashboard has open.
2. The dashboard's top-of-UI "agents" badge gets stuck in `agents loading`
   and never resolves (no eventual error or ready state).

## Finding

Full investigation trail, empirical verification methodology (index
inode+hash diffing across a plain repo and a linked worktree, git 2.43.0),
and file:line citations are recorded in the authoritative trace report this
ticket is based on:
`/tmp/claude-1000/-home-swkang-devenv--worktree-ws-dashboard-dev/177bb1e6-4422-43d5-958f-03163e45c70a/scratchpad/git-polling-lock-trace.md`.
Summary of the two distinct, independently-triggerable defects below.

### Defect 1 — unguarded `git diff --numstat` still takes the index lock (direct cause of symptom 2)

The frontend git toolbar polls the daemon every 5s for the selected, visible
WorkRoot (`ws-dashboard/frontend/src/gitToolbar.ts:234`,
`startGitRefreshScheduler`). The server-side handler's `changes_for_path`
(`ws-dashboard/crates/daemon/src/git_toolbar.rs:442`) runs
`git diff --numstat HEAD --` completely unguarded — no
`--no-optional-locks`, no `GIT_OPTIONAL_LOCKS=0` — every single poll tick,
against the exact working tree an agent commits in, with zero serialization
against the agent's own `git add`/`git commit`.

This call was empirically verified (index inode+md5 diffed immediately
before/after, reproduced in both a plain repo and a `git worktree add`
linked worktree) to opportunistically refresh and rewrite the on-disk index,
which requires taking `.git/index.lock`:

| Command | Index rewritten (new inode/hash)? |
|---|---|
| `git diff --numstat HEAD --` (no flag) | **Yes** |
| `git --no-optional-locks diff --numstat HEAD --` | **Yes — flag has no effect on `diff`** |
| `GIT_OPTIONAL_LOCKS=0 git diff --numstat HEAD --` | **Yes — env var also has no effect on `diff`** |
| `git diff-index --numstat HEAD --` (plumbing) | No |
| `git --no-optional-locks diff-index --numstat HEAD --` | No |

Prior ticket `260714-bug-git-status-poll-index-lock-staleness` (commit
`18e97569`) hardened the neighboring `git status --porcelain=v1` call in the
same function (`ws-dashboard/crates/daemon/src/git_toolbar.rs:455-463`) with
`--no-optional-locks`, and its unit test
(`git_toolbar.rs:610-625`,
`changes_for_path_reports_modified_and_untracked_without_index_lock`) only
asserts `.git/index.lock` is absent *after* the call returns — which cannot
detect a lock briefly created/written/renamed *during* a synchronous call.
That ticket's investigation explicitly claimed (and the ticket's own text,
plus `ai-docs/spec/ws-web-dashboard/index.md:697-699`, still assert today):

> "every other git invocation on the poll path (`git diff --numstat`, ...)
> is read-only plumbing that does not touch the index or its lock."

This is empirically false for `git diff --numstat`, per the table above. The
prior fix closed out having patched only half of `changes_for_path`; the
`diff` sub-call has been an unguarded, lock-taking invocation firing every 5
seconds since before — and after — that ticket's close.

Also confirmed: no mutex/serialization anywhere in the daemon coordinates
poll-tick git subprocesses against concurrent agent git operations
(`registry_persist_lock` in `git_worktree.rs:245,560` only protects the
daemon's own JSON registry writes, not git subprocess invocations).

On Windows the base race (self-resolving on Linux/ext4 — loser gets a clean
retry next tick) is plausibly worse because NTFS/AV scanning of the small
`index.lock` file can hold handles longer, and Git for Windows'
`compat/mingw.c` retries certain open/rename/unlink failures with backoff
instead of failing immediately — widening the collision window. This
Windows-specific timing detail is reasoned from known Git-for-Windows
behavior, not reproduced on Windows in the source investigation (see Open
questions).

### Defect 2 — activity badge has no client- or server-side timeout (direct cause of symptom 1)

`fetchWorkRootActivity` (`ws-dashboard/frontend/src/workRootActivity.ts:423-436`)
and its caller, the `useEffect` driving the "agents" badge
(`ws-dashboard/frontend/src/App.tsx:4520-4557`), have no
`AbortController`/`AbortSignal`/client-side timeout of any kind. Confirmed
this is not a one-off: `grep -rn "AbortController\|AbortSignal\|signal:"` across
`ws-dashboard/frontend/src/*.ts*` (excluding tests) returns zero matches
anywhere in the frontend — the same gap exists in `resourceRefresh.ts` (the
"Loading workbench resources" pane) and in `gitToolbar.ts`'s own
git-status/branches fetches.

There is also no server-side request timeout anywhere in
`ws-dashboard/crates/daemon` — no `tower_http` timeout layer, and none of the
`Command::new("git")...output()` call sites in `git_toolbar.rs`,
`git_worktree.rs`, `discovery.rs`, or `work_root_activity.rs` wrap the git
child process in `tokio::time::timeout`/`wait_timeout`/kill logic (only
`terminal.rs`/`servers.rs` have kill/timeout logic, for PTY/SSH processes,
unrelated to git).

The badge's own state projection is correct
(`workRootActivityBadge`, `workRootActivity.ts:977-1058`, a pure
`loading`/`error`/`ready` model that renders the right tone given an
`error` phase) — the gap is entirely upstream: `App.tsx:4520-4557` sets
`{ phase: "loading" }`, then awaits `fetchWorkRootActivity`'s bare
`await fetch(...)`, transitioning to `"ready"` on resolve or `"error"` on
reject. If the daemon accepts the TCP connection but never sends a response
— e.g. its `spawn_blocking` worker is parked inside a wedged `git` child
process — the fetch promise neither resolves nor rejects, and the badge is
stuck at `{ phase: "loading" }` indefinitely with no code path, timer, or
error boundary able to pull it out.

The two defects share a plausible common trigger (heavy git polling against
a busy working tree) — Defect 1's lock contention could plausibly stall a
daemon `spawn_blocking` worker long enough to trip Defect 2 — but each is
independently triggerable and independently fixable; Defect 2 would also
surface from any other sufficiently long daemon stall with Defect 1 fully
fixed.

## Impact

Degrades the Windows dogfooding surface for this dashboard:

- Spurious `index.lock` failures interrupt agents' own git operations in
  their live working tree, on a recurring 5-second cadence for as long as
  that WorkRoot's pane is open.
- The "agents" activity badge can get permanently stuck in `loading`, giving
  no indication of failure and no recovery path short of a page reload.

## Approach (proposed, not yet sage-gated)

### Phase 1 (proposed): Stop `changes_for_path`'s `diff` call from taking the index lock

Replace `git diff --numstat HEAD --`
(`ws-dashboard/crates/daemon/src/git_toolbar.rs:442`) with the plumbing
equivalent `git diff-index --numstat HEAD --`, which was empirically
confirmed lock-free and produces identical parseable
`<added>\t<removed>\t<path>` output for the modified-tracked-file case this
code parses. Do not rely on `--no-optional-locks` for this call — it is
verified ineffective on `diff` (see table above); it may still be added
alongside the plumbing swap as harmless defense-in-depth for git-version
proofing, since it costs nothing where it isn't honored.

Verification should re-confirm parity for rename and mode-change cases (not
just the modified-tracked-file case already spot-checked), extend or reuse
the existing fixture-repo test alongside a mid-call lock-presence check
(the prior ticket's `changes_for_path_reports_modified_and_untracked_without_index_lock`
test only checks lock absence *after* the call returns, which could not have
caught this gap), and confirm no `.git/index.lock` is created while
`git_status`/`changes_for_path` runs concurrently with an external process
holding the lock.

As part of this phase, correct the now-refuted claims left by the prior
ticket:
- `ai-docs/tickets/.done/260714-bug-git-status-poll-index-lock-staleness.md`
  investigation item 2 asserts `git diff --numstat` is lock-free plumbing —
  false, per this ticket's Finding.
- `ai-docs/spec/ws-web-dashboard/index.md:697-699` currently reads as if the
  whole poll is lock-free, when only the `status` sub-call is.

### Phase 2 (proposed): Add a timeout/abort path for the activity badge (and audit the same gap elsewhere)

Add a client-side fetch timeout via `AbortController`/`AbortSignal` to
`fetchWorkRootActivity` (`ws-dashboard/frontend/src/workRootActivity.ts:423-436`)
and its caller in `App.tsx:4520-4557`, transitioning to the existing
`error` badge phase on abort/timeout instead of hanging in `loading`
forever. Audit the identical gap in `gitToolbar.ts`'s git-status/branches
fetches and `resourceRefresh.ts`. Consider pairing with a server-side
request timeout (e.g. a `tower_http` timeout layer, or a bounded
`tokio::time::timeout` around each git `Command` invocation with a clear
failure result) so a wedged git child process degrades to a bounded error
response rather than an unbounded hang on the daemon side too.

Whether Phase 2 needs to precede or can follow Phase 1's landing is open —
Phase 1 removes the daemon as a lock contender and plausibly reduces the
trigger for Phase 2's stalls, but Phase 2 is an independent robustness gap
regardless of Phase 1's outcome, so the two are not strictly ordered
relative to each other.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md`,
`260524-ws-dashboard-git-aware-workroot-toolbar` section (currently at
approximately lines 697-699, which repeats the now-refuted "whole poll is
lock-free" claim).

- Phase 1 is expected to be an internal git-invocation change with
  behavior-preserving output (same `GitChangeSummary` parsing) — likely
  closeout-only: correct the existing sentence to scope the lock-free claim
  to the `status` sub-call only, and note the `diff` sub-call now uses the
  lock-free plumbing form. No caller-visible contract change expected.
- Phase 2 introduces a caller-visible activity-badge state transition (an
  `error`/timeout phase reachable from a stalled `loading` state that was
  previously unreachable) — this likely needs a sentence in the activity
  badge's spec anchor (if one exists distinct from the toolbar section) or a
  new addressed entry; whether the pane/activity anchor needs an addition or
  the git-toolbar section fully covers it was not resolved in this
  idea-stage capture.

This assessment is for the later sage completeness gate to confirm before
promotion, not a settled contract.

## Open questions / unverified

- Whether daemon-side `spawn_blocking` thread-pool exhaustion (from many
  concurrent/retrying git polls under Defect 1) is the actual mechanical
  link from Defect 1's lock contention to Defect 2's stuck badge, versus the
  badge stall being triggered by something else entirely (dropped
  connection, daemon restart mid-request, etc.) — plausible and consistent
  with the symptoms, not proven; no bounded/configured `spawn_blocking` pool
  size or exhaustion metric was found in this codebase.
- Windows-specific retry/backoff timing (Git for Windows' `compat/mingw.c`
  under real AV interference) was reasoned from well-known Git-for-Windows
  behavior, not reproduced on Windows in the source investigation (which ran
  on Linux).
- Whether multiple WorkRoot panes for the same repository/linked worktrees
  can be open simultaneously (multiplying poll contention pressure) was not
  traced end-to-end in the frontend workbench/tab model in the source
  investigation.
