---
title: Mailbox lookup_peers lists recent peers and the caller's correspondents
related:
  260913-feat-cross-session-mailbox-core: reverses its lookup_peers liveness filter (Decision 2) for listing only
  260913-research-cross-session-mailbox: identity, durability, and reply-id design source
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 519271ac53fe59ee
sage-review-completeness-reviewed: 519271ac53fe59ee
---

# Mailbox lookup_peers lists recent peers and the caller's correspondents

## Background

`mailbox.lookup_peers` lists only peers that are live right now: a presence
record survives the filter only while its PID is alive and its `last_seen` is
within the 10-minute liveness window (`mailboxPresenceLive`,
`agents-plugin-tool/internal/mcp/mailbox_runtime.go`). Dead records are
silently dropped from the listing (`handleMailboxLookupPeers`,
`agents-plugin-tool/internal/mcp/mailbox_tools.go`). In practice that
condition is too narrow: an agent that was corresponding with a session that
has since gone idle, crashed, or compacted cannot find the address again,
although `mailbox.send` queues mail without any liveness check and the
recipient drains it on return.

A second gap: reply-id sessions (every session that ever sent mail, with no
`WS_MAILBOX`/`WS_MAILBOX_AUTO` identity) never appear in `lookup_peers` at
all. They live in the machine-tier reply-id registry
(`mailbox-replyids.json`, `agents-plugin-tool/internal/wsmailbox/replyid.go`),
whose `ReplyEntry` holds only `last_seen`. On the maintainer's machine no
named `mailbox.json` exists at all, while the reply-id registry holds 12
sessions, so the live-named-only listing is effectively always empty.

The goal is "I was last talking to that session" recovery: the agent sees the
sessions it recently dealt with, online or offline, and mails one.

## Decisions

- **No online/offline display.** `lookup_peers` output carries no live/dead
  status. Liveness stays an internal predicate used only for name-claim
  conflict detection and heartbeat ownership, as today.
  Rejected: an explicit on/off marker per peer (the user chose to drop the
  on/off notion from output entirely).
- **Named peers: most recent, regardless of liveness.** For the queried
  scope, list presence records ordered by `last_seen` newest first, capped at
  20 per queried scope, each showing its `last_seen`. Public discoverability
  remains gated by the `WS_MAILBOX`/`WS_MAILBOX_AUTO` environment exactly as
  today; nothing new becomes publicly discoverable.
  Rejected: keeping the live-only filter (too narrow, the motivating
  problem).
- **Offline auto-identity records are excluded.** A `WS_MAILBOX_AUTO` stem is
  re-minted on server restart, so mail to a dead auto address is never
  drained. A non-live record with `auto: true` is omitted; a non-live
  explicit `WS_MAILBOX` record is listed (a restart reclaims the same name and
  drains its queue).
- **Correspondents: reply-ids limited to the caller's own exchange history.**
  `lookup_peers` adds a correspondents list holding only the addresses the
  calling session (its session key) has actually exchanged mail with: the
  target of each `mailbox.send` it made and the sender of each envelope it
  received through `mailbox.recv`. This includes both `id:<reply-id>` and
  named `slug@scope` counterparties.
  Rejected: listing every reply-id in the registry (would make every
  mail-sending session discoverable to all sessions, reversing the
  reply-ids-are-not-discoverable design; the user limited it to the caller's
  own correspondence).
- **Correspondent history is scoped to the session key.** It survives
  compaction and `lead-revive` (same key, same derived reply-id) and starts
  empty under a newly minted key from a fresh parent-less ferrule.
- **Correspondents ordered by last exchange time**, newest first; each entry
  carries its last exchange time.
- **Correspondents carry recognition metadata.** Each correspondent records
  the counterparty's `harness` and `cwd` at exchange time, so the agent can
  tell sessions apart after compaction when only the four-word reply-id is
  left. To make that available for reply-id counterparties, reply-id
  registration (`publishReplyID`) also stores the registering session's
  `harness` and `cwd` on its `ReplyEntry`.
- **Correspondents ignore the `scope` argument.** The correspondence history
  lives in the machine-tier reply-id registry, so it is returned on every
  `lookup_peers` call whatever `scope` is queried. This is compatible with
  core Decision 13 ("no cross-scope enumeration"): every entry is an address
  the caller already held or was handed, which is the delivery side of that
  decision, not enumeration of other sessions.
- **Correspondent history follows reply-id retention.** It is reaped with the
  caller's reply-id entry under the existing rule (empty queue and
  `last_seen` older than 30 days, `reapStaleReplyIDs`).
- **Correspondents capped at 20**, newest first, matching the named-peer cap.
  Stored contacts are trimmed to the 20 most recent on each write.
  Rejected: unbounded history (a long-lived session's entry is never reaped,
  so it would grow without limit).
