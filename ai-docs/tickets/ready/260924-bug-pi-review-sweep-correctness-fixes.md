---
title: Pi review-sweep correctness fixes (approval reconnect, reissue prompt, usage durability, removal gate, stop-during-launch)
related:
  260924-feat-pi-agent-channel-approval-decisions: approval protocol whose reconnect contract Decision 1 tightens
  260924-feat-pi-agent-channel-usage-rollup: durable descendantUsage copy Decision 3 repairs
  260924-bug-pi-retention-cross-owner-checkpoint-fold: eviction records and removal claims Decision 4 aligns with
  260924-bug-pi-channel-and-bridge-races: C1 registration-order pin Decision 6 replaces with a behavior test
  260923-research-pi-parent-child-loopback-control-channel: source ledger; approval waiting state need not survive a disconnect
  260924-refactor-pi-channel-and-cost-module-boundaries: structure cleanup from the same sweep, deliberately excluded here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 4c8227b81e7ce758
sage-review-completeness-reviewed: 4c8227b81e7ce758
---

# Pi review-sweep correctness fixes (approval reconnect, reissue prompt, usage durability, removal gate, stop-during-launch)

## Background

The release-gate review sweep of develop `a976a2b1..0a361491` found no Critical
issues. It did find several correctness gaps across the Pi channel tickets that
no single ticket's own review saw together. The sweep ran four reviewers: Pi
correctness, Pi fit, Pi test, and non-Pi. This ticket fixes the behavior
defects and the test gaps that touch Windows and load stability. Structural
cleanup that changes no behavior is in
`260924-refactor-pi-channel-and-cost-module-boundaries`.

### 1. An approval decision can cross a reconnect

Locations: `agents-plugin-pi/src/approval-protocol.ts:108-181` and the parent
side `attachApprovalChannel` in `spawner.ts` (around line 2689).

`ChildApprovalGate.attach` keeps early decisions per `cmd_id` for the whole
process lifetime, not per connection. `resume()` reports only open waits. The
parent's reconnect reconciliation reads "hello does not report the `cmd_id`"
as "consumed" and releases the request, which drops ownership protection.

Failure paths:

- **Buffered early decision.**
  1. A decision is buffered as early.
  2. The connection drops before the wait opens.
  3. The hello reports nothing.
  4. The parent releases the request.
  5. The child later consumes the old connection's decision.

  This breaks the approval ticket's rule that "only a decision sent over the
  new connection can be consumed". It also breaks protection "until the
  consumption acknowledgment arrives".
- **Hang.** The wait opens with an early decision between hello and welcome.
  1. `consume`'s send throws.
  2. The early entry is already deleted.
  3. The hello already went out without the `cmd_id`.
  4. The parent releases the request.
  5. The command waits forever.

  A scratch test against the real `ParentChannel` and `ChildChannel`
  reproduced this at a 10 s timeout.
- **Lost decision.** A decision is lost in transit, and the child reconnects
  before `execute()` opens the wait. The hello reports nothing, the parent
  releases the request, and the later wait hangs until `ws-agent-stop`.

### 2. A reissued approval can reach the lead twice

Location: `heldActionState` in `spawner.ts` (around line 1389).

A held `ws-agent-approval` push is actionable while
`pending.decision === undefined`. A reissue clears `decision`, so the original
held push becomes actionable again next to the reissued push. The lead then
sees two prompts for one `cmd_id`. A second `ws-approve` is rejected with
"already sent", so safety holds.

The `pending.decision === undefined` condition has no test.

### 3. The durable descendant-usage copy is erased or never written

Locations: `spawner.ts` around lines 738, 758-768, and 774-779;
`agent-usage-rollup.ts:46` and `:65-70`.

`telemetry.descendantUsage` is documented as the durable copy.

- When `refreshAgentTelemetry` resets telemetry (origin mismatch or failed
  reduce), `finish()` persists `undefined`, which wipes the copy.
- `acceptDescendantUsage` skips persisting while `record.telemetry` is
  undefined. A fork on the non-fresh "boundary unknown" branch never re-creates
  telemetry, so its copy is never written.

Both undercount `retentionEvictionCost` and revival after restart.

### 4. The sidecar treats an absent home as removed while a removal claim is held

Locations: `agent-sidecar.ts:263` and `:395`.

The sidecar parse and revival use `!existsSync(home) || hasEvictionRecord(...)`.
`CostEstimateState.reconcile` uses `isOwnedHomeGone` (`agent-storage.ts`
around line 289), which deliberately keeps an absent home while a removal claim
is held, because the remover may restore it.

