---
title: "Mailbox: notify the sender when a message is not read within 3 minutes"
completed: 2026-11-13
related:
  261111-feat-ws-mailbox-wait-without-process-relaunch: "the mailbox wait path a wake notice would ride; no shared decision"
---

# Mailbox: notify the sender when a message is not read within 3 minutes

## Background

User feedback (whiteboard, 2026-11-12): a session sent mail to a peer whose
session had already exited, and waited all night without knowing the message
never arrived. The user asked that when a sent message is not delivered
within 3 minutes, the sender receive a "message not delivered" notice that
wakes it for a new turn. Delivered means the recipient agent successfully
read the message, not that it replied.

Evidence (source read 2026-11-13, under `agents-plugin-tool/`):

- `mailbox.send` appends an `Envelope{From|ReplyTo, Content, SentAt}` to the
  recipient's queue (named inbox or reply-id queue), wakes the waits, and
  records a `sent` entry in the sender's history ring
  (`internal/mcp/mailbox_tools.go:60-130`; `Envelope` at
  `internal/wsmailbox/store.go:82-87`).
- A message is read exactly when `claimMail` removes it from the queue
  (`mailbox_tools.go:175-200`), used by `mailbox.recv` and by the daemon's
  delivery to hosted leads (`internal/mcp/lead_mail.go`); the claim writes a
  `recv` entry to the recipient's ring. Nothing links that entry back to the
  sender's `sent` entry, and the ring keeps 10 entries
  (`internal/mcp/mailbox_history.go:30-51`). So "still in the queue" is the
  only durable, exact "unread" signal.
- Presence has `LastSeen`/`PID` (`store.go:66-77`); a reply-id queue has a
  listening marker only while a wait runs (`internal/wsmailbox/listening.go`).
  A gone recipient is therefore a symptom, not a separate signal: its mail
  simply stays queued.
- The runtime already posts system mail from no session:
  `wsmailbox.PostSystem` (`internal/wsmailbox/replyid.go:156`, used at
  `internal/mcp/pi_tools.go:498`); the caller wakes the waits. The dashboard
  daemon is the one process that runs for every host, hosts the waits, and
  holds the wake hub (`lead_mail.go:77-140`,
  `internal/wsdashboard/daemon.go:101`).
- The sender's wake: an idle sender is woken by its own mailbox wait (or
  push, or the daemon for hosted leads) on any mail to its own reply-id or
  named queue, so a system mail there wakes it with no new path.

## Proposed Approach

