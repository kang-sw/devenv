---
title: "Pi completion mail has an unknown sender"
completed: 2026-11-13
---

# Pi completion mail has an unknown sender

## Background

Peer InspectTGV lead report, 2026-11-13 (wsflow 0.49.0): Pi completion mails
arrived as "mail from unknown", without a sender handle.

## Cause (code evidence)

- The completion mail is a system post: `PiTurnReporter`
  (`agents-plugin-tool/internal/mcp/pi_tools.go:490-498`) calls
  `wsmailbox.PostSystem` (`internal/wsmailbox/replyid.go:156-165`), which
  appends an `Envelope` with only `Content` and `SentAt`, no `From` or
  `ReplyTo`, by design (`Envelope`, `wsmailbox/store.go:82-87`).
- The push renderers fall back to `unknown` when both are empty:
  `formatMailEnvelope` (`internal/mcp/lead_mail.go:265-270`), the Pi adapter's
  `buildMailboxPushMessage` (`agents-plugin-pi/src/mailbox-waiter.ts:134`), and
  the Claude mod (`agents-plugin/hooks/claude-mod/watcher.ts:306`, `:327`).
  `mailbox.recv` and history label the same message `system` (`mailboxSender`,
  `internal/mcp/mailbox_tools.go:628-636`), so the two paths already disagree.
- `parseMailText` (`lead_mail.go:252-254`) maps an `unknown` head back to an
  empty sender.

## Decisions

Compatibility tier: T0. The change adds one optional envelope field (`label`,
`omitempty`) to a store whose reader ignores unknown fields; `From` and
`ReplyTo` keep their meaning (an address or `id:` handle), so no existing field
changes meaning and `schema_version` is unchanged. Skew: an older reader
ignores `label` and still shows `unknown`/`system`; the body's first line
(below) carries the reply instruction, so an old reader loses only the label.
Per `ai-docs/manuals/release-compatibility.md`.

Owner decision, 2026-11-13, option a: the completion mail's sender shows a
display label that is NOT a valid mailbox address, `pi:<subagent id>`, and the
body's first line tells the reader to reply with `pi.send(id: <id>)`.

- Rejected: a real mailbox address for the child. A lead would then reply by
  `mailbox.send` instead of `pi.send`.
- Rejected: leaving `unknown` and dropping the ticket.
- Rejected here: carrying the label in `From`. `From` is an address, and every
  reader records it as a correspondent (`recordMailboxRecvContacts`,
  `mailbox_tools.go:273-281`), which would add a non-peer to the contact list.
- The label cannot resolve as an address: `ParseAddress`
  (`wsmailbox/address.go:70-86`) accepts only `<slug>@<scope>` or
  `id:<reply-id>`; `pi:<id>` has no `@` and is not the `id:` prefix, so
  `mailbox.send` refuses it. A test pins this.

## Phases

### Phase 1: label field, renderers, reply line

1. Add `Label string` with tag `json:"label,omitempty"` to `Envelope`
   (`wsmailbox/store.go:82`). Add a `PostSystem` variant or parameter that
   sets it (`replyid.go:156`); update callers (`pi_tools.go:498`, test
   `wsagent/pi_worker_recovery_test.go:171`).
2. `PiTurnReporter` (`pi_tools.go:498`) posts Label `pi:<rep.Result.ID>` and
   prepends the first body line `Reply with pi.send(id: <id>), not
   mailbox.send.` (in the reporter only; `ReportText` also feeds the worker
   steer path at `wsagent/children.go:540`).
3. Renderers show the label when `From` and `ReplyTo` are empty, before the
   `unknown`/`system` fallback: `formatMailEnvelope` (`lead_mail.go:265`),
   `mailboxSender` (`mailbox_tools.go:628`), `buildMailboxPushMessage`
   (`mailbox-waiter.ts:134`), Claude mod `watcher.ts:306,327` (add `label` to
   its envelope type and `isEnvelope`). Keep the Go and TS head literals pinned
   together (see the comment on `formatMailBatch`).
4. `parseMailText` (`lead_mail.go:240-258`): a handle that fails
   `wsmailbox.ParseAddress` and is not `unknown` goes back as `Label`, never
   `From`, so contact recording skips it.
5. Tests: push head reads `mail from pi:<id> (<stamp>):` with the reply line
   first in the body; `ParseAddress("pi:<id>")` errors; a drained label
   envelope records no contact; the Pi adapter and Claude-mod suites cover the
   new field and the old-shape fallback. Read `ai-docs/manuals/ws-mcp.md` and
   `ai-docs/manuals/release-compatibility.md` before editing `internal/mcp/`;
   run `go test ./internal/mcp/... ./internal/wsmailbox/... ./internal/wsagent/...`
   and the Pi/Claude-mod node tests.

### Result (ca701289d) - 2026-11-13

- Added optional, display-only `Envelope.Label` and a label parameter to
  `PostSystem`. Pi completion mail uses `pi:<child id>` and starts its body
  with `Reply with pi.send(id: <id>), not mailbox.send.` `ReportText` is
  unchanged for worker steering; `From` and `ReplyTo` remain empty.
- Go push/recv/history, Pi push details, and Claude push/captions show the
  label after real reply handles. Returned push text restores non-address
  handles as `Label`, never as a contact. Existing unknown/system fallbacks
  remain; tests pin the same labelled push literal in Go and both adapters,
  address rejection, no contact pollution, and old-shape rendering.
- Canonical Claude mod and wsflow mirror remain byte-identical. Updated the
  MCP operations manual on contact.
- Merged develop through `3f882b14f` in `569c7817a` at the lead's request.
  Preserved streamed mailbox waits and both optional envelope fields
  (`label`, `undelivered_notified`); undelivered notices pass an empty label.
- Post-merge verification: locked w5090 full `go test ./...` passes with
  only the explicitly known
  `TestHostedConnectionSeeingALANChangeReplacesTheDaemon` excluded. Local
  locked Pi mailbox + stream suites: 111 passed; isolated Claude watcher
  suite: 130 passed. Go formatting and `git diff --check` were clean.
- Self-review and merge review: no remaining substantive findings.
- **Forward:** the broader local Pi package run (`npm test --
  --test-concurrency=2`, Windows Node 25.7.0) completed with 2411 passed,
  205 failed, 28 skipped. Representative `web-search.test.ts` failures at
  lines 146 and 244 reproduce unchanged on pre-ticket base `69481b31c`:
  Windows `d:` ESM URLs and unavailable symlink privilege (`EPERM`). Other
  full-package failures include unrelated fixture/runtime/skill assumptions
  and were not individually investigated; they are not claimed all to be
  baseline-proven. The required changed-surface suites pass. An earlier
  load-dependent readiness failure passes in isolation on both base and tip.

## Resolution

Phase 1 completed, including develop integration and affected-check reruns.
T0 additive optional field; no schema or mailbox-address semantic change.
Broader Windows Pi-suite limitations are recorded once in Forward above.
