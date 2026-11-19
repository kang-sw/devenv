---
title: "Mailbox in-memory-first store with a segmented JSONL restore ring (daemon-internal, patch release)"
parent: 261114-epic-ws-common-session-foundation
related:
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch map; not edited from here"
  261117-feat-ws-agent-tools-mailbox-routing: "keeps daemon-owned agent addresses off mailbox files; its daemon-owned mail persists through this ticket's restore ring, and this store lands before its daemon-owned routing phase (Decision 6)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3f0e73c12b2fbcae
sage-review-completeness-reviewed: 3f0e73c12b2fbcae
completed: 2026-11-18
---

# Mailbox memory-first store

## Background

Independent child of the epic (not stage-bound). Owner: the file-writing mailbox
is a relic of the plugin-hacking era. Today each send rewrites the whole scope
`mailbox.json` and a reply-id registry of roughly 135 KB
(`agents-plugin-tool/internal/wsmailbox/store.go`, `replyid.go`; search the
rewrite paths), and every sent or drained message also creates one history
file per session (`internal/mcp/mailbox_history.go`: temp+rename under
`<cache>/mailbox-history/<session-key>/`, trimmed to the last
`mailboxHistoryCap = 10` per session). The owner's release-gate rule on daemon file touches (see
Constraints) is why this matters: the release is blocked until daemon
file-system load is fixed, per `261117-research-ws-daemon-file-touch-map`.

## Decisions

