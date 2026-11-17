---
title: "Mailbox in-memory-first store with per-file entries and a periodic GC ring (daemon-internal, patch release)"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch map; not edited from here"
  261117-feat-ws-agent-tools-mailbox-routing: "keeps daemon-owned agent addresses off mailbox files; this ticket covers the remaining external/CLI path"
---

# Mailbox memory-first store

## Background

Independent child of the epic (not stage-bound). Owner: the file-writing mailbox
is a relic of the plugin-hacking era. Today each send rewrites the whole scope
`mailbox.json` and a reply-id registry of roughly 135 KB
(`agents-plugin-tool/internal/wsmailbox/store.go`, `replyid.go`; search the
rewrite paths). The owner's release-gate rule on daemon file touches (see
Constraints) is why this matters: the release is blocked until daemon
file-system load is fixed, per `261117-research-ws-daemon-file-touch-map`.

## Decisions

1. **In-memory first; files only as restore backing (13)=c.** Use per-file
   entries with a periodic GC ring. (14) is superseded.
2. **Not T3; a daemon-internal storage change shipped as a patch (15).** Mailbox
   handlers already run inside the single daemon
   (`agents-plugin-tool/internal/mcp/mailbox_tools.go`; the launcher `proxy_serve`
   only relays; `cmd/ws-mcp/hosted_cli.go`). Old launchers on a different minor
   are already refused on both sides (launcher version check and the daemon's
   `wsdashboard/proxy.go`). v0.50.2 is not blocked.
3. **Remaining risk: daemon downgrade.** An old daemon reads only `mailbox.json`,
   so unread mail held only in the new structure becomes invisible. Judge T1 vs
   T2 in Phase 1 with source citations (candidate mitigations: drain at
   switchover, or also write `mailbox.json`), per
   `ai-docs/manuals/release-compatibility.md`. The choice among candidates is
   made in that judgment, not here.
4. **`ws-mcp serve --stdio` direct-run is declared unsupported (15).** That path
   (`cmd/ws-mcp/main.go`) writes files in-process and the plugin does not use it.

## Constraints

- Go tests run on remote w5090 only, never local `go test`.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Release-gate file-touch rule (owner, 2026-11-17, machine note): minimize file
  touches at the root; files for backup/restore only; deltas (append) not
  rewrites; no heartbeat rewrites; no per-event O(N) rereads; interactive paths
  never wait on file I/O; judged by measured syscalls/s on the owner's PC.
- Do not edit `261117-research-ws-daemon-file-touch-map`.
- Persist only what restore needs; no behavior change to mailbox semantics
  (recv, history, reply ids) visible to callers.

## Compatibility

Patch release, daemon-internal storage change. Not T3. The T1/T2 downgrade
judgment is a required Phase 1 deliverable and cites source lines for which
version writes and reads what (new daemon, old daemon on downgrade), and whether
reopening the new version recovers it, per
`ai-docs/manuals/release-compatibility.md`. Whole-file loss of unknown data is not
T2 without that recovery.

## Phases

### Phase 1: Downgrade judgment and store design

Before implementation, record the T1/T2 judgment for daemon downgrade with
source citations and choose the mitigation; confirm the patch-release verdict
with the cited launcher/daemon version checks; fix the per-file entry layout and
the GC ring parameters as an implementation-time design step and record them. Mark
`serve --stdio` unsupported where it is documented.

Verification: a written verdict with citations in the Phase Result; a measured
baseline (syscalls/s per send on the owner's PC) recorded for comparison.

### Phase 2: Memory-first store implementation

Implement the in-memory-first mailbox with per-file entries as restore backing
and a periodic GC ring, replacing the whole-file rewrite and the full reply-id
registry rewrite on every send; apply the Phase 1 downgrade mitigation.

Verification: Go tests on remote w5090 for send/recv/history/reply-id parity,
restore after daemon restart, GC ring behavior, and the downgrade scenario from
Phase 1; measured syscalls/s on the owner's PC showing the reduction against the
Phase 1 baseline.
