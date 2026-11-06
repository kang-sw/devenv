---
title: Mailbox history ring and mailbox.history tool
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
whiteboard discussion, items (167), (175), (176), (182), (184), (185).

## Decisions

- **One ring per session, both directions, full bodies.** Each session keeps
  the last 10 messages it exchanged, received and sent together. Each entry
  carries the direction, the counterparty address, the exchange time, and the
  full message body. Received messages are appended once recv's drain write
  has succeeded, under the same drained-after-confirmed-write discipline the
  queues use. Sent messages are appended once the send has been enqueued. The
  capacity is a constant, 10, raised later if recovery needs more.
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

  With `body: false`, each entry shows only the direction, the counterparty,
  the time, and a title. This is the light "did I miss mail?" check, the most
  common call. `body: true` adds the full body, for recovery after a rewind.
- **Title is the first line of the body.** The mail envelope has no subject
  field (`Envelope`: `from`, `reply_to`, `content`, `sent_at`).
  Rejected: adding a subject field to `mailbox.send`, because it changes both
  the envelope schema and the send schema.
- **Storage: the session record.** The ring lives in the session record, the
  per-session-key state that holds agenda and todos. That record already
  survives compaction and is written only by its own session. (184)
  Rejected: the machine reply-id registry entry. Every peer's send locks that
  entry, and it would hold every session's message bodies in one machine-wide
  file.
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
- Any field added to a stored record is additive. A store written before the
  field existed reads as an empty ring, without error.

## Prior Art

- `recordMailboxRecvContacts` in `mailbox_tools.go` records per-counterparty
  contacts after a drain. The ring append sits on the same recv and send
  paths.
- `wsmailbox.SortedContacts` orders by RFC3339 UTC stamps lexically. The ring
  can use the same stamps.

## Phases

### Phase 1: history ring and tool

Add the ring on the recv and send paths, and register `mailbox.history` with
the arguments above.

Verification:
- `internal/mcp` tests cover:
  - recv appends the drained messages, from both the named inbox and the
    reply-id queue.
  - send appends the sent message.
  - The ring keeps only the newest 10.
  - `history` returns entries newest first.
  - `body: false` returns the first line as the title and no body;
    `body: true` returns the full body.
  - `direction` filters entries.
  - `limit` caps the result.
  - `history` leaves the queues and the ring unchanged.
  - A record without the ring field reads as empty.
- `go test ./internal/mcp -run 'Mailbox' -count=1`, then `go test ./...` in
  `agents-plugin-tool` (`internal/mcp` takes several minutes).
