---
title: "Consolidate dashboard-managed workroot files under .ws-dashboard/, shared across worktrees"
parent: 260525-feat-ws-dashboard-workroot-polishing-backlog
related:
  260711-idea-dashboard-command-bus-quick-open-shortcuts: first concrete consumer — custom command definitions are proposed to live at <root>/.ws-dashboard/scripts/
  260711-idea-dashboard-agent-facing-mcp-control-surface: a future consumer if worktree-management or custom-command MCP calls need workroot-scoped config
---

# Consolidate dashboard-managed workroot files under .ws-dashboard/, shared across worktrees

## Background

Owner direction (2026-07-11): dashboard-managed, workroot-scoped files
(starting with custom command definitions from
`260711-idea-dashboard-command-bus-quick-open-shortcuts`) should live
under a single `<workroot>/.ws-dashboard/` directory in the repo, rather
than each feature inventing its own location. Two concrete asks:

1. Files under `.ws-dashboard/` should be conditionally git-tracked: e.g.
   a filename pattern like `*.local.*` marks a file as local-only
   (`.gitignore`d), everything else tracks normally — mirroring the
   `.env`/`.env.local` convention, so shareable dashboard config (like
   team-wide custom commands) can be committed while personal overrides
   stay local.
2. `.ws-dashboard/` should be **shared across all worktrees of the same
   repo**, rooted at the root workroot (the original clone), not
   duplicated per worktree — so a custom command or config defined once
   is visible from every worktree checked out from that repo.

Today, confirmed by investigation: `.ws-dashboard/` does not exist
anywhere in the codebase — this is a new concept. All current
dashboard-managed state (`DashboardStateStore` /
`persistent_state.rs:485-503`: `OpenedWorkRoots` registry, root-picker
pins, linked servers) lives in a single global JSON file in the user's
XDG state dir / `%LOCALAPPDATA%`, not inside any workroot — so there is
no existing in-repo precedent to extend, only a global-state precedent
to diverge from for anything meant to be workroot-scoped and shareable.

## Findings relevant to the cross-worktree sharing mechanism

- `resolve_workspace_git` (`git_worktree.rs:429-474`) already resolves
  `git rev-parse --path-format=absolute --git-common-dir`, which — called
  from *any* worktree — always points at the original repo's `.git`. Its
  parent directory is therefore a ready-made "root workroot" anchor;
  the daemon does not need new bookkeeping to find where the root clone
  lives.
- Default worktree placement today is `<root>/.git/ws-worktree/<name>`
  (`git_worktree.rs:338-341,473`).
- git worktrees do not require touching `.git` itself to add a shared
  directory (each worktree's `.git` is already just a gitdir-pointer
  file) — a new `.ws-dashboard` entry is orthogonal to that structure, so
  no conflict there.
- **Windows**: true symlinks need admin rights or Developer Mode
  (`SeCreateSymbolicLinkPrivilege`). Directory **junctions** (`mklink /J`)
  need no elevated privilege but only work within the same volume — a
  worktree on a different drive/volume than the root clone would break a
  junction-based approach. This needs an explicit fallback decision (skip
  linking cross-volume? fall back to a real symlink attempt with a clear
  error? copy instead of link?).
- Ordering matters: `git worktree add` can fail if the target directory
  already has unexpected entries, so the link should be created *after*
  worktree creation succeeds, not before.
- Cleanup: `git worktree remove` will delete the worktree directory
  including any symlink/junction entry inside it (that's fine, the link
  is disposable), but the daemon is responsible for not leaving orphaned
  link state anywhere else and for handling worktrees that were removed
  outside the dashboard (see the sibling worktree-deletion gap in
  `260525-feat-ws-dashboard-workroot-polishing-backlog`, Phase 1).
- No `GitWorktreeBlockerCode` variant currently exists for a symlink/
  junction creation failure — the blocker-code schema would need a new
  variant if this is surfaced as a user-facing error during worktree
  creation.
- **Flagged 2026-07-11, resolved 2026-07-11** (see Decisions below): today's
  default worktree placement, `<root>/.git/ws-worktree/<name>`, puts real
  working-tree files inside `.git/`. Many tools treat `.git/` as opaque
  git-internal-only storage and skip or exclude it by default — file
  search/indexing (ripgrep/fd/IDE watchers), backup/sync tooling,
  antivirus/DLP scanners, and "reset/clean git internals" scripts that
  assume anything under `.git/` is disposable. This is a real design risk
  independent of the `.ws-dashboard/` sharing question, but the owner
  chose to leave the bare-repo edge case (`common_dir.parent()` not being
  a real working directory for a bare root) intentionally out of scope as
  too uncommon a setup to design around right now.

## Decisions