If an owner's session start parses its sidecar while a remover has the home
detached, and the remover then restores it, the orphan is dropped and the
sidecar cleared. The agent becomes permanently unrecoverable.

### 5. A stop during a launch produces a spurious `spawn-failed`

Locations: `spawner.ts` around line 3405 (spawn) and 3571 / 3601 (resume).

`stopAgent` does not invalidate the launch. The bind or hello wait then
rejects. On the resume path `ownsFailure()` is still true, because `stopAgent`
never bumps `launchGeneration` (`agents-plugin-pi/src/spawner.ts#L3591-L3602`,
`#L3803-L3870`); the spawn path has no `ownsFailure()` gate and its catch calls
`pushSpawnFailed` unconditionally (`agents-plugin-pi/src/spawner.ts#L3402-L3406`).
Either way `pushSpawnFailed` sends a
`ws-agent-settled reason:"spawn-failed"` on top of the stop's own outcome. The
new bind and hello waits widened this window and made the extra push
deterministic.

### 6. Test and hygiene gaps from the same sweep

- **Unguarded handler order.** `registerPushFlush` must be registered before
  the `publishSubtree` loop in `index.ts`. Only a source-text `indexOf` pin
  (`recursive-worker.test.ts:914-920`) guards that order. 928a48fa's AI Context
  claims the C1 behavior test also guards it. That is false: the C1 test runs
  on the `childProcess()` harness and never loads `index.ts`.
- **Stale comments after ef0961fc.**
  - `subtree-lifecycle.ts:39-41` says `turnOwed` "gates only the release of a
    settle the parent already holds".
  - The `spawner.ts` file header (around lines 59-64) says only a settle
    arriving while the subtree reads waiting is held.
- **Bare rename for eviction records.** `writeOwnerArtifact`
  (`agent-storage.ts:138`) uses a bare `renameSync`, not the
  `renameWithWindowsRetry` helper that the durable-write-hygiene ticket
  standardized. Eviction records, which gate removal, go through it. On
  Windows, EPERM or EBUSY aborts the removal with no retry.
- **Unguarded symlink tests.** `eviction-records.test.ts:609`, `:749`, and
  `:775` call `symlinkSync` without a win32 privilege guard. They fail with
  EPERM on Windows without Developer Mode.
- **Timing-thin tests** that can flake under load:
  - `agent-usage-rollup.integration.test.ts:368-375`: an exact send count
    ordered by a fixed 100 ms sleep.
  - `agent-channel.test.ts:225-231`: `rejects == ["timeout"]` depends on a
    close beating a 100 ms timer.
  - `agent-channel-launch.test.ts:126`: `elapsed < 250` against a 300 ms
    bound.

## Decisions

All mechanisms below are settled by the lead within the user's standing
direction: conservative options, and the lead settles mechanism.

1. **Approval release requires positive consumption evidence.**
   - **Positive consumption evidence.**
     - The child's hello resume section reports the still-waiting `cmd_id`, as
       today.
     - It also reports a bounded list of recently consumed `cmd_id`s, capped
       like `EARLY_DECISION_CAP`, oldest dropped first.
     - A new child always sends the approval section with a `consumed` array,
       even when it is empty. Omitting an empty section would make the parent
       take the older-child path below and keep the defect.
     - On a reconnect hello, the parent releases a discarded request only when
       its `cmd_id` appears in that consumed list.
   - **Reissue on missing evidence.** A request neither pending nor consumed
     is reissued, as a fresh ask to the lead. This follows the research
     ledger's confirmed decision that approval waiting state need not survive
     a disconnect and that a fresh request is acceptable.
   - **Connection-bound early decisions.**
     - An early decision is consumable only on the connection it arrived on.
     - When that connection ends, the child discards its buffered early
       decisions.
     - A decision is then consumed only after a reissue delivers it over the
       new connection.
   - **Failed acknowledgment.** A consumption acknowledgment whose send throws
     leaves the `cmd_id` waiting. The decision is discarded, as today. The
     consumed list is not updated.
   - **Rejected alternatives:**
     - Reporting buffered early decisions as pending. This does not cover a
       decision lost in transit.
     - A child-side "expected" set populated from the child's own tool-start
       event. It adds a second predicate that must mirror the parent's
       approval trigger.
     - Keeping "not reported" as consumption evidence. This is the defect.
   - **Accepted cost.** In the rare disconnect-before-wait case, the lead may
     be asked twice for one command. The command is never run on a stale or
     lost decision.
   - **Version skew.**
     - A hello with no `consumed` array, from an older child, keeps today's
       release-on-absence behavior. This fallback stays whether or not a
       mixed-version tree is possible.
     - The worker checks whether parent and child can load different plugin
       versions (for example a child started before a plugin update) and
       records the answer in the Result. It is not a stop condition.
