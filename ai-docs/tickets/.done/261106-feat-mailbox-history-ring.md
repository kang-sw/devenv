---
title: Mailbox history ring and mailbox.history tool
related:
  261106-feat-whiteboard-dashboard-panels: dependent; its full mailbox panel's recent-messages list reads this ring
  261007-bug-session-record-cross-process-write-race: related; the ring stays off the session record so it does not wait on this bug (204)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0d30b12c7c7b6d94
sage-review-completeness-reviewed: 0d30b12c7c7b6d94
completed: 2026-11-06
---

# Mailbox history ring and mailbox.history tool

## Background

`mailbox.recv` drains a session's queues and deletes the drained messages
(`handleMailboxRecv` in `internal/mcp/mailbox_tools.go`, the two
`delete(store.Queues, ...)` calls for the named inbox and the own reply-id
queue; no other path consumes a queue). Once drained, a message exists only in
the conversation that received it. When that conversation loses it, through a
tangled session or a mis-aimed rewind, the message is gone. The only history
the mailbox keeps is per-counterparty `Contacts` (last exchange time, no
content) on the reply-id registry entry.

The user reported that mail loss does happen in practice, and that it usually
comes from a tangled session or a rewind, not from a ws defect. So the goal is
recovery, not a fix to the delivery path. The ring this ticket adds also feeds
the recent-messages list of the whiteboard mailbox panel
(`261106-feat-whiteboard-dashboard-panels`). Decided in the 2026-11-06
whiteboard discussion, items (167), (175), (176), (182), (184), (185), and
(202)-(206).

## Decisions

- **One ring per session, both directions, full bodies.** Each session keeps
  the last 10 messages it exchanged, received and sent together. Each entry
  carries the direction, the counterparty address, the exchange time, and the
  full message body. Received messages are appended once recv's drain write
  has succeeded, under the same drained-after-confirmed-write discipline the
  queues use, one entry per message in queue order. Sent messages are
  appended once the send has been enqueued. The capacity is a constant, 10,
  raised later if recovery needs more.
  - The exchange time is when this session wrote the entry (the send, or the
    drain), not the envelope's `sent_at`, so a message that sat in a queue
    is not trimmed the moment it is written. `mailbox.history` returns it as an
    RFC3339 UTC stamp derived from the entry's file-name stamp.
  - The stored body is the envelope content as delivered, including a
    peer-lead note line `stampPeerLeadNote` put in front, since recovery
    wants exactly what the session saw.
  - A failed ring write (temp file or rename) is logged, not surfaced, on
    both paths, as a failed contact write is (4284354a): a send already
    enqueued must not report failure, and drained mail must still be
    returned.
  Rejected: separate received and sent rings (more sorting and merging for
  one list); storing previews only (the user wants the full body kept for
  recovery).
- **New read-only tool `mailbox.history`.** It returns ring entries newest
  first, and never consumes or changes anything.
  Rejected: a replay argument on `mailbox.recv`, because whether recv deletes
  mail would then depend on an argument, and a recovery attempt could lose
  mail again.
- **Arguments:**

  ```text
  mailbox.history(session_key,
    limit?:     number,              default 10
    body?:      boolean,             default false
    direction?: "recv"|"sent"|"all", default "all")
  ```

  A `limit` below 1 uses the default; a `limit` above the stored count
  returns every entry. With `body: false`, each entry shows only the
  direction, the counterparty, the time, and a title. This is the light "did I miss mail?" check, the most
  common call. `body: true` adds the full body, for recovery after a rewind.
- **Title is the first line of the body.** The mail envelope has no subject
  field (`Envelope`: `from`, `reply_to`, `content`, `sent_at`). A leading
  peer-lead note line (detected with `hasPeerLeadNote` in
  `mailbox_tools.go`, which every lead-to-lead message carries) is skipped,
  so the title is the first line the sender wrote. An empty body has an
  empty title; a first line longer than 120 characters (runes, not bytes) is cut there with an
  ellipsis. The ring's title is the one title rule: the whiteboard mailbox
  panel shows it rather than deriving its own.
  Rejected: adding a subject field to `mailbox.send`, because it changes both
  the envelope schema and the send schema.
- **Storage: one file per entry, collision-free by construction.** Each ring
  entry is its own file in a per-session directory,
  `<cache root>/mailbox-history/<session-key>/` (the same `wsstate.CacheRoot`
  that holds `keys/`). There is no lock and no read-modify-write, so
  concurrent writers to one session key (more than one MCP process can hold
  the same key) never lose an entry. The directory is keyed by session key,
  like the session record, so it survives compaction and keeps message bodies
  out of any file other sessions write. (184), (202), (203), (205)
  Rejected: a ring field on the session record. The record is written by more
  than one process (a lead's `session.note` writes `Note` onto a child key's
  record, and several MCP processes can hold one key) with no cross-process
  lock yet (`261007-bug-session-record-cross-process-write-race`), so a ring
  there could lose entries; this ticket does not wait on 261007. (204)
  Rejected: a lock around the ring write, which adds the locking 261007 has
  not settled for a structure that can avoid it. Rejected: the machine
  reply-id registry entry. Every peer's send locks that entry, and it would
  hold every session's message bodies in one machine-wide file. Rejected: a
  subdirectory of `keys/`, which is the flat directory of session record
  files; a sibling root leaves every enumerator of `keys/` untouched. (205)
