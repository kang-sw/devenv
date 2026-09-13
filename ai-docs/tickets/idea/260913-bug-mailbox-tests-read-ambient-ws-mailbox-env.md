---
title: "mailbox MCP tests read the ambient WS_MAILBOX env instead of controlling it (fail on a mailbox-configured dev shell)"
related:
  260913-feat-cross-session-mailbox-core: source — the self-address / inert-by-default tests that leak the ambient env live in this ticket's surface
  260913-test-mailbox-and-landing-gate-coverage-gaps: context — the ship-gate coverage work that these tests belong to
---

# mailbox MCP tests read the ambient WS_MAILBOX env instead of controlling it

## Background

Dogfood finding (2026-09-13), surfaced while hotfixing the `lookup_peers`
self-leak on a dev machine whose real shell exports `WS_MAILBOX=devenv@machine`
(the mailbox setup the maintainer runs for cross-session work).

On that shell, `cd agents-plugin-tool && go test ./...` fails on **unmodified**
code: `internal/mcp` `TestMailboxInertByDefault` and
`TestMailboxSelfAddressSurface` both read the ambient `WS_MAILBOX` /
`WS_MAILBOX_AUTO` from the process env instead of clearing/setting it
themselves, so the "inert by default" and self-address expectations flip. Proven
by `git stash` (fails on pristine tree too) and by re-running after
`unset WS_MAILBOX WS_MAILBOX_AUTO` (green). CI never caught it because CI runners
carry no `WS_MAILBOX`.

This is a test-isolation defect, not a production bug: the tests make an
environment assumption they should own. It is invisible on a clean environment
and only bites a maintainer who actually dogfoods the mailbox — the worst
audience to leak onto.

## Investigation

Needed: audit `agents-plugin-tool/internal/mcp/*mailbox*_test.go` (at minimum the
two named tests, and any sibling that resolves mailbox identity) for reads of
`WS_MAILBOX` / `WS_MAILBOX_AUTO` that are not first neutralized. The fix is to
have each affected test control the env deterministically via `t.Setenv`
(setting or clearing to the value the case intends) so the outcome does not
depend on the developer's shell. Check whether a package-level helper or
`TestMain` should clear both vars once for the whole `internal/mcp` mailbox
suite rather than per-test.

## Outcome Ledger

### Verified Findings

- On a shell with `WS_MAILBOX=devenv@machine`, `go test ./...` in
  `agents-plugin-tool` fails `TestMailboxInertByDefault` and
  `TestMailboxSelfAddressSurface` on the pristine tree; `unset WS_MAILBOX
  WS_MAILBOX_AUTO` makes the same suite green. The tests read ambient env they
  should control.

### Proposals

- Make the affected tests deterministic with `t.Setenv` (set the intended value,
  or clear it) so neither the pass nor the fail depends on the caller's shell;
  consider a shared clear in a mailbox-suite helper / `TestMain`. Add nothing to
  production code.

### Open Questions

- Should there be a single suite-wide guard (e.g. `TestMain` clearing both vars)
  vs. per-test `t.Setenv`? Per-test is more explicit and localizes intent;
  suite-wide is fewer edits but hides the dependency. Lean per-test unless the
  count is large.

### Rejected Alternatives

- "Just document that maintainers must unset WS_MAILBOX before running tests" —
  rejected: a test that depends on the developer's ambient shell is the defect;
  documenting the workaround leaves the trap armed for the next dogfooder.
