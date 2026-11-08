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

## Constraints

- The ledger stays payload-agnostic except for this blob path; signatures,
  causality, and rewind refusal apply to attachment commits unchanged.
- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing under
  `agents-plugin-tool/`.

## Phases

### Phase 1: blob storage and on-demand fetch

To be written at planning: write path (blob next to the event in the same
signed commit, oid verified against content), filtered clone and fetch
configuration, an on-demand fetch of one blob by oid, behaviour when the
host does not support filters (full fetch fallback or refusal, to decide),
and the limit check against the stored blob size.
