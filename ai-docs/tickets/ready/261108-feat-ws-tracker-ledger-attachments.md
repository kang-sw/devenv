---
title: "ws tracker ledger attachments: content-addressed blobs fetched on demand through a partial clone"
parent: 261107-epic-ws-personal-common-store
related:
  261107-feat-ws-tracker-ledger-protocol: the ledger layer this extends (per-clone refs, signed commits, events/YYYY/MM/DD.jsonl tree)
  261108-feat-ws-tracker-data-structure: the domain layer that already validates attachment metadata and attachments.max_bytes
  261109-feat-ws-tracker-shared-markdown-editor: the editor whose Phase 2 calls the upload and read routes; its accepted image types match this ticket's
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5625ad14f2d60479
sage-review-completeness-reviewed: 5625ad14f2d60479
---

# ws tracker ledger attachments: content-addressed blobs fetched on demand through a partial clone

## Background

The user (2026-11-08) asked what happens when people attach images or large
data to item bodies and comments: the whole git history would carry them and
every clone would grow. Blocking attachments outright would hurt use, so the
goal is on-demand loading of only the attachment a reader opens, not saving
bytes (board (337)).

The mockup and the Go domain layer (`agents-plugin-tool/internal/wstracker`,
merge `0a80fdbfb`) already model the metadata: item bodies and comments carry
`attachments: [{oid, name, size, mime}]` with `oid` the content's sha256, the
roles file holds `attachments.max_bytes` (default 10 MiB) changed by the admin
`roles.set_attachment_limit`, and an over-limit attachment newly added by a
write is flagged with a reason (board (338), (339), (345) b). This ticket
stores and fetches the content.

## Decisions

- **Storage** (user, board (337) a, amended by (71) a): attachment content
  is a content-addressed blob at `attachments/<sha256>` in the authoring
  member's ledger, on a separate attachment ref (below), not in
  the event commit. Readers fetch blobs on demand through a partial clone,
  so a clone holds event history and only the attachments it has opened.
  Rejected: Git LFS (a separate server and protocol the hosting repository
  may not offer); a plain blob in history with no filter (every clone
  downloads every attachment).
