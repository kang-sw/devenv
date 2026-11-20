---
title: "에이전트 메일·완료 전달 및 워커 컨텍스트 수정"
related:
  261120-refactor-ws-agent-workers-under-holders: "the observations came from a pre-holder shared daemon; current-source diagnosis must distinguish inherited behavior from holder-migration changes"
---

# Fix agent mail, completion delivery, and worker context

## Background

Five agent-v2 dogfood surprises were observed on 2026-11-20 while the shared
daemon still ran code from before
`261120-refactor-ws-agent-workers-under-holders`. The holder migration has now
landed in this checkout, but the live observations were not made against that
code. This ticket keeps the observations together because they cross the same
spawned-worker contract: how a child is contacted, how its descendants report
back, which ws tools it receives, and which session identity reaches its MCP
hello.

The diagnosis below is source-only. No runtime reproduction, build, or test was
performed while drafting this ticket. Runtime observations are therefore
marked INFERRED unless current source or the completed holder ticket records
them directly; source claims are marked VERIFIED with repository-relative
citations.

## Evidence and diagnosis

### 1. Codex child mail does not steer into a running turn

- **VERIFIED — symptom:** the completed holder ticket records that mailbox
  updates appeared together only after interruption and explicitly leaves
  message-delivery visibility for separate triage
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:630-634`).
- **INFERRED — additional live scope:** the separate dogfood report saw the
  same shape twice, but this drafting pass did not reproduce it: a Codex
  implementation worker missed two parent mails and a Codex renderer
  grandchild held five messages for more than 40 minutes.
- **VERIFIED — current source reproduces the static cause:** the Codex catalog
  clones the lead's native steer capability onto workers
  (`agents-plugin-tool/internal/wsagent/catalog.go:473-493`), but
  `codexProcess.processControls` supplies `steer` and omits the recoverable
  `reports` control (`agents-plugin-tool/internal/wsagent/codex.go:166-168`).
  Non-lead mail may steer only when `controlsFor(proc).reports` is non-nil, so
  Codex worker mail is held for a later turn despite the native capability
  grade (`agents-plugin-tool/internal/wsagent/runner_conv.go:397-425`).
- **INFERRED — suspected cause:** the worker capability declaration and the
  runner's acknowledged worker-steer contract disagree. This explains delayed
  visibility without requiring mailbox-store loss; the source does not prove
  which delayed live messages were claimed before interruption.
- **VERIFIED — holder-migration relation:** the migration preserved the common
  worker lifecycle but expressly made no cause claim for delayed mail
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:630-634`).
  Nothing in the current Codex control seam shows that the holder work fixed
  this item.
- **INFERRED — compatibility estimate:** T0 if repaired through the existing
  mail/steer behavior without changing tool schemas, arguments, hello fields,
  or persisted record shape. Re-grade if the selected approach changes one of
  those cross-version surfaces.

### 2. Codex grandchild completion was not delivered to its parent

- **INFERRED — symptom:** a Codex grandchild committed and its parent's
  `agent.list` showed a done last report, but the Codex conversation parent
  received neither a completion signal nor completion mail. This live report
  was not independently reproduced from current source.
- **VERIFIED — current source has a delivery path:** a settled child appends a
  `PendingReport` and schedules delivery
  (`agents-plugin-tool/internal/wsagent/spawn.go:305-350`); delivery atomically
  removes pending reports and calls the conversation parent's `takeReport`
  (`agents-plugin-tool/internal/wsagent/children.go:533-568`). The generic
  nested-worker regression proves a held parent final is published before the
  grandchild report starts the parent's successor turn
  (`agents-plugin-tool/internal/wsagent/agent_tools_test.go:1284-1308`).
- **VERIFIED — the suspected session-key mechanism is not supported by the
  forwarding code:** the worker-side handler validates that the supplied key
  is known, but it does not forward that key in the `agent.spawn` body; the
  capability-token route scopes the operation to the worker and records the
  conversation parent (`agents-plugin-tool/internal/mcp/agent_tools.go:118-144`,
  `agents-plugin-tool/internal/mcp/pi_forward.go:18-23`). A supplied-versus-
  developer-injected session-key mismatch can refuse the local call, but the
  inspected path does not use it to choose completion routing.
- **INFERRED — suspected cause:** the current source does not establish a
  cause. Plausible boundaries are an older pre-holder lifecycle/race or the
  same Codex worker delivery-visibility gap as Item 1, but neither is proven.
  An end-to-end Codex parent/grandchild regression is absent even though the
  generic fake-process path passes.
