---
title: "pi execute/approve: unsanitized cmd_id breaks approval file on Windows"
related:
  260904-feat-ws-pi-execute-approval-gateway: predecessor — introduced the filesystem approval decision channel and cmd_id binding
  260905-bug-ws-pi-approval-relay-deadlocks-under-agent-wait: related approval relay lifecycle
  260921-bug-pi-gutter-stale-pending-approval-deadlock: related pending-approval cleanup invariant
  260903-feat-ws-pi-subagent-rpc-ux: predecessor for persistent RPC children
  260920-feat-pi-agent-spawn-cwd-override: related execute-worker resume context
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 614367e4ab7ae20c
sage-review-completeness-reviewed: 614367e4ab7ae20c
---

# pi execute/approve: unsanitized cmd_id breaks approval file on Windows

## Background

The pi execute/approve gateway derives the approval decision filename by
interpolating the command id (`cmd_id`) raw into a path, with no sanitization
on either the writer or the reader side. `cmd_id` is adopted verbatim from the
pi harness's `toolCallId` (see `cmdId: evt.toolCallId` in
`agents-plugin-pi/src/spawner.ts`). The harness's **OpenAI (Responses API)**
provider adapter composes that id as `${call_id}|${item_id}` (e.g.
`call_abc123|fc_def456`), where `|` is the canonical intra-id separator the
adapter itself splits on. Provider IDs that already contain only portable
filename characters are unaffected.

On Windows, `|` is an illegal filename character. So when an execute-worker
runs an OpenAI Responses model:

1. `ws-approve` builds `.../approvals/call_abc123|fc_def456.decision.json`;
   because `|` is forbidden in Windows filenames, the decision file cannot be
   created.
2. The child `ws-worker-exec` polls for the same (never-created) filename in
   `waitForDecisionFile` and blocks indefinitely.
3. The gated verification command / `git push` is blocked before it can run.

This is provider-specific (only OpenAI Responses execute-workers hit it) and
platform-specific (Windows), which is why it escaped notice on Linux/macOS.
The same path also leaves every successfully consumed decision file in the
child home until whole-home retention or capacity cleanup, even though these
files are an IPC rendezvous rather than an audit log.

Evidence:

- id composition (`${call_id}|${item_id}`, split-on-`|`) lives in the pi
  harness's bundled OpenAI Responses adapter, not in this repo; the plugin
  treats `toolCallId` as opaque.
- Plugin adopts it verbatim: `agents-plugin-pi/src/spawner.ts` `cmdId:
  evt.toolCallId`.
- Raw, unsanitized filename derivation on both sides:
  - writer: `agents-plugin-pi/src/execute-gateway.ts` `approvalDecisionPath`
    (`join(sessionDir, "approvals", `${cmdId}.decision.json`)`), called by the
    `ws-approve` writer (`writeFileSync`).
  - reader: `agents-plugin-pi/src/execute-gateway.ts` child-side
    `${toolCallId}.decision.json`, polled by `waitForDecisionFile`.
- The id IS xml-escaped for display (`spawner.ts` `xmlEscape`), but the
  filesystem path is never sanitized — display-layer escaping does not reach
  the filesystem layer.
- Model resolution entry point: `resolveExecuteModelAlias` in
  `execute-gateway.ts`.

Point at code by search (`approvalDecisionPath`, `waitForDecisionFile`,
`cmdId: evt.toolCallId`) rather than trusting the line numbers above, which are
evidence for the claim, not edit coordinates.

## Constraints

1. **Both sides must use the identical transform.** The writer
   (`approvalDecisionPath`) and the child reader (`${toolCallId}.decision.json`
   derivation) must produce the same filename from the same id. A mismatch does
   not fix the bug — it replaces it with a permanent deadlock (writer writes
   file A, reader waits on file B). Factor the derivation into one shared
   function both call.
2. **The transform must be injective (collision-free).** This is a security
   approval gate: if two distinct `cmd_id`s can map to the same decision
   filename, one command's approval can be matched to a different command
   (cross-approval). Use one filesystem-only percent-encoding transform for
   the Windows-illegal set `<>:"/\|?*`, control characters, and `%`; encoding
   `%` prevents a literal `%7C` from colliding with an encoded `|`. A plain
   single-character substitution is forbidden because it is lossy. The
   logical `cmd_id` remains opaque and unchanged.
