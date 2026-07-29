---
title: Dashboard renders submodule workRoots as an empty projection after wsstate gained submodule support
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# Dashboard renders submodule workRoots as an empty projection after wsstate gained submodule support

## Background

`wsstate.gitIdentity` now resolves a git submodule working tree as an independent
single-worktree project (`45ca3cd6`, `ff19ddb4`, `53ac784e`). ws therefore writes
and reads real cache state under that submodule's own `projectKey`.

The dashboard daemon derives the same keys independently. `git_identity()` in
`ws-dashboard/crates/daemon/src/work_root_activity.rs` (~line 2278) is an explicit
mirror of the Go function — its doc comment says "matching `wsstate.gitIdentity`" —
and it still carries only the `.git`-basename guard on `--git-common-dir`. A
submodule's common dir is `<parent>/.git/modules/<path>`, so the Rust side returns
`None`.

`None` is the same signal the daemon uses for non-Git and bare roots, which the
WorkRoot Activity projection treats as an empty healthy projection. So a submodule
workRoot shows as empty rather than erroring, even while ws is actively writing
agent/session state under its project key. It degrades instead of failing, which is
exactly what makes it easy to miss.

This was found during the wsstate change and deliberately left out of scope there:
that ticket's contract confined the change to `internal/wsstate`, and the dashboard
is a separate crate with its own verification surface.

## Constraints

- The two implementations must agree on key derivation, not merely both "work".
  `ws-web-dashboard`'s coupling note already states this requirement for
  linked-worktree keys; submodules are now a second case under the same rule.
- The Rust side must keep returning `None` for genuinely unsupported layouts
  (bare repos, worktrees created inside a submodule at a path the superproject does
  not track). Widening the guard to accept anything would paper over the divergence
  rather than close it.
- Port the Go fix's shape, not just its outcome: probe
  `--show-superproject-working-tree` only on the guard's failure path. Probing
  first is what caused a real reclassification regression in the Go intermediate
  version, and the Rust mirror would inherit it.

## Prior Art

- `agents-plugin-tool/internal/wsstate/paths.go` `gitIdentity` — the reference
  implementation, including the error-path-only probe and the fail-loud boundary
  comment.
- `agents-plugin-tool/internal/wsstate/paths_test.go`
  `TestResolveTreatsSubmoduleWorkingTreesAsIndependentProjects` and
  `TestResolveRejectsWorktreeCreatedInsideSubmodule` — the git fixtures to mirror.
  Local-path submodule operations need `git -c protocol.file.allow=always`.

## Open Questions

- Is a hand-maintained Rust mirror of `gitIdentity` still the right shape? Two
  independent implementations of one key-derivation contract have now drifted once.
  A shared fixture corpus both sides run against, or having the daemon shell out to
  the ws binary for identity, may be cheaper than a third divergence.
- Should a submodule workRoot be offered in dashboard discovery at all, or only
  render correctly when the user already added it?