- **INFERRED — holder-migration relation:** holder continuity may already
  remove a pre-holder process-loss variant, and the migration added exactly-once
  completion coverage, but no cited current test reproduces this specific
  Codex grandchild-to-Codex-parent observation. Keep the item open until the
  named end-to-end regression proves fixed or fails.
- **INFERRED — compatibility estimate:** T0 if the resolution stays inside
  the existing pending-report and parent-delivery contract. The source is
  insufficient to estimate an alternate mechanism.

### 3. A child cannot see its parent's return address before receiving mail

- **VERIFIED — current source reproduces the ambiguity:** the Codex worker
  instruction says only to follow instructions supplied by "your parent" and
  to use `mailbox.send`; it does not name the parent's reply address
  (`agents-plugin-tool/internal/wsagent/instructions/agent-worker-codex.md:1-3`).
  The `agent.spawn` response tells the parent the child's address, not the
  child the parent's address
  (`agents-plugin-tool/internal/mcp/agent_tools.go:218-233`).
- **VERIFIED — an inbound envelope resolves the ambiguity later:** every
  ordinary send opens the sender's reply-id return channel and puts either a
  named `from` address or `reply_to` handle on the envelope
  (`agents-plugin-tool/internal/mcp/mailbox_tools.go:68-95`); hosted delivery
  renders that handle as `mail from <handle>` and adds a spawn-lineage relation
  (`agents-plugin-tool/internal/mcp/lead_mail.go:323-350`,
  `agents-plugin-tool/internal/wsagent/agent_tools.go:797-811`). Thus a child
  can answer its parent after parent mail arrives, but not derive the address
  from its initial spawn context alone.
- **INFERRED — suspected cause:** spawn communication is asymmetric: the caller
  receives the child's address, while the child receives only a generic parent
  label until its first envelope.
- **INFERRED — holder-migration relation:** holder ownership does not alter the
  worker instruction or spawn-response text, so the migration is not shown to
  fix this item.
- **INFERRED — compatibility estimate:** T0 for an additive prompt or envelope
  clarification that preserves mailbox address syntax and existing calls.

### 4. A review-handler child lacks `review.marker` and `review.stamp`

- **VERIFIED — current source reproduces the missing loadout:** the review
  handler requires `review.marker` and `review.stamp`
  (`agents-plugin/rsrc/review-handler/review-handler.md:61-71`), and the server
  defines both tools (`agents-plugin-tool/internal/mcp/server_schemas.go:961-984`).
  However, the exhaustive mini-lead and child-safe ws tool lists omit both,
  and every `agent.spawn` child receives the union derived from those lists
  (`agents-plugin-tool/internal/wsagent/builtin.go:102-152`,
  `agents-plugin-tool/internal/wsagent/builtin.go:170-185`).
- **INFERRED — suspected cause:** the review-handler playbook contract and the
  common spawned-worker tool profile drifted; runtime discovery correctly
  reflects the restricted profile rather than the handler's needs.
- **INFERRED — holder-migration relation:** this is a tool-profile issue, not a
  process-holder issue, and no inspected holder-migration seam changes the
  child tool list.
- **INFERRED — compatibility estimate:** T0 if the existing tools are added to
  the appropriate worker/profile allowlist while legacy calls and tool names
  remain unchanged. Permission behavior still must be verified for the
  handler's selected authority.

### 5. A fresh Claude worker inherits hosted-session environment

- **VERIFIED — symptom and migration boundary:** the completed holder ticket
  records that a fresh Claude worker's hello inherited
  `WS_HOSTED_SESSION`/`WS_HOSTED_SESSION_KEY`, calls it a pre-existing
  environment-filter issue, and states that rejoin preserves the baseline
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:624-628`).
- **VERIFIED — current source reproduces the cause:** every worker starts from
  the daemon's full `environ()` (`agents-plugin-tool/internal/wsagent/runner_process.go:125-140`).
  The common lead filter explicitly includes `WS_HOSTED_`, but the Claude
  worker's `claudeEnv` drops only `CLAUDE*` and `WS_MAILBOX*`
  (`agents-plugin-tool/internal/wsagent/claude.go:69-94`). The daemon's hosted
  variables therefore survive into a fresh Claude worker while Codex and agy
  use the broader lead filter.
- **INFERRED — suspected cause:** Claude worker environment construction
  predates the shared `leadDropEnv` boundary and did not adopt its hosted-
  identity exclusions.
- **VERIFIED — holder-migration relation:** the migration identified but
  deliberately did not broaden into this pre-existing filter bug
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:624-628`).
- **INFERRED — compatibility estimate:** T0 if the hello keeps the same schema
  and merely omits inherited hosted-identity values from fresh worker
  processes. Re-grade if the hello contract or connection refusal changes.

