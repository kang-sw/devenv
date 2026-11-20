---
title: "에이전트 메일·완료 전달 및 워커 컨텍스트 수정"
related:
  261120-refactor-ws-agent-workers-under-holders: "the observations came from a pre-holder shared daemon; current-source diagnosis must distinguish inherited behavior from holder-migration changes"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 8b93504a8f77679f
sage-review-design-reviewed: 8b93504a8f77679f
completed: 2026-11-20
---

# Fix agent mail, completion delivery, and worker context

## Background

Five agent-v2 dogfood surprises were observed on 2026-11-20 while the shared
daemon still ran code from before
`261120-refactor-ws-agent-workers-under-holders`. The holder migration has now
landed in this checkout, but the live observations were not made against that
code. Source diagnosis added a sixth instance at the same process-context
boundary: a dashboard session terminal copies the daemon environment into its
PTY. This ticket keeps the observations together because they cross the same
spawned-process contract: how a child is contacted, how its descendants report
back, which ws tools it receives, and which session identity reaches a worker
or terminal-launched harness.

The diagnosis below is source-only. No runtime reproduction, build, or test was
performed while drafting this ticket. Runtime observations are therefore
marked INFERRED unless current source or the completed holder ticket records
them directly; source claims are marked VERIFIED with repository-relative
citations.

## Decisions

1. **(1864) Codex and agy workers receive real mid-turn mail steer through the
   common worker steer path.** Their worker catalogs already promise native
   `ItemSteer`, so the implementation must make that capability true without
   requiring interruption or turn completion. **Rejected:** narrowing the
   capability or user contract to next-turn delivery; `agent.spawn`, the
   catalog, and Codex lead steering already promise native steer. Compatibility
   estimate: T0 because the correction fulfills an existing behavior contract
   without changing a tool, hello, or stored-record shape.
2. **(1865) First write a holder-backed Codex parent/grandchild end-to-end
   completion-delivery regression, and change code only if it fails.**
   **Rejected:** a speculative fix without a current reproducer. Compatibility
   estimate: T0 for either a no-code-change result or a correction confined to
   the existing pending-report route; re-grade any newly required mechanism.
3. **(1866) Put the direct parent's reply address into the child's worker
   context at spawn, at one shared seam used by every harness.** **Rejected:**
   relying on the first-mail envelope, because the child needs the address
   before inbound mail; relying on parent-side spawn metadata, because the
   child cannot see it. Compatibility estimate: T0 for additive worker context
   that preserves mailbox address syntax.
4. **(1867) Add `review.marker` and `review.stamp` to the common worker
   loadout (`AgentWSTools`).** The existing authority gate stays the
   least-privilege boundary: a read-only child is refused `review.stamp` and
   `review.marker(bootstrap: true)`, while non-bootstrap `review.marker` stays
   readable
   (`agents-plugin-tool/internal/mcp/permission_gate.go:18-24`). Owner answer
   2026-11-20 after design review. **Rejected:** a review-handler-only
   loadout, because it reverses the one-common-loadout rule (epic (85)=c,
   `261119-feat-ws-codex-hosted-worker` Decision (174)) and `agent.spawn` has
   no role signal to select a profile; a new spawn role argument or playbook
   binding would break the playbook-unaware spawn rule of
   `261117-feat-ws-hosted-prompt-assembly` Phase 4. Compatibility estimate: T0
   because existing tools become discoverable with no schema change.
5. **(1868) Use one shared environment drop set for every worker backend
   launch.** It includes `WS_HOSTED_*` and applies the existing `CLAUDE*` and
   `WS_MAILBOX*` exclusions where their harness boundaries require them.
   **Rejected:** a Claude-only exclusion, because every backend starts from the
   daemon environment. Compatibility estimate: T0 because filtering inherited
   process identity changes neither the hello schema nor its connection gate.
