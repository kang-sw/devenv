---
title: Readable word-key mailbox reply-id
---

# Readable word-key mailbox reply-id

## Background

The mailbox reply-id currently surfaces to callers as a 64-hex string (e.g.
`ccca158faf790d9428042d93650a550ba0a3ae70b36b7a08544997149ea7f50e`). It is
`hex(HMAC-SHA256(machine_secret, callerSessionKey))` — a per-caller-session
return-channel handle, deterministic and recomputed on every recv from the
session key (grep for `ReplyID` / `mailboxReplyID` in the `wsmailbox` package
and `internal/mcp`). It is **not** a per-message id and **not** content-derived:
`Envelope` carries no id field.

This handle is emitted by the **shared** ws-mcp recv path, not a host-specific
UI: the native recv text render falls back to the raw reply-id when a message
carries no `from` stamp (search the recv handler in `internal/mcp` for the
`handle := m.From` / `if handle == "" { handle = m.ReplyTo }` fallback), and the
JSON recv path returns the raw envelope. So every host (claude, codex, pi) sees
the raw hex whenever an **anonymous sender** — a process that published no
`WS_MAILBOX` / `WS_MAILBOX_AUTO` identity — sends mail. The pi waiter render
mirrors the same fallback and is not the source.

Long, high-entropy handles injected into an agent's context measurably degrade
agent performance; this is an agent-ergonomic / token-hygiene problem, not a
cosmetic one. Goal: replace the hex reply-id with a short, readable, **routable**
word-key so agents read and reply to `id:amber-tide-fox-...` instead of a
64-char hash, uniformly across all hosts.

## Decisions

- **D1 — Replace the hex reply-id with a scopeless 4-word deterministic key.**
  The reply-id mint becomes `wskey.Derive(callerSessionKey, 4)` (the existing
  deterministic word-key generator), producing a 4-word hyphen-joined handle in
  place of the HMAC hex. Rejected alternatives:
  - Status-quo HMAC-hex: unreadable, degrades agents — the problem being solved.
  - **β, mint-once + persist** a unique key in the session record
    (`GenerateUnique`, collision-free): eliminates the collision risk of D3 but
    adds a persisted session-record field plus "ensure-minted" logic on both the
    send and recv paths. Rejected: the required correctness level does not
    justify the extra wiring (see D3).
  - Reusing the `name@scope` slug machinery: rejected in D2.
- **D2 — Scopeless; stay in the flat machine-tier reply registry.** The reply-id
  lives in a single flat, scopeless registry (the `mailbox-replyids.json`
  machine-tier store, queues keyed by the id string, addressed as `id:<key>`
  with no `@scope`). `scope` (machine/worktree/clone) is an isolation concept for
  **published named inboxes**, which the reply-id deliberately is not. The
  word-key keeps this exactly: `id:word-word-word-word`, no scope, same registry
  structure — **only the key encoding changes**, not the registry, not the
  address family. Dragging the reply-id into `name@scope` was considered and
  rejected: it would invent scope the reply-id never had.
- **D3 — Determinism via recompute, not persistence.** recv recomputes the
  word-key from the session key exactly as the HMAC did today, so **no new
  persistence** is introduced and the existing "no reply-id is ever persisted"
  property is preserved. This admits a small cross-session collision risk (two
  distinct session keys deriving the same 4-word key would share a queue →
  mail misdelivery). Accepted: this channel does not demand that correctness
  level, and **4 words** (rather than 3) pushes the collision probability to
  negligible at this system's session counts. 4 is the chosen readable-length ↔
  collision-margin balance.
- **D4 — Accepted capability-model shift: the channel becomes publicly
  addressable.** "Readable + routable" necessarily means "short + guessable +
  addressable": once a short readable string routes to a queue, anyone can guess
  it and send. This drops the current property whereby an anonymous reply-id is
  an **unguessable reply-only bearer capability** (only a party that received a
  message could reply). This shift is intrinsic to the requirement and is the
  desired direction for agent ergonomics (stable, addressable peers), so it is
  accepted, not mitigated.
- **D5 — Full replacement (clean cutover), not parallel retention.** The hex form
  is removed, not kept alongside the word-key. Reply-ids are ephemeral
  (staleness-reaped, fast-draining queues), so a hex-keyed queue left in flight
  at upgrade is orphaned only briefly and is a non-issue at this repo's scale; no
  migration shim is in scope.

## Constraints

- **Format gate.** The reply-id validation pattern (currently `^[0-9a-f]{64}$`
  in the address parser) relaxes to the 4-word shape `^[a-z]+(-[a-z]+){3}$`.
  Disambiguation against the slug `name@scope` form is intact and requires no
  extra guard: the `id:` address form contains no `@`, and the slug form
  requires one, so `ParseAddress` separates them cleanly.
- **`machine_secret` retirement (contingent cleanup).** `EnsureMachineSecret`
  and the `mailbox-secret` cache file exist solely to make the reply-id
  unguessable. `Derive` uses only the session key, and the reply-id is a shared,
  handed-out value, so the secret protects nothing that survives this change.
  Verify the reply-id is the secret's only consumer, then retire the dead
  machinery in the same cutover; if another consumer exists, leave it and note
  why.
- **Shared surface, all hosts.** The change lives in the shared Go mint / recv /
  address-parse path, so claude, codex, and pi benefit uniformly. The pi waiter
  render already prefers `from` and mirrors the shared fallback, so no
  pi-specific change is required beyond what flows from the shared change.
- **Tests are the behavioral contract.** Reply-id format assertions,
  `ParseAddress` `id:` cases, and recv-render tests move with this change; the
  send→recv→reply round trip is re-pinned on the word-key. A behavior change with
  no test change is a review finding.
- **Path-scoped manual.** Edits under `agents-plugin-tool/internal/mcp/` carry
  the `ws-mcp.md` manual read obligation (AGENTS.md Implementation Conventions).
  No shipped-skill prose changes — this is runtime tool behavior only.

## Prior Art

- `wskey.Derive(seed, n)` — the deterministic SHA-256-seeded word-key generator
  (the swap target); `wskey.Generate` is the random variant used by
  `WS_MAILBOX_AUTO` and is unchanged here.
- The reply-id mint (`ReplyID` / `mailboxReplyID` / `publishReplyID`) in the
  `wsmailbox` package and `internal/mcp` is the swap site.
- The `name@scope` slug inbox already provides a routable-readable named-address
  form; this ticket deliberately does **not** route through it (D2) and stays in
  the flat reply-id registry.

## Phases

### Phase 1: Swap reply-id to a scopeless 4-word key

**Intended behavior.** The reply-id mint returns `wskey.Derive(sessionKey, 4)`
instead of the HMAC hex; the address-parser format gate accepts the 4-word shape
and rejects malformed handles; recv (text and JSON) surfaces the word-key, and
`id:<4-word-key>` routes back to the same flat queue. Once the last consumer is
confirmed gone, remove `EnsureMachineSecret` and the `mailbox-secret` file.

**Deferred scope.** The `name@scope` slug path and `WS_MAILBOX_AUTO`'s random
naming are untouched; no migration shim for in-flight hex-keyed queues (D5);
the capability-model shift (D4) is accepted as-is with no unguessability
mitigation.

**Verification boundary.** Existing mailbox tests green with format assertions
updated to the word-key; a round-trip test proves send stamps a 4-word
reply-id, recv renders it (no hex), and `id:<word-key>` delivers back to the
sender's queue; `ParseAddress` accepts `id:<4-word>` and rejects a malformed
handle while still separating `id:` from `name@scope`. If `machine_secret` is
retired, its removal builds cleanly and no test or caller still references it.
