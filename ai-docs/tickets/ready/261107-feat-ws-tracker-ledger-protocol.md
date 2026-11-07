---
title: "ws tracker ledger protocol: payload-agnostic signed multi-writer event log over git refs"
parent: 261107-epic-ws-personal-common-store
related:
  261107-epic-ws-personal-common-store: parent epic; Cross-Child Decisions (storage model, integrity, sync) bind this child
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1e676c1a6b0f890a
sage-review-completeness-reviewed: 1e676c1a6b0f890a
---

# ws tracker ledger protocol: payload-agnostic signed multi-writer event log over git refs

## Background

The tracker epic stores team and personal work in git: every writer appends
events to its own ref and every view is a client-side merge (epic
Cross-Child Decisions, "Storage model"). Tracker mockup v2
(`ai-docs/ref/tracker-mockup/`, README and `engine.js`) exercised the full
model in the browser. The user then split the work into two layers and asked
for the protocol layer first (board (235) a, 2026-11-07: "데이터 구조에 대해서는
제가 아직 그림이 선명하지가 않아서 일단 프로토콜부터 구현을 빼내서 먼저 하면
어떨까"):

- **Protocol layer (this ticket):** per-clone refs, commits, signing and
  verification, membership and keys, causality (`seen`), waiting and broken
  events, key revocation, sync, and rewind refusal. It never interprets event
  payloads.
- **Data-structure layer (a later child):** tracker event types and rules,
  lead relations and assignment reach, the fold, concurrent-change display,
  roll-ups, and views. Its board decisions are recorded in the mockup README's
  Settled table, not here.

The epic's Child Tickets section planned later children "only after" the
mockup settled the data structures ((139)); (235) a reorders that for the
protocol layer, which the data structures do not affect.

## Decisions

All board IDs refer to the 2026-11-07 session whiteboard topic
"트래커 목업 v2 확인" unless noted.

- **Layer boundary** ((236) ok). Identity (members, keys, admins) is protocol;
  lead relations and assignment reach are data structure. One genesis roles
  file holds both; the protocol reads `admins`, `members[].id/name/keys` and
  passes `leads` through untouched. The protocol hands the domain verified
  events plus their causal relation; the domain's concurrent-change marking
  uses "neither event is in the other's causal past" from the protocol.
  Event payloads are opaque JSON to the protocol. Rejected: the protocol
  interpreting the whole roles file, which leaks domain concepts downward.
- **Per-clone refs** ((212) a-variant: "다른 클론 = 다른 ref 정도로만"; (230) ok).
  Each clone of a store writes only its own ref,
  `refs/heads/tracker/m/<member-id>/<clone-id>`. There is no one-device-per-
  member rule. Branches under `refs/heads/` were chosen because host push
  permissions and protection rules are branch-based, so a Developer role can
  push and a later host protection rule (for example a `tracker/m/*`
  wildcard) uses the same tool. Rejected: one ref per member with seq
  renumbering on conflict; one device per member ((212) b, c); arbitrary
  `refs/` namespaces, whose acceptance by hosts was not verified.
- **Event identity** ((237) ok). An event id is `<member-id>.<clone-id>#<seq>`;
  `seq` is per clone stream, continuous across commits. `seen` maps each other
  clone stream to the last seq the writer had received. Views may group a
  member's streams as one person.
- **Commit batching** ((227) a, reversing (154) "one commit per write"). Writes
  go to a durable local journal. The open batch is closed into one signed
  commit when the writer receives new events (its `seen` would change) or when
  it pushes. All events in a commit share the commit's `seen`, which stays
  exact because the batch closes before anything new is received. Pushed
  commits are never rewritten. Rejected: one commit per write (commit count;
  the user asked whether hundreds of thousands of commits are acceptable).
- **Signing** (epic "Integrity"; (156)). The commit is the signed unit: SSH
  signing (`gpg.format=ssh`) with a local key file; events carry no
  signature. Every client verifies each commit's signer against the keys its
  author held in the commit's causal past. A commit carrying a `member.join`
  verifies against the key that join declares, since that key is not yet in
  its causal past, but only when the join itself is valid (it matches its
  claim and the id is not already held); otherwise the commit verifies
  against the keys the author held in its causal past (lead, auto-confirmed
  from design review).
- **Keys across PCs** ((233) c). A person's several PCs share one key file
  copied between them; per-clone refs keep their seqs apart. Losing a PC means
  an admin revokes the key and the person copies a new key to the remaining
  PCs. A clone whose signing key the store's identity state does not hold
  (for example a new key file copied in after a revocation in another
  store, since one key serves all stores) keeps its writes in the journal
  without closing a batch until an admin adds the key, and the pending
  report says so; nothing is committed under an unregistered key, which
  would be flagged permanently ((266) ok, protocol-ticket topic; follows
  from (201) and (211) a). Rejected: device-linking approval from an existing PC ((233) a);
  admin-only key addition ((233) b).
- **Membership by self-join** ((231) b; (232) b, note: "대부분 사내, 또 저장소
  접근 권한을 가진 유저 기반으로 사용할 것들이라 사실 그냥 신뢰 기반으로 가져가는
  게 낫지 않나"). A user with write access to the store's repository joins by
  writing a join event in their own ref with their member id, display name,
  and public key; they become a member at once, as a root with no lead. The
  first join to claim an id wins it, decided by the claim ref below. Admins later place the member under a
  lead (a data-structure event) and can remove the member or revoke the key
  when the join was an impersonation. Trust is the repository permission
  boundary (epic "Repository boundary", (124)). A member's further clones
  (another PC with the copied key, or a re-clone) write no join: their ref
  sits under the member's id and their commits are signed with a key the
  member holds, and a repeated join by a key the id already holds is a
  no-op.
  **Concurrent joins on one id** ((263) a, protocol-ticket topic; raised by
  design review): a join also creates the create-once claim ref
  `refs/heads/tracker/id/<member-id>`, whose single commit carries the join;
  like genesis it is never updated, and its first push wins because a losing
  push is rejected as non-fast-forward. A join counts only when it matches
  the claim. The losing client reports the id as taken and keeps its
  unpushed events local; it pushes its member ref only after its claim push
  succeeded. A moved or deleted claim ref is an anomaly handled like a
  rewound member ref.
  Claim details (lead, auto-confirmed from design and completeness review):
  store creation also writes the creator's claim ref, so every member id,
  the creator's included, has one. A join matches its claim when the claim
  commit carries that join (same event id and declared key) and is signed
  by the declared key. A join for an id that is already a member in its
  causal past is invalid unless it declares a key the id holds (the no-op
  case). A claim or genesis ref this client wrote but never pushed is
  replaced by the remote's after its push loses, so the loser's view
  converges with everyone's; rewind refusal applies only to a ref value
  that was once pushed or fetched. Consequence: a claimed id is never
  reclaimed, so an impostor-won or removed id cannot be re-joined; the
  person joins under a new id, or an admin re-keys the id ((201)).
  Rejected: a deterministic tie-break that flips an
  already shown join ((263) b); accepting both joins and leaving the
  impostor to an admin ((263) c). Rejected: admin approval
  before any write ((117)/(120)'s original join flow; (231) a); checking the
  join key against the host account's registered keys ((232) a); join
  requests outside the repository ((228) b). (228) a's own-ref event form
  carries into the self-join event.
- **Store creation** ((229) ok). Registering an empty repository URL creates a
  new store (empty means no `refs/heads/tracker/genesis`, so a repository
  that already holds other branches can host a store; lead, auto-confirmed
  from design review): the registrant writes the genesis roles file and
  the creator's claim ref and becomes its sole admin and a root. Registering a store's URL (one with a genesis ref) starts the join flow.
  When two people register the same empty repository at once, the genesis
  pushed first wins and the other falls back to joining.
- **Re-keying a revoked member** ((201) ok). A member whose key was revoked
  continues under the same id and refs after an admin adds a new key
  (`roles.add_key`). Rejected: removing and re-adding the member, which breaks
  the link to their earlier events and refs.
- **Causal past and waiting** ((204) ok; (167), (168)). An event's causal past
  is its stream's earlier events plus every event its commit's `seen` names,
  closed transitively. When `seen` names an event this client has not
  received, the event is **waiting**: listed with no verdict and not applied
  until the gap fills. Verdicts are computed against the causal past only, so
  a late arrival never changes an already given verdict, with the one
  exception below.
- **Broken events consume no seq** ((205) ok). An event that breaks a protocol
  rule (seq gap or duplicate, a commit in a ref its author does not own, a bad
  or missing signature) is flagged and does not occupy its seq, so the next
  valid event in the stream is not dragged into a gap or duplicate. A flagged
  event gets a synthetic id outside causality (as the mockup's envelope-flagged
  events do), so `EventID` stays unique, and a writer's next seq follows its
  highest valid seq (lead, auto-confirmed from design review).
- **Revocation cutoff** ((210) a; (168)). `roles.revoke_key` cuts at what the
  revoking admin had seen of each of that member's streams. Events signed with
  the revoked key above the cutoff are ignored even when their `seen` claims an
  older past. Events at or below the cutoff stay valid. This is the one rule
  that can turn an already shown event into ignored, and every client reaches
  the same result.
  **Concurrent revocations** ((264) a, protocol-ticket topic; raised by
  design review): a revocation is judged against its own causal past only,
  so a concurrent revocation never cuts it, and two admins revoking each
  other concurrently both take effect. A revocation that is itself ignored
  (its signer held no live admin key in its causal past) applies no cut. If
  those were the only admins, no admin with a live key remains; recovery is
  out of scope for v1. Rejected: neither revocation holding, which leaves a
  stolen key live ((264) b); a winner by ts tie-break, which a thief can
  backdate ((264) c). Consequence: a stolen admin key can revoke every other
  admin with a backdated `seen`, leaving no live admin, which v1 does not
  recover; a store should keep more than one admin and revoke a lost key
  promptly. Rejected: judging by each
  client's local arrival order ((210) b), which keeps verdicts stable per
  client but lets clients disagree.
- **Re-issuing a victim's cut events** ((211) a). When a member's own
  legitimate events fall above a revocation cutoff of their own old key, their
  client re-issues the same payloads as new events (new seqs) signed with the
  new key once one is registered. The ignored originals stay in history.
  Re-issue covers only events this clone wrote, as recorded in its local
  journal, never other events of the member's streams or events found on its
  own ref that it did not write, so a thief's events are never re-signed
  (lead, auto-confirmed from design review). Rejected: treating them as lost
  ((211) b).
- **Rewind refusal** ((234) ok). Member refs are append-only, so a remote ref
  that was deleted or moved to a non-descendant is always an anomaly. Clients
  do not take the update, keep their local copy, and report a warning. Any
  client can push its copy back with an explicit restore action, since signed
  events are self-verifying regardless of who pushes them. Restore is manual,
  so a person judges the cause. A remote advance of the clone's own ref that
  this clone did not write (a copied clone id, or a thief appending with a
  stolen key) is handled the same way: not taken, local copy kept, warning
  naming the ref (lead, auto-confirmed from design review). Signatures
  cannot stop deletion or pushes;
  they only decide which events count. Host protection rules stay optional
  hardening (epic "Integrity").
- **Sync cadence** (epic "Sync", (126), with push timing amended by (241)):
  poll every 3 minutes, immediate check when the drawer opens and when a lead
  session starts; no remote and long offline are the same path ((169)). The
  poll and the immediate checks are caller events driven through a
  receive-only `Fetch`: fetch and verify with no push; when new events
  arrive it first closes the open batch into a local, unpushed commit
  ((227) a). `Sync` is the apply path that also pushes, so polling never
  pushes ahead of the (241) delay (lead, auto-confirmed from design review).
  This child provides `Fetch`, `Sync`, and the push delay. The local clone is a bare repository
  under `~/.ws/tracker/<store>/`, user data, not cache ((185)). No snapshots
  in v1 ((184)). Event files sit under `events/YYYY/MM/DD.jsonl` in each ref
  ((183)).

- **Package and git driver** ((238), (239) ok, protocol-ticket topic). The
  layer lives in `agents-plugin-tool/internal/wsledger` and drives git by
  shelling out to the user's git through the existing `internal/wsgit`
  runner. SSH commit signing, allowed-signers verification, and company HTTP
  credential helpers come with the user's git. The runner has no stdin or
  env hook and merges stderr into stdout, so the plumbing passes content
  through files and `-c` options (`hash-object -w <file>`, `commit-tree -F`,
  `-c gpg.format=ssh -c user.signingkey=...`,
  `-c gpg.ssh.allowedSignersFile=...`). A call that cannot fit is added to
  `wsgit` without changing `Runner.RunGit` and stays covered by its
  execution log (260930-feat-git-exec-audit-log). Rejected: extending
  `internal/wsstore` (SQLite runtime state, unrelated) or a tracker-named
  package (the layer is payload-agnostic); go-git (a new dependency without
  the user's signing and credential setup).
- **No caller surface in this child** ((240) ok). No MCP tool, dashboard
  drawer, or config key; Go tests are the verification. The epic's exposure
  decision ((115)) lands with the data-structure child. Rejected: a debug MCP
  tool, which would freeze an interface before the domain needs it.
- **Protocol-owned identity events** ((242) ok). `member.join` (self-join),
  `roles.add_admin`, `roles.remove_admin`, `roles.add_key`,
  `roles.revoke_key`, `roles.remove_member`. `roles.set_lead` and every item,
  comment, and request event are domain events. Names follow the mockup
  README event table, with `member.join` replacing its `roles.add_member`
  stand-in per (231) b.
- **Domain veto hook** ((243) ok). Identity-event validation takes an
  optional domain veto hook; the tracker uses it for (209) "a removed member
  must lead nobody". Rejected: moving (209) into the protocol, since leads are
  domain state per (236).
- **API shape** ((244) ok). The protocol hands the domain verified events,
  causality, and identity state, not git objects. The sketch below is the
  agreed shape; names and helper types may be refined during implementation
  as long as these capabilities and the opaque payload stay. Rejected: handing
  raw commits to the domain.
  The sketch is not the whole surface (lead, auto-confirmed from completeness
  review): the store also needs open and create entry points that take the
  member id, display name, key directory, and clock; registration of a remote
  URL (Phase 2); a restore action for a named ref (Phase 2); the pending
  unpushed-writes report and the push-delay option ((250)). `NewEvent`,
  `Cursor`, `SyncReport`, `IdentityState`, the `roles.json` schema, and the
  identity event payload shapes are defined by the implementation and
  documented in the package doc comment. The push-delay timer runs inside an
  open store under the injected clock. `Sync` with no remote closes the open
  batch and verifies local refs.
  Further refinements (lead, auto-confirmed from design review): `Events`
  cannot be a positional cursor, because a long-offline clone's events and a
  filled gap sort before events already delivered and the revocation cutoff
  and waiting-to-valid change verdicts behind the cursor; the cursor is a
  per-stream watermark (version vector) and each call also returns events
  whose verdict changed since it, which needs a verdict generation counter
  in the cursor next to the watermark (lead, auto-confirmed from review;
  the full ordered set on every call was the alternative).
  The domain veto hook is registered through the open options and receives
  the identity event plus the verified events of its causal past with their
  verdicts; a non-nil result flags the event with that reason. Member
  identity, clone id, and journal location are injectable per store
  instance, so tests can run several writers over one repository. The
  journal keeps every event this clone wrote after batch close and push,
  as the record re-issue reads. A clone whose own remote ref was advanced
  by a foreign writer cannot push again; an explicit action moves its
  unpushed events to a fresh clone id and ref, and the sync report says so
  until it is taken. Re-issue ((211) a) runs during `Sync`, after
  verification shows a newly registered key for the writer.

  ```go
  package wsledger
  
  type EventID struct{ Member, Clone string; Seq int }   // "<member>.<clone>#<seq>"
  type Verdict int                                        // Valid, Waiting, Flagged, Ignored
  type Event struct {
      ID      EventID
      TS      time.Time         // the event line's own write time ((262) a)
      Origin  string            // "human" | "agent" | "agent-auto"; protocol carries, domain judges
      Type    string
      Payload json.RawMessage   // opaque except identity events
      Seen    map[string]int    // "<member>.<clone>" -> seq, from the commit
      Verdict Verdict
      Reason  string            // set when Flagged or Ignored
  }
  type Store interface {
      Append(ctx context.Context, events []NewEvent) ([]EventID, error) // durable journal write
      Sync(ctx context.Context) (SyncReport, error)    // apply: close batch, push, fetch, verify
      Fetch(ctx context.Context) (SyncReport, error)   // receive only: fetch, verify; no push
      Events(after Cursor) ([]Event, Cursor)           // causal order; ties by ts, member, clone, seq; also returns verdict changes
      Concurrent(a, b EventID) bool                    // neither in the other's causal past
      Identity() IdentityState                         // members, keys, admins, revocations, genesis leads passthrough
  }
  ```

- **Key and clone id storage** ((245) ok). One personal SSH ed25519 key at
  `~/.ws/tracker/keys/`, shared by all of a person's stores; a random clone id
  generated on first open and kept with the store's local bare repository.
  Rejected: a key per store, which multiplies the files (233) c has people
  copy.
- **One writer at a time per store directory** (lead, auto-confirmed from
  design review). Every lead session runs its own ws-mcp process and the
  dashboard daemon is another, all opening the same store with the same
  clone id. Seq allocation, journal writes, batch close, `Fetch`, and `Sync`
  are serialized across processes by a file lock in the store directory
  (`gofrs/flock`, already a module dependency used by `wsmailbox`), and the
  push delay reads a last-write time persisted in the store, not one
  process's in-memory timer. Rejected: a single owner process, which the
  epic's sessions-as-writers model does not have.
- **Commit format** ((246) ok; per-event `ts` by (262) a). The commit
  message carries a JSON header
  `{"stream": "<member>.<clone>", "seen": {...}, "ts": "..."}`; events are
  JSON lines `{"seq", "ts", "type", "origin"?, "payload"}` appended to
  `events/YYYY/MM/DD.jsonl`, the file dated by the commit's ts. An event's
  `ts` is its write time, which `Event.TS` and the order tie-break use, so an
  offline batch spanning days keeps each write's time. Rejected: the header
  inside the jsonl, since the file stays pure event lines and the signature
  covers the message; `Event.TS` as the commit's ts ((262) b), which collapses
  a batch's writes onto one time.
- **Verification approach** ((249) ok). Real `ssh-keygen` ed25519 keys and
  temporary bare repositories as remotes, with scenario tests mirroring the
  mockup fixtures: a revoked key with backdated `seen`, a long-offline clone,
  seq gaps and duplicates, a commit in someone else's ref, and two clones of
  one member writing offline. Rejected: mocked git, since signing and
  fast-forward behavior are what the protocol must get right.
- **Two phases** ((248) ok). Phase 1 is the local core against local
  repositories; Phase 2 adds remotes (URL registration, push and fetch, the
  push delay, rewind refusal and restore, join and genesis races). Rejected:
  one phase; Phase 2 builds on Phase 1's commit and verify paths.

- **Push timing** ((241) a with the user's note, (250), (252) ok,
  protocol-ticket topic; user: "이건 아예 자동 반영 딜레이를 5분 정도로 늘리고,
  apply 버튼을 위에서 쏙 내려오게 하는 방식이 어떨까 싶긴 하네요. 또 예를 들어
  패널을 말아 올릴때 push가 반영되게 하는 트리거도 두고. 30초는 뭐 하나 검토하고
  체크하면 사실상 per-action-commit이랑 다를 바가 없어질 거 같은 느낌이라.").
  Own writes are pushed after an idle delay since the last write, at an
  explicit apply, or when the tracker panel is collapsed, not immediately;
  this amends the epic's "Sync" decision ((126)). The protocol takes the
  delay as an option, default 5 minutes, restarted by every write with no
  cap; it reports pending unpushed writes (count and age of the oldest), and
  `Sync` is the apply path. A journal left unpushed at exit restarts the delay
  when the store is next opened: the deadline, persisted in the store, is
  the later of the last write and the last open plus the delay, so an open
  in another process moves it later and never earlier. The apply button that drops down from the
  top, the panel-collapse trigger, and any config key belong to the caller
  child. Rejected: immediate push (with (227) a, a commit per write online);
  a 30-second debounce (the user: a review-and-check pass would still commit
  per action); a cap measured from the first pending write (it would push in
  the middle of a review).
- **Genesis ref** ((247) ok). The genesis roles file `roles.json` is the
  single commit of a dedicated ref `refs/heads/tracker/genesis`, created once
  when the store is created. No second commit is ever added to that ref: the
  current identity state is genesis plus the identity events in members' own
  refs, so there is no second write path to arbitrate, and a moved or deleted
  genesis ref is an anomaly handled like a rewound member ref. Rejected:
  placing genesis in the creator's ref (every client would have to know the
  creator first); an editable roles file. With a dedicated ref, (229)'s
  "first pushed genesis wins" falls out of the losing push being rejected as
  non-fast-forward.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/` (AGENTS.md Implementation Conventions). The package
  is designed for any downstream team using ws, not for this repository.
- The protocol never parses event payloads beyond the identity events it owns.
- Out of scope: tracker event types and their rules, lead relations,
  assignment, the fold and views, search indexes, the dashboard drawer, MCP
  tools and skills, and config keys. They belong to the data-structure child
  and later children.
- SSH commit signing needs git 2.34 or later and `ssh-keygen` on the user's
  machine; errors that come from a missing or older tool name it.
- Shell out only through the `internal/wsgit` runner; no new Go module
  dependency.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Integrity: commits are SSH-signed (`gpg.format=ssh`) with a local key file. Push stays on whatever transport the remote uses... Every client verifies signatures against the member public keys in a roles file" — bearing: constrains
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Storage model: each member appends events to their own ref only. Every view is computed client-side by merging all refs. This removes write contention and merge conflicts" — bearing: supports
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Sync: background poll every 3 minutes; own writes pushed after a 5-minute idle delay, at an explicit apply, or when the tracker panel is collapsed (board (241), amending (126))" — bearing: supports
- 260924-feat-origin-ticket-ownership-index (2026-09-24, Result): "Namespace constant `ticket-index-larkspur`: remote ref `refs/ticket-index-larkspur/v1/index`; local cache, pending log, and fetch scratch refs under `refs/ticket-index-larkspur-local/v1/`" — bearing: constrains
- 260930-feat-git-exec-audit-log (2026-09-30, Decisions): "Every git process launched through `wsgit.ExecRunner` and the `wsindex` `ExecRunner` gets [an always-on execution log]; `Runner.RunGit(ctx, root, args...)` stays unchanged." — bearing: constrains
- 86d1a9cd (2026-05-03, commit): "The new `internal/wsgit` package invokes native Git through `exec.Command` argument arrays... centralizes native Git invocation" — bearing: supports
- 260806-feat-worktree-ticket-scope (2026-08-06, Result): "git sparse-checkout check-rules needs git >= 2.42. Below that the destination pre-flight fails open and the refusal arrives after the source write" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | new package agents-plugin-tool/internal/wsledger (store, journal, commit format, verify, identity, sync, tests); reuses agents-plugin-tool/internal/wsgit |
| scope.surface | internal | new internal package; exported API sketched in Decisions (API shape) consumed by a later child; no MCP tool, config key, or shipped playbook text |
| scope.new_public_symbol | yes | wsledger.Store, Event, EventID, Verdict, NewEvent, SyncReport, IdentityState (exported from a new internal package) |
| scope.new_type_contract | yes | Store interface and Event type in the API shape sketch; commit-message JSON header and events/YYYY/MM/DD.jsonl line format; roles.json genesis |
| scope.test_surface | new-files | search for agents-plugin-tool/internal/wsledger returned nothing; new scenario tests with real git and ssh-keygen; existing agents-plugin-tool/internal/wsgit/git_test.go must keep passing |
| complexity.reuse_points | confirmed | wsgit.Runner and ExecRunner at agents-plugin-tool/internal/wsgit/git.go#L20-L35 read; runner has no stdin or env hook and merges stderr into stdout via CombinedOutput |
| complexity.side_effect_risk | moderate | writes refs and journal in a local bare repository, generates a key under ~/.ws/tracker/keys/, pushes to remotes in Phase 2; all behind injectable directories and temp repos in tests |
| risk.correctness | high | signature verification against causal-past keys, revocation cutoff, waiting and broken-event rules, deterministic cross-client verdicts |
| risk.fit | moderate | wsgit.Runner lacks stdin and env and merges stderr (signing and commit-tree plumbing must fit it); new package outside existing module roles |
| risk.test | moderate | needs real git 2.34+ and ssh-keygen and many multi-writer scenarios; git here is 2.50.1 and /usr/bin/ssh-keygen exists |
| risk.security_or_contract | high | commit signing and verification are the integrity boundary; key file handling; the Store API is the contract the data-structure child builds on |

## Phases

### Phase 1: Local ledger core

Build `internal/wsledger` against local repositories only, with no remote.

- **Store open and creation.** Open a store from a local bare repository
  under a caller-given directory (the caller child picks
  `~/.ws/tracker/<store>/`). Creating a store writes the genesis
  `roles.json` (creator as sole admin and a root, `leads` passed through) as
  the single signed commit of `refs/heads/tracker/genesis`. The clone id is
  generated on first open and kept with the repository. The personal key
  lives at `~/.ws/tracker/keys/` (generated with `ssh-keygen` when absent);
  the key directory and clock are injectable so tests use temporary keys and
  a fake clock.
- **Journal and commits.** `Append` writes events durably to a local journal
  and returns their ids. The open batch closes into one signed commit on the
  writer's own ref `refs/heads/tracker/m/<member>/<clone>` when the writer's
  `seen` would change or on `Sync`, in the commit format above. Pushed
  commits are never rewritten.
- **Verification.** Read every ref under `refs/heads/tracker/m/` and
  `refs/heads/tracker/id/` plus genesis
  and give each event a verdict: valid, waiting, flagged, or ignored, with a
  reason. Rules: commit signature checked against the keys its author held
  in the commit's causal past (an allowed-signers file derived from identity
  state, verified through the user's git); the author owns the ref; seq
  continuity, with broken events consuming no seq; waiting when `seen` names
  an unreceived event; identity events (`member.join` valid only when it
  matches the id's claim ref, admin events by admins only, the revocation
  cutoff with concurrent revocations both holding, the domain veto hook);
  verdicts computed from the causal past only, the revocation cutoff being
  the one sanctioned flip.
- **Re-issue.** When the writer's own events are cut by a revocation of its
  old key and a new key is registered, the store re-issues their payloads as
  new events signed with the new key.
- **Read API.** `Events` in causal order with the documented tie-break,
  `Concurrent`, and `Identity`, per the API decision.
- **Tests.** Several writers (distinct member and clone ids, real ed25519
  keys, injected per store instance) may share one local repository to
  exercise multi-writer verification without push or fetch; there a writer
  "receives" when another writer's ref moved since its last batch close.
  Also: two processes appending to one store directory get distinct seqs. Scenarios: a revoked key with backdated `seen`
  (ignored above the cutoff, valid at or below), re-issue after re-keying,
  seq gaps and duplicates, a commit in someone else's ref, a bad signature,
  a waiting event that resolves when its gap fills, two clones of one member
  writing interleaved, a join id conflict (the second claim ref creation
  fails locally), two admins revoking each other concurrently (both hold), an
  offline batch whose events keep distinct `ts` values, and the veto hook
  refusing a removal, an impostor's join on the creator's id and on an
  already-held id (invalid, and other events in that commit fail the
  signature check), and a verdict flip behind a caller's cursor (a
  revocation cutoff, a filled gap) reaching that caller.

### Result (d930018ff) - 2026-11-08

Landed `agents-plugin-tool/internal/wsledger` (b9bf6f14b, d2593632d,
d930018ff): `Create` / `Join` / `Open`, the journal with batch close on
receive or `Sync`, re-issue, and a pure verification core (`verify.go`)
that never touches git. The package doc (`doc.go`) documents the ref
layout, commit format, `roles.json`, identity payloads, verdict rules,
journal, and cursor.

Verification: `go build ./...`, `go vet ./internal/wsledger/`, and
`go test -count=1 ./internal/wsledger/` pass with real git 2.50.1 and
`ssh-keygen`. The tests cover every Phase 1 scenario plus a `parsePatch`
table; `-race` passes on the concurrent-append and veto tests.

Decisions taken:
- A signature that does not verify cryptographically, or no signature, is
  an envelope break (no seq). A good signature from a key the author did
  not hold in the commit's causal past is a Flagged verdict, and its seq
  counts. git reports the signer fingerprint even when the allowed-signers
  file does not list the key, so verdicts are a pure function of the commit
  and are cached by oid. The allowed-signers file is still derived from the
  identity keys.
- Each revocation's validity is computed in its own scoped pass over its
  causal past, using only the cuts of revocations inside that past. This
  well-founded reading follows "judged against its own causal past only".
  The mockup's grow-only ignored set can keep cuts from a revocation that
  later turns invalid.
- Commit contents are read with one nonce-delimited `git log -p
  --unified=0` per moved ref. A commit counts only when it is a single
  pure append that starts at the tracked line count. A commit that is not
  marks the rest of its ref malformed, because later diffs no longer prove
  what each commit added.
- `Event` gained `Pending`: the open batch's events appear as a virtual
  commit with a provisional verdict, so the domain sees its own writes
  before the batch closes.
- `seen` records only the resolved (non-waiting) prefix of each stream. A
  writer's own events therefore never wait on its own view, and honest
  writers cannot form causal cycles.
- When an admin adds this clone's key, a batch whose causal past lacks the
  key takes the current `seen` before it closes. Without that it could
  never verify.
- The join exception applies when `member.join` is the first event of its
  commit. `Join` refuses a clone that already has commits, because a claim
  commit must be the root of its stream.
- `ssh-keygen` is run directly only to generate the key. Every git call
  goes through `wsgit.Runner`, and `wsgit` itself is unchanged.

#### Edition (4fbcaf8f0) - 2026-11-08

`verify.go` closed a gap in the signer check. A revocation's memoized
verdict used to be returned before its commit's signer result was set.
`Valid` is the zero verdict, so the events after a forged revocation in
the same commit skipped the signer check and verified Valid.
`TestRevocationFirstInCommitKeepsSignerCheck` reproduced this before the
fix.

### Phase 2: Remote sync, registration, and rewind refusal

Add remotes on top of Phase 1's commit and verify paths.

- **Registration.** Registering a repository URL clones it into the local
  bare repository. When the identity state already holds the local key for
  the member id (a second PC with the copied key, a re-clone, or a key added
  by `roles.add_key`), registration writes no join and no claim. Otherwise, a
  repository with no genesis ref creates the store and pushes genesis and
  the creator's claim;
  if that push loses (genesis already exists), fetch and continue as a
  joiner. A repository with a genesis ref leads to the join: a `member.join`
  event in the joiner's own ref plus its claim ref, pushed claim first; a
  rejected claim push reports the id as taken and pushes nothing else.
- **Sync.** `Sync` closes the open batch, pushes the writer's own ref
  (fast-forward only), and fetches the narrow refspec
  `refs/heads/tracker/*` (a fetch with nothing new transfers no objects),
  then verifies what arrived. A failed sync leaves pending
  writes in place for the next attempt and reports the failure.
- **Push timing.** The idle push delay (default 5 minutes, restarted by every
  write, no cap) triggers `Sync`; `Fetch` never pushes; pending unpushed
  writes are reported; an unpushed journal restarts the delay on open.
- **Rewind refusal and restore.** Fetched refs land somewhere that does not
  overwrite the local copies; a member, claim, or genesis ref deleted on the remote
  or moved to a non-descendant is not taken, the local copy stays, and the
  sync report carries a warning naming the ref. An explicit restore action
  pushes the local copy of a named ref back.
- **Tests.** Temporary bare repositories as remotes: two registrations of
  one empty repository (one creates, one joins), two joins claiming one id,
  registering a second clone of an existing member (no join, no claim), both
  racing clients ending with the same view after a genesis race and a
  claim race, a long-offline clone catching up, a remote member ref and a
  remote claim ref deleted and then restored, a foreign-advanced own ref
  recovered by moving to a fresh clone id,
  a remote ref force-moved to a non-descendant, a remote advance of the
  clone's own ref it did not write, a poll through `Fetch` that pushes
  nothing, a push delay that restarts
  on each write under a fake clock, and a pending journal surviving reopen.

### Result (acf2ff17a) - 2026-11-08

Landed `remote.go` (1f804457d, b95429231, 4fbcaf8f0, acf2ff17a):
- `Register`.
- Remote-aware `Sync` and `Fetch`.
- The persisted idle push deadline.
- Rewind refusal, with `Restore` and `MoveToFreshClone`.
- A per-stream journal (`journal.<member>.<clone>.jsonl`).

The package doc's `# Remote` section covers all of these.

Verification: `go build ./...`, `go vet ./internal/...`, and
`go test -count=1 ./internal/wsledger/ ./internal/wsgit/` pass. Each
Phase 2 scenario has a test against temporary bare remotes. The tests
also cover:
- a Sync before any Fetch not undoing an own-ref deletion or rewind;
- a refused push failing Sync and Register;
- Restore refusing a remote that moved ahead, or one that moved after
  its own fetch.

The genesis and claim races are forced deterministically by a test
`Runner` hook that completes the rival's registration just before the
contested push.

Decisions taken:
- **Three ref namespaces.** Fetched refs land in `refs/ledger/remote/`. A
  never-pruned `refs/ledger/known/` records the last value seen on or
  pushed to the remote. A deletion, which has a known value, therefore
  stays distinct from a ref that never reached the remote.
- **Create-once refs.** A genesis or claim this clone wrote but never
  pushed is replaced by the remote's after a lost push. That is how a lost
  genesis race continues as a joiner and a lost claim race becomes
  `IDTaken`. Any value once known on the remote is protected.
- **Leased own-ref push.** The own ref is pushed on a lease of its
  last-known remote value rather than as a plain fast-forward. A Sync run
  by the push-delay timer before any fetch therefore cannot undo a remote
  deletion or rewind. The fetch that follows every push classifies the
  rejection and warns. Leased pushes carry no `+` refspec prefix, because
  git lets `+` override the lease.
- **Push refusals.** Only a client-side `[rejected]` counts as a lost race.
  A `[remote rejected]` (hook, branch protection, permission) or a
  transport failure is an error, and the writes stay pending.
- **Push deadline.** The deadline is one file per stream, so processes
  sharing a store directory agree. A write or an open only moves it later.
- **`MoveToFreshClone`.** It renames the old stream's journal to `.moved`.
  Reopening the old clone id therefore cannot publish the moved events
  twice.

Lite review: two Important findings, both fixed in acf2ff17a:
- a remote-rejected push passed for a lost race;
- `Restore` could rewind a remote that had moved ahead.

Minor findings, left as follow-ups:
- `credential.interactive=false` is not a core git option. An HTTPS remote
  with no credential helper can still reach a terminal prompt; the runner
  has no environment hook to set `GIT_TERMINAL_PROMPT=0`.
- No test moves or deletes a genesis or claim ref after it was pushed, and
  no test covers a forked member ref.
- `Events`, `Identity`, and `Pending` wait on the store mutex while `Sync`
  holds it across network calls.
- The two-process seq test uses two instances in one process, separated
  only by the file lock.

## Verification

From `agents-plugin-tool/`: `go build ./...`, `go vet ./internal/...`, and
`go test -count=1 ./internal/wsledger/`. Each phase's scenario tests above
pass with real `git` and `ssh-keygen`.