6. **Item 6 consequence of (1868): the dashboard session terminal applies the
   same shared drop set before spawning its PTY.** This is an announced
   consequence of the confirmed common process-boundary rule, not a separate
   mechanism choice. Compatibility estimate: T0 because ordinary machine and
   user environment remains intact while stale harness/session identity is
   omitted from a newly spawned process.

## Evidence and diagnosis

### 1. Codex and agy child mail does not steer into a running turn

- **VERIFIED — symptom:** the completed holder ticket records that mailbox
  updates appeared together only after interruption and explicitly leaves
  message-delivery visibility for separate triage
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:630-634`).
- **INFERRED — additional live scope:** the separate dogfood report saw the
  same shape twice, but this drafting pass did not reproduce it: a Codex
  implementation worker missed two parent mails and a Codex renderer
  grandchild held five messages for more than 40 minutes.
- **VERIFIED — current source reproduces the static cause for both backends:**
  the Codex and agy catalogs clone each lead capability, including native
  `ItemSteer`, onto workers
  (`agents-plugin-tool/internal/wsagent/catalog.go:473-493`,
  `agents-plugin-tool/internal/wsagent/catalog.go:537-557`). Their process
  controls expose `steer` but not the acknowledged `reports` control
  (`agents-plugin-tool/internal/wsagent/codex.go:166-168`,
  `agents-plugin-tool/internal/wsagent/agy.go:242-244`). The common
  `steerWorker` path currently returns unless the worker is Pi and has that
  `reports` control (`agents-plugin-tool/internal/wsagent/children.go:601-609`),
  while non-lead mail eligibility also requires `reports`
  (`agents-plugin-tool/internal/wsagent/runner_conv.go:397-425`). Codex and agy
  therefore advertise native steer that the worker mail path cannot use.
- **INFERRED — suspected cause:** the worker capability declaration and the
  runner's acknowledged worker-steer contract disagree. This explains delayed
  visibility without requiring mailbox-store loss; the source does not prove
  which delayed live messages were claimed before interruption.
- **VERIFIED — holder-migration relation:** the migration preserved the common
  worker lifecycle but expressly made no cause claim for delayed mail
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:630-634`).
  Nothing in the current Codex control seam shows that the holder work fixed
  this item.
- **INFERRED — live agy scope:** no agy incident was reported or reproduced;
  its inclusion follows the same VERIFIED catalog/control mismatch.
- **VERIFIED — compatibility decision:** T0 under Decision (1864), provided
  implementation keeps the existing tool schemas, arguments, hello fields,
  and persisted record shape.

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
  use the broader lead filter (`agents-plugin-tool/internal/wsagent/codex.go:66`,
  `agents-plugin-tool/internal/wsagent/agy.go:140`). Pi workers inherit them
  too: `piEnv` drops only `WS_PI_` and `WS_MAILBOX`
  (`agents-plugin-tool/internal/wsagent/pi.go:398`).
- **INFERRED — suspected cause:** Claude worker environment construction
  predates the shared `leadDropEnv` boundary and did not adopt its hosted-
  identity exclusions.
- **VERIFIED — holder-migration relation:** the migration identified but
  deliberately did not broaden into this pre-existing filter bug
  (`ai-docs/tickets/.done/261120-refactor-ws-agent-workers-under-holders.md:624-628`).
- **INFERRED — compatibility estimate:** T0 if the hello keeps the same schema
  and merely omits inherited hosted-identity values from fresh worker
  processes. Re-grade if the hello contract or connection refusal changes.

### 6. A dashboard session terminal inherits daemon/session environment

- **VERIFIED — current source reproduces full daemon-environment inheritance:**
  the dashboard's default terminal shell starts from `os.Environ()`, copies
  every entry, and changes only an absent or incapable `TERM`
  (`agents-plugin-tool/internal/wsdashboard/terminal_shell.go:21-48`,
  `agents-plugin-tool/internal/wsdashboard/terminal_shell.go:61-91`). The
  session route passes that shell to terminal creation, which gives the map
  unchanged to the holder's PTY spawn
  (`agents-plugin-tool/internal/wsdashboard/terminal_routes.go:403-415`,
  `agents-plugin-tool/internal/wsdashboard/terminals.go:235-268`).