- **Narrowed symlink/junction scope** (owner, 2026-07-11): not everything
  under `.ws-dashboard/` needs a physical link into each worktree.
  Anything the dashboard daemon itself reads (e.g.
  `.ws-dashboard/scripts/` for custom commands) can be resolved
  pragmatically at read time via the `common_dir`-anchored root path —
  the daemon always knows how to find the root workroot from any
  worktree, so it never needs the file to physically exist inside the
  worktree's own directory tree. A physical symlink/junction is only
  needed for the subset of content that something *other than the
  daemon* must see at a normal relative path inside the worktree itself
  — e.g. a human browsing the worktree in a plain file explorer/editor,
  or a non-dashboard-aware script/tool running with that worktree as its
  cwd.
- **Collapsed top-level name for the physically-linked subset** (owner,
  2026-07-11): the physically-linked content is named `.ws-dashboard-shared`
  at the worktree root, a sibling of `.ws-dashboard/`, rather than nesting it
  as `.ws-dashboard/shared/`. Reasoning: `.ws-dashboard/` itself never
  needs to physically exist inside a non-root worktree (everything under it
  is daemon-resolved via `common_dir`), so nesting the one physically-linked
  path under it would force the daemon to also create an otherwise-pointless
  empty `.ws-dashboard/` container directory in every worktree just to hold
  the `shared/` link target — pure overhead with no content behind it. A
  flat `.ws-dashboard-shared` entry avoids that: it is the only
  dashboard-managed entry that physically exists per-worktree, and it exists
  standalone. Only `.ws-dashboard-shared` needs the fragile Windows
  symlink-vs-junction-vs-cross-volume handling from the Findings above;
  `scripts/` and similar daemon-only-read data under `.ws-dashboard/` need
  zero link machinery at all.
- Room should be left for a third category later: per-worktree,
  intentionally *not* shared dashboard metadata (name TBD, e.g.
  `.ws-dashboard/worktree-local/` or similar) — distinct from both the
  daemon-resolved shared data and the physically-linked `.ws-dashboard-shared`
  data. Since this category (like `scripts/`) is daemon-read only, it stays
  nested under `.ws-dashboard/` without needing the flat-naming treatment
  above.
- **Default worktree placement moves under `.ws-dashboard/`** (owner,
  2026-07-11): long-term direction is to move default worktree placement
  from `<root>/.git/ws-worktree/<name>` to `<root>/.ws-dashboard/worktrees/<name>`,
  resolving the `.git/`-placement tooling risk flagged in Findings above.
  This keeps worktrees out of `.git/` entirely while staying inside a
  dashboard-owned, daemon-resolved namespace (worktrees themselves are not
  something a non-dashboard-aware tool needs to find at a fixed relative
  path — `git worktree list` is the actual discovery mechanism — so this
  location needs no physical link either, consistent with the `scripts/`
  pattern). Sequencing: land as part of
  `260525-feat-ws-dashboard-workroot-polishing-backlog` Phase 1's real
  delete-operation work, not as a standalone migration — owner judged
  migration risk low since no worktree-path assumption has ever shipped.
- **Link creation timing** (owner, 2026-07-11): the `.ws-dashboard-shared`
  link is created at two points — (1) dashboard-driven worktree creation
  time, as part of the create flow, and (2) worktree *first-access* time
  from the dashboard frontend (i.e. the first time the dashboard opens/
  attaches to an existing worktree that lacks the link, e.g. one created
  outside the dashboard via a plain `git worktree add`), so pre-existing
  worktrees get backfilled lazily rather than requiring a proactive scan.
- **Linking-failure policy** (owner, 2026-07-11): best-effort, silent skip
  on failure — if the link cannot be created, that worktree simply does
  not get `.ws-dashboard-shared` (no blocking error, no copy-then-diverge
  fallback). Cross-volume placement, the main scenario that would break a
  Windows junction, is not considered in-scope in the first place: default
  worktree placement now lands under `<root>/.ws-dashboard/worktrees/`
  (same-volume, same-repo-root by construction per the placement decision
  above), so a worktree ending up on a different volume than its root
  clone is an unsupported configuration, not a case this mechanism needs
  to handle gracefully.

## Open Questions (owner flagged this needs more UX/policy discussion — not decided yet)

- What, concretely, needs to live in `.ws-dashboard-shared` rather than
  being daemon-resolved? No confirmed consumer needs physical linking
  yet — `scripts/` (the only concrete consumer so far) turned out not to
  need it. This directory may stay empty/reserved until a real
  use case appears.

## Non-Goals

- Deciding the internal layout of `.ws-dashboard/scripts/` itself — owned
  by `260711-idea-dashboard-command-bus-quick-open-shortcuts`.
- Migrating the existing global `DashboardStateStore` (opened-workroots
  registry, pins, linked servers) into `.ws-dashboard/` — that state is
  intentionally global/cross-repo today; this ticket is about new
  workroot-scoped, shareable data only, not a wholesale storage migration.
