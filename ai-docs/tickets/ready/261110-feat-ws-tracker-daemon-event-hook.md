---
title: "Tracker: the dashboard daemon runs a configured local command on tracker events"
parent: 261110-epic-ws-tracker-team-feedback
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 48e74a17552cf078
sage-review-completeness-reviewed: 48e74a17552cf078
---

# Tracker: daemon event hook

## Background

From the team introduction feedback (user, 2026-11-10 whiteboard, topic
"tracker 팀 피드백"): connect the tracker to mail. Mail integration is
downstream work. The user's direction is for the daemon to expose
tracker-related hooks that a downstream setup or a client plugin can use;
per-PC desktop notifications (for example Windows toasts) can later use the
same hook. Today the daemon pushes tracker changes only to browser pages
over SSE, and nothing reaches an outside program.

## Decisions

1. **The hook is a configured local command that the daemon runs, with the
   event as JSON on stdin (lead technical decision, user ok on (615)).**
   Rejected: an HTTP webhook, or an event-stream endpoint for clients to
   subscribe to. Those need a server or an open port and fail offline. A
   local command works offline, a Windows toast is one PowerShell script, and
   a webhook can still be made from the script with curl.
2. **By default the hook receives only events that concern the local
   member, and a setting switches it to every store event (user, (616) c).**
   Events that concern the local member are: an assignment or request to me,
   and a comment or status change on an item I created or hold, plus a due
   date approaching on an item I hold. A "request" is a pending assignment
   to me. The member's own actions are excluded from the mine scope. The
   all scope delivers raw store events that every clone already holds; the
   epic's org-tree visibility rule covers per-member aggregates such as the
   balance view, not this stream, so the all scope is not filtered by the
   org tree (technical, review-raised). Rejected: local-member events only
   (a); every event only, leaving the filter to the script (b).
3. **Configuration (technical, (264); home, user, (284) c).** Hooks live in
   a dedicated file in the daemon state directory (named in planning, such
   as `hooks.json`), outside the config system: an array of entries, each
   with its command and its scope (mine or all). The due threshold of
   Decision 5 lives in the same file. No committed layer can reach it, so a
   repository file never chooses what runs. The daemon reads the file at
   start and on change, and an invalid file is logged and runs no hook.
   Rejected: a single hook, which makes a toast script and a mail script
   branch inside one script; extending the config manifest schema with
   array and object types ((284) a), a cross-cutting config interface
   change; one JSON-string key in the dashboard config that ignores the
   repo scope ((284) b), which keeps a committed layer one bug away from
   choosing the command.
4. **Versioned event JSON (technical, (265)).** The event carries an integer
   `v`, starting at 1; adding fields keeps `v`, changing a field's meaning
   bumps it. Script authors ignore `kind` values they do not know, and the
   playbook says so, so a later kind (such as claim events) needs no bump
   (technical, review-raised).
5. **Due approaching (user, (266) b).** The threshold is a setting with a
   24-hour default. A due event fires once per (item, due date), recorded in
   a daemon-local state file. Rejected: a fixed 24 hours ((266) a).
6. **Catch-up after downtime (user, (267) b).** When the daemon starts, it
   delivers the events since the last one it delivered, dropping events older
   than 24 hours. Rejected: at-most-once ((267) a), which loses an
   assignment that arrived while the daemon was down; unbounded catch-up,
   which floods toasts after a long downtime.
7. **Execution (technical, (268)).** Each hook has its own serial queue, so
   its events arrive in order; a run times out after 30 seconds; a failure
   is written to the daemon log and not retried. Rejected: parallel runs,
   which break order; retries, which fire twice.
8. **Dedup by ledger event id (technical, (269)).** A per-store daemon-local
   cursor file, the same state catch-up reads, records the ledger's own
   `Cursor` (per-stream watermarks plus `Gen` and `Epoch`, as
   `Ledger.Events(after Cursor)` takes it) and the ids of every event
   processed in the last 24 hours, delivered or filtered out, so an event from a teammate's clone that syncs late
   is still delivered and a reload never delivers one twice. When the
   `Epoch` changes and `Events` returns everything again, events older than
   24 hours are dropped (Decision 6) and the recorded ids are skipped. On a
   first start with no cursor file, delivery begins at the current head
   (technical, review-raised). The `Epoch` is minted each time a Ledger is
   opened (`wsledger/store.go`), so a saved Cursor places events only within
   one Ledger instance; across a daemon restart the 24-hour window and the
   processed-id set are what prevent redelivery and backfill. The mine
   filter is evaluated against the fold state at the event, not at
   delivery, so a restart re-reading an event reaches the same verdict. An
   id stays recorded until both its processing time and its `ts` are older
   than 24 hours. The cursor and the id set advance when an event is queued
   to every hook; at shutdown, queued events not yet run are dropped
   (at-most-once beyond the queue) (technical, review-raised). Rejected: diffing views across reloads; a
   single last-delivered event id, which cannot place events from several
   streams and loses late-synced ones.
9. **Event JSON fields (user, (281)).** `v`; `id` (the ledger event id);
   `kind` (`assigned`, `assign_pending`, `comment`, `status`, `closed`,
   `due_soon`); `ts`; `store` {`id`, `name`}; `item` {`id`, `alias`,
   `title`, `kind`}; `actor`; `target` (optional); `detail` {`status`,
   `comment` (its first 500 characters), `due`}; `url`. The tracker
   playbook documents the shape for script authors. The all scope delivers
   the same six kinds without the per-member filter, with `due_soon` for
   every item in the store that has a due date and an assignment not done;
   it adds no other kinds. A `due_soon` event, which comes from the clock,
   takes the stable id `due:<item id>:<due date>`. `actor` and `target` are
   member objects {`id`, `name`}; `ts` and `due` are RFC 3339; `url` opens
   the item in the dashboard (technical, review-raised). Rejected: passing
   raw ledger events, which ties scripts to the internal event format.
