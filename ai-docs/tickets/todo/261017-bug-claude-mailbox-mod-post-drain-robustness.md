---
title: Claude mailbox push mod loses drained mail and leaks loop state on edge paths
related:
  261017-feat-claude-mailbox-active-push-mod: source feat whose Decisions this ticket keeps; its range sweep 6222b1cf3..4d46a0660 raised these findings
  260917-feat-ws-pi-mailbox-waiter-slug-wake: pi precedent for resolving the own named-inbox address through lookup_peers
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
  untested, as are the second-failure-streak, rebind, notice-submit-failed,
  and drain-error notice branches.
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
  framing. Only an empty string or a literal empty array is an empty drain.
  Rejected: the current empty-drain path (recv already removed the mail, so
  delivering the unparsed text is the only lossless outcome).
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
- **ws resolves its wake slug from `lookup_peers`.** At bind time the direct
  (ws) mode resolves its own named-inbox address from `lookup_peers`' self
  address through the same `$.mcp.call` path recv uses, and passes it as
  `--slug`; when it does not resolve, it falls back to `WS_MAILBOX`. wsflow
  keeps `WS_MAILBOX` only, since it makes no tool calls itself. This mirrors
  260917-feat-ws-pi-mailbox-waiter-slug-wake: a re-derived `WS_MAILBOX_AUTO`
  stem differs from the server's. Rejected: an address argument on the bind
  tool (changes the 261017 bind tool shape and trusts an agent-supplied
  address).
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

- Convention: `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/wsflow-mirroring.md` (declared for `agents-plugin/hooks/`
  and `agents-plugin-wsflow/`). The wsflow copy stays byte-identical; resync
  it after editing the source.
- Pi's mailbox waiter is unchanged.

## Phases

### Phase 1: Lossless post-drain handling and loop-state fixes

Implement the Decisions in one landing in `agents-plugin/hooks/mailbox-push/`,
then resync the wsflow copy byte-identically.

Verification: `claude plugin test agents-plugin` and
`claude plugin test agents-plugin-wsflow` pass with new tests for each
behavior change; `TestWsflowHooksMirrorUpToDate` passes; `go test` of the
concurrency test passes under `-race`.
