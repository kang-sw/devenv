---
title: opened-workroots.json persists mixed path separators, so one root can hash to two WorkRootIds
related:
  260726-refactor-ws-dashboard-git-fs-watch-invalidation: works around this with a
    separate `WatchKey` and lists the unification as a Non-Goal; this ticket is that
    deferred cleanup
  260726-idea-dashboard-moved-workroot-red-with-no-recovery-affordance: the
    duplicate-id case is most visible for a root that is `Moved` or missing, which is
    the state that ticket is about
---

# opened-workroots.json persists mixed path separators, so one root can hash to two WorkRootIds

## Background

Observed while measuring git-spawn load on the dogfood daemon (2026-07-26). The
persisted registry stores paths with inconsistent separators inside a single
entry, e.g.

```text
"D:/Workspace/Repos/InspectTGV_AIDriven/.git\\ws-worktree\\jpeg"
```

— forward slashes for the drive-relative part and backslashes for the segments
appended by worktree creation.

`normalize_registered_root` only strips the `\\?\` verbatim prefix, so it does not
canonicalize separators. `WorkRootId` is derived from the normalized string, which
means the same filesystem location written two ways produces **two different
ids**. The frontend keys nav selection, workbench panes, and browser-local
`workNavOrder.ts` persistence on that id.

This is latent rather than actively broken: in practice each root is written once
and read back in the same spelling. It surfaces when a path arrives from a second
source with different separators — a re-registration, a hand-edited registry
file, or an API caller passing a normalized path.

## Why it was deferred

`260726-refactor-ws-dashboard-git-fs-watch-invalidation` needed a stable key for
its caches and watcher. Rather than change `local_work_root_id_for_path` — which
would churn ids the frontend persists against, invalidating saved nav order and
pane layout for every user — that ticket introduced a separate `WatchKey`
(canonicalize → normalize → `\` ⇒ `/` → lowercase on Windows) for internal use
only, and recorded the persisted-spelling unification as a Non-Goal.

So the watcher work is unblocked. What remains is the underlying inconsistency.

## Topics

### What the fix has to preserve

Changing the id derivation is a migration, not a patch. Any fix must decide:

- Whether existing `workNavOrder.ts` / pane state survives the id change, or
  whether a one-time reset is acceptable and how it is communicated.
- Whether the migration rewrites `opened-workroots.json` in place on load
  (canonicalizing separators once) or whether the derivation is made
  separator-insensitive and the file left alone.
- Whether two entries that turn out to be the same root after canonicalization are
  merged, and which one's metadata wins.

The in-place rewrite is the cheaper direction: it fixes the data rather than
teaching every reader to tolerate both spellings, and it makes `WatchKey` and
`WorkRootId` converge naturally instead of permanently maintaining two keys.

### Where the separators come from

Worth confirming before designing the fix: the mixed form suggests the drive-relative
part comes from one code path (normalized to `/`) and the worktree segments from
another (native `\`). Identifying the write site may make this a one-line fix at the
source plus a migration for already-persisted entries, rather than a change to id
derivation at all.

### Whether `WatchKey` can then be retired

If the persisted spellings are canonicalized and `WorkRootId` becomes stable under
both forms, the separate `WatchKey` introduced by the watcher ticket becomes
redundant. Retiring it is the cleanup that makes this ticket worth doing rather
than just documenting.

## Non-Goals

- Blocking the FS-watch work. That ticket has a working answer and this one does
  not gate it.
- Changing `WorkRootId` derivation without a migration path for persisted frontend
  state.
