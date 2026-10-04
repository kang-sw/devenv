---
title: Readable word-key mailbox reply-id
sage-review-design: completed
sage-review: required
sage-review-completeness: completed
sage-review-design-reviewed: 15d7d6f7cfb25733
sage-review-completeness-reviewed: 15d7d6f7cfb25733
---

# Readable word-key mailbox reply-id

## Background

Mailbox reply handles currently expose a 64-character lowercase hex encoding
of HMAC-SHA256(machine_secret, callerSessionKey). They identify a caller
session's return channel, not a message. Anonymous senders have no published
name, so shared recv text, JSON envelopes, peer lookup, and host waiter paths
can expose the long handle to agents. The user reports that verbose handles
can disrupt agent interaction and wants a shorter routable identifier.

## Decisions

- **D1 — Four full-pool words, concatenated without delimiters.** Derive four
  words deterministically from the caller session key using the existing full
  wskey word list, then concatenate them with no spaces or hyphens. Keep the
  wire address family `id:<joinedwords>`; for example,
  `id:ambertidefoxriver` illustrates the format, not a guaranteed generated
  value. The current full pool has 7,772 words with maximum word length nine,
  so the emitted body is at most 36 lowercase letters. Rejected: the old
  64-hex representation, hyphen-joined output, and the short-word pool for
  this new reply-ID derivation.
- **D2 — Preserve existing wskey.Derive semantics.** Existing short-pool
  derivation and ticket-derived branch identities remain unchanged. Add a
  separate full-pool derivation entry point or reuse a pool-parameterized
  internal helper without changing existing callers' results. Random
  WS_MAILBOX_AUTO naming is unchanged.
- **D3 — Session-bound, deterministic, no new persistence.** Derive directly
  from the caller session key, not the old keyed HMAC. Recomputing for the
  same session returns the same reply ID, including after restart. The reply
  registry remains flat, machine-tier, and scopeless; named inbox scopes do
  not become reply-ID scopes. Rejected: a new persisted alias/owner map,
  atomic collision reservations, and mint-once session-record wiring.
- **D4 — Accept rare local collisions and reduced guess resistance.** Four
  concatenated words are not a strict uniqueness guarantee. Distinct word
  tuples may concatenate identically; word boundaries need not be recovered.
  A collision can share a queue and misdeliver mail. The user explicitly
  accepts this local-use risk; do not add collision machinery or claim
  cryptographic uniqueness. Short IDs are routable addresses, not a retained
  256-bit bearer-capability guarantee.
