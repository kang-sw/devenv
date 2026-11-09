---
title: "ws tracker ledger attachments: content-addressed blobs fetched on demand through a partial clone"
parent: 261107-epic-ws-personal-common-store
related:
  261107-feat-ws-tracker-ledger-protocol: the ledger layer this extends (per-clone refs, signed commits, events/YYYY/MM/DD.jsonl tree)
  261108-feat-ws-tracker-data-structure: the domain layer that already validates attachment metadata and attachments.max_bytes
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

- **Storage** (user, board (337) a): attachment content is a content-addressed
  blob at `attachments/<sha256>` in the authoring member's ledger tree,
  committed with the event that references it. Readers fetch blobs on demand
  through a partial clone (`--filter=blob:limit=N`), so a clone holds event
  history and only the attachments it has opened. Rejected: Git LFS (a
  separate server and protocol the hosting repository may not offer); a
  plain blob in history with no filter (every clone downloads every
  attachment).
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
- **Host without filter support** (user; board (47) a): when the hosting
  remote refuses partial-clone filters (`uploadpack.allowFilter` off), the
  store falls back to a full fetch: attachments still work, the clone just
  holds every attachment, and the store reports the fallback once.
  Rejected: refusing attachments on such a store (pasted images would fail
  outright on those hosts).
- **Blob in the event commit** (board (48) ok): the ledger verifier lets an
  event commit also add `attachments/<sha256>` blobs, each referenced by an
  attachment in that commit's appended events and whose content hashes to
  its path, and nothing else. Rejected: a separate blob-only commit ((337) a
  commits the blob with the event that references it, and one commit per
  write keeps causality simple).
- **Staged upload** (board (49) ok): `POST /tracker/stores/<id>/attachments`
  takes the bytes; the daemon computes the sha256, checks the type and
  `attachments.max_bytes`, stages the blob, and returns
  `{oid,name,size,mime}`. The next write carrying that metadata commits the
  staged blob with its event; unreferenced staged blobs expire. Rejected:
  base64 bytes inside the event write (the tracker write body limit is
  256 KiB and events carry only metadata).
- **Read route** (board (50) ok): `GET /tracker/stores/<id>/attachments/<sha256>`
  serves a blob found in a member's ledger tree, fetching it on demand from
  the promisor remote, under the access rules of the other tracker routes,
  with `Content-Type` from the stored metadata and
  `X-Content-Type-Options: nosniff`. Rejected: serving through the session
  image route (attachments belong to a store, not a session).

## Constraints

- The ledger stays payload-agnostic except for this blob path; signatures,
  causality, and rewind refusal apply to attachment commits unchanged.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.

## Phases

### Phase 1: blob storage and on-demand fetch

- Ledger (`internal/wsledger`): the verifier accepts an event commit that
  also adds `attachments/<sha256>` blobs under the rule above (each path
  referenced by that commit's appended events, content hashing to its
  name, no other path); the write path stages a blob and commits it in the
  same signed commit as the event that first references it; clone and
  fetch use `--filter=blob:limit=N` against the promisor remote, with the
  full-fetch fallback and its one-time report when the host refuses the
  filter; reading one blob by oid fetches it on demand when the partial
  clone lacks it. Signatures, causality, and rewind refusal apply
  unchanged.
- Domain and daemon: the staged upload route and the read route above
  (`wsdashboard/tracker_routes.go`, beside the existing tracker store
  routes), the type check (png, jpeg, gif, webp, matching the editor
  ticket's accepted images) and the `attachments.max_bytes` check against
  the uploaded byte size, staging expiry for blobs no write referenced, and
  the write path committing the staged blobs a write's attachment metadata
  names. A write naming an oid that is neither staged nor already in the
  ledger is refused.
- Verification: `go test ./internal/wsledger/ ./internal/wstracker/
  ./internal/wsdashboard/`. Tests over local bare remotes (`newRemote`)
  cover: an event commit with a referenced blob verifies; an unreferenced
  blob, a blob whose content does not hash to its path, or any other added
  path is refused; a partial clone over a `file://` URL with
  `uploadpack.allowFilter=true` lacks the blob until it is read, then
  fetches only that blob; a remote with the filter off falls back to a full
  fetch and reports it once; upload over the size limit or of a non-image
  type is refused; the read route sets `nosniff` and the stored type and
  refuses a viewer the other tracker routes refuse; an unreferenced staged
  blob expires. A hosted private repository is exercised only at live
  acceptance and is listed as unverified in the Result.
