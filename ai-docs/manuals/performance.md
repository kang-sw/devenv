---
summary: Required before changing how native tooling or the daemon stores data, touches files, or spawns processes - the five storage types (ring, SQLite, files), memory-first state, delta appends, whole-file retention, held handles, I/O-free interactive paths, and per-action file-operation measurement
---

# Performance

Target machines are corporate Windows PCs with heavy security software
(antivirus such as AhnLab V3). The scanner runs on every file touch, so each
file syscall costs 10-100x what it costs on a clean machine. The goal is the
fewest file touches per user action.

## Rules

New or changed persisted data first names its type from `## Storage Types`
and uses that type's store; the rules below then apply to that store.

1. **Memory is the source of truth; files are for recovery.** Reads come from
   memory only; files are read once, at startup. Why: a per-call or per-event
   reread makes every action O(N) in file I/O.
2. **Changes are delta appends.** One change is one appended line; an
   unchanged state writes nothing. Why: a rewrite-and-rename touches the file, a
   temp file, and the directory on every change, and a heartbeat rewrite
   repeats that with no new data. Pattern: a JSONL log with one `write()` per
   change.
3. **Retention deletes whole files.** Make each file self-sufficient for
   recovery, with a full snapshot as its first line, so recovery reads one
   file. Why: per-record files cost a directory listing and deletes on every
   add; in-place pruning rewrites the live file. Pattern: a ring of fixed-size
   segments; rotation writes the snapshot line into the new segment and
   deletes the oldest segment.
4. **Keep handles and locks open.** Open and lock once at startup and hold
   them until shutdown; no per-call open, lock, or `stat`. Why: each is a
   scanned file touch.
5. **Interactive paths do not wait on file I/O.** Chat send -> delivery ->
   processing start, and page load, complete from memory. Recovery trims a torn
   last line instead of relying on `fsync`. Why: a scanned write on the
   interactive path is user-visible latency. Pattern: update memory, append,
   return.
6. **Spawn a process only when no in-process route exists.** Why: a new
   process is scanned at launch and pays every file touch of its own startup.
7. **Decide by measurement.** Follow `## Measuring`.
8. **Exceptions need the owner's approval.** A change that breaks a rule above
   states which rule, why, and its measured cost, and waits for that approval.

## Storage Types

Classify data by how it grows and how it is read, not by which feature owns
it.

| Type | Shape | Store | Examples |
|---|---|---|---|
| 1. Append-only log | Grows by appending; read back by recent range or replay; never queried by field | Segment ring: append to the newest segment, in-memory index, drop the oldest segment whole | Session transcripts, mailbox, audit events |
| 2. Small mutable state | Few records changed in place | SQLite | Agent records, permission and support records, dashboard UI state, tracker cursors, session key store, live snapshots |
| 3. Queue | Enqueued, claimed, removed | SQLite | Dashboard inbox |
| 4. Queried structured history | Kept and queried by field, filter, or aggregate | SQLite | Whiteboard boards, usage numbers |
| 5. Blob or Git-tracked file | Opaque content, or a file people or Git read directly | Plain file | Attachments, tickets, notes, config files |

- **Type 1 capacity.** Every ring has a per-session byte cap (drop the oldest
  segment), a per-store total cap that evicts whole sessions in
  least-recently-used order, and a time retention after a session's last
  activity. All three are knobs; segment size is an implementation choice.
- **Types 2-4 share one SQLite discipline.** Open the database once and hold
  the connection (Rule 4); serve reads from memory where the data fits
  (Rule 1); one change is one WAL commit, which stands in for Rule 2's
  appended line; an unchanged state commits nothing.
- **Type 5 stays a file** because something outside the daemon reads it. Do
  not move a file another process reads raw into SQLite without changing
  that reader first.
- **Unsure between 1 and 4** → type 1 unless a caller filters or aggregates
  by field over the whole history; search over a ring scans the in-memory
  index.

Measured on the whiteboard board store (ticket
`261119-refactor-ws-whiteboard-sqlite-board-store`, macOS): moving one board
from a per-record folder to SQLite took a steady update from 103 to 19
wrapper file operations, and p50 latency from 63.7 ms to 1.25 ms.

## Worked Example: Mailbox Store

Ticket `261117-feat-ws-mailbox-memory-first-store`; commits `e973b72b4`
(memory-first store with a segmented JSONL restore ring), `10f95a154`, and
`898f0a872` (follow-up fixes) on `track/agent-v2`. "Before" is `develop`
prior to that work.

| Concern | Before | After |
|---|---|---|
| Reads | Every call locked the file and reread all of it | Memory only; zero disk access |
| Writes | Each send, recv, and heartbeat rewrote the whole JSON and renamed it | One change is one JSONL line append (a single `write()`) |
| Expiry | One file per record; a directory listing and deletes on each add | In memory; on disk, the oldest 1 MiB segment of a ring of 10 is deleted whole |
| Handles and locks | Open/lock/close per call | Held for the daemon's lifetime |
| File operations per send | 99.8-106 | 1 append, plus about 12 at segment rotation |

Each segment's first line is a full snapshot, so recovery replays only the
newest segment. A graceful close writes the legacy files once, so an older
version can still read the store after a downgrade. This one-time whole-file write at shutdown is an owner-approved
exception to Rule 2 under Rule 8, made for downgrade compatibility and off the
interactive path.

## Measuring

- **Unit:** file operations per user action (one send, one page load), counted
  before and after the change with a syscall or file-activity tracer.
- **Baseline:** a Windows PC running heavy security software. A clean machine,
  Linux, or fixture CPU time hides the scanner cost and is supporting
  evidence only.
- **Latency:** time from the action to its visible effect (send -> delivery ->
  processing start; request -> rendered page) on that same baseline.
- **Report:** the before/after counts and latency in the commit or ticket
  result, next to the action they were measured on.