- **Storage: a `contacts` map on the caller's own `ReplyEntry`**, keyed by
  counterparty address. It reuses the registry lock and the 30-day reaping
  and is already keyed by the session-derived reply-id.
  Rejected: a separate store file (second lock and retention rule for the
  same lifetime).
- **Recv counterparty is the envelope's `From`, else its `ReplyTo`** - the
  same handle `mailbox.recv` already displays and replies to.
  Rejected: a separate sender field on the envelope.
- **No deduplication across sections.** A named peer that is also a
  correspondent appears in both lists; the sections answer different
  questions (scope recency vs. own history).
  Rejected: deduplicating, which hides one ordering.
- **Output shape.** JSON `self` is unchanged; each `peers[]` entry gains
  `last_seen`; a top-level `correspondents` array is added. Text output adds
  `last_seen=` to peer lines, replaces `no live peers` with `no peers`, and
  appends a `correspondents:` block.
- **No CONFLICT marker on a non-live record.** The conflict check is defined
  only between live processes (dead-PID override in `mailboxPresenceLive`).

### Implementation choices settled at design review

These were raised by Sage review as implementer-level gaps; the lead settled
them here so the worker does not re-derive them.

- `publishReplyID` merges into the existing `ReplyEntry` (refresh
  `last_seen`, `harness`, `cwd`; keep `contacts`) instead of replacing it.
- A contact write creates the caller's `ReplyEntry` when absent (a
  recv-only named-inbox owner has none) and refreshes its `last_seen`, so
  recording a correspondent counts as activity for 30-day retention. A
  `mailbox.recv` that drains nothing records nothing and creates no entry.
- Last exchange time is the caller's own send or recv time, not the
  envelope's `sent_at`; state this in the tool description.
- Correspondent JSON entry: `{"address", "last_exchange", "harness", "cwd"}`
  (`harness`/`cwd` omitted when unknown). Text block: a `correspondents:`
  line, then one line per entry
  `<address> last_exchange=<t> harness=<h> cwd=<c>` (absent fields omitted,
  matching peer lines); `no correspondents` when empty.
- The contact key is the literal counterparty address as sent to or
  received (`id:<reply-id>` or `slug@scope`); no cross-form normalization.
- Self-exclusion: mailing one's own reply-id or own named inbox records no
  contact. For named peers, self-exclusion runs before the 20 cap.
- Contact recording on `mailbox.send` folds into the same reply-registry
  lock write `publishReplyID` already performs, not an extra `WithReplyLock`
  round-trip. A named counterparty's presence record is read from its scope
  store before taking the reply lock, never nested inside it.

## Constraints

- `lookup_peers` JSON `self` must stay shape-compatible: the Claude mod
  (`agents-plugin/hooks/claude-mod/register.ts`, mirrored in
  `agents-plugin-wsflow/`) and the Pi waiter
  (`agents-plugin-pi/src/mailbox-waiter.ts`) read `self.address` to arm wakes.
- `mailbox.send` stays liveness-agnostic; this ticket adds no delivery check.
- Out of scope: presence-record GC or pruning of dead records; the 20-entry
  listing cap bounds output, and pruning would change store semantics beyond
  the listing goal.
- Named-inbox registration, conflict detection, PID-gated heartbeat, and
  owner rebind semantics are unchanged.