- **Write, trim, read.** (206)
  - Write: create a temp file in the session directory, then rename it to a
    fresh name `<fixed-width UTC timestamp>-<short random hash>.json`. A fresh
    name per entry means no two writers ever replace the same file, on any
    OS, and a reader never sees a partial entry. The timestamp has
    nanosecond width and is made strictly increasing within the process, so
    the entries of one drain keep their order and same-stamp ties are left
    to separate processes. The RFC3339 seconds stamp of `mailboxNowString`
    is too coarse for the name.
  - Trim: after the write, list the directory and remove all but the newest
    10 by name, ignoring every remove error. Trimming is best effort; a
    concurrent trim or a failed remove only leaves extra entries until the
    next write.
  - Read: list the directory, sort by name newest first, and skip a file that
    cannot be read or parsed. Entries with the same timestamp order
    arbitrarily.

  Rejected: appending to one file, where concurrent appends interleave or
  overwrite depending on the OS.
- **Retention follows the session records.** The prune pass that removes
  key records idle past `keyRetentionAge` (`maybePrune`,
  `session_prune.go`) also removes a history directory whose mtime is older
  than the same age, mtime-only like the key pruning. Creating, renaming,
  and removing entry files updates the directory's mtime; no explicit touch. Rejected: no reaping, which leaves full
  message bodies on disk after `mailbox.history` can no longer reach them.
- **The tool description is the only shipped text added.** The
  lead-use-mailbox playbook is unchanged. The user knows when a rewind
  happened and can ask for a history check. (185)
  Rejected: a playbook line telling the lead to check history when mail may
  have been missed. It adds mirroring work for a rare case, and can be added
  later if agents fail to find the tool.

## Constraints

- Manuals: `ai-docs/manuals/ws-mcp.md` (edits under `internal/mcp/`) and
  `ai-docs/manuals/shipped-surface-boundary.md`, since the tool's description
  is text that ships downstream.
- A missing history directory reads as an empty ring, without error. No
  existing stored record changes shape.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- `recordMailboxRecvContacts` in `mailbox_tools.go` records per-counterparty
  contacts after a drain. The ring append sits on the same recv and send
  paths.
- `wsmailbox.SortedContacts` orders by RFC3339 UTC stamps lexically. The ring
  sorts by file name the same way, with a finer stamp (see Write).
- `maybePrune` in `session_prune.go` is the mtime-only, marker-throttled
  prune pass the history directories join.

## Prior Decisions

- 3b89c6a5 (2026-11-06, commit): "The work is split in two (item 183). The ring and tool are mailbox capability that is also used for rewind recovery, and they touch wsmailbox and the mailbox tools. The panels touch wswhiteboard..." — bearing: supports
- 261022-feat-mailbox-recent-peers-and-correspondents (2026-10-22, Decisions): "Storage: a `contacts` map on the caller's own `ReplyEntry`, keyed by counterparty address. It reuses the registry lock and the 30-day reaping..." — bearing: constrains
- 4284354a (2026-10-22, commit): "Recv records contacts in one separate WithReplyLock after both drains... A failed contact write is logged, not surfaced (drained mail must still be returned)." — bearing: constrains
- 4284354a (2026-10-22, commit): "Consequence: a send records the contact before the queue append, so a failed delivery can leave a contact with no delivered mail. Accepted: folding the queue append in too would widen the change for a rare path." — bearing: constrains
- 261022-feat-mailbox-recent-peers-and-correspondents (2026-10-22, Decisions): "Correspondents capped at 20, newest first, matching the named-peer cap. Stored contacts are trimmed to the 20 most recent on each write. Rejected: unbounded history..." — bearing: supports
- 5fac273b (2026-10-17, commit): "The test passes as-is (-race -count=20): recv's drain is already a load-modify-save under the store flock (WithLock / WithReplyLock), so no fix to recv or the store was needed." — bearing: constrains
- 260913-feat-cross-session-mailbox-core (2026-09-13, Result): "`mailbox.recv`'s drain-then-persist ordering accumulates into block-local slices and only merges into the returned result after the store write is confirmed error-free..." — bearing: constrains
- 261106-feat-whiteboard-dashboard-panels (2026-11-06, Decisions): "Mailbox panel... Correspondents with their last exchange time..." and the recent-messages list reads this ticket's ring — bearing: supports

## Route Facts