- **D5 — Clean cutover.** Remove acceptance of legacy 64-hex reply addresses;
  do not migrate, drain in parallel, or alias legacy queues. In-flight legacy
  queues may become orphaned indefinitely: normal cleanup reaps only stale
  entries with empty queues and never deletes undelivered mail
  (agents-plugin-tool/internal/mcp/mailbox_runtime.go#L394-L419;
  agents-plugin-tool/internal/mcp/mailbox_runtime_test.go#L15-L50). This retains the earlier
  ticket's full-replacement decision, acknowledged during this settlement.
- **D6 — Retire reply-only secret machinery when unused.** Verify whether
  EnsureMachineSecret and the mailbox-secret cache have consumers other than
  reply-ID derivation. If none remain, remove the dead machinery and its
  obsolete tests/references in this cutover. If another consumer exists,
  retain it and record why. Rejected: keeping HMAC as the new derivation seed
  solely for reply IDs; the user selected direct session-key derivation.
- **D7 — Review before execution.** Populate facts and run one design reviewer
  and one completeness reviewer before ready promotion. This request does not
  authorize implementation.

## Constraints

- Apply the new identity consistently across mint/publication, send, recv,
  lookup, and wait paths that calculate or expose reply IDs, including shared
  native tooling and Pi consumers. Anonymous-sender text and JSON must expose
  the same routable ID as the underlying queue identity.
- Address validation accepts the new bounded lowercase, separator-free body
  and rejects legacy 64-hex addresses, empty bodies, delimiters, invalid
  characters, and overlength bodies. It need not reconstruct or validate word
  boundaries; `id:` remains distinct from `name@scope`.
- Preserve named inbox addressing, machine/worktree/clone scopes, recv
  authorization to the caller's own session channel, queue cleanup policy,
  registry layout, and existing deterministic branch identities.
- Read ai-docs/manuals/shipped-surface-boundary.md before editing shipped
  runtime strings or native tooling. Read ai-docs/manuals/ws-mcp.md before
  edits under agents-plugin-tool/internal/mcp/. If implementation requires
  shipped skill/playbook/convention changes, also apply the path-matching
  skill-authoring and wsflow-mirroring manuals declared in AGENTS.md.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Art

- agents-plugin-tool/internal/wskey/wskey.go: search Generate, Derive,
  wordPool, and shortWordPool to find the existing random and deterministic
  word-selection algorithms.
- agents-plugin-tool/internal/wsmailbox/replyid.go and agents-plugin-tool/internal/mcp/: search
  ReplyID, mailboxReplyID, publishReplyID, and EnsureMachineSecret for
  derivation and registry consumers.
- agents-plugin-tool/internal/mcp/impl_identity.go: the existing deterministic word derivation
  participates in implementation branch identity; its outputs are preserved
  (agents-plugin-tool/internal/mcp/impl_identity.go#L5-L10; no root-level
  internal/mcp/impl_identity.go exists).

## Prior Decisions

- d32d091c (2026-08-27, commit): "Generate() stays purely random over the full 7772-word pool for session-key minting; Derive() draws only from the <=5-char sub-pool" — bearing: constrains
- a5370cd1 (2026-06-10, commit): "Generator/policy separation is a hard brief boundary: wskey must not import mcp or auth packages; uniqueness enforcement belongs to the session registry in the mcp package." — bearing: constrains
- 260913-feat-cross-session-mailbox-core (2026-09-13, Result): "Decision 11's lazy-expiry reaping (`reapStaleReplyIDs`, 30-day retention) only reaps entries whose queue is already empty, never dropping undelivered mail." — bearing: constrains
- 260913-research-cross-session-mailbox (2026-09-13, Confirmed Decisions): "Delivery stamping is decided by the **sender's server**, which knows its own identity (reply-id + any published slugs)" — bearing: constrains
- 260917-feat-ws-pi-mailbox-waiter-slug-wake (2026-09-18, Result): "`self.address` is populated from the caller's own resolved identity (`mailboxOwnerCheck`) independent of which scope is queried" — bearing: constrains
- 260914-feat-ws-pi-mailbox-native-steer-push (2026-09-15, Result): "on exit code 0 the loop drains authoritatively through the `mailbox.recv` MCP tool (`createBridgeDrain`, `format:\"json\"`)" — bearing: constrains
- 260913-bug-mailbox-lookup-peers-self-leak (2026-09-13, commit 06988235): "a caller's own reply-id never appears in peers[] because it lives in a separate store (the reply-id registry)" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wskey/wskey.go, agents-plugin-tool/internal/wsmailbox/replyid.go and address.go and wait.go, agents-plugin-tool/internal/mcp/mailbox_runtime.go and mailbox_tools.go; Pi consumer verification in agents-plugin-pi/src/mailbox-waiter.ts and src/index.ts |
| scope.surface | cross-module | Go wskey/wsmailbox/MCP/CLI and Pi adapter consume the reply channel; id: address bodies and send/recv/self-lookup output change publicly |
| scope.new_public_symbol | unknown | D2 permits a separate exported full-pool entry point or a pool-parameterized internal helper; exact symbol exposure is intentionally left to implementation |
| scope.new_type_contract | yes | D1/D3/D5 replace the accepted reply-address encoding and session-to-queue identity; existing ReplyID takes secret plus session key at agents-plugin-tool/internal/wsmailbox/replyid.go#L121-L130 |
| scope.test_surface | existing | agents-plugin-tool/internal/wskey/wskey_test.go, internal/wsmailbox/address_test.go and replyid_test.go and wait_test.go, internal/mcp/mailbox_tools_test.go and mailbox_runtime_test.go and impl_identity_test.go, cmd/ws-mcp/mailbox_test.go; agents-plugin-pi/test/mailbox-waiter.test.ts and mailbox-bootstrap.integration.test.ts |
| complexity.reuse_points | confirmed | wskey wordPool and shortWordPool plus Derive read at agents-plugin-tool/internal/wskey/wskey.go#L20-L42 and #L73-L89; embedded eff_large_wordlist.txt count 7772, min length 3, max 9, all ASCII lowercase letters (Python split/count/max check); branch suffix uses unchanged Derive(stem, 3) in internal/mcp/impl_identity.go#L5-L10 |
| complexity.side_effect_risk | moderate | Shared registry map keys change across MCP mint/drain/piggyback and CLI wait; ReplyStore uses unrestricted string-keyed maps with no fixed ID width or entry-count cap (agents-plugin-tool/internal/wsmailbox/replyid.go#L141-L181); AppendQueue retains newest 200 messages per key (store.go#L97-L100 and #L257-L269) |
| risk.correctness | high | Derivation must agree in publish/send/recv/self-lookup/piggyback (agents-plugin-tool/internal/mcp/mailbox_tools.go#L49-L118, #L169-L188, #L321-L330, #L452-L462) and native wait (internal/wsmailbox/wait.go#L169-L179); accepted collisions can merge queues |
| risk.fit | moderate | Preserve Derive results and named addressing while replacing HMAC throughout; repository-wide search for EnsureMachineSecret, MachineSecretPath, mailboxSecret and mailbox-secret found only reply-ID derivation, its cache, and mailbox tests, so D6's removal condition is met for the current tree |
| risk.test | moderate | Existing text return-path/restart tests at agents-plugin-tool/internal/mcp/mailbox_tools_test.go#L78-L185 need new encoding expectations and JSON evidence; Pi passes the same session key to native wait and mailbox.recv, without deriving IDs itself (agents-plugin-pi/src/mailbox-waiter.ts#L295-L304 and #L405-L408; src/index.ts#L812-L844), and renders reply_to verbatim (#L89-L103) |
| risk.security_or_contract | high | D4 explicitly accepts reduced guess resistance and queue misdelivery on collision; D5 intentionally rejects old addresses, while nonempty legacy queues are retained indefinitely by unchanged cleanup (agents-plugin-tool/internal/mcp/mailbox_runtime.go#L394-L419); historical HMAC secrecy is explicitly superseded, not silently preserved |

## Phases

### Phase 1: Replace reply handles with concatenated full-pool word keys

**Intended behavior.** Introduce deterministic four-word full-pool derivation
from the caller session key without altering existing Derive output. Replace
reply-ID minting and address validation, and align every send/recv/lookup/wait
consumer with the new session-derived queue key. Shared anonymous-sender text
and JSON expose `id:<joinedwords>` or its correctly prefixed reply-handle field,
with no legacy hex fallback. Perform contingent reply-only secret retirement.

**Deferred scope.** No alias mapping, collision reservations, legacy address or
queue migration, new reply scopes, named-inbox format changes, random auto-name
changes, or unrelated branch-identity changes.

**Verification boundary.** Test deterministic four-word full-pool selection,
separator-free lowercase output and the current 36-character upper bound;
prove full-pool words longer than five letters are eligible. Preserve existing
short-pool Derive and branch-identity tests. Test parser acceptance of generated
IDs and rejection of legacy 64-hex, empty, delimited, invalid-character, and
overlength forms, while named inbox addressing remains unchanged. Verify an
anonymous send -> recv -> reply round trip routes the readable reply handle
back to the original session queue, with both text and JSON evidence. Exercise
peer/self lookup, caller-ID recomputation on recv/wait and restart stability,
including relevant Pi waiter integration. Verify different test sessions have
separate expected IDs without asserting collision-free uniqueness. Run the
relevant Go and Pi suites as applicable; if secret machinery is removed, ensure
no remaining caller, test, or shipped string references its retired contract.
