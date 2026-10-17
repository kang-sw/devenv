---
title: Claude mailbox push mod loses drained mail and leaks loop state on edge paths
related:
  261017-feat-claude-mailbox-active-push-mod: source feat whose Decisions this ticket keeps; its range sweep 6222b1cf3..4d46a0660 raised these findings
  260917-feat-ws-pi-mailbox-waiter-slug-wake: pi precedent for resolving the own named-inbox address through lookup_peers
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 01cc3f86052ef986
sage-review-completeness-reviewed: 01cc3f86052ef986
---

# Claude mailbox push mod loses drained mail and leaks loop state on edge paths

## Background

The range sweep `6222b1cf3..4d46a0660` (stamped `concern`) found that the
Claude mailbox push mod (`agents-plugin/hooks/mailbox-push/`, byte-identical
copy in `agents-plugin-wsflow/hooks/mailbox-push/`) can drop mail that the
server has already removed from its queue, and that several lifecycle edges
are wrong or unpinned.

Server side, `mailbox.recv` removes mail as part of the call and returns an
error only when it drained nothing (`handleMailboxRecv` in
`agents-plugin-tool/internal/mcp/mailbox_tools.go`: a storage error with an
empty drain is an error; a partial drain still returns what it drained). So
once recv returns a non-error result, the mod is the only holder of that mail.

Findings in scope (sweep IDs in parentheses):

- (I1) `parseRecvText` maps any non-empty text that is not a JSON array to
  `[]`; the loop then takes the empty-drain branch and the mail is gone with
  only a debug-log line.
- (M-a) When `$.prompt.submit` fails after a drain, the batch is only written
  to the debug log.
- (I2) The recv `tool.call` observer's double-drain guard
  (`.catch(... next.called ? ... : next(e))`) and its post-`next` try/catch
  are untested; the own-suspension resume in `suspend()`/`notify()` (the
  rebind race fixed in 79c1d0a80) is unpinned.
- (M-g) The 4d46a0660 notice wiring to `$.ui.log`'s default destination is
  untested (the engine tests stub `ui.log` and discard it), and the watcher
  tests do not assert which notices are emitted on a second failure streak
  after recovery, on a rebind (`armed` for the new key), on a drain re-arm
  (none), or on the notice-submit-failed and drain-error back-off paths.
- (M-b) Only `WS_MAILBOX` is passed to the wait as `--slug`; mail to a
  `WS_MAILBOX_AUTO` named inbox does not wake the loop. Not a regression (the
  retired Stop hook read only `WS_MAILBOX`); recv still drains that inbox.
- (M-c) Each `consumeWait` and each normally-completed `sleep` leaves an abort
  listener on the loop signal; one accumulates per cycle for a binding's life.
- (M-d) The hot-reload restore awaits `$.mcp.connect` between reading
  `$.store` and calling `bind`, so an agent bind made in that window is
  overridden by the stale stored key.
- (M-e) A drain in flight when `session.end` stops the loop is still
  submitted ("deliver it even if a rebind raced in"), so it lands in the next
  conversation after `/clear` or `/resume`.
- (M-f) The `register.ts` header says no package name is spelled, but
  `drainModeFor` compares the plugin name with `ws`.
- (M-i) `mailbox_recv_concurrency_test.go` calls `callToolWithKey`, which
  `t.Fatal`s, from goroutines; a failure surfaces as spurious lost-envelope
  errors.

## Decisions

- **The 261017 feat's Decisions stay in force**: explicit bind, per-package
  drain mode, no new MCP tool or CLI subcommand, no `$.tool.check`, and wsflow
  never drains or calls tools itself. This ticket fixes behavior against that
  contract; it is not a redesign.
- **An `isError` recv result keeps its retry/back-off path.** The server
  returns an error only when it drained nothing, so the 261017 "mail stays
  queued" premise holds server-side.
- **Unparseable recv text is delivered, not dropped.** A non-empty recv text
  that does not parse as a JSON array is submitted raw inside the batch
  framing. After trimming surrounding whitespace, only an empty string,
  `null` (the server's JSON encoding of an empty drain: `handleMailboxRecv`
  marshals a nil slice), or `[]` is an empty drain. Array elements that fail
  the envelope shape (no string `content`) are delivered raw as well, for the
  same reason. Rejected: the current empty-drain path (recv already removed
  the mail, so delivering the unparsed text is the only lossless outcome).
