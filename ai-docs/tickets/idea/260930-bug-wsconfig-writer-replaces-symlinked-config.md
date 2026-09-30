---
title: wsconfig shared writer replaces a symlinked config file with a regular file
related:
  260929-bug-config-tune-agents-tier-lost-update: origin (introduced the shared writer for agents.tier)
---

# wsconfig shared writer replaces a symlinked config file with a regular file

## Background

`updateConfigFile` (`agents-plugin-tool/internal/wsconfig/config.go`) is the
single writer for project and global config files since b66cb36cd. It writes
through `os.CreateTemp` in the config file's parent directory followed by
`os.Rename` onto the config path. Two edge cases follow:

- **Symlinked config is detached.** When the config path is a symlink (for
  example a dotfiles-managed `~/.ws/config.json`), the rename replaces the
  link with a regular file. The link target is left stale and later edits to
  it no longer reach ws. The `os.Stat` mode lookup follows the link, so only
  the target's permission bits carry over.
- **Non-writable parent directory fails.** A writable config file inside a
  non-writable directory now fails at `CreateTemp`, where a plain in-place
  write would have succeeded.

Resolver override writes already used temp-write + rename before b66cb36cd;
`agents.tier` set/unset writes (via `config.tune`) are newly exposed. Neither
behavior is recorded in the `260929-bug-config-tune-agents-tier-lost-update`
Result.

Found by the release-gate review of `2295a7bae..develop` (finding 2, Minor);
the user chose to capture it as an idea rather than block the release.

## Phases

### Phase 1: Decide and fix or record

Nothing is decided yet. Candidate directions surfaced by the review, all
unconfirmed:

- Resolve the config path with `filepath.EvalSymlinks` before the temp-write
  and rename, so the rename lands on the link target. Open interaction: the
  sibling `<path>.lock` must stay the same lock path for every writer of one
  file, so whether the lock is taken on the link path or the resolved path
  needs a decision.
- Accept the current behavior and record it as a known limitation.

The non-writable-directory case needs its own decision (it is inherent to
atomic rename).
