---
title: "Test-coverage gaps from the ship-gate sweep (mailbox + dependency-landing gate)"
related:
  260913-feat-cross-session-mailbox-core: source — the store/queue/secret coverage gaps are in this ticket's package
  260913-feat-cross-session-mailbox-wake: source — the rebind-refusal and wait-clamp gaps are in this ticket's surface
  260913-feat-promotion-dependency-landing-gate: source — the dispatch_blocked JSON-contract and fail-open gaps are in this ticket's surface
  260909-epic-ws-worker-interpreter-refoundation: constraint — the landing-gate coverage touches the shared worker-interpreter tooling
---

# Test-coverage gaps from the ship-gate sweep (mailbox + dependency-landing gate)

## Background

The release-gate range sweep for ws ship (`c7761e2d..develop`, the mailbox
core + wake work and the dependency-landing gate) returned clean on the
correctness and fit partitions but non-clean on the test partition: 7
Important findings, all missing-test-case coverage gaps rather than behavioral
defects. The behavior was verified correct by static read against the ticket
contracts; these gaps are regression exposure, not live bugs. The release
proceeded under explicit owner override (no critical policy issue); this ticket
tracks closing the debt so a future ship clears the gate on its own merit.

Full findings artifact (this run):
`.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/2011b4cc-01-ship-gate-c7761e2d-develop.md`
(cache path, not committed — the itemized list below is the durable copy).

## Decisions

- Scope is test-only: add the missing assertions/cases below without changing
  production behavior. If a test reveals a real defect, split that into its own
  bug ticket rather than widening this one.

## Phases

### Phase 1: Close the Important coverage gaps

Add these test cases (each alongside the named production surface):

1. `internal/wsmailbox/store_test.go` `TestAppendQueueTrimsToMaxLen` — seed
   distinct/ordinal content and assert the surviving slice is the newest N in
   order, so a wrong-end trim or reorder fails (today identical `Content:"m"`
   only checks length).
2. `internal/mcp/mailbox_runtime.go` `rebindMailboxOwnerAtFerrule` — add a test
   where a *different, genuinely live* PID holds the presence record and assert
   the security-sensitive refusal branch fires (existing conflict tests only use
   `os.Getppid()` on the flagging path).
3. `internal/wsmailbox/store.go` `decodeStore`/`Load` — feed a corrupt/malformed
   JSON store file and assert the `json.Unmarshal` error path is handled.
4. `internal/wsmailbox/replyid.go` `EnsureMachineSecret`/`readValidSecret` — seed
   a truncated/wrong-length secret and assert repair (the doc-claimed recovery),
   not propagation of a bad value.
5. `internal/mcp/ticket_dispatch_gate_test.go` — add a `tickets.query
   format:"json"` test pinning the `dispatch_blocked` JSON contract
   (`blocking_stem`/`reason` field names and shape), so a serialization/rename
   regression on this public contract fails.
6. `internal/mcp/server.go` `DispatchBlockFor` — inject a board-scan error and
   assert the documented fail-open contract (gate degrades silently, does not
   error the call, `blockErr == nil` check not inverted).
7. `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` — add substring pins
   for the new `lead-run.md` / `worker-stop-protocol.md` obligation text against
   the *wsflow* copy, mirroring `agents-plugin/tests/test_skill_dispatch_contracts.py`'s
   pin of the `agents-plugin` copy (the byte-mirror carried the text but nothing
   pins the wsflow side).

Verify: `cd agents-plugin-tool && go test ./...` green; `python3 -m unittest
discover agents-plugin-wsflow/tests` green; each new test fails when its target
behavior is deliberately broken (mutation spot-check on at least items 1, 5, 6).

### Phase 2: Close the Minor coverage gaps (optional, batchable)

Lower-priority cases surfaced by the same sweep, safe to fold into Phase 1 or
defer:
- `store.go` `WithLock` lock-acquisition-timeout path.
- `address_test.go` `namePattern` 64-char boundary (63/64/65).
- `tickets_deps_test.go` self-referential `blocked-by:<own-stem>` edge.
- `wait_test.go` partial-final-sleep clamp branch (`remaining < poll`).
- `process_alive_test.go` PID-reuse hardening (assert `ProcessState.Exited()`).
- `test_skill_dispatch_contracts.py` branch-awareness pins for the
  stack-vs-return / base-derivation / dirty-tree text (not just the mechanical
  `ws/git.status` call + field names).

Verify: same suites green; no production change.

## Non-goals

- The three Minor correctness/fit observations from the sweep
  (`mailbox.recv` json empty-inbox `null` vs `[]`; idle-incumbent duplicate-slug
  false-negative; Windows `hooks.json` missing `|| true`; stop-hook control-flow
  duplication) are behavior/design notes, not test gaps — if actioned they get
  their own bug/refactor ticket, not this one.