2. **Reissue identity.**
   - Each approval request carries an issue number that is incremented on
     every reissue.
   - A held `ws-agent-approval` push records the issue number it was built
     for. It is actionable only while that number equals the request's current
     issue number and no decision is recorded.
   - The original push stays superseded after a reissue.
   - Rejected: dropping the original held push from `heldPushQueue` at
     reissue. That mutates the queue from the reconnect path and races the
     batch flush.
3. **The durable descendant-usage copy is independent of own-telemetry
   resets.**
   - The durable copy moves out of `AgentTelemetry` into a sibling ownership
     field, `OwnershipMetadata.descendantUsage` (`agent-storage.ts`), written
     through its own update path. A telemetry reset or an undefined telemetry
     therefore never touches it.
   - `acceptDescendantUsage` persists the sibling field whether or not
     `record.telemetry` exists. This supersedes the usage-rollup Result's
     "keeps its descendant value in memory only", which was chosen only
     because the copy then lived inside telemetry, so persisting it could
     clear own-usage fields. The sibling field removes that coupling.
   - Readers (`descendantUsageOf`, the eviction-record and revival paths)
     prefer the sibling field and fall back to a legacy
     `telemetry.descendantUsage` so records written before this change still
     count. New writes do not populate the legacy location.
   - **Legacy migration on first touch.** Before any telemetry persist that
     would drop a legacy `telemetry.descendantUsage` (a reset or a rewrite in
     `refreshAgentTelemetry`'s `finish()`), the value is copied into the
     sibling field if the sibling field is absent on disk. It uses the same
     compare-against-disk guard as `persistOwnershipTelemetry`. The legacy
     copy is never erased before the sibling copy exists.
   - **Sidecar revival source.** The sidecar format is unchanged. Revival
     reads the sibling field from the ownership record it already reads
     (`readOwnership`, around `agent-sidecar.ts:407`), with the same legacy
     fallback, instead of relying on the serialized `record.telemetry`.
   - Rejected: making `AgentTelemetry.origin` optional, which weakens
     `parseTelemetry` validation for every reader; and a synthesized origin,
     which on a fork would make the next reduce count inherited history as
     own usage.
   - Readers must keep treating a missing own-usage field as unknown, not as
     zero.
4. **One removal predicate for the sidecar.**
   - The sidecar parse and revival paths become claim-aware for both
     disjuncts of `!existsSync(home) || hasEvictionRecord(...)`:
     - While a removal claim is held, the entry is "unknown, keep", whether
       the home is absent or present and whether an eviction record exists.
       `removeOwnedAgentHome` writes the record under the claim before it
       detaches the home, and its `finally` deletes the record when the
       removal fails, so a record under a held claim is not yet proof of
       removal.
     - With no claim held, today's meaning stays: an eviction record, or an
       absent home, means removed. This keeps the legacy no-record
       absent-home case (pre-record checkpoint fold) dropping. Reconcile's
       `record && gone` is therefore not adopted as is.
   - The check reads in a stable order, as 03305a62's home -> lock -> home
     does. A remover takes the claim before writing the record, so a record
     counts as removal only if the claim is still absent when read again
     after the record read.
   - Such an entry is neither dropped nor cleared from the sidecar. The next
     parse decides.
   - Rejected: a sidecar-local retry loop.
   - Unifying all three predicates into one helper belongs to the refactor
     ticket. This ticket only makes the sidecar's two call sites claim-aware.
5. **A stop during a launch is the single terminal.**
   - `stopAgent` marks an in-flight launch as stopped. The marker is separate
     from `launchGeneration`, which `stopAgent`'s own generation check
     (around `spawner.ts:3847`) relies on staying unchanged.
   - A launch failure caused by that stop does not call `pushSpawnFailed`.
   - The stop's own result or notice is the only lead-visible push.
   - On the spawn path the `ws-agent-spawn` tool call still rejects, because
     no agent was launched. Its error says the launch was stopped, not that
     the spawn failed.
   - A genuine launch failure that is not caused by a stop still pushes
     `spawn-failed`, on both the spawn and resume paths.
6. **Registration order gets a behavior test.**
   - Add a test that loads the real `index.ts` extension factory, with the
     harness pattern the fork-lifecycle tests use, and fails when the
     `registerPushFlush` / `publishSubtree` order is swapped.
   - Keep the text pin only if such a test is infeasible. The Result then
     states why and corrects the false claim.
   - Do not rewrite 928a48fa's message.
7. **Hygiene.**
   - Update the two stale comments.
   - Route `writeOwnerArtifact`'s rename through `renameWithWindowsRetry`.
   - Guard every unguarded `symlinkSync` test with one shared probe helper
     that skips when symlink creation fails with EPERM. Files:
     `eviction-records.test.ts` (`:609`, `:749`, `:775`),
     `agent-storage.test.ts` (eight calls, `#L88-L416`), and
     `agent-footer.test.ts` (`#L217`, `#L296`).
   - Replace the timing-thin tests' fixed sleeps and wall-clock bounds with
     event-driven waits or deterministic seams. Keep each assertion's contract.

## Constraints

- Same-user forgery stays out of the threat model. Do not add authentication
  beyond today's channel credential.
- Do not edit any ticket other than this one.
- Windows: no new Unix-only assumptions. New tests run on win32 or carry an
  explicit, justified skip.
- Keep `agents-plugin-pi/rsrc/` untouched unless a playbook changes. None
  should.

## Prior Decisions

- e1094fb0 (2026-09-24, commit): "A hello that reports no pending cmd_id after a discard counts as consumed (ticket decision); ... A held approval push is actionable only while no decision is in flight or discarded." — bearing: superseded by Decisions 1 and 2 (release now needs positive consumption evidence; held-push actionability also checks the issue number)
- 306197ba (2026-09-24, commit): "Correctness Minor (early decision): the retired file rendezvous persisted until read; the channel does not, so the gate keeps early decisions. The early keep is per cmd_id (unique tool-call ids)" — bearing: superseded by Decision 1 (early decisions are bound to the connection they arrived on)
- 260923-research-pi-parent-child-loopback-control-channel (2026-09-24, Confirmed Decisions): "Approval *waiting* state need not survive a disconnected child channel; a fresh approval request is acceptable. This does not authorize replay or automatic re-execution of a command" — bearing: supports
- 260924-feat-pi-agent-channel-usage-rollup (2026-09-24, Result 19e57c98 / aba83809): "A child with no own-usage telemetry keeps its descendant value in memory only." — bearing: superseded by Decision 3 (sibling ownership field removes the reason for the in-memory-only choice)
- 03305a62 (2026-09-24, commit): "Check order home -> lock -> home closes the rename-back window: a remover renames back before releasing its lock." — bearing: supports
- b59f060f (2026-09-24, commit): "The gate sits before the disk re-validation. It requires a valid descriptor for the same agent whose home does not exist, so an unreadable or mismatched home still falls through to the legacy record" — bearing: constrains
- 19e57c98 (2026-09-24, commit): "existsSync is false on any stat error, so an unstat-able home reads as removed. Chose to qualify the comment rather than narrow to ENOENT" — bearing: constrains
- 928a48fa (2026-09-24, commit): "Test M5 (the textual order pin): the C1 behavior test already fails if the order is wrong." — bearing: corrected by Decision 6 (the claim is false; a behavior test replaces the text pin)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/approval-protocol.ts, spawner.ts, agent-usage-rollup.ts, agent-sidecar.ts, agent-storage.ts, subtree-lifecycle.ts, index.ts plus tests eviction-records, recursive-worker, agent-usage-rollup.integration, agent-channel, agent-channel-launch, agent-storage, agent-footer, agent-sidecar |
| scope.surface | cross-module | parent-child channel hello resume approval section and approval-consumed handling span approval-protocol.ts and spawner.ts; sidecar, storage, and rollup modules also change |
| scope.new_public_symbol | yes | a consumed cmd_id list in the exported approval resume section and a per-request issue number on pendingApproval and held approval pushes; names not yet chosen |
| scope.new_type_contract | yes | hello resume approval section gains a consumed list; PendingApprovalState at spawner.ts#L381 and HeldPush at spawner.ts#L1251 gain an issue number; OwnershipMetadata gains a sibling descendantUsage field (Decision 3), leaving parseTelemetry's origin requirement intact |
| scope.test_surface | existing | agents-plugin-pi/test/approval-protocol.test.ts, spawner.test.ts, push-wake.test.ts, execute-gateway.test.ts, agent-channel-launch.test.ts, eviction-records.test.ts, agent-storage.test.ts, agent-footer.test.ts, agent-sidecar.test.ts, recursive-worker.test.ts, agent-usage-rollup.integration.test.ts, agent-channel.test.ts; a shared symlink probe helper is new |
| complexity.reuse_points | confirmed | ChildApprovalGate.EARLY_DECISION_CAP, isOwnedHomeGone at agent-storage.ts#L289-L294, renameWithWindowsRetry in atomic-write.ts, fork-lifecycle.integration.test.ts index.ts loader harness |
| complexity.side_effect_risk | high | changes approval release semantics, durable ownership telemetry writes, sidecar orphan dropping, and launch-failure pushes across processes |
| risk.correctness | high | approval consumption across reconnects must never run a stale or lost decision nor hang, and removal gating decides permanent unrecoverability |
| risk.fit | moderate | moving the durable descendant copy to a sibling ownership field supersedes the usage-rollup in-memory-only choice and needs a legacy read fallback |
| risk.test | high | reconnect races and timing-thin rewrites require deterministic seams and 50-run load passes at 2x CPU oversubscription |
| risk.security_or_contract | moderate | parent-child hello protocol gains fields with a version-skew fallback; same-user forgery stays out of scope |

## Phases

Both phases share these gates:

- Every verification item has a test that fails under its matching mutation.
- Full `npm test` in `agents-plugin-pi`.
- Load: at least 50 runs per touched test file at 2x core CPU
  oversubscription, with zero failures. Produce the oversubscription with
  2x logical-core busy-loop processes (for example `yes > /dev/null` on
  Unix, or an equivalent `node -e "for(;;){}"` loop on any platform), and
  record the exact command, core count, and run counts in the Result so later
  runs are comparable.

### Phase 1: Approval reconnect, reissue identity, and stop during launch

Implement Decisions 1, 2, and 5. If a Phase 1 test lands in a file that
holds a timing-thin assertion from Decision 7 (for example
`agent-channel-launch.test.ts:126`), fix that assertion in Phase 1 so the load
gate measures only this phase's work, and say so in the Result.

**Verification.**

- **Decision 1:**
  - A buffered early decision, then a disconnect, then a reconnect: the parent
    reissues, the old decision is never consumed, and a fresh decision over the
    new connection is consumed.
  - A wait that opens between hello and welcome with an early decision does
    not hang. It is reissued.
  - A decision lost in transit, then a reconnect before the wait opens: the
    request is reissued, not released.
  - A consumed-then-disconnected decision: the reconnect hello lists it as
    consumed, and the parent releases the request.
  - A new child with nothing consumed sends an empty `consumed` array, and the
    parent reissues rather than releases.
  - A hello with no `consumed` array keeps today's behavior.
  - A failed acknowledgment send leaves the `cmd_id` pending and absent from
    the consumed list.
- **Decision 2:**
  - Sent and discarded decisions supersede the held push on the same
    `cmd_id`.
  - After a reissue, only the reissued push is actionable.
  - A `cmd_id` change supersedes, as today.
- **Decision 5:**
  - A stop during the resume bind window, and a stop during the spawn hello
    wait, each produce no `spawn-failed` push.
  - The stopped spawn's tool call rejects with a "stopped" error.
  - A genuine hello failure still produces a `spawn-failed` push.

### Phase 2: Usage durability, removal gate, order test, and hygiene

Implement Decisions 3, 4, 6, and 7.

**Verification.**

- **Decision 3:**
  - A telemetry reset keeps the persisted `descendantUsage`.
  - A non-fresh fork with undefined telemetry persists `descendantUsage`.
  - Retention eviction cost and sidecar revival read the persisted sibling
    value.
  - A record carrying only the legacy `telemetry.descendantUsage` is still
    read, and a telemetry reset on it copies the value into the sibling field
    before the legacy copy is dropped.
- **Decision 4:**
  - An absent home under a held removal claim survives a sidecar parse. After
    the claim is released with the home gone, the next parse drops it.
  - An eviction record with the home present under a held claim survives a
    sidecar parse; after the removal rolls back (record deleted, claim
    released), the entry is still revivable.
  - With no claim, a legacy no-record absent home is still dropped.
- **Decision 6:** The behavior test fails when the order in `index.ts` is
  swapped.
- **Decision 7:**
  - `writeOwnerArtifact` retries an injected EPERM or EBUSY rename through
    `renameWithWindowsRetry` (use that helper's existing injection seam, or
    pin that `writeOwnerArtifact` routes through it).
  - The symlink tests in all three named files skip cleanly when symlinks are
    unavailable.
  - The rewritten timing tests pass the load gate with zero failures.