`(NN)` are owner board IDs from the 2026-11-17 lead-discuss session (see the
parent epic's Cross-Child Decisions).

1. **In-memory first; a segmented JSONL ring only as restore backing (owner
   (13)=c, the in-memory-first option, refined by (71), owner 22:26; (14) is
   superseded).** The mail queue lives entirely in memory, and history
   (including what `ws/mailbox.history` returns) is served from an in-memory
   cache. On disk, a segmented JSONL ring is read only to recover the backlog
   when the daemon starts: a new segment starts once the current one passes
   1 MiB, at most 10 segments are kept, and the oldest is deleted to make
   room. The segment size and count are Phase 1 defaults; the owner's "32KB"
   is an example size that Phase 1 may use for its in-process encode buffer,
   never a buffer that holds records across sends (see the per-send bar).
   The per-session history files (`mailbox_history.go`) are retired into the
   same ring; history keeps today's per-session window (the last 10 sent or
   drained messages per session).
   - **One strand per daemon (owner 22:42).** The single shared daemon
     collects the mail of every session and every scope tier into one ring;
     scope (machine, clone, worktree) and recipient are fields of each
     record. Storage is not split per session or per scope the way today's
     per-scope `mailbox.json` files and reply registry are
     (`wsmailbox/store.go` `Paths`). Phase 1 picks the ring's location. Each
     record carries what the downgrade export and any direct-binary fallback
     need to resolve per-scope paths (the scope root, not only the tier name;
     `store.go:117-150`). The store is one daemon-held instance injected into
     every `mcp.Server` the daemon builds (session servers, the hosted CLI's
     per-call server at `cmd/ws-mcp/hosted_cli.go:74-88`, the lead mail
     watch), not a per-Server field.
   - **Per-send bar.** One `mailbox.send` costs exactly one JSONL append to the
     end of an already-known file, and zero repeated file reads (uncached
     config files excepted; Phase 1 names them). The append is one write to a
     long-lived file descriptor, completed before `mailbox.send` returns, so
     a returned send survives a daemon crash; no per-send fsync (an OS crash
     may lose the tail) and no batching of records across sends. Counting
     rule for the check: write, open, close, rename, unlink and read calls on
     mailbox data files per send, traced with a macOS tool such as
     `fs_usage`. The bar applies to steady-state sends; a send that crosses
     the segment size and triggers rotation is the amortized exception,
     measured separately, and Phase 1 states whether rotation runs inline or
     off the send path. The synchronous append does not conflict with the
     release-gate rule that interactive paths never wait on file I/O: it is a
     single `write()` to a long-lived descriptor with no fsync, and
     `mailbox.send` is an agent tool call, not a UI path.
   - **One record per send ((80)=ok).** The record carries every component of
     the message: recipient (`to`, its name and scope), sender (`from`,
     `reply_to`, `label`), `content`, `sent_at`, and the reply id with its
     contact record (today's envelope: `wsmailbox/store.go:83-90`), plus the
     sender's and receiver's session identity, which per-session history
     needs (a reply id is a one-way derivation of the session key,
     `replyid.go` `ReplyID`). The reply-id registry and per-session history
     are rebuilt in memory from the log at start. Should a
     send ever address several recipients (today `mailbox.send` takes one
     `to`, `internal/mcp/mailbox_tools.go:37`; there is no cc), it is still one
     record listing all of them, not one line per recipient.
   - **Consumption ((81)=ok).** recv appends one ack record to the same log, so
     a restart never redelivers mail already received.
   - **Rotation ((82)=ok).** Before the oldest segment is deleted, every
     piece of live state it holds is carried into the newest segment (as
     re-appended records or a compaction snapshot; Phase 1 picks): undelivered
     mail, live reply-id entries with their contacts (today never reaped
     while live; `replyid.go:30-61`, `MaxContacts = 20`), presence and
     listening records, `UndeliveredNotified` flags, and each session's
     last-10 history window. Only history outside that window is dropped, so
     one busy session cannot evict another session's history and caller-
     visible history matches today. The 10-segment count is a soft cap: live
     state is never dropped to honor it; if live state alone outgrows the
     ring, the ring grows past the cap and the daemon logs a warning.
     Carry-forward applies today's reaping, so the live set stays bounded:
     history of a session idle past the session prune age is dropped
     (`pruneMailboxHistory`, `internal/mcp/session_prune.go:26-27, 118-122`),
     reply entries expire by `LastSeen` as `replyid.go` does today, and
     presence records keep their current lifetime.
     Carry-forward runs only at rotation (once per segment of appends), which
     bounds the write amplification.
   Rejected: a cross-send append buffer (a returned send could be lost and a
   ring reader would miss it); carrying forward only undelivered mail (loses
   live reply ids, presence and history on restart); a hard segment cap that
   drops live state; one ring per session or per scope tier (multiplies open files
   and appends, against the one-strand direction); per-file entries with a
   periodic GC ring (the earlier reading of
   (13)=c; one file per entry multiplies file creates and deletes); appending
   the reply-id registry separately (two appends per send, against the bar);
   not recording consumption (read mail returns after restart); rewriting any
   file per send (the release-gate file-touch rule); keeping history forever
   (defeats the ring).
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
   Judge T1 vs T2 in Phase 1 with source citations, per
   `ai-docs/manuals/release-compatibility.md`; the mitigation is a drain or
   export at switchover, and Phase 1 settles its form: when it runs (the new
   daemon cannot foresee a downgrade, so the candidates are a one-time export
   of live undelivered mail into the per-scope `mailbox.json` files at
   graceful shutdown and an import-and-clear of those files at start), how
   the single ring splits back into per-scope files and how per-scope
   old-daemon writes merge back into the ring, and whether a once-per-
   shutdown export fits the release-gate rule (it is not a per-event
   rewrite). The judgment also covers a crash or kill of the new daemon
   before a downgrade (no graceful export ran, so `mailbox.json` is stale)
   and the per-session history directory and reply registry, which an old
   daemon also reads and the new daemon stops updating. If neither T1 nor T2
   is reachable, Phase 1 stops and returns to the lead: a T3 boundary needs
   the owner's approval.
   **Upgrade (first start over an existing install).** The new daemon's first
   start imports today's state into the ring: undelivered mail from every
   per-scope `mailbox.json`, the live reply-id registry with its contacts,
   and each session's last-10 history files, so replies to already-received
   mail and `ws/mailbox.history` do not change across the upgrade. Phase 1
   records the import and Phase 2 tests it with a fixture of today's files.
   The import skips history directories already past the session prune age.
   **Export/import reconciliation.** The daemon exports on every graceful
   shutdown and imports on every start, and today's envelope has no message
   id (`store.go:83-90`), so a naive import would duplicate mail on each
   plain restart, and after a real downgrade the ring would redeliver mail
   the old daemon already consumed. The rule: the exported files are
   authoritative for the exported set. A start whose source files are
   unchanged since the last export (an export marker in the ring records
   what was written) clears them and keeps the ring as is; a start whose
   files changed reconciles each exported queue against them: exported mail
   still in the file is kept once, exported mail missing from it is acked
   (the old daemon consumed it), and mail the old daemon added is imported
   as new, together with its reply-registry and history changes. The
   export covers every surface the old daemon reads for delivery: the
   per-scope `mailbox.json` files and the queues of `id:` reply handles in
   the machine reply registry (`mailbox-replyids.json`,
   `replyid.go:20-22`), plus the live reply entries and last-10 history the
   old daemon serves. After both the upgrade import and a reopen import the
   source files are cleared or marked, so a later downgrade never reads
   mail already received from them. First start is told apart from a
   reopen by the ring's presence (no ring: upgrade import; ring with an
   export marker: reconciliation). Phase 1 fixes the matching mechanism (an
   export manifest, an envelope tuple, or another key that survives an old
   daemon's whole-file rewrite, which drops unknown fields). Rejected ((83)=ok):
   also keep writing `mailbox.json` (a whole-file rewrite per send, against
   the Decision 1 bar).
4. **`ws-mcp serve --stdio` direct-run is declared unsupported (15).** That path
   (`cmd/ws-mcp/main.go`) writes files in-process and the plugin does not use it.
5. **Direct-file readers read the memory store, not fresh files (fact
   population 2026-11-17, auto-confirmed technical; call sites re-checked
   2026-11-17).** Code outside the send/recv handlers reads `mailbox.json` and
   the reply registry straight from disk through `wsmailbox`: the armed wait
   (`wsmailbox.Wait` / `Stream`, `wsmailbox/wait.go`, around 378-434), the
   Codex Stop-hook peek (`ShouldNotifyNamedInboxUnread`,
   `wsmailbox/hook_peek.go`, around 41 and 128) and the dashboard foreign-input
   look (which today stamp-watches the mailbox files; without a file rewrite
   the stamp no longer changes, so it needs an in-process change signal from
   the store, e.g. the existing `publishQueueChange` hub). Where they run
   decides the path:
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
     These have no daemon connection. Writers are included: a direct
     `ws-mcp call mailbox.send` runs an in-process server with no host
     (`hosted_cli.go:522-533`) and would write `mailbox.json` while the
     daemon runs, so the daemon would see that mail only at its next start.
     Phase 1 designs the path for these readers and writers: a daemon
     query client plus a file fallback that reads the restore ring
     segments, or an unsupported declaration beside Decision 4's `serve
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
   vehicle that keeps those regressions covered; the per-send measurement runs
   on macOS ((71)).
6. **Persistence vehicle for daemon-owned agent mail (batch-review coherence
   C1, auto-confirmed technical).** Mail to daemon-owned addresses that
   `261117-feat-ws-agent-tools-mailbox-routing` routes in memory (its
   Decision 2), including mail to a sleeping child that may wait up to 15 days
   (its Decision 16), persists through this ticket's restore ring, with the same recv, history and reply-id semantics as external
   mail. Landing order: this ticket's store (Phase 2) lands before that
   ticket's daemon-owned routing phase. Rejected: a separate agent-tools mail
   store with its own restore files and history behavior (two restore
   formats, two downgrade judgments, divergent history).

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
  never wait on file I/O; judged by measured syscalls/s on the owner's PC.
  For this ticket ((71)): a per-send syscall trace on this macOS home machine
  against the Decision 1 bar (one append, zero repeated reads); a Windows AV
  machine measurement is not a merge gate.
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
| scope.span | multi-file | agents-plugin-tool/internal/wsmailbox/store.go, replyid.go, wait.go, hook_peek.go; internal/mcp/mailbox_tools.go, mailbox_runtime.go, mailbox_history.go; cmd/ws-mcp/main.go, hosted_cli.go |
| scope.surface | cross-module | WithLock, WithReplyLock, Load, LoadReplyStore, PostSystem are exported from wsmailbox and called from internal/mcp and internal/wsdashboard |
| scope.new_public_symbol | unknown | the log record and ring API is fixed in Phase 1 design; none named yet |
| scope.new_type_contract | unknown | record layout and ring parameters (segment size and count, in-process encode buffer) are Phase 1 deliverables; no type named yet |
| scope.test_surface | existing | agents-plugin-tool/internal/wsmailbox/*_test.go (13 files) and internal/mcp/mailbox_*_test.go |
| complexity.reuse_points | unconfirmed | publishQueueChange memory hub (wsmailbox/queue_changes.go) exists, but no memory store or ring component is named to reuse |
| complexity.side_effect_risk | high | wait.go, hook_peek.go and the dashboard foreign look read mailbox.json and the reply registry directly from disk |
| risk.correctness | high | send/recv/reply-id parity and restore after restart must hold while removing the flock rewrite path |
| risk.fit | moderate | fits the release-gate file-touch rule and the routing ticket's in-memory direction, but direct-file readers outside the store are not yet placed |
| risk.test | moderate | restore, ring rotation and downgrade scenario are testable; the per-send bar is checked by a syscall trace on macOS ((71)); Windows reader code is covered only by Windows CI |
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
log record layout (send, ack, and any other record kinds) and the ring
parameters (segment size, segment count, in-process encode buffer) as an
implementation-time design step and record them; the design record shows that
rotation never drops undelivered mail (including Decision 6's sleeping-child
mail), how the reply-id registry and queues are rebuilt from the log at start,
and recv, history and reply-id parity with today. It also records how
presence and listening-marker changes persist under the release-gate rule
(append, no rewrite), since they are not per-send writes, and which of
registrations, receipts and hire records (`PostSystem` consumers) move into
the ring. Inventory the direct-file readers (Decision 5):
per reader, where it runs (daemon or direct binary), its memory-store path, its
behavior with no daemon, and whether it still reads valid files, including
the dashboard look's change signal; record the direct-binary path choice for
readers and writers. Mark `serve --stdio` unsupported where it is
documented.

Verification: a written verdict with citations in the Phase Result; two
downgrade-test methods fixed, one per direction: (ii) a fixture that writes
only `mailbox.json` the way the old daemon would, then reopens the new daemon;
(i) today's `Load` and delivery path run against the files the new daemon
leaves behind after its export, showing no redelivery of acked mail and no
loss of undelivered mail; a round-trip fixture (export, then the old
daemon's code consumes, appends, replies and writes history, then the new
daemon reopens) asserting no duplicate, no redelivery and no loss; a plain
restart asserting no duplication; a per-send syscall
trace of today's store recorded on this macOS machine as the baseline ((71)).

### Result (e973b72b4) - 2026-11-17

Downgrade verdict: T2 in both directions, patch release; no T3 boundary.

- Version gate. The launcher and daemon admit only the same minor
  (`internal/wsdashboard/proxy.go:250-252`, `SameMinor`), so a patch
  downgrade replaces the daemon whole: one daemon per cache root, and no
  old executable runs beside the new one.
- (i) New -> old. The new daemon keeps mail in memory and the ring, so the
  legacy files go stale between opens. On graceful shutdown,
  `ClosePersistent` (`cmd/ws-mcp/dashboard.go`, right after
  `stopMailboxSweep`) runs `instance.export` (`internal/wsmailbox/legacy.go`).
  It writes the live presence and queues to each scope's `mailbox.json`, the
  entries and queues to `mailbox-replyids.json`, and the history windows to
  `mailbox-history/<key>/`, each through the same flock the old code takes.
  The old daemon reads exactly these files (base `store.go:192` `Load`,
  `:227` `WithLock`; `replyid.go:125` `LoadReplyStore`, `:176` `WithReplyLock`).
  Acked mail is absent from the export, so it is not redelivered.
  - A killed new daemon exports nothing. The import at open already cleared
    the files' queues, so an old daemon after a crash sees no mail rather
    than stale mail.
  - That mail is still in the ring. The next new open restores it, which is
    the T2 recovery.
- (ii) Old -> new. The old daemon's whole-file rewrite drops the unknown
  `memory_store_export` field (encoding/json). At the next open, the new
  daemon diffs each file against the baseline it recorded in the ring
  (`reconcileNamedPaths`, `reconcileReply`, `reconcileHistory`):
  - mail in the file but not in the baseline is imported;
  - baseline mail gone from the file is acknowledged;
  - changed presence and reply entries are taken;
  - new history entries are imported.
  A file still carrying the mark is the store's own write, which closes the
  crash window between the file write and its baseline record. Envelopes
  match as a multiset, ignoring `UndeliveredNotified`.
- The ring directory is unknown to the old version and ignored by it. Nothing
  the old version reads is lost: the new reopen restores it from the ring
  plus the reconcile.

Store design:

- Records. One JSONL line per committed transaction, `{kind:"tx", op, at,
  named, reply, history, listening, baseline}`, carrying deltas only:
  - per named path, presence upserts or deletes and per-queue
    `{drop:n, add:[...]}`;
  - reply entries and queues, in the same shape;
  - appended history entries;
  - listening set and clear;
  - baseline updates.

  `op` is one of send, recv, presence, listen, unlisten, history, system,
  sweep, import, import-clear, or export. Send and ack are not separate
  kinds: a recv is the drop half of the same delta form.

  `{kind:"snapshot", state}` opens each segment.
- Ring parameters:
  - Segments are `<cache>/mailbox-ring/<16-digit seq>.jsonl`. A segment rotates
    after 1 MiB of deltas, and 10 segments are kept.
  - The encode buffer is one `bytes.Buffer` per instance, reused. Each
    record goes out in one `write()` through a descriptor the daemon keeps
    open.
  - There is no fsync.
  - Rotation: reap idle history windows (`HistoryRetention`, 30 days) and
    expired, empty reply entries (`ReplyRetention`). Then write the whole
    live state as the next snapshot (temp file plus rename), and delete
    segments past 10.
  - Live queues are never dropped. A state larger than a segment grows the
    segment and logs a warning (soft cap). This is a deviation in form from
    Decision 1's wording "the ring grows past the cap": every segment
    opens with a whole snapshot, so extra segments would add nothing. The
    count stays at 10, and an oversized segment holds the overflow.
  - Restore reads only the newest segment that has a whole snapshot. A
    torn last line is cut.
- Reply-id registry and queues: these are part of the snapshot plus the
  deltas, rebuilt at open.
- Parity:
  - `mailbox.send` and `claimMail` are each one `Store.Update` with the same
    queue, contact, reply-entry, and history logic as before. History names
    keep the legacy `<stamp>-<hex>.json` form, with stamps monotonic across
    opens.
  - Reply-entry reaping (`ReapReplyEntries`) runs on publish, as before, and
    at rotation.
  - Store changes publish `wswake.Mailbox.StoreChanged` for changed queue
    paths only, as `publishQueueChange` did.
- Presence and listening markers are ring records (append only, no
  rewrite). Presence heartbeats are one record each. Listening markers
  moved from `mailbox-listening/*.json` into the store.
- `PostSystem` consumers (Pi turn reports, whiteboard nudge notices, the lead
  mail return, the hire outcome mail): only the mail moves into the ring.
  Registration, receipt, and hire records stay in their own files, which
  this ticket does not touch.
- Decision 5 reader inventory:

  | Reader | Path | Notes |
  |---|---|---|
  | `wsmailbox.Wait` peek | `Store.NamedQueue` / `Store.ReplyQueue` | Daemon-hosted, memory. |
  | Codex stop hook (`ShouldNotifyNamedInboxUnread`, `PeekNamedInboxUnread`) | `Store.NamedQueue` | Daemon-hosted, memory. |
  | Undelivered sweep | `Store.QueuePaths` plus `Update` | Daemon. |
  | Dashboard page and panels | `Store.LoadReply`, `Store.Load`, `Store.History`, `Store.NamedQueue` | Daemon. |

  The dashboard look's change signal is now a mail print folded from the
  store (reply print, history names, listening PID). It is renewed by
  `StoreChanged`, nudges, or the 5-minute foreign look. The mailbox file
  watches are gone.

  With no daemon, each call is transient: lock the ring, load, reconcile,
  run, release. While the daemon runs, a transient read uses a lock-free
  ring view, and a transient write is refused. Direct-binary mailbox calls
  (and `serve --stdio`) are unsupported; `ai-docs/manuals/ws-mcp.md` gains
  a "Mailbox store" section saying so.
- Baseline trace (macOS, DYLD file-call interposer). `fs_usage` needs sudo,
  so an interposer stands in. Today's store costs 99.8 file ops per reply-id
  send and 106 per named send:
  - the reply registry: 3 opens, 6 reads, and 2 renames;
  - a history entry file create and rename;
  - `sessions/<key>/key.json`: 6 opens and 12 reads.
- Downgrade test methods:
  - Direction (ii): a fixture rewrites `mailbox.json` the way the old
    daemon does, then the store reopens.
  - Direction (i): `LoadFile`/`LoadReplyFile`/legacy history against the
    export.
  - The round trip and plain restart are tests in Phase 2.

### Phase 2: Memory-first store implementation

Implement the in-memory-first mailbox with the segmented JSONL ring as restore
backing (Decision 1), replacing the whole-file rewrite and the full reply-id
registry rewrite on every send; apply the Phase 1 downgrade mitigation.

Verification: Go tests for send/recv/history/reply-id parity,
restore after daemon restart (including no redelivery of acked mail and a
send surviving a daemon kill right after it returns), ring rotation (all live
state of Decision 1 carried forward, a quiet session's reply id and last-10
history surviving rotation and restart, only out-of-window history dropped,
the soft cap growing instead of dropping live state), the dashboard look
refreshing on a memory-only send, every Decision 5 reader seeing
memory-only mail and never stale mail, and the downgrade scenario from Phase 1
(both directions, with both Phase 1 fixtures, the round-trip fixture and
the plain-restart no-duplication test); rotation reaping an idle session's
history and an expired reply entry; the first-start upgrade import
from a fixture of today's files (undelivered mail, live reply ids, last-10
history) with replies and `ws/mailbox.history` unchanged across it; Windows CI passing for the
Windows reader code (Decision 5); a per-send syscall trace on this
macOS machine showing exactly one append and zero repeated file reads per
steady-state send, with the rotating send measured separately, compared with
the Phase 1 baseline ((71)).

### Result (e973b72b4) - 2026-11-17

Implemented as designed. New files:

- `internal/wsmailbox/memstore.go`: the store, `Tx`, the registry,
  persistent and transient instances, and the lock-free view.
- `ring.go`
- `legacy.go`: import, clear, and export.
- `history.go`

The scope `mailbox.json` path stays each named store's key. Every caller in
`internal/mcp`, `internal/wsdashboard`, and `cmd/ws-mcp` moved to
`Paths.Store()`. `dashboard_history_cache.go` was deleted. The daemon calls
`EnablePersistent` and `OpenPersistent` at start and `ClosePersistent`
before its linger. A successor waits up to 30 s for the ring lock.

Verification:

- `go test ./...` on macOS passes, except for these tests, which fail
  identically on base dfcd71832:
  - `internal/mcp`: `TestDashboardNotesUsesHostedCallerStoreIdentity`,
    `TestBoardStateFilterAsks`, and `TestLateReloadSignalsOnlyAChangedView`;
  - the JS `TestSearchPalette*` tests and
    `TestHistoryScriptWholeItemWindowsAndGapNavigationUnderNode`.

  `internal/wsagent` `TestLeadRejoinDoesNotStallBehindALongReplay` timed
  out once under the full parallel run. It passes 3/3 in isolation, and
  this branch does not touch its code.
- Follow-up fixes:
  - 10f95a154 (from the lite review) keeps the 30 s successor lock wait
    outside the registry lock.
  - 898f0a872: a transaction's reply working copy now deep-copies each
    entry's `Contacts` map. Before this, a contact written in place
    changed the committed state behind the diff, so it never reached the
    ring. The commit adds a regression test. It also points the
    process-env tests at their server's cache root and drops the removed
    reply and scope cache counters from the counters test.
- `GOOS=windows go vet ./...` is clean. Windows CI was not run locally.
- New tests (`internal/wsmailbox/memstore_test.go`) cover:
  - a kill right after a send survives;
  - no redelivery of acked mail after a crash;
  - a torn record is cut;
  - rotation: carry-forward at the 10-segment cap, a quiet session's mail
    survives, the idle history window and the expired reply entry are
    reaped, and the soft cap grows instead of dropping;
  - upgrade import of named, reply, and history files, with a bad history
    file skipped;
  - an unreadable legacy store is skipped;
  - the downgrade export (i) and re-import (ii) round trip;
  - plain restarts, including export then crash, do not duplicate;
  - history stamps rise across opens;
  - the lock-free read view versus a refused write;
  - successor lock wait;
  - Decision 5 readers see memory-only mail and never stale files.
- Existing parity tests for send, recv, history, and reply ids pass
  unchanged in assertion. The dashboard mail print moves on a history entry
  and a listening marker (`dashboard_page_test`).
- Per-send trace (persistent mode, the same harness): 51.0 ops per send for
  both reply-id and named sends. Mailbox files account for exactly one
  `write` (the ring append) and zero reads. The rest are
  `sessions/<key>/key.json` (6 opens, 12 reads, 12 stats per send), outside
  this ticket.
- A rotating send measured separately costs about 12.6 ring ops: the
  append, temp snapshot create, write, and close, the rename, opening the
  new segment, closing the old one, listing the directory, and an unlink.
