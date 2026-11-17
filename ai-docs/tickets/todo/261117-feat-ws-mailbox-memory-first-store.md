---
title: "Mailbox in-memory-first store with per-file entries and a periodic GC ring (daemon-internal, patch release)"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch map; not edited from here"
  261117-feat-ws-agent-tools-mailbox-routing: "keeps daemon-owned agent addresses off mailbox files; its daemon-owned mail persists through this ticket's restore entries, and this store lands before its daemon-owned routing phase (Decision 6)"
sage-review-design: required
sage-review-completeness: required
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

`(NN)` are owner board IDs from the 2026-11-17 lead-discuss session (see the
parent epic's Cross-Child Decisions).

1. **In-memory first; files only as restore backing (owner (13)=c, the
   in-memory-first option).** Use per-file entries with a periodic GC ring.
   (14) is superseded.
2. **Not T3; a daemon-internal storage change shipped as a patch (owner (15):
   patch release, direct-run unsupported).** Mailbox
   handlers already run inside the single daemon
   (`agents-plugin-tool/internal/mcp/mailbox_tools.go`; the launcher `proxy_serve`
   only relays; `cmd/ws-mcp/hosted_cli.go`). Old launchers on a different minor
   are already refused on both sides (launcher version check and the daemon's
   `wsdashboard/proxy.go`). v0.50.2 is not blocked.
3. **Remaining risk: daemon downgrade, in both directions.** An old daemon
   reads only `mailbox.json`, so (i) unread mail held only in the new structure
   becomes invisible to it, and mail the new daemon already delivered but no
   longer rewrote out of `mailbox.json` is redelivered from that stale file;
   and (ii) mail the old daemon writes to `mailbox.json` during the downgrade
   must be imported when the new daemon reopens, else it is lost (not T2).
   Judge T1 vs T2 in Phase 1 with source citations (candidate mitigations: drain at
   switchover, or also write `mailbox.json`), per
   `ai-docs/manuals/release-compatibility.md`. The choice among candidates is
   made in that judgment, not here.
4. **`ws-mcp serve --stdio` direct-run is declared unsupported (15).** That path
   (`cmd/ws-mcp/main.go`) writes files in-process and the plugin does not use it.
5. **Direct-file readers read the memory store, not fresh files (fact
   population 2026-11-17, auto-confirmed technical; call sites re-checked
   2026-11-17).** Code outside the send/recv handlers reads `mailbox.json` and
   the reply registry straight from disk through `wsmailbox`: the armed wait
   (`wsmailbox.Wait` / `Stream`, `wsmailbox/wait.go`, around 378-434), the
   Codex Stop-hook peek (`ShouldNotifyNamedInboxUnread`,
   `wsmailbox/hook_peek.go`, around 41 and 128) and the dashboard foreign-input
   look. Where they run decides the path:
   - **In the daemon process (the shipped path).** The lead mail watch
     (`internal/mcp/lead_mail.go:127`, the only in-daemon `wsmailbox.Wait`
     caller under `internal/`). The `mailbox wait` CLI and the Codex Stop hook
     (`agents-plugin/.codex-plugin/hooks.json` runs
     `ws-mcp-launcher.py mailbox codex-stop-hook`) also execute in the daemon:
     the launcher relays every product command as CLI frames (`proxy_cli`;
     `proxy_mode` sends all non-lifecycle commands to the daemon) and the
     daemon runs them through `hostedCLI.run`
     (`cmd/ws-mcp/hosted_cli.go:137-153`), which calls `runMailboxWait` ->
     `wsmailbox.Wait` / `Stream` (`cmd/ws-mcp/mailbox.go:203-207`) and
     `runCodexStopHook` -> `ShouldNotifyNamedInboxUnread`
     (`cmd/ws-mcp/mailbox.go:470`). Being in the daemon does not help today:
     they still read the files. Phase 1 rewires them in process to the
     in-memory store. The Stop hook carries no ws session_key (a bare OS
     subprocess reading `WS_MAILBOX`, `hook_peek.go:7-11`), so its query stays
     an ownership-agnostic unread count by slug. With the daemon unreachable
     the launcher fails the command without running the binary (`mailbox
     wait` exits 1; the Stop hook fails open with exit 0); that stays.
   - **Out of the daemon.** Only a direct binary run: `cmd/ws-mcp/main.go`
     dispatches `ws-mcp mailbox wait` in process to `mailboxWait`
     (`cmd/ws-mcp/mailbox.go:91`), and other direct CLI commands (including
     `mailbox codex-stop-hook`) run an in-process `hostedCLI` with no host.
     These have no daemon connection. Phase 1 designs their path: a daemon
     query client plus a file fallback that reads the new per-file restore
     entries, or an unsupported declaration beside Decision 4's `serve
     --stdio`; the choice is recorded in the Phase 1 design record.
   - **Invariant for every reader.** An armed wait never misses mail held
     only in memory and never returns stale mail (mail already received).
   None of the shipped readers is declared unsupported: the Claude CLI tier is
   frozen with its current features kept, and its mod's mailbox push runs
   `mailbox wait` (`agents-plugin/hooks/claude-mod/watcher.ts`); the hook
   peek serves the Codex Stop-hook adapter. Rejected: keeping the restore files
   current for these readers (a per-event rewrite, against the release-gate
   file-touch rule); declaring them unsupported (breaks the frozen CLI tier and
   the Codex Stop hook).
   Windows coverage: the 2026-10 Windows reader hotfixes (ab460d35, retrying
   the atomic replace on transient holds; da5a0359, retrying sharing
   violations on unlocked reads) live in build-tagged
   `wsmailbox/replace_file_windows.go` and `wsmailbox/read_file.go`; local
   macOS `go test` never compiles the Windows file. Windows CI
   (`.github/workflows/ws-mcp-ci.yml`, `windows-latest` matrix) is the
   vehicle that keeps those regressions covered; the measurement machine is a
   separate question, (71).
6. **Persistence vehicle for daemon-owned agent mail (batch-review coherence
   C1, auto-confirmed technical).** Mail to daemon-owned addresses that
   `261117-feat-ws-agent-tools-mailbox-routing` routes in memory (its
   Decision 2), including mail to a sleeping child that may wait up to 15 days
   (its Decision 16), persists through this ticket's per-file restore
   entries, with the same recv, history and reply-id semantics as external
   mail. Landing order: this ticket's store (Phase 2) lands before that
   ticket's daemon-owned routing phase. Rejected: a separate agent-tools mail
   store with its own restore files and history behavior (two restore
   formats, two downgrade judgments, divergent history).

## Open Decision Queue

Last ID: (83)

(71) [confirmed] [policy] Mailbox file management and pass bar (owner 22:26,
  recorded as the lead's reading; the owner may veto the 32 KB reading): the
  mail queue is fully in memory; history is served from an in-memory cache;
  a segmented JSONL ring backs it on disk (start a new segment once the
  current one passes 1 MiB, keep at most 10, delete the oldest to make room),
  read only for backlog recovery at daemon start. Pass bar: per send, exactly
  one JSONL append to the end of an already-known file and zero repeated file
  reads (uncached config files excepted). The owner's "32KB" is read as an
  append-buffer example; the segment numbers are Phase 1 defaults. Measured
  as a per-send syscall trace on this macOS machine. Supersedes Decision 1's
  "per-file entries with a periodic GC ring". Stays here until (80)-(83)
  settle.

(80) [open] [technical] One log record per send carries both the message and
  its reply id, and the reply-id registry is rebuilt in memory from the log at
  start, over appending the registry separately - a second append per send
  breaks the (71) bar; today the ~135 KB registry is rewritten per send.

(81) [open] [technical] recv (consumption) appends one ack record to the same
  log, so restart does not redeliver read mail, over not recording
  consumption (read mail returns after restart) or rewriting a file (against
  the release-gate rule).

(82) [open] [technical] Before deleting the oldest segment, undelivered mail
  in it is re-appended to the newest segment; history pushed out of the ring
  is dropped, over keeping history forever - rotation must never lose
  undelivered mail, and unbounded history defeats the ring.

(83) [open] [announced] Drop "also keep writing `mailbox.json`" from Decision
  3's downgrade mitigations; Phase 1 judges T1/T2 with the remaining option
  (drain/export at switchover). - Rewriting the whole file per send conflicts
  with the (71) one-append bar.

## Constraints

- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Release-gate file-touch rule (owner, 2026-11-17, machine note): minimize file
  touches at the root; files for backup/restore only; deltas (append) not
  rewrites; no heartbeat rewrites; no per-event O(N) rereads; interactive paths
  never wait on file I/O; judged by measured syscalls/s on the owner's PC
  (this ticket's measurement machine and pass bar: (71)).
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

## Prior Decisions

- 261117-feat-ws-agent-tools-mailbox-routing (2026-11-17, Decisions): "In-memory routing for daemon-owned addresses (12). The daemon Runner routes daemon-owned agent addresses in memory, not through mailbox files; persist only what restore needs." — bearing: supports
- 261116-feat-ws-daemon-hub-first-polling e99ef18a (2026-11-16, commit): "T0: WithLock/WithReplyLock publish optional memory-only queue paths after atomic replacement, comparing already-read queues. Serialized mailbox/receipt/hire/registration shapes ... unchanged" — bearing: constrains
- 261116-feat-ws-daemon-hub-first-polling (2026-11-17, ticket Result 6a4b4b432): "Foreign inputs share one daemon-wide look ... the mailbox stores behind the mail print and contacts." Mailbox files are stamp-watched by the dashboard look; a memory-first store must keep that input valid — bearing: constrains
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "the mailbox memory-first store is an independent child (261117-feat-ws-mailbox-memory-first-store, (13)=c, (15))" — bearing: supports
- 261117-research-ws-daemon-file-touch-map bf7a5c5a (2026-11-17, commit): "Owner release gate (12:54) and 14:06 escalation: daemon file-system load is costing hours on the AhnLab V3 PC" — bearing: supports
- 261116-bug-ws-dashboard-daemon-idle-file-touches (2026-11-16, Decisions): "Existing on-disk heartbeat writes needed by older readers remain mandatory." — bearing: constrains
- ab460d35 (2026-10-08, commit): "Unlocked readers exist in other ws-mcp processes: the mailbox wait poll (wait.go peek ..." (Windows reply-registry rename hotfix; taking the flock in readers was rejected) — bearing: constrains
- 260923-bug-ws-mailbox-windows-store-read-sharing-violation da5a0359 (2026-10-22, commit): "A reader opening mailbox.json while another ws-mcp process's replaceFile is in flight failed at once, failing mailbox wait and Windows CI" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsmailbox/store.go, replyid.go, wait.go, hook_peek.go; internal/mcp/mailbox_tools.go, mailbox_runtime.go; cmd/ws-mcp/main.go |
| scope.surface | cross-module | WithLock, WithReplyLock, Load, LoadReplyStore, PostSystem are exported from wsmailbox and called from internal/mcp and internal/wsdashboard |
| scope.new_public_symbol | unknown | per-file entry and GC ring API is fixed in Phase 1 design; none named yet |
| scope.new_type_contract | unknown | entry layout and ring parameters are Phase 1 deliverables; no type named yet |
| scope.test_surface | existing | agents-plugin-tool/internal/wsmailbox/*_test.go (13 files) and internal/mcp/mailbox_*_test.go |
| complexity.reuse_points | unconfirmed | publishQueueChange memory hub (wsmailbox/queue_changes.go) exists, but no memory store or ring component is named to reuse |
| complexity.side_effect_risk | high | wait.go, hook_peek.go and the dashboard foreign look read mailbox.json and the reply registry directly from disk |
| risk.correctness | high | send/recv/reply-id parity and restore after restart must hold while removing the flock rewrite path |
| risk.fit | moderate | fits the release-gate file-touch rule and the routing ticket's in-memory direction, but direct-file readers outside the store are not yet placed |
| risk.test | moderate | restore, GC ring and downgrade scenario are testable; the syscalls/s machine and pass bar wait for (71); Windows reader code is covered only by Windows CI |
| risk.security_or_contract | high | on-disk store shape is a cross-version surface; an old daemon reads only mailbox.json (downgrade T1/T2 judgment pending) |

## Phases

### Phase 1: Downgrade judgment and store design

Before implementation, record the T1/T2 judgment for daemon downgrade with
source citations and choose the mitigation. The judgment names both directions
of Decision 3: (i) the new daemon consumed messages and stopped rewriting
`mailbox.json`, so an old daemon would redeliver already-received mail from a
stale file; (ii) mail an old daemon writes during the downgrade must be
imported when the new daemon reopens, else it is lost (not T2). Confirm the
patch-release verdict with the cited launcher/daemon version checks. Fix the
per-file entry layout and the GC ring parameters as an implementation-time
design step and record them; the design record states which entries the GC ring
may drop (unread mail, including Decision 6's sleeping-child mail, is never
among them, by the no-semantics-change constraint) and shows recv, history and
reply-id parity with today. Inventory the direct-file readers (Decision 5):
per reader, where it runs (daemon or direct binary), its memory-store path, its
behavior with no daemon, and whether it still reads valid files; record the
direct-binary path choice. Mark `serve --stdio` unsupported where it is
documented.

Verification: a written verdict with citations in the Phase Result; the
downgrade-test method fixed as a fixture that writes only `mailbox.json` the way
the old daemon would, then reopens the new daemon; a measured baseline
(syscalls/s per send) recorded for comparison, on the machine settled by (71).

### Phase 2: Memory-first store implementation

Implement the in-memory-first mailbox with per-file entries as restore backing
and a periodic GC ring, replacing the whole-file rewrite and the full reply-id
registry rewrite on every send; apply the Phase 1 downgrade mitigation.

Verification: Go tests for send/recv/history/reply-id parity,
restore after daemon restart, GC ring behavior, every Decision 5 reader seeing
memory-only mail and never stale mail, and the downgrade scenario from Phase 1
(both directions, with the old-daemon fixture); Windows CI passing for the
Windows reader code (Decision 5); measured syscalls/s against the Phase 1
baseline, on the machine and pass bar settled by (71).