The daemon sweeps the mailbox queues periodically. An envelope still queued
`N` minutes after `SentAt` (default 3) that has a sender (`From` or
`ReplyTo`) and no notice yet gets one system mail posted to the sender's
queue ("message to <recipient> sent <time> was not read; the recipient
session may have exited"), carrying the message's first line. The envelope
is marked notified with an optional field so the sweep never repeats. The
message stays queued: a late read still delivers it.

Compatibility tier: T0 - one optional `Envelope` field and one optional
config key (`dashboard.mailbox_undelivered_minutes`); no new tool or flag. Caveat: an older binary that round-trips
the store may drop the field and allow one duplicate notice; the notice is
idempotent in meaning, so this is tolerated within a minor.

## Constraints

- A late read after the notice gets no follow-up notice (see `## Decisions`).
- Manuals: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/skill-authoring.md`.
- Notices never trigger notices: system mail (no `From`, no `ReplyTo`) and
  notice mail are never swept.

## Phases

### Phase 1: undelivered sweep and notice

Daemon sweep over the reply-id and named queues; post the notice with
`PostSystem` into the sender's queue, wake the waits, mark the envelope.
Tests with a fake clock: unread past N notifies once; a claim before N never
notifies; system mail never notifies.

### Result (c9ae4d757) - 2026-11-13

Implemented a daemon-owned 30-second sweep over reply-id, machine, worktree,
and clone queues, including queues without live dashboard sessions. Notices
use the system-mail append path and wake the existing mailbox hub. The optional
`undelivered_notified` marker is persisted under the source lock; original mail
is not explicitly removed, and claims need no new hook. Tests advance an
explicit clock through early/overdue/repeated sweeps, early and late claims,
system-mail exclusion, all supported return routes, and bounded self-mail.

Remote `w5090` full `go test ./...` completed with only the known failure
recorded under Forward below. Self-review refinement `bdd24e921` preserves
successful notice markers when another destination fails; its regression and
the full wsmailbox package passed remotely.

### Phase 2: configuration

Add `dashboard.mailbox_undelivered_minutes` (integer, minimum 1, default 3) to
`agents-plugin/dashboard-config-manifest.json` and
`agents-plugin-wsflow/dashboard-config-manifest.json`; the daemon sweep reads it
(re-read per sweep so a tune applies without a restart); the manifest parity
test must pass. Document the key in the mailbox playbook (generic text only; if
playbook text changes, mirror per `ai-docs/manuals/wsflow-mirroring.md` and read
`ai-docs/manuals/skill-authoring.md`). Test: a threshold tune changes when the
notice fires.

### Result (95f335508) - 2026-11-13

Added the global-only integer key (minimum 1, default 3) to ws and wsflow,
and to Pi because the existing three-package parity contract requires it.
The daemon reads the global value on every sweep; tests verify a live threshold
change, invalid-value fallback, scope refusal, and startup delivery/wake without
listed sessions. The mailbox playbook documents the notice and its return
routing; wsflow and Pi resources and content manifests are synchronized.

Remote `w5090` full `go test ./... -skip
TestHostedConnectionSeeingALANChangeReplacesTheDaemon` passed. wsflow package
checks passed (24 tests, 2 skips). After the self-review fix and fresh-reader
wording fixes, targeted mailbox, config, daemon wake, and resource-mirror checks
passed remotely. No changed Invariants/Constraints directives.

## Decisions

Compatibility tier: T0 (one optional `Envelope` field and one new optional
config key `dashboard.mailbox_undelivered_minutes`; no new tool or flag), per
`ai-docs/manuals/release-compatibility.md`.

- The dashboard daemon sweeps the queues, because it is the only always-running
  process that holds the wake hub and every queue, while an idle sender cannot
  run a timer of its own. Rejected: a timer in the sender's mailbox wait (absent
  for hosted senders and between model-armed waits).
- Undelivered means "still in the recipient's queue `N` minutes after `SentAt`",
  because the queue is the only exact, durable record of unread, and a gone
  recipient shows up as the same condition. Rejected: history-ring matching (no
  link between entries, 10-entry cap); a recipient-liveness trigger (no reliable
  liveness for reply-id queues).
- The notice is one system mail posted with `PostSystem` into the sender's own
  queue, deduped by an optional notified field on the queued envelope, because
  `PostSystem` already wakes idle senders on every host and the field keeps
  dedupe atomic with the queue lock. Rejected: a separate notice-state file (a
  second store to keep consistent).
- [policy] The threshold `N` is a config key, default 3 minutes:
  `dashboard.mailbox_undelivered_minutes` (integer, minimum 1, default 3),
  declared in `agents-plugin/dashboard-config-manifest.json` and in
  `agents-plugin-wsflow/dashboard-config-manifest.json` (same key; only
  `default` may differ per package). It sits in the dashboard namespace because
  the sweep runs in the dashboard daemon and a manifest key must carry its
  file's namespace (`claude.` in `config-manifest.json` is Claude-only). T0:
  a new optional manifest field. Rejected: a fixed 3 minutes (false positives
  against peers in long turns) and a fixed 10 minutes.
- [technical] A read after the notice sends no follow-up "delivered" notice; the
  notice already says the message may still be read. Rejected: a follow-up on a
  late claim, which adds a hook into `claimMail` and a second wake.
- Lead implementation clarification: `default_scope` is `global`, and other
  scopes are refused/ignored because one daemon serves the machine.

## Resolution

Both phases are complete. T0 remains unchanged: one optional envelope marker
and an optional config key; no new tool, flag, protocol, or claim behavior.
The work was implemented and self-reviewed once on
`impl/develop/mailbox-undelivered`.

### Forward

- Known environment failure: the user-declared w5090-only
  `cmd/ws-mcp TestHostedConnectionSeeingALANChangeReplacesTheDaemon` predecessor
  shutdown timeout occurred in the initial full check and was excluded from
  the final full check. No other failures remain; the holder tests passed.
- Minor, accepted storage limitation: when source and sender queues occupy
  different files, a crash or source-write failure after posting but before
  persisting the source marker can duplicate an idempotent notice. No second
  state store or transaction layer was added; old writers dropping the marker
  also retain the already-accepted duplicate-notice caveat.
- Minor fresh-reader wording follow-up: the playbook could explicitly say the
  threshold starts at send time and replace "tune" with "threshold change".
  The existing text adds no gate, and tests pin send-time-based behavior.
