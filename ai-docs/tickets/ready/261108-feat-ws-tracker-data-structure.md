---
title: "ws tracker data structure: Go domain layer (events, validation, fold, view state) over wsledger"
parent: 261107-epic-ws-personal-common-store
blocked-by: 261107-feat-ws-tracker-ledger-protocol
related:
  261107-feat-ws-tracker-ledger-protocol: prerequisite; the wsledger package this layer reads and writes through
  261107-epic-ws-personal-common-store: parent epic; Cross-Child Decisions bind this child
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 432e361218a76e73
sage-review-completeness-reviewed: 432e361218a76e73
---

# ws tracker data structure: Go domain layer (events, validation, fold, view state) over wsledger

## Background

The tracker epic splits into a payload-agnostic protocol layer
(`261107-feat-ws-tracker-ledger-protocol`, package
`agents-plugin-tool/internal/wsledger`) and a data-structure layer that gives
event payloads their meaning (board (235) a, (236) ok). The unstyled mockup
`ai-docs/ref/tracker-mockup/` settled the data structures: v3 (merged
d3275366c, 2026-11-08) applies the board decisions (268)-(293) and walks nine
user scenarios in `SCENARIOS.md`. Its `engine.js` is the reference
implementation and its README is the specification: Data model, Event types,
Assignments, Derived state, View state, Validation and concurrency, and the
Settled table.

The user scoped this child on 2026-11-08 (board (283) a): the Go domain layer
only, on top of `wsledger` — event types, validation, the fold, and the
derived view-state JSON matching the mockup's View state shape, with the
mockup fixtures and scenarios ported to Go tests. MCP and skill exposure
((115)) and the dashboard drawer UI go to the next child.

## Decisions

- **Specification source** ((283) a, (275) ok). The mockup README sections
  named above, the `engine.js` rules, and the Settled rows are this ticket's
  behavior contract; this ticket does not restate them. Where README and
  `engine.js` disagree, stop and report (`(c) contract broken`) rather than
  picking one. The decisions below are only what the mockup does not settle
  or what this ticket changes.
- **Package** ((296), lead technical call, assumed overnight under (282) a).
  New package `agents-plugin-tool/internal/wstracker`. It reads the
  protocol through a narrow interface holding only what it uses
  (`Events(after Cursor)`, `Concurrent(a, b)`, `Identity()`, plus `Append`
  for writes), which `*wsledger.Ledger` satisfies, so tests feed an
  in-memory source. Rejected: code inside `wsledger` (the protocol stays
  payload-agnostic, (236)); a tracker-named file set inside an existing
  package.
- **Layer split at the event boundary** ((236), (242)). The protocol owns
  the identity events (`member.join`, `roles.add_admin`,
  `roles.remove_admin`, `roles.add_key`, `roles.revoke_key`,
  `roles.remove_member`) and their verdicts. The domain owns
  `roles.set_lead`, `roles.set_priority_levels`, and every `item.*` and
  `comment.*` event in the README table. `member.join` replaces the
  mockup's `roles.add_member` stand-in ((231) b): a joined member is a root
  with no lead. An event the protocol marks Waiting, Flagged, or Ignored
  keeps that verdict; a protocol-Valid domain event then gets the domain
  verdict (applied or flagged with the README's reason) judged against the
  state folded from its causal past only.
- **Identity effects folded from the stream** ((296)). Admin status, removal,
  and membership used by domain rules are folded by the domain from the
  identity events in each event's causal past (their payload shapes are in
  the `wsledger` package doc), not read from `Identity()`, which is the
  current state only. Genesis `leads` and `priority` come from the genesis
  roles passthrough (`IdentityState.Genesis` / `Member.Leads`); when
  `priority` is absent the store uses three levels named 알림 / 업무 / 긴급,
  default 1 ((272) c, (277) c).
- **Removal effects** ((287)). A Valid `roles.remove_member` ends the
  member's live assignments as `unassigned` and pending ones as `withdrawn`
  in the fold. (209) "a removed member must lead nobody" is enforced through
  the protocol's veto hook: `wstracker` exports a `wsledger.VetoFunc`, and
  the Phase 2 open helper sets it on `wsledger.Options.Veto`; the hook
  computes leads from the past events it receives. The
  other half of (209), "not an admin", is already a protocol rule
  (`wsledger` flags `roles.remove_member` of an admin).
