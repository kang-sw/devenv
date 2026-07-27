---
title: chmod-based unreadable-file tests skip silently on DrvFs and as root, so the
  guard they provide is filesystem-dependent
related:
  260726-refactor-retire-spec-planned-marker-mechanism: its Phase 1 hit this twice
    and removed one instance; the surviving one is pre-existing
  260721-bug-review-partition-empty-artifact: adjacent case of a check that passes
    without the thing it checks for being present
---

# chmod-based tests skip silently where chmod does not restrict

## Topic

`agents-plugin-tool/internal/wsdoc/tickets_graph_test.go`'s
`TestTicketGraphUnreadableTicketDegradesToSilence` creates a file, `chmod 000`s
it, and skips when the read still succeeds. It was introduced by `b81bb3df`.

The skip fires on at least three real configurations:

- a `TMPDIR` on a DrvFs / 9p mount — `/mnt/c/...` on this WSL host, which is an
  ordinary place for a developer to point `TMPDIR`
- running as root, where mode bits do not restrict reads
- Windows

Measured during `260726-refactor-retire-spec-planned-marker-mechanism` Phase 1:
under `TMPDIR=/mnt/c/...` the sibling legacy-marker test using the same pattern
reported `--- SKIP`, and both mutations that killed the behavior it guards
survived with the full suite green. On ext4 the same mutations were caught. The
guard is present or absent depending on where the developer's temp directory
lives, and the suite reports success either way.

## Why it matters

The pattern is worse than an untested branch. An untested branch is visibly
untested; this one reads as covered. A reviewer sees the test, a mutation run on
ext4 confirms it catches, and the conclusion "this behavior is guarded" is
recorded — while on another machine the same commit ships unguarded.

The behavior being guarded makes it sharper. Both instances guard *degradation on
read failure* — precisely the paths that decide whether a tool stays silent or
emits a destructive instruction. Phase 1 of the retirement ticket found a case
where one unreadable ticket silently flipped every advisory to "orphaned; strip
it". A guard against that class must not itself be conditional on the filesystem.

## Direction

The replacement used in `legacy_marker_test.go` is a read failure that does not
depend on permissions: make the scanned root exist but not be a directory, so the
walk fails deterministically on every platform and uid. Two things to settle
before porting it:

- **It does not fit the graph test's premise.** That test needs *one* unreadable
  ticket among readable ones, and a non-directory root destroys the whole scan
  including the subject ticket it verifies. A different deterministic
  single-file failure is needed — candidates worth measuring: a dangling symlink,
  a path whose parent component is a file, or injecting the read failure through
  a seam rather than the filesystem.
- **Whether the seam is the better answer generally.** If `scanTickets` took a
  reader, both tests would express "this file fails to read" directly and no
  filesystem trick would be needed. That is a wider change than the two tests,
  so it wants its own judgment.

Also worth sweeping: how many other tests in this tree use `chmod` to provoke a
failure, and which of them guard a degradation path. Grep for `0o000` / `chmod`
under `agents-plugin-tool/`.

A cheaper partial mitigation, if the full fix is deferred: make the skip loud —
fail rather than skip when the environment cannot produce the condition, so the
gap is visible on the machine where it exists instead of being reported as a
pass.

## Prior art

- `b81bb3df` — introduced the pattern.
- `260726-refactor-retire-spec-planned-marker-mechanism` — its Phase 1 review
  found the second instance, measured the survival, and replaced it; the `###
  Result` records why the graph test was left alone.