- Applicable manuals: `ai-docs/manuals/ws-mcp.md`
  (`agents-plugin-tool/internal/mcp/`),
  `ai-docs/manuals/shipped-surface-boundary.md` (tool descriptions and
  shipped skill text), `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (the `lead-use-mailbox` playbook and
  its wsflow/Pi mirrors).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 260913-feat-cross-session-mailbox-core (2026-09-13, Decisions): "Self-registration (2): ... a peer is dead in `lookup_peers` after a bounded number of missed heartbeats (or a stale pidfile) ... duplicate live name ... surface the conflict through `lookup_peers` output" — bearing: constrains
- 260913-feat-cross-session-mailbox-core (2026-09-13, Decisions): "Reachability / durability asymmetry (13): discovery is scope-bounded (visibility only; `lookup_peers` filters by scope, no cross-scope enumeration); delivery is capability-gated (need a reply-id/slug you hold)." — bearing: constrains
- 1b43fc57 (2026-09-13, commit): "address is discoverable-not-durable (per-server-process stem, new on restart); a self-address surface ... is mandatory because the address is unpredictable" — bearing: supports
- 06988235 (2026-09-13, commit, 260913-bug-mailbox-lookup-peers-self-leak): "a named inbox shares the same Presence map as every other peer in scope, so it needs an explicit exclusion by owned identity" — bearing: constrains
- c0e0816a (2026-09-13, commit, 260913-feat-cross-session-mailbox-core): "from:/reply-to: stamping and the piggyback badge are both gated by the same caller==owner check (mailboxOwnerCheck) ... must not be able to claim the named inbox's slug identity" — bearing: constrains
- 260917-feat-ws-pi-mailbox-waiter-slug-wake (2026-09-17, Decisions): "Resolve the self slug from `mailbox.lookup_peers`, never from env. At arm time the waiter asks the bridge client for `lookup_peers` and reads `self.address`" — bearing: constrains
- 732aeda3 (2026-10-05, commit): "the reply-id cutover changed the visible lookup_peers self value but no test asserted it ... Mutation-checked: appending a suffix to self[\"reply_id\"] makes TestMailboxSelfAddressSurface fail." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/mailbox_tools.go, mailbox_runtime.go, server.go, agents-plugin-tool/internal/wsmailbox/replyid.go, agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md plus wsflow and Pi mirrors |
| scope.surface | public-interface | mailbox.lookup_peers MCP output shape and tool description, shipped lead-use-mailbox playbook |
| scope.new_public_symbol | yes | lookup_peers JSON peers[].last_seen and top-level correspondents array |
| scope.new_type_contract | yes | wsmailbox.ReplyEntry gains harness, cwd, contacts map in mailbox-replyids.json |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/mailbox_tools_test.go, mailbox_runtime_test.go, mailbox_recv_concurrency_test.go |
| complexity.reuse_points | confirmed | wsmailbox.WithReplyLock, reapStaleReplyIDs, publishReplyID, mailboxPresenceLive in mailbox_runtime.go read |
| complexity.side_effect_risk | moderate | send and recv gain registry writes under the machine-tier reply lock shared by all sessions |
| risk.correctness | moderate | ordering, cap, auto exclusion, self exclusion and counterparty attribution on both send and recv paths |
| risk.fit | moderate | playbook text change must propagate to wsflow and Pi mirrors per wsflow-mirroring.md |
| risk.test | moderate | many behaviors to pin including restart survival and cross-session invisibility; ambient WS_MAILBOX env leaks into mailbox tests per 261007-bug-mailbox-tests-leak-ambient-ws-mailbox-env |
| risk.security_or_contract | moderate | correspondents must stay caller-scoped to keep reply-ids non-discoverable; JSON self consumed by Claude mod and Pi waiter |

## Phases

### Phase 1: Recent named peers and caller-scoped correspondents

Implement the Decisions above in `agents-plugin-tool/internal/mcp/`
(`mailbox_tools.go`, `mailbox_runtime.go`) and
`agents-plugin-tool/internal/wsmailbox/` (`replyid.go`, store types):

- `lookup_peers` named-peer listing: drop the live-only filter, exclude
  non-live auto records, order by `last_seen` desc, cap 20, expose
  `last_seen`; keep the caller's own named inbox excluded from peers as
  today.
- `ReplyEntry` gains `harness`, `cwd`, and the correspondents history;
  `publishReplyID` stamps `harness`/`cwd`; `mailbox.send` and `mailbox.recv`
  record the counterparty with last exchange time and its `harness`/`cwd`
  (from the named presence record or the counterparty's `ReplyEntry`, empty
  when neither is known).
- `lookup_peers` returns the caller's correspondents regardless of `scope`.
- Update the `mailbox.lookup_peers` tool description in
  `agents-plugin-tool/internal/mcp/server.go` (drop "live peers"/"Dead peers
  are filtered out"; describe recency listing, auto exclusion, and
  correspondents) and the `lead-use-mailbox` playbook source
  `agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md` plus its mirrors
  `agents-plugin-wsflow/rsrc/lead-use-mailbox/lead-use-mailbox.md` and
  `agents-plugin-pi/rsrc/lead-use-mailbox/lead-use-mailbox.md` per
  `wsflow-mirroring.md`.

Verification: Go tests in `agents-plugin-tool/internal/mcp` covering
- an offline explicit named record is listed with `last_seen`; an offline
  auto record is not; ordering is `last_seen` desc and capped at 20;
- no CONFLICT on a non-live record; live conflict behavior unchanged;
- correspondents appear after send and after recv (both reply-id and named
  counterparties), carry last exchange time plus `harness`/`cwd`, are absent
  for a session that exchanged nothing, are not visible to an unrelated
  session, survive a server restart under the same session key, and are
  returned for every `scope`;
- a self-send records no contact; repeated `publishReplyID` calls keep
  existing contacts; a recv-only caller's history is created and retained;
  the text-form `correspondents:` block and `no correspondents` empty state;
- JSON `self` unchanged (existing self-address tests keep passing).
Plugin package tests (`agents-plugin`, `agents-plugin-wsflow`, Pi) pass after
the playbook text change.