- **Pending handoff whose original ended** ((295) a, assumed overnight under
  (282) a). When the original assignment of a pending handoff ends another
  way (unassign, removal, return), the pending handoff is ended as
  `withdrawn` in the derived state, with no event; its `ended` points at the
  event that ended the original, and accept and decline of it are then
  flagged as "no pending assignment". An unassign, removal, or return of
  the original that is concurrent with an accept of its handoff wins, as
  removal beats writes in (213): the accepted handoff ends `withdrawn` and
  both events are marked (lead, auto-confirmed from design review). When
  the accept folds first, the ending event reverses it: the original takes
  the ending event's state (`unassigned` or `returned`), the handoff becomes
  `withdrawn`, and the new holder's later status writes show as valid but
  not in effect, as in (213). This
  changes the mockup v3 behavior (it stayed pending, accept flagged); this
  ticket updates `engine.js` and every README place that states the old
  rule: the "Open questions" paragraph, the `item.accept` row ("flagged if
  the original already ended"), the flagged-reasons bullet "accepting a
  handoff whose original already ended", and the state diagram's withdrawn
  edge, and removes the engine check that becomes unreachable. Rejected:
  keeping it pending.
- **View state** ((275) ok, (296)). `Derive(viewer stream, now, staleDays)`
  returns a Go struct whose JSON encoding is the README "View state" shape
  with the same field names, except `store`, `sync`, and `clones`, which the
  next child fills from the `wsledger` sync report. `heads` is derived from
  the received events. `STALE_DAYS` stays a caller option, default 3
  ((284)). Badge acknowledgement ("opening the store clears it") is caller
  state; the domain returns the badge keys only.
- **Write path** ((294) a, assumed overnight under (282) a). `Dispatch`
  validates each event against the viewer clone's current local state (the
  state its verdict will be judged on, since a new event's causal past is
  exactly what the clone holds) and refuses one that would be flagged,
  returning the README reason and appending nothing from that call. Valid
  events are appended through the source in one `Append`. A call is
  all-or-nothing, and each event is validated against the state including
  the call's earlier events. Identity events other than `member.join`
  (which stays on `wsledger`'s `Join`) are pre-validated with the domain's
  folded identity state, mirroring every identity rule in `wsledger`'s
  `verify.go` (admin-only, last admin, an admin as removal target, unknown
  or removed member, already admin or not admin, key in use, key already
  revoked or not held, already removed, malformed payload) plus (209); a
  key-fingerprint helper is an allowed pure accessor. When the clone holds a waiting event, the
  new commit's `seen` covers it, so a pre-validated event can still be
  judged differently once the gap fills; this is accepted and documented
  at the site (lead, auto-confirmed from design review). Rejected: the
  mockup's append-and-report-flagged behavior, which exists only to demo
  flagged events and would leave permanent noise in history.
- **Algorithm and scale** ((298)). Verdicts must equal "fold the event's
  causal past, then judge" and be deterministic across clients. Per-event
  causal-past replay as in the mockup is acceptable when a generated
  5,000-event store derives in under 5 s in `go test` on this machine;
  otherwise implement the incremental apply of the README engine note [181]
  (per-stream cursor, thread-local re-replay, full re-replay on a roles
  change). The test that measures it stays in the suite with a generous
  bound, not as a benchmark gate.
- **Verification by golden parity** ((297)). A Node generator script runs the
  mockup `engine.js` over `fixtures.js` (team-alpha and hana-personal) and
  writes Go testdata for a fixed set of viewers (ada.pc root, maya.pc and
  maya.lap lead, lena.pc and omar.pc members, hana's clone) at a fixed
  `now`. Per viewer clone it writes the commits that clone holds in the
  engine state (no simulated viewer-switch fetch), each event's
  protocol-owned verdict (every verdict the protocol owns: envelope flags,
  waiting, revocation cut, author not a member or removed, and the rule
  verdicts of identity events including the (209) veto), and the expected
  view state; the Go in-memory source is fed exactly that set. The fixture's
  `roles.add_member` stand-in (`ada.pc` seq 11, adding pia under raj) has no
  place in either layer, so the generator translates the fixtures in memory
  before running the engine: pia moves into genesis as a root and that
  event becomes `roles.set_lead {pia, [raj]}`, so her later entry into the
  "everyone within L2" range and the "blocked below" roll-up are still
  exercised through a domain event (lead, auto-confirmed from design
  review). Rejected: the Go package recognizing `roles.add_member`. The script is
  committed next to the testdata; `go test` needs no Node. `engine.js` and
  `fixtures.js` attach to `window`, so the generator supplies a `window`
  shim rather than changing them. Mock bookkeeping
  (`_local`, `_open`, `_late`, `events[].local/open/late`), the three
  caller fields, and `stats.commits` (a `wsledger` event carries no commit
  identity; the caller child fills it) are excluded from comparison, and any other excluded field
  is listed in the test with its reason. Rejected: hand-written expectations
  only, which cannot catch drift from the reference implementation.
- **No caller surface** ((283) a, (299)). No MCP tool, skill text, config
  key, dashboard drawer, or search index in this child; (115) lands with the
  next child. This supersedes the protocol ticket's line "(115) lands with
  the data-structure child"; that ticket is closed, so the epic's Child
  Tickets section carries the re-pointing.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/` (AGENTS.md Implementation Conventions). The package
  is designed for any downstream team using ws, not for this repository.
- Do not change `wsledger` behavior. A missing read capability (for example
  the genesis passthrough not exposing a field the domain needs) may be
  added to `wsledger` as a pure accessor with a test; anything beyond that
  is a stop.
- Read the `wsledger` package as merged (864d85305); the protocol ticket's
  API sketch was refined during its implementation.
- No new Go module dependency. Node is used only by the testdata generator.
- Mockup edits are limited to the (295) behavior change in `engine.js`,
  the README places listed under that decision, and the generator script;
  the mockup stays unstyled.
- Out of scope: MCP and skill exposure, the drawer, search indexes, config
  keys, multiple leads, snapshots, labels and tags ((278) a).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "Domain veto hook ((243) ok). Identity-event validation takes an optional domain veto hook; the tracker uses it for (209) "a removed member must lead nobody"." — bearing: supports
- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "Layer boundary ((236) ok). Identity (members, keys, admins) is protocol; lead relations and assignment reach are data structure." — bearing: supports
- 43564d58 (2026-11-08, commit): "Known follow-up for the data-structure child: one member's two clones editing one item body concurrently now conflicts (README's single-writer claim predates per-clone refs)." — bearing: constrains
- ac71f46d (2026-11-08, commit): "(288) is implemented as a filter before the ts winner (drop an agent head when the same member has a concurrent human head), not as a pairwise comparator ... the Go port needs an order-independent rule." — bearing: constrains
- ac71f46d (2026-11-08, commit): "(291) restores only when the original holder is still a member and holds no other open assignment on the item; otherwise the return just ends the handoff. (285) skips the notice for the person who did the return." — bearing: constrains
- ac71f46d (2026-11-08, commit): "Open (README): a pending handoff whose original ended another way stays pending; accept is flagged, decline or withdraw still work." — bearing: supports
- d3275366 (2026-11-08, commit): "Mockup v3 is the reference the Go data-structure child ports overnight (board (283) a); the README view-state shape is the future backend API ((275) ok)." — bearing: supports
- e11589f5 (2026-11-08, commit): "Stale = live, doing or blocked, no status write for 3 days by the mock clock; todo work is not stale ... Threshold is a constant." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | new agents-plugin-tool/internal/wstracker; edits to ai-docs/ref/tracker-mockup/engine.js and README.md; Node generator script and Go testdata |
| scope.surface | cross-module | new package reads agents-plugin-tool/internal/wsledger through a narrow interface and may add a pure accessor there; mockup edits under ai-docs/ref/tracker-mockup/ |
| scope.new_public_symbol | yes | wstracker Derive, Dispatch, the tracker type, and the view-state struct (new internal package) |
| scope.new_type_contract | yes | view-state struct whose JSON is the README View state shape; domain event payload types; the narrow source interface |
| scope.test_surface | new-files | search for agents-plugin-tool/internal/wstracker returned nothing; golden parity, scale, scenario, and wsledger integration tests are new; wsledger tests must keep passing |
| complexity.reuse_points | confirmed | wsledger Events, Concurrent, Identity, Append, Options.Veto (VetoFunc), IdentityState.Genesis, Member.Leads on develop since merge 864d85305 (agents-plugin-tool/internal/wsledger/types.go, store.go); engine.js is the reference port |
| complexity.side_effect_risk | moderate | new package plus engine.js and README behavior change for (295) that the golden parity depends on; no caller surface |
| risk.correctness | high | large rule set (assignment state machine, multi-value registers, causal-past verdicts, removal effects) that must match engine.js and be deterministic across clients |
| risk.fit | moderate | new package beside wsledger; identity pre-validation must mirror wsledger verify.go rules without changing wsledger behavior |
| risk.test | moderate | golden parity depends on a Node generator over a browser-only engine.js (window global) and on faking protocol verdicts for in-memory events |
| risk.security_or_contract | moderate | view-state JSON becomes the next child's API contract; verdicts must agree across clients; no auth or secret handling |

## Phases

### Phase 1: Domain fold, validation, and view state

Build `internal/wstracker` over the narrow source interface, tested with an
in-memory source.

- Domain event payload types and the README validation rules with the
  engine's flagged reasons, each judged against the event's causal past.
- The fold: items, assignments and their state machine (forced, pending,
  accept, decline, return, handoff with status carry-over (290) and restore
  (291), unassign and withdraw, removal (287)), comments, field writes as
  multi-value registers with the (203) display winner, the same-member
  rules (271), (288), (289), concurrent-change markers from `Concurrent`,
  removal against writes ((213)), roll-ups, derived notices (285), stale
  and overdue, priority display.
- `Derive` producing the view state struct.
- The (295) change in the mockup `engine.js` and README.
- The Node testdata generator and the golden parity tests for every listed
  viewer; unit tests for each flagged reason the engine words.
- The scale test.

Done when: the golden parity tests pass for every listed viewer, every
flagged reason has a unit test, and the scale test passes.

### Phase 2: wsledger binding, write path, and scenarios

Depends on Phase 1.

- A tracker type over the narrow source interface, with an open helper
  for `*wsledger.Ledger` that sets the (209) `VetoFunc` on the options:
  `Dispatch` with pre-validation ((294) a) and incremental refresh through
  the `Events` cursor including verdict changes behind it (revocation cut,
  filled gap).
- Go ports of the nine `SCENARIOS.md` scenarios as dispatch sequences with
  assertions on the view state, run over the in-memory source seeded with
  the translated team-alpha fixture (`wsledger` creates only a
  creator-only genesis, so a 14-member store cannot be seeded through it);
  the in-memory source simulates per-clone holdings and sync for scenarios
  6 and 7. A unit test calls the exported `VetoFunc` directly with a
  crafted `past` (a removal of a member who leads someone; one after an
  earlier `roles.set_lead` moved their follower away). Plus cases the fixtures lack: an admin
  handoff of a member's assignment (286) then `roles.remove_member`, a
  removal ending live and pending assignments, (295), and `Dispatch`
  refusing a would-be-flagged event with its reason.
- One integration test over a real `wsledger` store (temporary bare
  repository, real `ssh-keygen` keys, two writers) that dispatches, syncs
  between the writers, and checks both derive the same view state. This is
  the only real-ledger coverage.

Done when: the scenario ports, the extra cases, and the integration test
pass, and Phase 1's tests still pass.

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/...`,
`go test -count=1 ./internal/wstracker/ ./internal/wsledger/`. The golden
parity tests pass for every listed viewer, the scenario ports pass, and
`node --check` passes on the mockup scripts and the generator.