- **Separate attachment ref** (user; board (71) a): blobs live on one
  attachment ref per member ref (member refs are per clone, so a member
  with two clones has two), named
  `refs/heads/attach/<the member ref's name under refs/heads/tracker/>`,
  a chain of commits signed with the member's key, each adding only
  `attachments/<64 lowercase hex>` paths and never changing or removing one;
  rewind refusal and the remote/known snapshot namespaces apply to it as to
  event refs. Event commits keep touching exactly one events file, so a
  client that predates this ticket (whose verifier refuses a commit touching
  more than one file, `wsledger/gitio.go`, and which fetches only
  `refs/heads/tracker/*`, `wsledger/remote.go` `fetchTracker`) keeps
  reading every event and only cannot show the images. A write pushes the
  attachment ref and the event ref in one `git push --atomic`, attachment
  first in the local commit order. Rejected: blobs in the event commit
  (old (48); an older client poisons the author's whole ref, and a
  50-60-member store always has mixed versions); a store-level format
  marker gating attachment writes (an admin's wrong call still poisons);
  accepting the divergence until upgrade.
- **Limit** (user, board (338) a, (345) b): the roles-file
  `attachments.max_bytes` caps an attachment newly added by a write; an
  attachment carried over unchanged from the body or comment the author saw
  is exempt, so lowering the limit never locks old posts.
- **Tests** (board (341)): automated tests keep local bare remotes
  (`newRemote` in `internal/wsledger/remote_smoke_test.go`); a partial clone
  is exercised locally with `uploadpack.allowFilter=true` over a `file://`
  URL (a plain path clone ignores the filter). A hosted private repository is
  used only at live acceptance, where HTTPS auth, host filter support, and
  latency show.
- **Fetch modes** (design review, auto-confirmed; follows (71) a): event
  refs are fetched unfiltered, so verification never lazy-fetches;
  attachment refs are fetched with `--filter=blob:none` (commits and trees
  only), the remote is marked as the promisor. Git writes
  `remote.<name>.partialclonefilter` on the first filtered fetch and later
  plain fetches inherit it (checked on git 2.43 in delta design review), so
  every event-ref fetch (`fetchTracker` and the fetch inside `Restore`)
  passes `--no-filter` explicitly, and each filtered fetch unsets
  `partialclonefilter` afterwards so a downgraded pre-ticket binary on the
  same clone does not inherit it. Attachment refs are fetched before event
  refs; when the attachment fetch fails, new event tips from that sync are
  not absorbed (the next sync retries both), so an absorbed event never
  names an oid missing from every local attachment-ref tree. Reading a
  blob the clone lacks runs one explicit fetch of that oid from the
  promisor remote (ledger git runs keep `GIT_NO_LAZY_FETCH=1`, so no git
  command fetches implicitly). Event-ref commit reads (`logCommits`,
  `git log -p`) never touch attachment trees. Rejected: one
  `--filter=blob:limit=N` for both (a size filter cannot tell event day
  files from small screenshots).
- **Host without filter support** (user; board (47) a): when the hosting
  remote does not honor partial-clone filters (`uploadpack.allowFilter`
  off), git itself falls back to a full fetch: the filtered fetch exits 0,
  prints `warning: filtering not recognized by server, ignoring` on stderr,
  and fetches everything, so no retry is needed and detection reads stderr
  of a successful run (the ledger's git runner must expose it).
  Attachments still work; the clone just holds every attachment. The
  fallback is reported once per
  store per process through the ledger's existing sync-warning channel (the
  one rewind refusals use). Rejected: refusing attachments on such a store
  (pasted images would fail outright on those hosts).
- **Attachment-ref verification** (board (48) superseded by (71) a; (63)
  carried over): the verifier accepts an attachment-ref commit when it is
  signed by the ref's member, adds only well-formed `attachments/<sha256>`
  paths, and changes or removes nothing. It does not read blob content (a
  partial clone lacks it) and does not cross-check event references (an
  unreferenced blob is never rendered, since the editor renders only oids
  in the text's own attachment list). The content hash is checked by the
  writer before it commits and by the read path every time it serves a blob
  (a mismatch is refused, never served). Rejected: fetching every blob to
  verify (defeats on-demand loading); trusting the git blob oid (it is not
  the sha256 of the content).
- **Host boundary** (board (64), auto-confirmed from fact population):
  `wsdashboard` imports no tracker package (commit 057a2ab05), so staging,
  its expiry, and blob reads live in `internal/wstrackerhost` behind new
  tracker-host methods (`cmd/ws-mcp/tracker_host.go` adapts them) that take
  and return bytes capped by `attachments.max_bytes`; the dashboard routes
  stay thin, and the store-op dispatcher (`trackerStoreOps`) gains the
  `attachments` and `attachments/<sha256>` paths. Rejected: importing
  `wstracker` into `wsdashboard`.
- **Staged upload** (board (49) ok): `POST /tracker/stores/<id>/attachments?name=<file name>`
  takes the raw image bytes as the request body (not multipart; `name` is
  reduced to its base name, at most 255 bytes, and defaults to `image`); the daemon computes the sha256, identifies the type from
  the bytes' magic numbers (png, jpeg, gif, webp; anything else refused,
  whatever the request claims), checks `attachments.max_bytes`, stages the
  blob, and returns `{oid,name,size,mime}`. The next write carrying that
  metadata commits the staged blob to the attachment ref with its event; a
  write naming an oid that is neither staged nor present in some member's
  attachment-ref tree (a path in the tree; the local clone need not hold
  the blob) is refused. Rejected: base64 bytes inside the event write (the
  tracker write body limit is 256 KiB and events carry only metadata).
- **Staged blob handoff** (delta design review, auto-confirmed): the
  ledger stays payload-agnostic, so the domain write passes the newly added
  blobs' bytes to the wsledger write API alongside the payload; wsledger
  writes the git objects at write time and records their sha256 oids in the
  journal record, and `closeBatch` builds the attachment-ref tree from
  those records, so the sweep cannot race the commit. Once a blob is
  committed its staged copy is deleted. Rejected: wsledger parsing payloads
  for oids (breaks payload-agnosticism); closeBatch reading the staging
  directory later (races the sweep).
- **Staging expiry** (completeness review, auto-confirmed): a staged blob
  expires 24 hours after upload unless a pending journal event names it
  (a committed blob's staged copy is already gone); the sweep runs at store open and hourly, under the same
  store lock the write path holds, so a write never commits a blob the
  sweep is deleting. Rejected: expiring at batch close (a pasted image left
  in an open editor would vanish).
- **Atomic push** (delta design review, auto-confirmed): `push()` maps
  outcomes for both refs of the `--atomic` push (the second ref is reported
  rejected because the atomic transaction failed), and the
  `--force-with-lease` path and `Restore` cover both refs. A host without
  the atomic capability fails the push with an error naming it. Rejected: a
  non-atomic two-step fallback (an event could land without its blob).
- **Read route** (board (50) ok, hardened by design review,
  auto-confirmed): `GET /tracker/stores/<id>/attachments/<sha256>` serves a
  blob found in a member's attachment-ref tree (fetching it on demand;
  since the verifier does not read content, another member may hold a
  wrong blob at the same path, so the route tries every tree holding the
  path, the referencing author's first, and serves the first blob whose
  content hashes to the oid), a
  blob staged on this clone, or one named by this clone's pending journal
  events, under the access rules of the other tracker routes. The response
  type comes from the blob's own magic numbers and only png, jpeg, gif, or
  webp is served (anything else is refused with the route's 404, whatever
  any event's metadata claims, since metadata is member-authored), with
  `X-Content-Type-Options: nosniff`, `Cross-Origin-Resource-Policy:
  same-origin`, and the `Sec-Fetch-Site` check of the out-of-root image
  route (2c1ae57e3). Rejected: serving the metadata's `mime` (a member's own
  client can write `text/html` and get stored XSS on the daemon origin);
  serving through the session image route (attachments belong to a store,
  not a session).

## Constraints

- The event ledger stays payload-agnostic: event commits are unchanged;
  signatures, causality, and rewind refusal apply to attachment refs as to
  event refs.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.

## Prior Decisions

- 057a2ab05 (2026-11-08, commit): "The dashboard stays free of wstrackerhost/wstracker/wsledger: the interface speaks JSON-level types and the cmd/ws-mcp adapter converts ... Rejected: importing wstrackerhost and serving its types directly." — bearing: constrains
- 057a2ab05 (2026-11-08, commit): "Stores() runs on every tracker route (after method and same-origin checks): its error is how a host that could not be created reaches the page (503), and its list is the store-id allow-list (404 \"unknown store\")." — bearing: constrains
- 1a3c3cbc0 (2026-11-08, commit): "User decision on board (345) b: after an admin lowers the limit, a body edit that keeps an older larger attachment must not be flagged. Exemption keys on oid only (content hash fixes size)" — bearing: supports
- 0a80fdbfb (2026-11-08, commit): "Open follow-ups put to the user on the board: body-less item.edit carrying mentions/attachments flagged; ... lowering attachments.max_bytes flags edits that carry an older larger attachment." — bearing: supports
- c9b2a9bb3 (2026-11-08, commit): "Board (346) ok: the storage half of (337) a was deferred to a ledger ticket when the mockup took attachment metadata; captured as a todo child with (337), (338), (341), (345) b settled" — bearing: supports
- 261107-feat-ws-tracker-ledger-protocol (2026-11-08, Result / 1f804457): "Fetched refs land in refs/ledger/remote/ and a never-pruned refs/ledger/known/ records the last value seen on or pushed to the remote ... any value once known on the remote is protected." — bearing: constrains
- 592ddd994 (2026-11-08, commit): "Deviation: the env also sets GIT_NO_LAZY_FETCH=1 so a partial clone never fetches a missing object over the network." (git.search over store clones) — bearing: constrains
- b9bf6f14 (2026-11-08, commit): "Pending journal events appear in Events as a virtual commit (Event.Pending, provisional verdict) so the domain sees its own writes before the batch closes" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsledger verify.go format.go gitio.go remote.go store.go, agents-plugin-tool/internal/wstracker check.go, agents-plugin-tool/internal/wsdashboard/tracker_routes.go, and the host adapter agents-plugin-tool/internal/wstrackerhost plus cmd/ws-mcp adapter since the dashboard imports no tracker package per tracker_routes.go#L12-L15 |
| scope.surface | cross-module | new daemon HTTP routes POST and GET /tracker/stores/<id>/attachments, new wsledger verifier rule and fetch mode, JSON-level host interface in wsdashboard extended |
| scope.new_public_symbol | yes | wsledger blob read and stage API and a wsdashboard tracker host method for upload and read; names not yet fixed |
| scope.new_type_contract | yes | upload response {oid,name,size,mime}; new per-member ref refs/heads/attach/<member> whose commits add only attachments/<sha256>; event commits unchanged per format.go#L31 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsledger/remote_smoke_test.go newRemote at L11, remote_test.go, agents-plugin-tool/internal/wstracker/ledger_test.go, agents-plugin-tool/internal/wsdashboard/tracker_routes_test.go |
| complexity.reuse_points | confirmed | wsledger fetchTracker remote.go#L92, putPath/writeTree gitio.go#L174-L218, trackerStoreOps tracker_routes.go#L159-L165, nosniff header pattern attachment_route.go#L58-L63, wstracker oid and max_bytes check check.go#L99-L167 |
| complexity.side_effect_risk | high | changes ledger clone and fetch mode to a partial clone with promisor remote and adds staged blob files with expiry on disk |
| risk.correctness | high | attachment-ref verifier, atomic two-ref push, promisor fetch modes with explicit by-oid fetch, full-fetch fallback detection, and staged-blob expiry racing writes |
| risk.fit | moderate | trackerStoreOps maps a single op segment so attachments/<sha256> needs a dispatcher change, and the dashboard must reach blobs through the JSON-level host interface |
| risk.test | moderate | partial clone needs file:// URL with uploadpack.allowFilter; hosted remote only at live acceptance |
| risk.security_or_contract | high | serves user-uploaded content from the daemon; type check, nosniff, store access rules, and signed-ledger verifier contract change |

## Phases

### Phase 1: blob storage and on-demand fetch

- Ledger (`internal/wsledger`): the attachment ref with its
  verifier rule (signed by the member, adds only well-formed
  `attachments/<sha256>` paths, changes or removes nothing); the write path
  committing staged blobs to the author's attachment ref and pushing it
  with the event ref in one `--atomic` push; the fetch modes above (event
  refs unfiltered, attachment refs `--filter=blob:none`, promisor without a
  default filter), the full-fetch fallback and its one-time sync warning
  when the host refuses the filter; reading one blob by oid with an
  explicit fetch when the clone lacks it; staged blob bytes handed through
  the write API into the journal record; `push()` outcome mapping, lease,
  and `Restore` over both refs. Event-ref fetches gain `--no-filter`; event
  refs' verifier and `logCommits` are otherwise unchanged.
- Domain and daemon: the staged upload route and the read route above
  (`wsdashboard/tracker_routes.go` over new tracker-host methods in
  `internal/wstrackerhost` and `cmd/ws-mcp/tracker_host.go`), the
  magic-number type check (png, jpeg, gif, webp, matching the editor
  ticket's accepted images) and the `attachments.max_bytes` check against
  the uploaded byte size, staging expiry under the store lock, and the write
  path committing the staged blobs a write's attachment metadata names and
  refusing an oid that is neither staged nor in an attachment-ref tree.
- Verification: `go test ./internal/wsledger/ ./internal/wstracker/
  ./internal/wstrackerhost/ ./internal/wsdashboard/ ./cmd/ws-mcp/`. Tests
  over local bare remotes (`newRemote`) cover: an attachment-ref commit
  with well-formed added paths verifies; one that is unsigned, changes or
  removes a path, or adds any other path is refused; the event commit of a
  write with an attachment still touches one file, and a pre-change
  verifier reading the event ref accepts it; the writer refuses to commit a
  staged blob whose content does not hash to its oid, and the read path
  refuses to serve such a blob; a partial clone over a `file://` URL with
  `uploadpack.allowFilter=true` lacks the blob until it is read, then
  fetches only that blob, and verification of event refs fetches no blob;
  a push whose event-ref update is refused leaves the attachment ref
  unmoved too (atomic), and the local attachment commit precedes the event
  commit; a rewound or force-pushed remote attachment ref is refused like
  an event ref, and attachment refs appear under `refs/ledger/remote/` and
  `refs/ledger/known/`; a filtered attachment fetch followed by new event
  commits, an event fetch, and verification leaves no event blob missing;
  a failed attachment fetch leaves new event tips unabsorbed; a wrong blob
  at the same path in another member's tree does not shadow the right one
  on read; a remote with the filter off falls back to a full attachment
  fetch and warns once; upload over the size limit or of non-image bytes (including
  a non-image sent with an image type) is refused; the read route serves
  the magic-number type with `nosniff` and CORP, refuses a blob whose bytes
  are not an allowed image whatever its metadata, serves a staged blob and
  one named by a pending journal event, and refuses a viewer the other
  tracker routes refuse; an unreferenced staged blob expires and one named
  by a pending event does not, and a committed blob's staged copy is
  deleted. A hosted private repository is exercised
  only at live acceptance and is listed as unverified in the Result.
