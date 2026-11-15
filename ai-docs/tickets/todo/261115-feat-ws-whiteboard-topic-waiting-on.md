---
title: Whiteboard topic waiting-on dependencies with unblocked surfacing and reverse view
related:
  261115-feat-ws-whiteboard-topic-state-badge: coordinate; landed in develop 02a2e5722, owns the header badge and toolbar state filter this ticket extends without changing lifecycle values
  261115-feat-ws-revive-topic-state-summaries: coordinate; revive summaries read folded topic state and should list waiting targets once this lands
---

# Whiteboard topic waiting-on dependencies with unblocked surfacing and reverse view

## Background

Owner request (relayed by MAIN, 2026-11-15): "각 토픽 사이에 의존성을 거는
기능… 진행 중, 대기 중인 항목들이 대체 뭘 기다리고 있는건지 파악하고
싶은데" (a feature that puts dependencies between topics; the owner wants to
see what in-progress and waiting items are actually waiting for). Owner chose
option **(901)=a**.

MAIN's contract for (a):
- an optional `on` attribute on `<wb:state value="waiting" .../>` holding a
  space-separated list of `topic:<name>`, `ticket:<stem>`, `owner`,
  `exec:<N>` (consider making exec a free label);
- declarative: re-declaring `wb:state` replaces the whole list; no add/remove
  commands;
- rendered as chips on the topic;
- when a topic target closes or a ticket target reaches `.done`, the chip
  shows ✓, the topic surfaces as "진행 가능" (unblocked), and a lead nudge
  fires, reusing the existing overdue-nudge path;
- reverse view on the blocker topic ("이 토픽을 기다리는 곳 N");
- `owner` may be derived automatically when the topic has open policy/check
  items;
- T0: optional fields only, existing lines byte-compatible;
- coordinate with the topic-state badge (just merged) and topic_default_state;
- acceptance is the owner's visual check (UI-fast principle).

Today (develop `02a2e5722`, paths under `agents-plugin-tool/internal/`):
- `wb:state` grammar is `wswhiteboard/commands.go` ~22-40 (`value`, `owner`,
  `eta_minutes`); unknown attributes are refused atomically
  (`wsxmlcmd/wsxmlcmd.go` ~802-824). Values are `discussing`, `waiting`,
  `implementing`, `awaiting-confirmation`; only `implementing` takes owner
  and ETA (`wswhiteboard/topic_state.go` ~18-26, ~255-280).
- Topic state is folded from immutable system entries carrying
  `Entry.TopicState` raw JSON (`entries.go` ~26-43, `topic_state.go`
  ~57-130). A same-value declaration is a no-op (~290-300); the record
  validator refuses `set` with `from == value` and turns unknown ops into
  unsupported diagnostics (~188-196, ~242-249). `Entry` round-trips unknown
  members (`entry_json.go`); `State`/`TopicDoc` are typed structs.
- Badge: `assets/page.tmpl` ~42-48 renders `.topic-state > .stb` with
  `data-state`; `assets/ago.js` ~52-63 rebuilds `.topic-state` children when
  `[state, owner]` changes; toolbar state filter reads `data-state`
  (`wsdashboard/assets/cards.js` ~290-310).
- Nudges: daemon-owned scheduler `mcp/whiteboard_nudges.go` (30 s reconcile,
  pokes on board updates) -> `FolderStore.Notify` (`board_notice.go`
  ~230-283) -> `wsmailbox.PostSystem` to the owning lead. Receipt parsing
  accepts only `hourly`/`overdue` reasons (`board_notice.go` ~69-90).
- Closure: `wb:close` sets `ClosedTopics` (`store.go` ~708-750); render knows
  closure via `topicClosed()`; topic names resolve board-locally
  (`topics.go` ~53-65).
- Tickets: whiteboard has a host-supplied `TicketResolver`
  (`view.go` ~249-255); production resolves via `wsdoc.TicketStatus`
  (five `os.Stat` checks, `.done` -> `done`; `wsdoc/tickets.go` ~202-221).
- Open asks: `AskCount()` = open policy + open checks per topic
  (`view.go` ~357-390); `OpenAskCount()` board-wide (`store.go` ~160-174).
- No `topic_default_state` setting or contract exists at `02a2e5722`; the
  topic-state view tests forbid inventing a state for legacy topics.

## Decisions

- **(901)=a and MAIN's contract** as quoted in Background. Technical calls
  below are EXEC4's (MAIN delegated them, 2026-11-15).
