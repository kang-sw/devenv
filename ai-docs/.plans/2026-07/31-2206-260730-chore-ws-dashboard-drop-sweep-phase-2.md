# Plan: ws-dashboard drop sweep — archive tag, git-surface teardown, doc and board removal — Phase 2: Tear down the dashboard git surface and code tree

## Relevant Ticket Contract
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L46-L50` — do not close PRs or delete refs/files until `origin` advertises the annotated `archive/ws-dashboard` tag.
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L78-L97` — close PRs before deleting their branches; every remote deletion needs a fresh path-based scan and the Phase 1 exact-SHA lease. A changed target or a new non-exempt candidate stops the sweep; never move the archive tag.
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L201-L220` — close PRs #8 then #7, remove the six remote and five local branches named there, remove the 110 tracked `ws-dashboard/` files, then remove any ignored build bulk from disk.
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L222-L232` — success is a property scan of `refs/heads` and `refs/remotes`, not merely deleting the named branches; `origin/discuss` is the sole permitted remaining candidate and `refs/tags` must be excluded because the archive tag intentionally retains the history.

## Out of Scope
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L234-L248` — Phase 3 documentation, spec, mental-model, README, index, and ticket-board changes.
- Creating a replacement dashboard, merging any dashboard line into `main`, changing `archive/ws-dashboard`, or altering the ws/wsflow plugin and release behavior.

## Codebase Findings
- `ai-docs/ref/verify-dashboard-archive-recovery.sh#L5-L10` — the archive verifier pins the annotated tag, archived source ref, and captured ordered parent SHAs; it is the required pre-deletion recovery check.
- `ai-docs/ref/verify-dashboard-archive-recovery.sh#L21-L32` — the verifier checks the archive object type, ordered parents, captured-main tree equality, and both remote tag-object and peeled-commit advertisements.
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L106-L128` — path-based prior art identifies the misleadingly named/unrelated `dashboard` local branch, the non-name-matching `impl/nav-row-two-line-open-state-phase1` line, and the retained `origin/discuss` exception.
- `ws-dashboard/README.md#L3-L16` — the deleted tree is self-contained as the dashboard product surface, with Rust core/daemon/harness crates and a frontend; there is no reusable component to extract in this phase because the authority calls for the whole tree to be removed.
- `.github/workflows/ws-mcp-release.yml#L3-L9` — release tags match `v*`; the archive tag and dashboard-tree removal require no release-workflow edit.
- `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L218-L220` — the tracked tree has 110 files and the historical ignored-bulk estimate is 9.2G. In this worktree neither `ws-dashboard/target` nor `ws-dashboard/frontend/node_modules` is present, so deletion must be conditional rather than assume those paths exist.

## Implementation Plan
1. In the repository root, run `sh ai-docs/ref/verify-dashboard-archive-recovery.sh`, then `git fetch origin --prune` and confirm `git ls-remote origin refs/tags/archive/ws-dashboard` still returns the published tag. Stop before all destructive work if either check fails.
2. Use the complete Phase 1 remote-deletion SHA snapshot below, then compare it to a fresh path-based scan of `refs/heads` plus `refs/remotes` using `git rev-list --count main..<ref> -- ws-dashboard/`. Retain `origin/discuss` as the only exception. The verifier preserves only four archive-parent SHAs, so this plan records the remaining deletion leases explicitly; if any differs, stop and obtain an explicit new safety decision rather than silently treating a later fetch as Phase 1.

   | Remote branch | Phase 1 SHA lease |
   | --- | --- |
   | `impl/helper-liveness-probe` | `5d5f6ade126c880b9aeae667a4314259e4892770` |
   | `impl/nav-row-two-line-open-state-phase1` | `6384bf8af3c8b18d49620612ce648bb686098e6a` |
   | `goal/ws-dashboard-dev/copper-heron-vale` | `57a58ce7481ab5f4a54b8935721be0bd38e71436` |
   | `goal/ws-dashboard-dev/velvet-arbor-quill` | `8c7fbc0f9a992f76f433e320b6678123a56d364d` |
   | `ws-dashboard-dev` | `1b41a37b4a9bc2f106de447de76939fd612898b6` |
   | `implement/dashboard-server-scoped-forwarding-phase-7` | `7f2c8c58037633eb13124075b7cc76026dd666df` |
3. Close GitHub PR #8 (`impl/helper-liveness-probe` to `goal/ws-dashboard-dev/copper-heron-vale`) before PR #7 (`goal/ws-dashboard-dev/copper-heron-vale` to `ws-dashboard-dev`) via `gh pr close`, leaving a comment on each with `archive/ws-dashboard`, `git fetch --tags && git checkout -b revive archive/ws-dashboard`, and `260730-chore-ws-dashboard-drop-sweep`. Verify both PR states are `CLOSED` and that neither targets `main`.
4. Immediately before each remote branch deletion, fetch, rerun the path-based candidate scan, and compare the branch's current SHA to its Phase 1 lease. Only on an exact match delete `origin/impl/helper-liveness-probe`, `origin/impl/nav-row-two-line-open-state-phase1`, `origin/goal/ws-dashboard-dev/copper-heron-vale`, `origin/goal/ws-dashboard-dev/velvet-arbor-quill`, `origin/ws-dashboard-dev`, and `origin/implement/dashboard-server-scoped-forwarding-phase-7`; stop on moved/new candidates rather than force-updating any ref or tag.
5. After remote deletion succeeds, delete the five local branches listed in `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md#L213-L217`, checking `git worktree list` first so none is checked out. Then run `git rm -r ws-dashboard` to stage only the 110 tracked dashboard files, and conditionally remove `ws-dashboard/target` and `ws-dashboard/frontend/node_modules` if they exist.
6. Before committing the implementation, rerun the branch-property scan over `refs/heads` and `refs/remotes` only: every ref must report zero dashboard-path commits beyond `main` except `origin/discuss`. Confirm `ws-dashboard/` is absent, `git diff --cached --name-only` contains only the dashboard tree removal, and the PR checks remain closed. Do not perform Phase 3 document/board cleanup here.

## Verification Plan
- Run `sh ai-docs/ref/verify-dashboard-archive-recovery.sh` before the first deletion; it must pass all archive and recovered-evidence invariants.
- Query PRs #8 and #7 with `gh pr view <number> --json state,headRefName,baseRefName` and require `CLOSED` with the expected heads/bases.
- After each remote mutation, fetch and run the ticket-prescribed `git for-each-ref`/`git rev-list --count main..<ref> -- ws-dashboard/` property scan over heads and remotes, treating changed SHA or a new non-`origin/discuss` candidate as a stop condition.
- Confirm `git ls-files ws-dashboard` is empty after `git rm`, the working tree has no `ws-dashboard/` directory after ignored-bulk cleanup, and `.github/workflows/ws-mcp-release.yml` needs no change because it triggers only `v*` tags.

## Escalations
- The ticket-driven Result checkpoint is authorized: read and append the
  Phase 2 `### Result` in
  `ai-docs/tickets/ready/260730-chore-ws-dashboard-drop-sweep.md` after the
  source checkpoint. This is Phase 2 closeout documentation, not Phase 3
  board or documentation-surface cleanup.