| fact | allowed values | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/mailbox_tools.go recv and send paths, a new history store beside them (per-entry files under wsstate.CacheRoot), internal/mcp/session_prune.go, internal/mcp/server.go tool registration |
| scope.surface | public-interface | new shipped MCP tool mailbox.history and its description |
| scope.new_public_symbol | yes | mailbox.history |
| scope.new_type_contract | yes | the on-disk history entry file and the history entry shape direction, counterparty, time, title, body |
| scope.test_surface | existing | internal/mcp/mailbox_tools_test.go, internal/mcp/mailbox_correspondents_test.go |
| complexity.reuse_points | confirmed | recordMailboxRecvContacts call site mailbox_tools.go#L217, wsstate.CacheRoot as used by keysDir in session_auth.go, UTC stamps as in wsmailbox.SortedContacts |
| complexity.side_effect_risk | low | every send and recv now also writes one new file and trims its directory; no existing store is touched, and per-entry files need no lock |
| risk.correctness | moderate | append must follow confirmed drain writes on two queues with partial-success handling; trim is best effort and must never fail the send or recv |
| risk.fit | low | follows the existing contacts recording sites, their logged-not-surfaced failure handling, and the mtime-only prune pass |
| risk.test | moderate | mailbox tests are known to read ambient WS_MAILBOX env per 261007-bug-mailbox-tests-leak-ambient-ws-mailbox-env and internal/mcp suite is slow |
| risk.security_or_contract | moderate | new shipped tool schema plus full message bodies persisted in per-session directories under the cache root |

## Phases

### Phase 1: history ring and tool

Add the ring on the recv and send paths, and register `mailbox.history` with
the arguments above.

Verification:
- `internal/mcp` tests cover:
  - recv appends the drained messages, from both the named inbox and the
    reply-id queue.
  - send appends the sent message.
  - `history` returns only the newest 10 after more than 10 exchanges.
  - `history` returns entries newest first.
  - `body: false` returns the first line as the title and no body;
    `body: true` returns the full body.
  - `direction` filters entries.
  - `limit` caps the result.
  - `history` leaves the queues and the ring unchanged.
  - A missing history directory reads as empty.
  - On disk, writes beyond 10 leave only the newest 10 entry files.
  - The entries of one multi-message drain keep queue order.
  - The title skips a leading peer-lead note line; an empty body gives an
    empty title; a long first line is cut at 120 characters.
  - `limit` below 1 returns the default count.
  - A failed ring write is logged and the send or recv still succeeds.
  - The prune pass removes a history directory older than the retention
    age and keeps a fresh one.
  - Concurrent appends to one session from two writers (for example two
    goroutines, 5 entries each) all land.
  - An unreadable or unparsable entry file is skipped.
  - A failed trim does not fail the send or recv.
  - Tests point the cache root at a temp directory (`WS_CACHE_HOME` or an
    explicit root), never the developer's real cache.
- `go test ./internal/mcp -run 'Mailbox' -count=1`, then `go test ./...` in
  `agents-plugin-tool` (`internal/mcp` takes several minutes).

### Result (b1771b9df) - 2026-11-06

Landed the per-session history ring and the read-only `mailbox.history`
tool.
- `internal/mcp/mailbox_history.go`: one file per entry under
  `<cache root>/mailbox-history/<session-key>/`, written temp+rename to
  `<nanosecond UTC stamp>-<random hex>.json`; best-effort trim to the
  newest 10; read newest first, skipping unreadable/unparsable/unstamped
  entries; the title rule; the tool handler.
- `mailbox_tools.go`: send appends after the enqueue succeeds; recv appends
  the confirmed-drained messages in the order it returns them. Ring write
  failures log `mailbox.history_write_error` and are never surfaced.
- `server.go`: dispatch and schema for `mailbox.history`. `session_prune.go`:
  `maybePrune` also reaps history directories whose mtime is older than
  `keyRetentionAge`.
- `mailbox.history` added to the ws, wsflow, and Pi `runtime.json`
  contracts (the wsflow capability test requires an exact match).

Verification: `go test ./internal/mcp -run 'Mailbox' -count=1` ok;
`go test ./...` in `agents-plugin-tool` ok (internal/mcp 250s);
`python3 -m unittest` of the wsflow runtime contract and the
shipped-surface neutrality tests ok. Lite review: clean, one minor
(the failed-write test counts events in the capped process-wide debug
buffer; only theoretical flakiness).

Decisions:
- The entry file stores direction, counterparty, and body only; the time
  is derived from the file-name stamp, the single source.
- Recv entries follow recv's returned order (its stable `SentAt` sort),
  which keeps each queue's order and matches what the session saw.
- Text output by default with `format: "json"` opt-in, matching the
  sibling mailbox tools; `body: false` omits the JSON `body` field.
- Tests went to a new `mailbox_history_test.go` instead of the existing
  mailbox test files the route facts named.
