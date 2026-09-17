---
title: pi mailbox waiter should also wake on the owned named inbox (slug), not only the reply-id channel
related:
  260914-feat-ws-pi-mailbox-native-steer-push: prerequisite
  260913-research-cross-session-mailbox: context
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1e8761298e5a571a
sage-review-completeness-reviewed: 1e8761298e5a571a
---

# pi mailbox waiter should also wake on the owned named inbox (slug), not only the reply-id channel

## Background

pi's native mailbox active-push waiter wakes only on the caller's reply-id
queue, never on the session's owned named inbox (the `WS_MAILBOX` slug). During
dogfooding, mail sent to a pi session's named slug address was never surfaced
and produced no signal (silent non-delivery): the recipient sat idle while its
owned inbox filled.

Root cause (confirmed by code read): the waiter spawns `mailbox wait
--session-key <key>` with **no** `--slug`
(`agents-plugin-pi/src/mailbox-waiter.ts`, the `createSubprocessWait` argv), so
its block-until-mail signal watches the reply-id queue only. The named inbox is
drained by `mailbox.recv` — which does read both queues — but only
opportunistically, i.e. when a reply-id wake happens to fire for some other
reason. With no reply-id traffic, slug-addressed mail is never woken on and can
sit undelivered indefinitely.

This is an explicitly documented Phase 1 limitation, not a defect of that phase:
see `260914-feat-ws-pi-mailbox-native-steer-push` ("Phase 1 arms the reply-id
inbox only (no `--slug`); `mailbox.recv` still drains an owned named inbox
opportunistically. Documented limitation, not a defect."). It was never carried
forward as a follow-up ticket; this ticket is that follow-up. The parent
research is `260913-research-cross-session-mailbox`, whose Decision 14 frames
"opt into a slug for a durable inbox + wake" as the intended contract — which
the pi adapter does not yet honor on the wake side.

## Decisions

- **Resolve the self slug from `mailbox.lookup_peers`, never from env.** At arm
  time the waiter asks the bridge client for `lookup_peers` and reads
  `self.address` (the server's registered named identity). It must **not**
  re-read `WS_MAILBOX` / `WS_MAILBOX_AUTO` from its own environment: `wait.go`
  warns that a fresh CLI process re-deriving `WS_MAILBOX_AUTO` mints a different
  random stem than the already-registered server process, so the env value is
  not a reliable identity for a child process. `lookup_peers` returns the
  authoritative address the server actually registered, covering both explicit
  `WS_MAILBOX` and `WS_MAILBOX_AUTO`.
  - Rejected: reading `process.env.WS_MAILBOX` directly — simpler but wrong for
    the `WS_MAILBOX_AUTO` case and duplicates identity resolution the server
    already owns.

- **When no self slug exists, keep today's reply-id-only behavior.** An env-less
  session (no registered named inbox) has no slug to watch; the waiter arms
  exactly as it does now. Adding a slug is strictly additive.

- **Silent-failure concern is absorbed here as motivation, not a separate
  ticket.** The observable bug (owned slug mail never surfaced) disappears once
  the waiter watches the slug; a standalone bug ticket would duplicate this
  scope.

## Constraints

- **Adapter-only, best-effort.** This must not alter the host-neutral mailbox
  `Envelope` or contract — the pi waiter's own doc comment states this
  invariant. The change lives entirely in the pi adapter's wait-arming and its
  argv to the shared `mailbox wait` CLI.
- **Owner-gate safety already exists in the CLI.** `mailbox wait --slug`
  owner-gates the named inbox and degrades to a reply-id-only wait (with a
  stderr warning) when the slug is not owned by the given `--session-key`
  (`agents-plugin-tool/cmd/ws-mcp/mailbox.go`). The adapter does not need to
  re-implement that guard; passing an unowned/stale slug is safe by
  construction.
- Not triggered: `skill-authoring.md` (no skill/prompt/convention edit) and
  `wsflow-mirroring.md` (pi-specific adapter, not shared `rsrc/` playbooks).

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/mailbox-waiter.ts, agents-plugin-pi/test/mailbox-waiter.test.ts, and the arm site agents-plugin-pi/src/index.ts#L646-649 (all now named in Phase 1) |
| scope.surface | internal | SubprocessWaitOptions/createSubprocessWait are agents-plugin-pi-internal (agents-plugin-pi/src/mailbox-waiter.ts#L212-233), no external package boundary crossed |
| scope.new_public_symbol | yes | a self-slug resolution helper called at arm time (via mailbox.lookup_peers); exact name unfixed |
| scope.new_type_contract | yes | SubprocessWaitOptions (agents-plugin-pi/src/mailbox-waiter.ts#L212-223) gains a slug field feeding a new `--slug` argv entry (createSubprocessWait, agents-plugin-pi/src/mailbox-waiter.ts#L233-244) |
| scope.test_surface | existing | agents-plugin-pi/test/mailbox-waiter.test.ts (ticket states extension); existing injected-fake style (scriptedWait etc.) |
| complexity.reuse_points | confirmed | createSubprocessWait (agents-plugin-pi/src/mailbox-waiter.ts#L233-275), mailbox.lookup_peers self.address (agents-plugin-tool/internal/mcp/mailbox_tools.go#L232-334), --slug owner-gate degrade (agents-plugin-tool/cmd/ws-mcp/mailbox.go#L61-90) |
| complexity.side_effect_risk | low | additive-only fallback to today's reply-id-only behavior when no self slug resolves; CLI owner-gates an unowned/stale slug by construction |
| risk.correctness | moderate | new async self-slug resolution at arm time plus absent-case fallback logic, not yet exercised by any test |
| risk.fit | low | Decision matches wait.go's own documented guidance to pass the address "already known... from lookup_peers' self entry" rather than re-deriving from env (agents-plugin-tool/internal/wsmailbox/wait.go#L32-39) |
| risk.test | moderate | ticket specifies both new test cases against injected fakes in the existing style, but no test precedent yet for a lookup_peers-based resolution fake |
| risk.security_or_contract | low | adapter-only; shared Envelope/contract untouched; CLI already owner-gates unowned/stale slugs (agents-plugin-tool/cmd/ws-mcp/mailbox.go#L61-90) |

## Phases

### Phase 1: Arm the pi waiter on the owned named inbox when a self slug resolves

Behavior: at waiter arm time (the owner-lead `session_start` arm site,
`agents-plugin-pi/src/index.ts#L646-649`, where `mailboxHandle.client` is in
scope), resolve the session's own registered address via the bridge client's
`mailbox.lookup_peers` — reached through the same `MailboxToolCall` seam
currently used only for `mailbox.recv` — and read `self.address`. When present,
pass it as `--slug <address>` into the `mailbox wait` invocation so the
block-until-mail signal covers the named inbox in addition to the reply-id
queue. When absent — or when the `lookup_peers` call itself errors (best-effort:
a transport failure must not block arming) — arm reply-id-only exactly as today.
Drain (`mailbox.recv`) and the push/admit path are unchanged — this phase only
widens the wake signal.

Verification: extend `agents-plugin-pi/test/mailbox-waiter.test.ts` with (a) a
slug-arm path asserting the resolved `self.address` reaches the wait invocation
as `--slug`, and (b) an address-absent fallback asserting the reply-id-only
invocation is unchanged. Both against injected fakes (no live mailbox), matching
the existing waiter test style. Manual dogfood check: send mail to a pi
session's owned slug while it is idle and confirm it now wakes and surfaces.

Deferred: no change to reply-id lifetime, discovery, or the host-neutral mailbox
contract; no new push family or message shape (the existing `ws-mailbox`
custom-message path carries the drained envelopes regardless of which queue woke
the waiter).