- **Grammar.** `on` is an optional attribute of `wb:state`, valid only with
  `value="waiting"` (refused with any other value, as `owner`/`eta_minutes`
  are refused outside `implementing`). Its value is a space-separated list of
  targets: `topic:<name>` (resolved with the existing board-local topic
  resolution; an unknown topic or the declaring topic itself is refused; an
  already closed topic is accepted and shows resolved at once),
  `ticket:<stem>` (syntax `YYMMDD-category-name`; existence is not checked at
  apply, since the store has no ticket resolver - a missing stem renders as
  an unknown chip), `owner`, and `exec:<label>` where the label is free text
  matching `[A-Za-z0-9._-]{1,32}` (so `exec:3` still works). Duplicates are
  dropped keeping first occurrence; declared order is kept; at most 8
  targets; any invalid token refuses the whole transaction. Rejected: numeric
  `exec:<N>` bound to a session registry - no board-local executor identity
  exists (mailbox peers and hosted labels are not numbered), so binding would
  invent a registry for a label; add/remove commands - the contract is
  declarative.
- **Declaration semantics.** A `waiting` declaration's list is the whole
  list: with `on` it replaces the list, without `on` it clears it. A
  declaration whose value and normalized list both equal the current ones
  stays a no-op. Leaving `waiting` (any other value, or close) clears the
  list. Cycles between topics are allowed and simply render. Rejected:
  keeping the old list when `on` is omitted - contradicts whole-list
  declaration.
- **T0 encoding.** The list is an optional `waiting_on` member on a system
  entry: `{"v":1,"targets":[...]}` (normalized tokens). Entering `waiting`
  with `on` puts the member on the same state-set entry; a waiting->waiting
  list change appends a new system entry ("waiting on changed: ...") whose
  only machine payload is `waiting_on`, never a `TopicState` record. The fold
  takes the newest `waiting_on` while the folded state is `waiting`. Older
  daemons ignore the unknown member and show the plain system text; existing
  lines and records stay byte-identical. Rejected: a waiting->waiting `set`
  record (old validators refuse `from == value` and would turn it into an
  unsupported-state diagnostic); a new `TopicState` op (unknown ops become
  diagnostics in older readers); a field on typed `State`/`TopicDoc` (older
  writers reconstruct and drop it).
- **Target resolution.** `topic:` resolves when that topic is closed;
  reopening it un-resolves. `ticket:` resolves when `TicketStatus` returns
  `done`; `dropped` renders as a distinct dropped chip and also counts as
  resolved (nothing is left to wait for); a missing stem renders as unknown
  and does not resolve. `owner` resolves when the waiting topic has no open
  policy or check item (the existing `AskCount()` predicate; technical and
  announced items never count). `exec:` never resolves automatically; the
  agent re-declares. Rejected: treating `exec:` as resolved for readiness
  (it would surface topics as unblocked on no evidence).
- **Derived owner.** A waiting topic with open policy/check items shows an
  `owner` chip even when `on` does not list `owner`, styled as derived
  (render-only, never persisted); listing `owner` explicitly and deriving it
  render one chip. Rejected: persisting the derived target (it would go stale
  as asks settle).
- **Readiness ("진행 가능").** A waiting topic is ready when its effective list
  (declared plus derived owner) is non-empty and every target is resolved.
  Readiness is derived presentation only: the lifecycle value stays
  `waiting`, `data-state` is unchanged, the toolbar's waiting filter still
  includes it, and the topic gains `data-waiting-ready` plus a "진행 가능"
  chip. A target that un-resolves drops readiness live. Rejected: a fifth
  lifecycle value (changes the state contract and the filter); auto-leaving
  `waiting` (the agent owns state declarations).
- **Chips placement.** A server-rendered `.topic-deps` row follows
  `.topic-state` in the topic header, outside `.topic-state`, so the badge
  reconciler's child rebuild never deletes it; each chip names its target
  (topic chips link to that topic through the existing in-board fragment
  links, ticket chips show the stem) and carries ✓ when resolved. No new
  client timer. Rejected: chips inside `.topic-state` (needs a reconciler
  rewrite for no gain).
- **Reverse view.** A topic that some waiting topic on the same board lists
  as `topic:` target shows a "이 토픽을 기다리는 곳 N" chip whose disclosure
  lists those topics as fragment links; closed waiters and non-waiting topics
  are not counted. Rejected: cross-board or cross-session reverse lookup
  (topic names are board-local; it needs another identity contract).
