---
title: Terminal registry entries have no schema versioning, so adding a field orphans live helpers permanently
related:
  260723-feat-dashboard-detached-terminal-helper: introduced the helper-owned registry and boot_reconcile whose upgrade path this breaks
  260725-research-ws-dashboard-pty-agent-pivot: found this while designing the PTY-agent launch-context injection; carries the Option + serde(default) constraint as a local mitigation
related-mental-model:
  - ws-web-dashboard
---

# Terminal registry entries have no schema versioning

## Background

Found 2026-07-25 during a design review of the PTY-agent notification path.
Not specific to that pivot — this is a property of the shipped `260723`
registry design and affects any future field addition.

`TerminalRegistryEntry` (`ws-dashboard/crates/daemon/src/terminal_registry_file.rs:14-27`)
carries no `version` field and no `#[serde(default)]` on any field.
`scan_registry_dir` (same file, L98-110) reads each `<terminal_id>.json`, and
on a deserialization failure logs `"skipping malformed terminal registry entry"`
at `warn` and continues.

## Failure mode

The registry is helper-owned: create-on-spawn, delete-on-exit, with the daemon
only pruning entries it has positively confirmed dead (file header, L1-6).
Helpers are detached and are designed to outlive the daemon — that is the whole
point of `260723`, and `boot_reconcile` exists precisely to re-adopt them after
a daemon restart.

So a daemon upgrade that adds a non-`Option` field to `TerminalRegistryEntry`
produces this sequence:

1. Helpers spawned by the OLD daemon are still running, each with an on-disk
   entry written in the old shape.
2. The NEW daemon starts and runs `boot_reconcile`
   (`terminal.rs:196`, called from `server.rs:99` before serving).
3. `scan_registry_dir` cannot deserialize those entries and silently skips
   them.
4. Boot reconcile therefore never sees the helpers. Because every drop/kill
   path is driven from a scanned entry, the drop path never runs either.
5. The helper, its PTY, its child shell, and its socket are orphaned
   permanently. Nothing will ever reap them, and the browser has no way to
   surface them because the daemon does not know they exist.

The `warn` log is the only trace, which makes this a silent failure in
practice.

## Why this is worth fixing beyond a coding-convention note

The obvious mitigation — make every new field `Option<T>` + `#[serde(default)]`
— works but is an unenforced convention that has to be remembered at exactly
the moment someone is focused on something else. The PTY-agent pivot is already
the first caller that wants to extend this struct, so the first opportunity to
get it wrong has arrived.

There is also an asymmetry worth deciding on: entries are already written with
deliberate `0600` permissions and an atomic temp-rename
(`terminal_registry_file.rs:48-52`), so the file format was clearly treated as
a durable contract. Versioning is the missing half of that treatment.

## Candidate directions (not decided)

- Add an explicit `version` field and make the reader tolerate older versions
  rather than skipping them.
- Keep the struct unversioned but enforce `Option` + `#[serde(default)]`,
  and add a test that deserializes a minimal historical fixture so the
  invariant fails loudly at CI time rather than silently at runtime.
- Make a deserialization failure something the daemon surfaces (quarantine the
  entry, keep the pid visible as an unadoptable row) instead of dropping it, so
  an orphan is at least observable.

## Relations

Tickets: `260723-feat-dashboard-detached-terminal-helper`,
`260725-research-ws-dashboard-pty-agent-pivot`.

Spec anchors: `#260516-ws-web-dashboard-workroot-io-restore-model`,
`#260516-ws-web-dashboard-terminal-registry-pty-spawn`.
