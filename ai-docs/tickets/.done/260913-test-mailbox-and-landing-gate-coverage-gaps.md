---
title: "Test-coverage gaps from the ship-gate sweep (mailbox + dependency-landing gate)"
related:
  260913-feat-cross-session-mailbox-core: source — the store/queue/secret coverage gaps are in this ticket's package
  260913-feat-cross-session-mailbox-wake: source — the rebind-refusal and wait-clamp gaps are in this ticket's surface
  260913-feat-promotion-dependency-landing-gate: source — the dispatch_blocked JSON-contract and fail-open gaps are in this ticket's surface
  260909-epic-ws-worker-interpreter-refoundation: constraint — the landing-gate coverage touches the shared worker-interpreter tooling
completed: 2026-09-13
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

### Result (e31576b8) - 2026-09-13

All 7 Important gaps closed, test-only (zero production change):
1. `internal/wsmailbox/store_test.go` `TestAppendQueueTrimsToMaxLen` rewritten —
   seeds ordinal `m0..mN`, asserts survivors are the newest N in send order.
2. `internal/mcp/mailbox_tools_test.go`
   `TestRebindMailboxOwnerRefusesLiveDifferentPIDHolder` — a genuinely-live
   different PID (`os.Getppid()`) takes the record; asserts the
   `p.PID != pid && mailboxPresenceLive` refusal branch fires (the
   post-registration re-verify branch the flagging-path tests never reached).
3. `store_test.go` `TestLoadRejectsCorruptStore` — malformed JSON surfaces a
   `parse mailbox store` error, not a silently-zeroed store.
4. `internal/wsmailbox/replyid_test.go`
   `TestEnsureMachineSecretRepairsTruncatedFile` — a wrong-length secret is
   regenerated to a valid `secretByteLen` secret, not propagated.
5. `internal/mcp/ticket_dispatch_gate_test.go`
   `TestTicketsQueryJSONDispatchBlockedContract` — pins
   `dispatch_blocked`/`blocking_stem`/`reason` via raw-substring and
   tagged-struct unmarshal (json-tag rename now fails).
6. `ticket_dispatch_gate_test.go`
   `TestTicketsQueryPointResolveDispatchGateFailsOpen` — injects a board-scan
   error (unreadable `.dropped/` file, scanned only by `boardByStem`); asserts
   the ticket still resolves and omits `dispatch_blocked`. Mutation-verified
   (propagating `blockErr` made it fail, then reverted).
7. `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`
   `test_wsflow_run_and_stop_protocol_carry_merge_obligation_text` — mirrors
   agents-plugin's substring pins against the wsflow `lead-run.md` /
   `worker-stop-protocol.md` copies (template vars preserved verbatim).

Verified: `go build/vet ./...` OK; `go test -count=1 ./...` all packages ok
(uncached); `python3 -m unittest discover agents-plugin/tests` 69 OK;
`agents-plugin-wsflow/tests` 12 OK. No production defect found.

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

### Result (e31576b8) - 2026-09-13

Minor gaps folded in, test-only:
- `store_test.go` `TestWithLockTimesOutWhenLockHeld` — pins the real
  lock-acquisition-timeout error (`gofrs/flock` surfaces the expired context via
  `acquire mailbox store lock: context deadline exceeded`).
- `address_test.go` `TestIsValidNameLengthBoundary` — 63/64/65-char
  `namePattern` boundary.
- `wait_test.go` `TestWaitClampsFinalSleepToRemaining` — Timeout=1200ms,
  Poll=500ms; asserts the final block is the clamped 200ms remainder with no
  overshoot.
- `internal/wsstate/process_alive_test.go` — hardened with a
  `cmd.ProcessState.Exited()` assertion against PID reuse.
- `agents-plugin/tests/test_skill_dispatch_contracts.py`
  `test_run_pins_branch_awareness_reasoning` — pins the stack-vs-return /
  base-derivation / dirty-tree text.

Not added (deliberate): the self-referential `blocked-by:<own-stem>` case. Read
`tickets_deps.go`: a ticket listing its own stem resolves to itself and blocks
via the generic bare-stem "not `.done/`" branch already covered by
`TestDispatchBlockForPredicate` — a self-ref test would re-assert an existing
branch with no new coverage, so it was skipped rather than adding a redundant
test.

## Non-goals

- The three Minor correctness/fit observations from the sweep
  (`mailbox.recv` json empty-inbox `null` vs `[]`; idle-incumbent duplicate-slug
  false-negative; Windows `hooks.json` missing `|| true`; stop-hook control-flow
  duplication) are behavior/design notes, not test gaps — if actioned they get
  their own bug/refactor ticket, not this one.


## Resolution (2026-09-13)

Both phases implemented test-only (zero production change); all Go packages (uncached) and both Python suites green. See the per-phase ### Result sections.