- **Unblocked nudge.** The daemon nudge scheduler evaluates readiness during
  its existing reconcile and pokes; a topic's transition to ready posts one
  lead mail through the same eligibility gate, mailbox transport and
  enqueue-then-receipt ordering as overdue nudges, naming the topic and its
  resolved targets. Dedupe key is (topic dir, id of the entry holding the
  current `waiting_on`), so a re-declaration can nudge again and a
  re-block-then-resolve under the same declaration does not. Its receipt is
  a separate optional Entry member (`unblocked_receipt`) on its own system
  entry, never a new value in the existing receipt's reason list. Ticket
  targets are resolved in the scheduler through an injected resolver over
  the session root's ticket tree (`wsdoc.TicketStatus`), only for boards with
  waiting topics that have ticket targets, at the existing 30 s cadence.
  Completion is worktree-local: a ticket closed in another clone counts once
  it is merged into this worktree. Rejected: adding `unblocked` to the
  existing receipt reasons (older parsers reject the whole receipt and lose
  overdue/hourly dedupe); page-refresh watches as the trigger (no page may be
  open); a ticket-tree file watcher (new machinery for a 30 s-tolerant
  signal).
- **Playbook.** Document `on`, the target kinds, whole-list replacement and
  the meaning of "진행 가능" in the `wb:state` section of
  `agents-plugin/rsrc/whiteboard/whiteboard.md`, regenerated into the wsflow
  mirror and kept in step with `agents-plugin-pi/rsrc/whiteboard/whiteboard.md`.
  Wording tolerates older daemons that refuse `on` (the agent retries without
  it).
- **topic_default_state.** No such setting or contract exists at
  `02a2e5722`; dependency lists are only ever declared, never defaulted. A
  later default-state feature must not supply `on`.
- **Compatibility tier T0.** Optional `waiting_on` and `unblocked_receipt`
  Entry members, a new optional attribute, render-only chips; no folder schema
  bump, no MCP tool or argument change. Old plugin text with a new daemon:
  `on` is simply never sent. New plugin text with an old daemon: `on` is
  refused as an unknown attribute, and the playbook tells the agent to retry
  without it.
- **Acceptance.** Owner UI-fast principle (relayed by MAIN, 2026-11-15): the
  owner's on-screen check after merge and daemon swap; no screenshot matrix
  or capture-comparison round.

## Constraints

- No change to the four lifecycle values, the badge markup inside
  `.topic-state`, or the toolbar filter order.
- Owner run rules: offline env `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`;
  never the full `./internal/wsdashboard` package, only `-run`; never the
  shared dashboard daemon or port 29417; end with an owned process scan.

## Phases

### Phase 1: Waiting-on targets, chips, readiness, reverse view and unblocked nudge

Scope: `wb:state` `on` parsing and normalization; `waiting_on` entry member
and fold; target resolution and derived owner in the view; `.topic-deps`
row, "진행 가능" chip, `data-waiting-ready`, reverse-view chip; scheduler
readiness evaluation with injected ticket resolver, unblocked mail and
`unblocked_receipt`; playbook text in all three package copies; i18n keys in
every `wsi18n` table.

Verification:
- `wswhiteboard`: parse/refusal cases (non-waiting value, unknown topic,
  self, bad stem, bad exec label, >8 targets, duplicate drop); whole-list
  replace and clear; unchanged declaration no-op; leaving waiting clears;
  waiting->waiting change appends a system entry with `waiting_on` and no
  `TopicState` record; historical-writer byte preservation
  (`topic_state_history_test.go` pattern).
- View: chips with ✓ for closed topic and done/dropped ticket, unknown
  ticket chip, derived owner chip, readiness attribute and chip, reverse
  count excluding closed/non-waiting waiters, escaping; badge reconciler
  leaves `.topic-deps` intact (`topic_state_badge_test.go` pattern); state
  filter unchanged (`-run` only).
- Nudges: transition to ready posts one mail; re-declaration nudges again;
  re-block/resolve under one declaration does not; restart keeps the
  receipt; old overdue/hourly receipts still parse
  (`board_notice_test.go`, `whiteboard_nudges_test.go` patterns).
- `wsi18n` completeness tests; wsflow mirror drift tests.
- Acceptance: owner's on-screen check after merge and swap.