- **VERIFIED — normal current startup removes only part of the risk:** the
  dashboard command drops hosted-session values, the harness PID, MCP session
  URL/token, and worker permission before reading or spawning from its
  environment (`agents-plugin-tool/cmd/ws-mcp/dashboard.go:40-42`,
  `agents-plugin-tool/internal/mcp/process_env.go:335-347`). It does not remove
  `WS_MAILBOX` or `WS_MAILBOX_AUTO`. The launcher passes the caller environment
  into daemon startup while normalizing path-valued entries, selecting package
  resources where applicable, and setting runtime, harness-PID, and CLI
  variables (`agents-plugin/bin/ws-mcp-launcher.py#L1320-L1337`,
  `agents-plugin/bin/ws-mcp-launcher.py#L1459-L1486`,
  `agents-plugin/bin/ws-mcp-launcher.py#L2326-L2333`), so a daemon started from a
  named-mailbox session can still pass that identity to every terminal.
- **VERIFIED — a terminal-launched Claude/ws-mcp forwards inherited values:**
  the launcher copies its environment into the proxy hello and only normalizes
  path-valued entries (`agents-plugin/bin/ws-mcp-launcher.py:1320-1337`,
  `agents-plugin/bin/ws-mcp-launcher.py:1489-1508`). An inherited explicit
  mailbox value becomes that server's process-lifetime mailbox identity
  (`agents-plugin-tool/internal/mcp/mailbox_runtime.go:109-140`). Thus running
  Claude in such a terminal can bind the daemon starter's mailbox rather than
  begin without a named mailbox.
- **VERIFIED — inherited hosted values do not by themselves adopt the wrong
  hosted key:** the launcher replaces `WS_MCP_HARNESS_PID` with its direct
  parent PID (`agents-plugin/bin/ws-mcp-launcher.py:2326-2333`), and hosted-key
  resolution accepts the inherited key only when the registration row's
  `host_pid` matches that harness PID; a row naming another process yields an
  ordinary session (`agents-plugin-tool/internal/mcp/process_env.go:274-305`).
  A missing or not-yet-populated row can still impose a bounded wait and
  warning (`agents-plugin-tool/internal/mcp/process_env.go:286-318`).
- **INFERRED — live incident and holder-migration relation:** no terminal was
  launched during this diagnosis, so an actual wrong-mailbox collision or
  hosted-session warning is not runtime-reproduced. The worker-holder migration
  does not change the current dashboard terminal shell or PTY environment
  seam, so it is not shown to fix this source-reproducible boundary.
- **VERIFIED — compatibility decision:** T0 under the Item 6 consequence of
  (1868); filtering occurs before process spawn and changes no MCP wire or
  stored-data shape.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Each implementation result must state the directional compatibility tier with
  source lines for old/new read, write, refusal, and recovery behavior where
  applicable.
- Mail send, claim, steer, and completion delivery must remain memory-first and
  off per-message file reads; report file-operation deltas for any changed
  interactive path.
- Do not expose session keys, process identifiers, bearer tokens, or
  machine-local paths in tests, logs, ticket results, or user-visible error
  text.
- Preserve direct-child ownership checks, authority ceilings, mailbox return
  channel semantics, once-only conversation-parent pending-report delivery, and
  holder survival from `261120-refactor-ws-agent-workers-under-holders`; external
  completion mail can duplicate if the daemon dies between append and the
  reported-turn mark (`agents-plugin-tool/internal/wsagent/spawn.go#L305-L350`,
  `agents-plugin-tool/internal/wsagent/spawn.go#L355-L365`).
- Build the shared drop set as an in-memory launch-boundary filter used before
  worker and terminal holder spawn; do not add interactive-path file reads or
  per-message work.