- **Submit failure after a drain leaves a user-visible record.** When
  `$.prompt.submit` fails after a drain, the formatted batch is written to the
  transcript through `$.ui.log` at its default (user-visible) destination, as
  well as to the debug log. Rejected: re-sending the mail to the session's own
  reply-id (rewrites the sender and return handle, and can loop while submit
  keeps failing).
- **An in-flight drain at `session.end` is not submitted.** A batch drained
  while `session.end` stops the loop is written to the transcript through
  `$.ui.log` (user-visible) and the debug log, the same record as a submit
  failure. A rebind to a new key within the same conversation still delivers,
  as today. Rejected: delivering into the next conversation (the current
  behavior; the model receiving it has no context for it, and it defeats the
  session.end stop); re-sending to the old key's reply-id (rewrites the
  sender, and the old key may never bind again).
  The loop needs a way to tell a stop from a rebind: today both `stop()` and
  `bind()` only abort the controller. Every stop path (`session.end`, and the
  `session.start` re-entry stop) takes the record path; only a rebind
  delivers. On a `session.end` that is a process exit the transcript record
  may never be shown; the debug-log copy remains.
- **ws resolves its wake slug from `lookup_peers`.** At bind time the direct
  (ws) mode resolves its own named-inbox address from `lookup_peers`' self
  address through the same path recv uses (`$.tool.call` on the tool name
  resolved from `$.mcp.connect`), and passes it as `--slug`; when it does not
  resolve or is refused, it falls back to `WS_MAILBOX`. wsflow keeps
  `WS_MAILBOX` only, since it makes no tool calls itself. This mirrors
  260917-feat-ws-pi-mailbox-waiter-slug-wake: a re-derived `WS_MAILBOX_AUTO`
  stem differs from the server's. Rejected: an address argument on the bind
  tool (changes the 261017 bind tool shape and trusts an agent-supplied
  address).
  Consequences: the slug is carried per binding into the wait argv (today the
  wait edge closes over one session-start slug) and re-resolved on every
  rebind, since the named-inbox owner moves with the key; overlapping binds
  resolve latest-wins (pi's sequence guard in `createMailboxWaiterSlot` is the
  precedent); the restore's "no newer bind" check also runs after its slug
  await. In a prompting permission mode the lookup can raise a dialog inside
  the bind call, accepted as the 261017 direct-drain dialog is.
- **Abort listeners are removed when their await settles.** Each wait's and
  sleep's abort listener is removed when that wait or sleep settles. Rejected:
  one shared listener per loop (a larger restructuring for the same bound).
- **The hot-reload restore yields to a newer bind.** The restore binds the
  stored key only when no binding was made since the restore started.
  Rejected: serializing restore and bind through a lock (new state for a
  window a single check closes).
- **Approved sweep cleanups ride along:**
  - fix the `register.ts` header comment (M-f);
  - make the concurrency test's goroutine calls non-fatal and report their
    errors on the test goroutine (M-i);
  - add the I2 and M-g tests.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- The wsflow copy stays byte-identical; resync it after editing the source.
- Pi's mailbox waiter is unchanged.

## Prior Decisions

- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Result 79c1d0a80): "Residual: a `WS_MAILBOX_AUTO` address is not known to the mod, so its named inbox does not wake the loop; recv still drains it. A `prompt.submit` failure after a successful ws drain leaves that mail logged only to the debug log." — bearing: supports
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Result 79c1d0a80): "`session.end` stops the loop for every reason, not only `clear`. On `resume` the process continues under another session id, and a loop still bound would drain old-key mail into the new conversation." — bearing: constrains
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Decisions): "`ws` always drains through recv itself (`$.mcp.call`, server name from `$.mcp.connect`) with no pre-check, and submits the whole drain as one prompt, mirroring pi." — bearing: constrains
- 99f22d9d (2026-10-17, commit): "The wait adds --slug from WS_MAILBOX so a named-inbox session is woken, covering what the retired Stop hook read from the same variable; the wait owner-checks the slug and falls back to reply-id-only." — bearing: supports
- 99f22d9d (2026-10-17, commit): "The bound key persists in $.store keyed by session id, not $.state: a $.state ref must spell its owning plugin as a literal, which a source shared byte-identically by ws and wsflow cannot" — bearing: constrains
- 79c1d0a8 (2026-10-17, commit): "on `resume` the process continues under another session id, so a loop still bound to the old key would drain that key's mail into an unrelated conversation in ws direct mode." — bearing: constrains
- 4d46a066 (2026-10-17, commit): "The \"mail waiting\" status is dropped without a replacement: the submitted notice prompt is already visible in the transcript." — bearing: supports
- 261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers (2026-10-09, commit e5202ac3): "A throwing admit now loses the whole drain instead of one envelope; same failure class as before (mail is already drained either way)." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/mailbox-push/{register.ts,watcher.ts,engine.test.ts,watcher.test.ts}, byte-identical copy in agents-plugin-wsflow/hooks/mailbox-push/, agents-plugin-tool/internal/mcp/mailbox_recv_concurrency_test.go |
| scope.surface | cross-module | the mod is mirrored into a second package and the Go test lives in agents-plugin-tool; exports touched are watcher.ts parseRecvText and the WatcherEdges contract |
| scope.new_public_symbol | unknown | the slug-resolution step and the transcript-record edge are specified as behavior only; no symbol is named in the ticket |
| scope.new_type_contract | unknown | delivering unparseable recv text raw likely widens DrainResult or parseRecvText's return (watcher.ts#L81-L94), but the ticket names no signature |
| scope.test_surface | existing | agents-plugin/hooks/mailbox-push/engine.test.ts, watcher.test.ts; agents-plugin-tool/internal/mcp/mailbox_recv_concurrency_test.go; new tests land in these files |
| complexity.reuse_points | confirmed | pi's lookup_peers self-slug resolution (agents-plugin-pi/src/mailbox-waiter.ts, resolveMailboxSelfSlug) and the existing $.tool.call recv path in register.ts#L170-L173 |
| complexity.side_effect_risk | moderate | changes the drain loop and session lifecycle of a mirrored mod that can drop or misdeliver mail; the wsflow copy must resync |
| risk.correctness | high | async races (hot-reload restore vs bind, in-flight drain vs session.end, abort listeners) on a path where mail is already removed server-side |
| risk.fit | moderate | bounded by the 261017 feat Decisions and a pi precedent, but the mod API is early access and wsflow must keep making no tool calls |
| risk.test | moderate | the lifecycle races need engine-level tests over a stubbed $ with timing control; only existing harnesses exist |
| risk.security_or_contract | moderate | drained mail content is now written to the user-visible transcript on submit failure and session.end; recv contract (error only on empty drain) is relied on |

## Phases

### Phase 1: Lossless post-drain handling and loop-state fixes

Implement the Decisions in one landing in `agents-plugin/hooks/mailbox-push/`,
then resync the wsflow copy byte-identically.

The transcript record (submit failure, stop path) is one `$.ui.log` entry
that names why the batch was not submitted and then carries the formatted
batch exactly as it would have been submitted.

Verification:

- `claude plugin test agents-plugin` and `claude plugin test
  agents-plugin-wsflow` pass, with a new test per Decision: unparseable text,
  `null`/`[]`/blank, and shape-failing elements; submit failure writes the
  record; a stop-path drain writes the record and a rebind drain submits;
  slug resolved from `lookup_peers` with `WS_MAILBOX` fallback and re-resolved
  on rebind; listener count bounded across cycles; restore yields to a newer
  bind; the I2 guards and the M-g notice assertions.
- `go test -race ./internal/wsrsrc/ -run TestWsflowHooksMirrorUpToDate` and
  `go test -race ./internal/mcp/ -run
  TestMailboxRecvConcurrentCallsNeverDoubleDeliver` pass under
  `agents-plugin-tool/`; a forced recv error in that test is reported once on
  the test goroutine, without spurious lost-envelope errors.