10. **Running the command (technical, review-raised).** The command is an
    argv array run without a shell; its working directory is the daemon
    state directory. A timeout kills the process tree. Each hook applies to
    every store the daemon serves, and the event names its store. When the
    hooks file changes, a running hook finishes, the queues of removed hooks
    are dropped, and added hooks start from the current cursor with no
    backfill. The 24-hour drop of Decision 6 applies to the catch-up read at
    daemon start and after an `Epoch` change, measured as event `ts` against
    the current time; events seen while the daemon runs are delivered
    whatever their `ts`, so a late-synced teammate event is not lost
    (technical, review-raised). Rejected: a shell string, which adds quoting
    and injection surface for no stated need.

## Constraints

- The command runs with the daemon's environment and privileges. It is
  configured locally by the machine's own user, never taken from store
  content, because a store is shared and its content must not be able to
  choose what runs.
- The daemon process code is shipped surface:
  `ai-docs/manuals/shipped-surface-boundary.md` applies.
- `261110-feat-ws-tracker-self-assign-claim` may land first: if it has, the
  mine scope also covers a take request on the member's assignment or item,
  and a claim or approval that affects the member; if it has not, the claim
  ticket adds them when it lands. A claim or approval that gives the member
  an assignment is `assigned`; a take request on the member's assignment
  or item is a new kind, `take_request`.

## Prior Decisions

- 25d11857 (2026-11-10, commit): "Children carry only the settled answers: (613) a subtree visibility, (614) a current load only, (615) ok local-command hook, (616) c member-scoped default plus an all-events setting." — bearing: supports
- 261109-bug-ws-tracker-external-write-refresh (2026-11-10, Result): "Runner hook: wsagent.Event.TrackerWrite is raised by the Claude and Pi parsers on a non-error tracker.update result ... conv.handle calls wsagent.Options.TrackerChanged." — bearing: supports
- 261108-feat-ws-tracker-drawer-ui (2026-11-08, Decisions): "Local UI state split by kind: what the viewer has seen - notification read marks, handle-badge seen marks - is stored per clone on the local filesystem by the daemon, never in browser storage" — bearing: constrains
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Local UI state: what a viewer has seen ... lives per clone on the local filesystem outside git, read and written by the daemon; it is never an event and never browser storage." — bearing: constrains
- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Decisions): "Daemon-local state file: the daemon keeps the store-to-conversation mapping ... in a local state file on disk, per machine, outside git, following local UI state (451) a." — bearing: constrains
- 78af82e67 (2026-11-08, commit): "Notification mapping uses event ids already in the view state: assign / pending -> assignment.via; accepted / handoff -> accepted.id; ... reply / mention -> the comment id" — bearing: supports
- 261110-feat-ws-tracker-support-agent-ui-commands (2026-11-10, Decisions): "UI commands travel as markers in the conversation stream, the same path as cards, not as a new SSE event" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/ (daemon.go wiring, a new hook runner), agents-plugin-tool/internal/wstrackerhost/ (event source via Ledger.Events), the daemon state directory hooks file, tracker playbook copies |
| scope.surface | public-interface | a new user-configured hook command and its stdin event JSON are a downstream-facing contract |
| scope.new_public_symbol | yes | the hooks file format (Decision 3); file name chosen in planning |
| scope.new_type_contract | yes | the versioned event JSON (Decisions 4, 9) and the hooks file shape |
| scope.test_surface | new | hook runner tests with a fake command beside agents-plugin-tool/internal/wsdashboard/*_test.go |
| complexity.reuse_points | confirmed | wsledger Ledger.Events(after Cursor) and Cursor (store.go#L1282, types.go#L134); tracker change signal TrackerChanged in wsdashboard/sse.go |
| complexity.side_effect_risk | high | the daemon spawns an external process with its own environment and privileges on store-driven events |
| risk.correctness | high | per-stream cursor dedup, once-only due events, epoch changes, and catch-up after downtime (Decisions 5, 6, 8) |
| risk.fit | low | a daemon-local file outside the config system (Decision 3) |
| risk.test | moderate | process spawning, timeouts, and time-based due events need deterministic seams (an injectable clock and runner) |
| risk.security_or_contract | high | command execution must never be selectable from shared store content, and the event JSON becomes a versioned contract |

## Phases

### Phase 1: hook

Scope:
- Decisions 1-10 in the dashboard daemon: the hooks file and its reload,
  event derivation and filtering, the per-hook queue and runner, the cursor
  and due state files, catch-up, and the playbook text documenting the file
  and the event JSON for script authors.

Verification:
- Tests with a fake hook command: a mine-scoped hook receives an assignment
  to the local member and not an unrelated event or the member's own
  action; an all-scoped hook receives both; events arrive in order; a timeout and a failing command are logged
  and the next event still runs; a reload delivers no duplicate; after a
  restart, events since the cursor are delivered and events older than 24
  hours are not; an event from a second clone synced after later local
  events is still delivered; an `Epoch` change redelivers nothing already
  delivered; a first start delivers nothing from before it; a due event
  fires once at the configured threshold; a restart after adding an
  all-scope hook delivers no backfill; a restart delivers no event the mine
  filter dropped at the time.
- An invalid hooks file is logged and runs nothing; a changed file is
  picked up without a restart.
- The playbook's documented event shape matches Decision 9, and the
  playbook mirror test passes for its copies.
- A hook applies to two served stores; a timed-out hook's process tree is
  killed; an event with an old `ts` seen while running is delivered.
- The event JSON carries `v: 1` and the Decision 9 fields; the command runs
  without a shell in the daemon state directory.