- Retain ordinary machine and user environment required by interactive shells
  and harnesses, including shell selection, `PATH`, home/config locations, and
  unrelated credentials. Remove only inherited harness/session identity and
  backend-specific parent state covered by Decision (1868).
- Keep the hosted `host_pid` confirmation guard even after launch filtering;
  filtering is defense in depth, not a replacement for connection-side
  identity validation.
- Do not treat Item 2's supplied-session-key suspicion as established. First
  reproduce the current Codex parent/grandchild path or record that the named
  regression passes without a code change.

## Prior Decisions

- 261117-feat-ws-agent-tools-mailbox-routing (2026-11-18, commit): "Lead tests that relied on mail waiting for the turn end now expect a steer, per Decision 4 (every mailbox.send is a steer where supported)." — bearing: supports
- c6a2d1b3 (2026-11-19, commit): "Defined one harness-neutral allowlist (ChildSafeWSTools in internal/wsagent/builtin.go) and ensured AgentWSTools includes all entries" — bearing: constrains
- 261112-feat-ws-mcp-pi-spawn-recursion (2026-11-13, commit): "PendingReports is a slice taken atomically under the child's lock, so a report is delivered once even across a parent wait, a steer, and a restart" — bearing: supports
- 261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart (2026-11-13, Decisions): "a daemon crash exactly between mailbox append and that mark may duplicate one completion mail, explicitly accepted. This is not an exactly-once guarantee." — bearing: constrains
- 261119-feat-ws-codex-hosted-worker (2026-11-19, Decisions): "Lead-side parity with `pi.*` (turn-end completion mail, follow-up to the same worker, transcript read, interrupt/stop) comes through the existing `agent.*` and mailbox tools" — bearing: supports
- 261110-feat-ws-dashboard-terminal (2026-11-11, commit): "The shell rule is a pure function of (GOOS, environment, lookup) so the Windows order is tested on any host." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/children.go, agents-plugin-tool/internal/mcp/agent_tools.go, agents-plugin-tool/internal/wsdashboard/terminal_shell.go, agents-plugin/rsrc/review-handler/review-handler.md |
| scope.surface | cross-module | worker prompts, ws-mcp tool profiles, and dashboard terminal launch environment cross wsagent, mcp, and wsdashboard |
| scope.new_public_symbol | no | existing agent.spawn, mailbox.send, review.marker, and review.stamp names remain; ## Decisions |
| scope.new_type_contract | unknown | parent return address is to enter shared worker context, but the phase does not name a type or signature |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/agent_tools_test.go, agents-plugin-tool/internal/wsagent/worker_holder_recovery_test.go, agents-plugin-tool/internal/mcp/mailbox_tools_test.go, agents-plugin-tool/internal/wsdashboard/terminal_routes_test.go; named Item 1–6 regressions are absent from rg -n 'Test(CodexAndAgyWorkerMailSteersIntoRunningTurn|CodexGrandchildCompletionReachesConversationParentExactlyOnce|SpawnedWorkerContextNamesDirectParentReturnAddress|AgentWSToolsIncludeReviewLedgerToolsUnderReadOnlyGate|WorkerBackendsDropInheritedHarnessSessionEnv|SessionTerminalDropsInheritedHarnessSessionEnv)' agents-plugin-tool |
| complexity.reuse_points | confirmed | existing PendingReports delivery, AgentWSTools, leadDropEnv, and pickShell read at agents-plugin-tool/internal/wsagent/children.go#L533-L568, agents-plugin-tool/internal/wsagent/builtin.go#L170-L188, agents-plugin-tool/internal/wsagent/claude.go#L76-L94, agents-plugin-tool/internal/wsdashboard/terminal_shell.go#L21-L48 |
| complexity.side_effect_risk | high | mail steer and completion timing, worker authority, and inherited process identity change together |
| risk.correctness | high | Codex and agy advertise steer but current worker path requires Pi reports control at agents-plugin-tool/internal/wsagent/children.go#L601-L609 |
| risk.fit | moderate | review ledger tools join the common AgentWSTools and must keep the read-only gate refusing mutation at agents-plugin-tool/internal/wsagent/builtin.go#L170-L188 and agents-plugin-tool/internal/mcp/permission_gate.go#L16-L37 |
| risk.test | high | holder-backed Codex parent/grandchild delivery and six backend/terminal boundaries require new regressions; current generic test is agents-plugin-tool/internal/wsagent/agent_tools_test.go#L1284-L1308 |
| risk.security_or_contract | high | parent return address, review ledger mutation authority, and leaked hosted/mailbox identity affect control boundaries |