3. **Regression test required.** Add a test exercising a `cmd_id` that contains
   `|` (and ideally the rest of the Windows-illegal set) end-to-end through the
   write-then-read path, asserting the writer and reader agree on the filename
   and that it contains no illegal character. The existing
   `approvalDecisionPath` tests in `agents-plugin-pi/test/execute-gateway.test.ts`
   only use safe ids (`call-1`, `call-2`; `execute-gateway.test.ts#L232-L240`),
   which is the gap that let this through.
4. **Already-safe ids must keep working unchanged.** IDs containing no encoded
   characters must round-trip through the new transform without altering their
   existing on-disk filenames. This keeps provider-safe IDs visually intact
   for compatibility and debuggability.
5. **Consumed decisions are transient IPC.** After the child successfully
   reads and parses an approve, deny, or run-instead decision, it must attempt
   to delete that decision file. Parse failures retain the file for polling;
   deletion failure is best-effort and must not block the approved outcome.

## Prior Decisions

- 260904-feat-ws-pi-execute-approval-gateway (2026-09-05, Result): "Landed behavior: ws-execute/ws-approve lead verbs ... per-agent filesystem decision channel using Pi's own toolCallId as cmd_id ... cmd_id race-binding" — bearing: constrains
- 260905-bug-ws-pi-approval-relay-deadlocks-under-agent-wait (2026-09-05, Result): "applyRpcEvent's pendingApproval branch now calls settleWaiters; waitForAgents/harvestWinner gained a firstPendingApprovalAgentId fast-path returning reason:approval-pending" — bearing: supports
- 260921-bug-pi-gutter-stale-pending-approval-deadlock (2026-09-21, Decisions): "Clear record.pendingApproval = undefined in clearLiveState. It is the single chokepoint that both markAgentExited and stopAgent route through" — bearing: constrains
- 260903-feat-ws-pi-subagent-rpc-ux (2026-09-04, commit): "Settled surface: ws.execute(command?, prompt, complex?); ws.approve(agent_id, cmd_id, approve/deny/run-instead); abort factored to ws-agent-stop" — bearing: supports
- 260920-feat-pi-agent-spawn-cwd-override (2026-09-21, Result): "Added cwd_override to ws-agent-spawn. The selected directory is retained on RpcAgentRecord and applied for both initial launch and dormant resume" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/execute-gateway.ts, agents-plugin-pi/test/execute-gateway.test.ts |
| scope.surface | internal | no MCP tool schema or cross-package API change; approvalDecisionPath (agents-plugin-pi/src/execute-gateway.ts#L384-L386) and the reader-side derivation (agents-plugin-pi/src/execute-gateway.ts#L622) stay module-internal |
| scope.new_public_symbol | no | the shared transform can be module-private; approvalDecisionPath is already the exported helper |
| scope.new_type_contract | no | a pure string-to-string transform needs no interface beyond existing ApprovalDecision/PendingApproval types |
| scope.test_surface | existing | agents-plugin-pi/test/execute-gateway.test.ts#L232-L240 covers approvalDecisionPath; waitForDecisionFile cases cover successful parsing and retained malformed JSON at #L397-L438 |
| complexity.reuse_points | not-applicable | Constraint 2 requires filesystem-only percent encoding, so a hash-based helper is neither required nor permitted as the transform |
| complexity.side_effect_risk | moderate | writer/reader mismatch produces a permanent deadlock; a non-injective transform enables cross-approval |
| risk.correctness | moderate | both sides must agree, including literal-percent collision handling |
| risk.fit | low | the transform belongs with the existing approvalDecisionPath pure helper in agents-plugin-pi/src/execute-gateway.ts |
| risk.test | moderate | regression coverage must distinguish successful cleanup from malformed-JSON retry behavior and exercise approve, deny, and run-instead |
| risk.security_or_contract | high | a non-injective decision filename can associate one command's approval with another command |

## Phases

### Phase 1: Sanitize the approval-file id derivation on both sides

Introduce a single shared id-to-filename percent-encoding transform satisfying
the Constraints above, route both the `ws-approve` writer
(`approvalDecisionPath`) and the child `ws-worker-exec` reader through it, and
remove a decision file best-effort after successful child-side consumption.

Verification boundary: the new/updated `agents-plugin-pi` test suite passes,
including the new `|`/illegal-char and literal-`%` collision cases; the writer
and reader derive an identical, illegal-char-free filename; already-safe IDs still resolve
unchanged; successfully consumed approve, deny, and
run-instead files are removed; parse failures remain retryable; and cleanup
failure does not block the decision. Manual Windows reproduction is out of
scope for the phase (the illegal-char test stands in for it), but note any
residual Windows-only risk under Result.