## Constraints

- Convention: `ai-docs/manuals/ws-mcp.md` (declared for
  `agents-plugin-tool/internal/mcp/`).
- Convention: `ai-docs/manuals/release-compatibility.md` (declared for
  `agents-plugin-tool/internal/`). Each implementation result must state the
  directional compatibility tier with source lines for old/new read, write,
  refusal, and recovery behavior where applicable.
- Convention: `ai-docs/manuals/performance.md` (declared for
  `agents-plugin-tool/`). Mail send, claim, steer, and completion delivery must
  remain memory-first and off per-message file reads; report file-operation
  deltas for any changed interactive path.
- Do not expose session keys, process identifiers, bearer tokens, or
  machine-local paths in tests, logs, ticket results, or user-visible error
  text.
- Preserve direct-child ownership checks, authority ceilings, mailbox return
  channel semantics, exactly-once completion reporting, and holder survival
  from `261120-refactor-ws-agent-workers-under-holders`.
- Do not treat Item 2's supplied-session-key suspicion as established. First
  reproduce the current Codex parent/grandchild path or record that the named
  regression passes without a code change.

## Open Decision Queue

Last ID used: (1868)

(1864) [open] [technical] Choose whether Codex workers gain an acknowledged recoverable mid-turn steer path or the worker capability and user-facing contract are narrowed to queued next-turn delivery.

(1865) [open] [technical] Choose the Item 2 fix only after the end-to-end Codex grandchild regression distinguishes a current delivery defect from a pre-holder observation already removed by the migration.

(1866) [open] [technical] Choose where to expose the direct parent's reply address to a new child: injected worker context, the initial prompt envelope, or another existing spawn metadata seam.

(1867) [open] [technical] Choose whether review-ledger tools belong in the common agent worker loadout, a review-handler-specific profile, or a lead-owned stamping handoff.

(1868) [open] [technical] Choose whether Claude worker identity filtering reuses the shared worker/lead drop set or adds a backend-specific hosted-session exclusion.

## Phases

### Phase 1: Reproduce each boundary and implement the settled corrections

1. Add the named regressions below before changing behavior. Item 2 must first
   run against the current holder-backed source; if it passes, preserve the
   no-reproduction result and limit code changes to defects the tests expose.
2. Implement only the approaches settled from (1864)-(1868). Keep existing
   tool names, mailbox address syntax, session-key authority, completion
   exactly-once behavior, and holder ownership checks unless a later confirmed
   decision explicitly changes one.
3. Run focused offline tests for the affected `internal/wsagent` and
   `internal/mcp` packages, then the repository's configured Go verification.
   Use `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` for Go commands. Report
   before/after file-operation counts for any changed interactive mail or
   completion path; do not claim a Windows antivirus latency result without
   measuring it on that baseline.

Verification expectations, one named regression per dogfood item:

- Item 1: `TestCodexWorkerMailSteersIntoRunningTurn` proves parent mail reaches
  an active Codex worker without interruption, turn end, duplication, or loss.
- Item 2: `TestCodexGrandchildCompletionReachesConversationParentExactlyOnce`
  proves a Codex grandchild's terminal result reaches its Codex conversation
  parent once and releases the parent's held final in the documented order;
  `TestAgentForwardSessionKeyDoesNotSelectCompletionParent` pins the token-
  scoped routing boundary.
- Item 3: `TestSpawnedWorkerContextNamesDirectParentReturnAddress` proves a
  newly started child can identify the direct parent, not merely the root
  lead, before any follow-up mail; `TestChildMailEnvelopeKeepsParentRelation`
  preserves the later envelope behavior.
- Item 4: `TestReviewHandlerWorkerLoadoutIncludesReviewLedgerTools` proves the
  review handler can discover and call `review.marker` and `review.stamp` at
  its intended authority while unrelated worker profiles do not gain an
  unapproved mutation surface.
- Item 5: `TestClaudeWorkerHelloDropsInheritedHostedSessionEnv` starts from a
  daemon environment containing hosted-session values and proves a fresh
  Claude worker's hello omits them while retaining its own agent session key
  and allowed-tool/permission context.