## Phases

### Phase 1: Reproduce each boundary and implement the settled corrections

1. Add the named regressions below before changing behavior. Item 2 must first
   run against the current holder-backed source; if it passes, preserve the
   no-reproduction result and limit code changes to defects the tests expose.
2. Implement only the approaches settled in `## Decisions`. Keep existing
   tool names, mailbox address syntax, session-key authority, once-only
   conversation-parent pending-report delivery, the accepted external-mail
   crash window, and holder ownership checks unless a later confirmed
   decision explicitly changes one (`agents-plugin-tool/internal/wsagent/spawn.go#L305-L350`,
   `agents-plugin-tool/internal/wsagent/spawn.go#L355-L365`).
3. Run focused offline tests for the affected `internal/wsagent` and
   `internal/mcp` packages, then the repository's configured Go verification.
   Use `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` for Go commands. Report
   before/after file-operation counts for any changed interactive mail or
   completion path; do not claim a Windows antivirus latency result without
   measuring it on that baseline.

Verification expectations, at least one named regression per dogfood item:

- Item 1: `TestCodexAndAgyWorkerMailSteersIntoRunningTurn` proves parent mail
  reaches active Codex and agy workers through the common worker steer path
  without interruption, turn end, duplication, or loss. Relaxing the Pi-only
  check in `steerWorker` (`agents-plugin-tool/internal/wsagent/children.go:604-610`)
  is not enough: Codex and agy must implement the acknowledged, restart-safe
  `reportSteerer.ClearSteered` contract
  (`agents-plugin-tool/internal/wsagent/backend.go:194-197`), and the test
  covers a daemon restart between steer and acknowledgement.
- Item 2: `TestCodexGrandchildCompletionReachesConversationParentExactlyOnce`
  is holder-backed and proves a Codex grandchild's terminal result reaches its
  Codex conversation parent once and releases the parent's held final in the
  documented order; code changes are permitted only if this test first fails.
  `TestAgentForwardSessionKeyDoesNotSelectCompletionParent` pins the token-
  scoped routing boundary.
- Item 3: `TestSpawnedWorkerContextNamesDirectParentReturnAddress` proves a
  newly started child can identify the direct parent, not merely the root
  lead, before any follow-up mail; `TestChildMailEnvelopeKeepsParentRelation`
  preserves the later envelope behavior.
- Item 4: `TestAgentWSToolsIncludeReviewLedgerToolsUnderReadOnlyGate`
  proves every worker backend's common loadout lists `review.marker` and
  `review.stamp`, a workdir-write child can call them, a read-only child is
  still refused `review.stamp` and `review.marker(bootstrap: true)`, and a
  read-only child can still call non-bootstrap `review.marker`. If the
  `lead-review` fallback sentence for hosts whose subagents "cannot call the
  handler's tools ... (Pi today)" (`agents-plugin/rsrc/lead-review/lead-review.md:41-42`)
  no longer describes a real limitation, update it and its wsflow/generated
  mirrors in the same change.
- Item 5: `TestWorkerBackendsDropInheritedHarnessSessionEnv` covers every
  worker backend from a contaminated daemon environment and proves each fresh
  worker omits hosted/mailbox/parent-harness identity while retaining its own
  agent session key, allowed tools, permission context, and required
  backend-specific environment. The drop set is the union of `leadDropEnv`
  (`agents-plugin-tool/internal/wsagent/claude.go:82`) and the dashboard
  command drop list (`agents-plugin-tool/internal/mcp/process_env.go:335-347`);
  `claudeKeptEnv` entries stay, and per-launch values (agent session key,
  permission level, allowed tools, hosted-lead `WS_HOSTED_*` at
  `agents-plugin-tool/internal/wsagent/runner_process.go:358-366`) are set
  after the drop. The test also covers the holder rejoin/attach path, which
  restores a worker with its filtered attach environment (commit `b1668601`),
  so launch and rejoin agree.
- Item 6: `TestSessionTerminalDropsInheritedHarnessSessionEnv` proves the
  holder PTY receives no hosted, MCP-session, permission, or mailbox identity
  from a contaminated daemon while retaining `PATH`, home/config, shell, and
  normalized `TERM`; launching ws-mcp from that shell must create an ordinary
  non-mailbox session without the hosted-row wait.

### Result (f9332e85) - 2026-11-20

- Codex and agy workers now expose the acknowledged report control and use the
  common worker-steer path. Both report and mailbox steers persist before the
  holder write, and holder reattach restores their existing `Record.Steered`
  text for acknowledgement replay (`children.go:601-660`,
  `runner_conv.go:420-450`, `codex.go:168-173,344-363`,
  `agy.go:244-249,787-804,948-965`). The successful interactive path still
  performs one steered-record persistence; only an immediate backend refusal
  adds two rollback persistences (persist then remove) before the existing
  next-turn queue update. No per-message reads were added.
- The holder-backed Codex parent/grandchild completion regression passed before
  any completion-routing production edit, so the current token-scoped parent
  route was preserved. The restart assertion now waits for the holder scan and
  quiesces both conversation loops instead of using a fixed sleep.
- Every spawned worker context now names its direct parent's `id:<reply-id>` at
  the shared instruction seam (`runner_process.go:816-824`); the session key
  remains local authorization and is not forwarded as parent-selection input.
- `review.marker` and `review.stamp` are in the common worker loadout
  (`builtin.go:132-190`). The unchanged gate still permits non-bootstrap marker
  reads and refuses marker bootstrap plus stamps for read-only workers
  (`permission_gate.go:16-27`). The lead-review fallback now states only the
  still-real tier ceiling, and canonical, wsflow, Pi resource, manifest,
  workflow-bundle, and native first-party mirrors were regenerated.
- Worker launches and dashboard terminals share the in-memory
  `wsharnessenv` drop set; Windows matching is case-insensitive while ordinary
  user state and explicit child overlays remain (`wsharnessenv/env.go:10-42`,
  `terminal_shell.go:35-55`). This adds zero file operations at launch or on
  interactive paths.
- Compatibility is T0 in both directions. The change adds no tool name,
  argument, hello field, persisted field, or record shape: old binaries read
  and write the same records, new binaries read old records, and reopening the
  new binary recovers accepted steers from the pre-existing `Steered` field.
  Existing review tools become discoverable without changing their schemas;
  environment filtering and worker prompt text are launch-only corrections.
- Verification passed with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:
  `go test ./internal/mcp -timeout 45m -count=1` (1022.743s),
  `go test ./internal/wsagent -count=1` after review fixes (140.995s), and
  `go test ./internal/wsdashboard ./internal/wsharnessenv -count=1`
  (324.830s / 0.602s). The pre-review integrated `internal/wsrsrc` check also
  passed, as did all six named ticket regressions and the focused review-fix
  regressions.
- Lite review found five Important issues and no Criticals: very-early agy
  steering, mailbox persist ordering, Codex lead refusal propagation, Windows
  environment-name casing, and a fixed-sleep restart assertion. All five were
  fixed in `f9332e85`; the one allocated review pass is resolved with no open
  finding.
