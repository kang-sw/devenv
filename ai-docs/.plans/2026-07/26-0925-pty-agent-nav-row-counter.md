# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 7: nav-row presentation

## Relevant Ticket Contract

- Fill the deferred agent-counter slot on the two-line work-root nav row with a
  SPLIT count — working N (spinner) / ready M (orange bell) — and add the
  owner-requested Windows-11-style orange flash as an INDEPENDENT OVERLAY LAYER
  (`## Constraints`: rules already write `background` on `.resource-row`, so an
  animation on `background` would fight all of them).
- **Aggregation (PINNED)**: a server row shows the highest-priority state among
  its work roots — `ready` outranks `working` outranks none.
- **Acknowledgement (PINNED)**: the nav badge is DERIVED and never separately
  acknowledged. Acknowledging a tab clears that terminal's state; a row's badge
  clears when no child terminal is still pending. Exactly one ack watermark.
- **Carrier (PINNED)**: the profile recorded on the pane in Phase 2 — NOT
  "terminals that have posted a hook event", which reads zero for a freshly
  spawned agent that has not finished a turn.
- **No double count (PINNED)**: an agent terminal counts in the AGENT counter
  ONLY, never also in the terminal count, or this ticket and
  `260725-feat-dashboard-nav-row-two-line-open-state` double-count the same pane.
- Phase 3's turn-start spike answered POSITIVE, so the `working` spinner half
  stays (three-state `working`/`ready`/`idle` vocabulary).
- Verification: browser acceptance asserting the counter split, no double count,
  a badge on a work root that is NOT selected, and that acknowledging the last
  pending tab clears the row badge without a separate action. Browser-level
  verification is BINDING (`ws-web-dashboard` `## Domain Rules`); build/tsc/curl
  do not close UI-facing work.
- Spec Impact owned here: `#260516-ws-web-dashboard-inspectable-navigation-shell`
  — the `{#260725-nav-row-open-surface-counts-and-open-state}` paragraph
  (`ai-docs/spec/ws-web-dashboard/index.md:1011-1031`) currently states
  "Agent counts are not part of this line." Phase 7 amends landed, asserted
  behaviour; append a note to `260725-feat-dashboard-nav-row-two-line-open-state`
  (`ai-docs/tickets/.done/`) rather than letting it be discovered as a broken
  assertion.

## Out of Scope

- Phase 8 (`document.title` flash, favicon badge, `Notification` opt-in,
  Settings entry). Independent of this phase.
- Any daemon change. See the decision below — this phase adds no Rust.
- **Workspace-presentation rows.** The nav ticket's Decision 4 excluded workspace
  rows from the second line entirely (`App.tsx:7808`
  `showOpenSurfaceCounts = presentation !== "workspace"`), and this phase's pinned
  aggregation rule names only SERVER rows. A multi-root workspace row therefore
  shows no aggregate while its child worktree rows do. Deliberate, not an
  oversight — record it in the phase Result rather than widening scope.
- Fixing `260726-bug-dashboard-agent-profile-provenance-lost-on-restart`
  (`reconcile_entry`'s adopt arm passes `profile_id: None`, so a daemon-restart-
  adopted agent terminal reverts to being counted as a plain terminal). Known,
  ticketed; do not let the browser gate accidentally depend on it either way.
- Pruning `attentionAcknowledgements` (Phase 6 Minor 2, refused on re-review).

## Codebase Findings

### The reserved slot and the counter's existing plumbing

- `ws-dashboard/frontend/src/App.tsx#L7730-L7741` — **the deferred agent-counter
  slot, as it actually exists.** `ResourceRow`'s prop comment: "Agent/agentChat
  surface counts are explicitly out of scope for this phase - do not extend this
  to a third count without a data source." Sibling reservation at
  `App.tsx#L2966-L2968` (`ResourceNavigation`'s prop doc: "agent counts are
  deferred"). The slot is a comment plus two `number | undefined` props
  (`terminalCount`, `documentCount`), not a placeholder element.
- `ws-dashboard/frontend/src/resourcePresentation.tsx#L166-L176` —
  `formatOpenSurfaceCounts(terminalCount, documentCount)`, the ONE formatter for
  the second line. `#L153-L164` — `countByRootKey`, the shared pure grouper.
  Both are exported and unit-tested (`npm run test:resource-presentation`).
- `ws-dashboard/frontend/src/App.tsx#L4218-L4248` — **where the existing terminal
  count is computed**, and therefore the concrete site for the no-double-count
  rule: `countByRootKey(Object.values(terminalPanes), pane =>
  serverScopedIdentity(pane.session.serverRoute, pane.session.workRootId))`,
  wrapped in a deliberate `terminalCountByRootSignature` change-detector because
  `terminalPanes` churns on every batched output-cursor flush. Pushed up to
  `App()` through `onTerminalCountByRootChange` (`App.tsx#L4247`, wired at
  `App.tsx#L2163` to `setTerminalCountByRoot`, state at `App.tsx#L556`).
- Render chain for the count, all prop-drilled and all already carrying the two
  existing counts: `App()#L2060` → `ResourceNavigation` (`#L2932`) →
  `ServerRows` (`#L3098`) → `WorkspaceRows` (`#L3255`) → `ResourceRow`
  (`#L7685`); the compact row consumes it at `App.tsx#L7542`, the plain
  work-root row at `#L7609`, and the second line renders at `#L7906-L7911`.
- `ws-dashboard/frontend/src/App.tsx#L3145-L3160` — **server-row assembly**
  (`.server-row`, `data-server-kind`/`data-server-status`); `#L3178-L3196` is
  where `WorkspaceRows` is mapped under it. That `<div className="server-row">`
  is the only element for the pinned server aggregation rule.

### Whether the nav row can see the attention state (threading answer)

- `ws-dashboard/frontend/src/App.tsx#L488-L512` — `attentionByKey` and
  `attentionAcknowledgements` ALREADY live at `App()` level, keyed by
  `serverScopedIdentity(serverRoute, terminalId)`.
- BUT `terminalPanes` lives inside `WorkbenchShell` (`App.tsx#L3721`), a separate
  component — Phase 6 hit exactly this and had to thread the two maps DOWN as
  props (`App.tsx#L2150-L2152`). The analogous path here runs the OPPOSITE
  direction and already exists: compute the agent counts in `WorkbenchShell`
  (where the panes are) and push them UP through a new callback prop mirroring
  `onTerminalCountByRootChange`. **No new downward threading is required and no
  new state location is invented.**
- `ws-dashboard/frontend/src/workbench/terminalWorkbenchPane.tsx#L19-L36` —
  `TerminalAttentionInput` and `terminalAttentionKey(pane)`, documented as "the
  ONE key-builder both the writer and this reader use. Hand-rolling the join
  here would silently desync." The nav counter is a third reader and must reuse
  the same join. `#L86-L97` is the existing consumer shape.
- `ws-dashboard/frontend/src/agentAttention.ts#L96-L107` —
  `pendingAttentionStateFor(entry, acknowledgedUpdatedAtMs, sessionStatus)`
  applies the liveness gate, the `idle` suppression, and the ack watermark, and
  returns `null` for "show nothing". Counting THROUGH this function satisfies
  the pinned acknowledgement rule for free: the row badge clears exactly when
  the last child terminal is acknowledged, with no second watermark.
- `ws-dashboard/frontend/src/terminals.ts#L14-L23` — `TerminalSessionView` carries
  BOTH `status` and `profileId: string | null`. `profileId != null` is the
  pane-recorded carrier the ticket pins; `status` is the liveness input.
- `ws-dashboard/frontend/src/App.tsx#L3927`, `#L4090` — `listTerminals`
  reconciliation runs per OPEN root, not only the selected one, so
  `terminalPanes` holds panes for open-but-unselected roots. This is what makes
  "a badge on a work root that is NOT selected" derivable at all.

### The stale-entry finding — decision, with the rejected option's cost

**Decision: (a) cross-reference the live session list in the browser. Do NOT
wire `attention.forget` into the daemon's IPC-death path in this phase.**

- Phase 6's stated obstacle still binds, verified fresh:
  `ws-dashboard/crates/daemon/src/terminal.rs#L1854-L1857` —
  `spawn_ipc_reader_task(session: Arc<TerminalSession>, reader)` receives only
  the `Arc`; `#L1842-L1852` `mark_ipc_closed(&self)` and `#L1825-L1836`
  `apply_helper_status(&self, ...)` are `TerminalSession` methods with no
  registry, `AppState`, or `AttentionHub` reach. `AttentionHub` IS `Clone`
  (`terminal.rs#L273-L275`), so (b) is *possible* — it means adding a hub handle
  to `TerminalSession`'s construction path and to `boot_reconcile`'s adopt arm,
  i.e. reopening the Phase 4/5 seam.
- **The aggregate-count requirement does not change the calculus in (b)'s
  favour — it removes (b)'s relevance.** Three independent reasons, in
  increasing order of finality:
  1. The attention entry is `{ terminalId, workRootId, state, updatedAtMs }`
     with NO profile field (`agentAttention.ts#L17-L22`). The pinned carrier is
     the profile on the PANE. A counter derived from `attentionByKey` cannot
     tell an agent terminal from a shell terminal, and cannot count a freshly
     spawned agent at all. The ticket's own carrier rule already forecloses
     map-derived counting; (b) would not unlock it.
  2. Even a perfect daemon-side forget leaves entries for terminals in work
     roots the BROWSER has closed (`openWorkRootKeys` is bare in-memory
     `useState` with no persistence — see `dashboard-acceptance.spec.ts:2886`).
     Those entries are legitimately live daemon-side, so the row for a closed
     root would badge while its own second line reads "no open surfaces". Only
     the pane join suppresses that.
  3. (a) costs nothing: `terminalPanes` IS the browser's reconciled view of the
     daemon session list, `pendingAttentionStateFor` already applies
     `status === "running"` per pane, and counting by iterating PANES (never the
     map) makes a dead agent's surviving entry structurally unreachable rather
     than filtered out.
- **Cost of the rejected option (b), priced so it is not re-litigated**: an
  `AttentionHub` handle threaded into `TerminalSession` construction and
  `boot_reconcile`'s adopt arm, new unit tests on the IPC-death path, and risk
  inside the seam Phase 4 and Phase 5 review each found a Critical bug in —
  buying nothing this phase needs, because points 1 and 2 still require the pane
  join afterwards. The daemon-side leak remains real for the SNAPSHOT served on
  reconnect and stays recorded as debt (Phase 5 Result finding 1; Phase 6 Result
  decision 1): do not silently close it here, and do not claim it fixed.

### Flash overlay

- `ws-dashboard/frontend/src/styles.css#L1030-L1045` — base `.resource-row` is
  `display: grid` with **no `position`**, so a pseudo-element overlay needs
  `position: relative` added here. Safe: the only absolutely-positioned
  descendant menu already has its own containing block
  (`.workspace-row-menu-wrap { position: relative }`, `styles.css#L3651-L3653`).
- The `background` cascade the ticket warns about, at CURRENT line numbers (the
  ticket's 2729/2743/2757 are stale): base `.resource-row`
  `styles.css#L2748-L2756`, `:hover` `#L2772-L2774`, `-error` `#L2785-L2789`,
  plus two rules the ticket predates — the openness rule `#L2797-L2806`
  (specificity (0,3,0)) and its hover re-assertion `#L2819-L2822` ((0,4,0)).
  **Five rules now write `background` on this element**, which strengthens
  rather than weakens the pseudo-element requirement.
- `ws-dashboard/frontend/src/styles.css#L2338-L2354` — the in-repo precedent for
  an attention animation: `.activity-ribbon-item[data-dirty="true"]
  .activity-ribbon-cue { animation: activity-cue-breathe 1.8s ease-in-out
  infinite }`. LEVEL-driven off a data attribute, not edge-triggered. Follow it.
- **What makes the flash "independent" of the counter** — two senses, both
  honoured: (i) CSS-layer independence, i.e. a pseudo-element animating
  `opacity`/`box-shadow` and never `background`, so it does not fight the
  five-rule cascade above; (ii) presentation independence, i.e. the flash is
  driven by the row's aggregate STATE attribute while the counter renders the
  NUMBERS, so neither rendering path needs to know about the other. It is NOT
  independent in the sense of owning its own dismissal — an edge-triggered flash
  with its own timer would BE a second ack watermark, which the ticket forbids.
  Level-driven from the same derived state is the only shape consistent with the
  pinned acknowledgement rule.
- `styles.css` contains **no `prefers-reduced-motion` block anywhere** (verified
  by grep). Adding one for the new flash is a one-rule addition and is the right
  call for an infinitely-animating nav element.

### Existing e2e coverage — where the new acceptance belongs

- `ws-dashboard/frontend/e2e/agent-attention-indicator.spec.ts` (489 lines) —
  Phase 6's gate. It already owns EVERY piece of harness this phase needs:
  `WS_DASHBOARD_STATE_HOME` temp state dir (`#L94-L102`), `readCallbackToken`
  reading `terminal-tokens/<id>.json` off disk (`#L198-L216`), `postTurnState`
  driving the Phase 4 route from Node with no cookie (`#L221-L246`),
  `openWorkRootMinimal`/`selectWorkRootMinimal`/`resolveWorkRootId`
  (`#L122-L172`), `terminalTab`/`terminalTabsLocator` (`#L174-L188`),
  `closeTerminalById` + `forceCloseTerminals` teardown (`#L248-L287`), and the
  `dummy-echo-hooked` spawn (`#L299-L322`).
  **Add a SECOND `test(...)` to this file; do not create a third spec.** The
  helpers are module-local and unexported, so a sibling file would duplicate
  ~240 lines and boot a third daemon. Do NOT rename the file (Phase 6's Result
  and existing `--grep` invocations name it).
- `ws-dashboard/frontend/e2e/agent-spawn-profile.spec.ts` — Phase 2's gate and
  the precedent for why these live outside the main suite. It asserts
  `data-profile-id`, not nav counts, so this phase cannot break it.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2855-L2935` — the
  nav-row Phase 1 assertions. **It imports `formatOpenSurfaceCounts` from
  `../src/resourcePresentation.js` and compares row text against its output**
  (`:2861`, `:2913`, `:2926`). A signature change is therefore a COMPILE-TIME
  break here, which is the concrete "amend that ticket's acceptance step" site
  from `## Spec Impact`. Known unrelated failure at `:3779` (fitNow
  short-viewport, `260725-bug-dashboard-fitnow-short-viewport-shrink`) — never
  try to fix it; judge runs by failure SITE, not exit code.
- `ws-dashboard/crates/daemon/src/agent_profile_registry.rs#L125-L152` —
  `DUMMY_ECHO_HOOKED_PROFILE`: always compiled in, real `hook_config` (so spawn
  mints a callback token), deliberately EMPTY event list (so no hook ever
  fires), `sleep 180` chosen to match the Playwright per-test timeout. Reuse it;
  do not invent a second mechanism.

### Shortcut-risk signals

- `shouldUpdateDockviewWorkbenchPanelParams` (`dockviewLayoutModel.ts#L63-L70`)
  is NOT on this phase's path — the nav row is plain React, not a Dockview
  param. Do not copy that fix's shape here.
- The signature change-detector at `App.tsx#L4218-L4232` exists for a measured
  reason (`terminalPanes` churns per output flush). A new agent-count map
  computed WITHOUT an equivalent signature gate would re-render the whole nav
  tree on every terminal output byte. Reuse the technique, and put the derived
  attention state — not the raw maps — into the signature string.
- The existing `terminalCountByRoot` must gain a filter. Adding a *second* map
  while leaving the first counting agents too is the exact double count the
  ticket forbids.

## Implementation Plan

1. **Pure aggregator, in `frontend/src/agentAttention.ts`.** Add
   `type NavAttentionCounts = { agents: number; working: number; ready: number }`
   and a pure `aggregateNavAttentionCounts(panes, attention)` that, for each pane
   with `profileId != null`, buckets by `serverScopedIdentity(serverRoute,
   workRootId)` and classifies with `pendingAttentionStateFor(
   attention.attentionByKey[k], attention.acknowledgements[k], status)` where
   `k` is the `serverScopedIdentity(serverRoute, terminalId)` join
   `terminalAttentionKey` builds. Also add pure
   `navAttentionTone(counts): "ready" | "working" | null` implementing the
   pinned `ready > working > none` priority, and `aggregateNavAttentionTone` for
   the server roll-up. These live here (not `resourcePresentation.tsx`) because
   they depend on `pendingAttentionStateFor`; tested by
   `npm run test:agent-attention`.
   IMPORT DIRECTION: `agentAttention.ts` must NOT import from `workbench/` (the
   barrel back-import trap in the mental model's `## Common Mistakes`). Declare a
   minimal structural pane type locally rather than importing
   `terminalWorkbenchPane.ts`; keep the key string identical.
2. **Compute in `WorkbenchShell` and push up.** Beside `App.tsx#L4218-L4248`, add
   `agentAttentionByRoot` using the SAME signature-gate technique (the signature
   must include each root's `agents/working/ready` triple, so ack and turn-state
   changes propagate while output churn does not), plus a new
   `onAgentAttentionByRootChange` prop mirroring `onTerminalCountByRootChange`
   (declare at `App.tsx#L3747`/`#L3815`, wire at `#L2163`, new `useState` beside
   `#L556`).
3. **Close the double count at its one site.** In the existing
   `terminalCountByRoot` `useMemo` AND its signature (`App.tsx#L4218-L4246`),
   filter to `pane.session.profileId == null`. Pin in a comment that
   `profileId != null` is the agent predicate — the Phase 2 pane-recorded
   carrier — not `hook_config` presence and not "has posted a hook event".
4. **Formatter.** Extend `formatOpenSurfaceCounts`
   (`resourcePresentation.tsx#L166-L176`) with the counts triple as a third
   argument, appending an agent segment ONLY when `agents > 0` so the zero-agent
   strings stay byte-identical (`"no open surfaces"` /
   `"N terminals, M documents"`) and `dashboard-acceptance.spec.ts`'s existing
   comparisons keep passing on their existing inputs. Render the split with
   working before ready, both halves shown whenever `agents > 0`, so the spinner
   half is visible per the Phase 3 gate.
5. **Row rendering.** Thread `agentAttentionByRoot` down the same chain as
   `terminalCountByRoot` (`ResourceNavigation` → `ServerRows` → `WorkspaceRows`
   → `ResourceRow`), pass at `App.tsx#L7542` and `#L7609`, and in `ResourceRow`
   (a) feed the formatter at `#L7906-L7911`, (b) add
   `data-row-attention={navAttentionTone(counts) ?? "none"}` to the
   `.resource-row` element (`#L7813`), and (c) render the two glyph halves
   (spinner + bell) inside `.resource-row-counts`, each gated on non-zero.
   Delete the now-false "agent counts are deferred" comments at `#L2966-L2968`
   and `#L7730-L7741` instead of leaving them contradicting the code.
6. **Server-row aggregation.** In `ServerRows` (`App.tsx#L3145-L3196`) compute the
   highest-priority tone across that server's work roots from the same map and
   put `data-row-attention` on the `.server-row` element. No count on the server
   row — the pinned rule says "shows the highest-priority state", and the server
   row has no second line.
7. **Flash overlay CSS.** In `styles.css`: add `position: relative` to
   `.resource-row` (`#L1030`); add
   `.resource-row[data-row-attention="ready"]::after` and the `.server-row`
   sibling as an absolutely positioned, `pointer-events: none`, `inset: 0` layer
   animating `opacity`/`box-shadow` in the orange `--ws-color-state-warning`
   family, following the `activity-cue-breathe` precedent (`#L2338-L2354`), with
   a quieter `working` variant on `--ws-color-state-info`. **Never touch
   `background`.** Add a `@media (prefers-reduced-motion: reduce)` block that
   disables the animation while keeping the static tint. State the new rules'
   specificity in the comment, per the mental model's `.resource-row` cascade
   rule.
8. **Docs.** Amend the `{#260725-nav-row-open-surface-counts-and-open-state}`
   paragraph (`ai-docs/spec/ws-web-dashboard/index.md:1011-1031`), replacing
   "Agent counts are not part of this line." with the agent split, the
   no-double-count rule, the derived-never-separately-acknowledged rule, the
   server-row aggregation rule, and the flash-as-overlay rule. Append an Edition
   note to
   `ai-docs/tickets/.done/260725-feat-dashboard-nav-row-two-line-open-state.md`
   recording that its Phase 1 acceptance step and its "counts derived from
   `terminalPanes` wholesale" behaviour are amended here.

## Verification Plan

Baselines entering this phase: `cargo test -p ws-dashboard-daemon --lib` = 204
passed / 0 failed / 2 ignored. `cargo test --test routes` has 2 KNOWN
pre-existing failures
(`dashboard_resources_refresh_prunes_workspace_without_available_work_roots`,
`online_missing_work_root_returns_bounded_unavailable_without_path_leak`).
**This phase adds no Rust**, so both must come back unchanged — a changed daemon
number is itself a finding.

### Pure TypeScript

- `npm run test:agent-attention` — new cases for `aggregateNavAttentionCounts`
  (agent-vs-shell partition by `profileId`; an agent whose `status !== "running"`
  contributes to neither `working` nor `ready`; an acknowledged entry
  contributes to neither; a fresh agent with no entry still increments
  `agents`) and for `navAttentionTone`'s pinned `ready > working > none` order.
- `npm run test:resource-presentation` — the extended formatter, including the
  byte-identical zero-agent strings.
- `npm run build`, plus the suites Phase 6 ran (`test:workbench`,
  `test:terminals`, `test:work-root-activity`) since `App.tsx` and
  `terminals.ts`-adjacent types are touched. A green `npm run build` (Bundler)
  does not imply green `npm run test:*` (NodeNext).

### Browser acceptance — one new `test(...)` in `agent-attention-indicator.spec.ts`

Setup (extend `beforeAll` with a SECOND temp work root, `workRootB`): open root
A and root B; in root B spawn one plain terminal `T0` and two
`dummy-echo-hooked` terminals `A1`, `A2` through the existing direct-POST +
reload + reselect path; read both callback tokens off disk. Wrap the body in
`try/finally` with unconditional `forceCloseTerminals` (Phase 6 Minor 3). The
whole test must finish inside the profile's `sleep 180`.

Each required assertion, named, with the production mutation that must break it:

1. **`counter split`** — after `postTurnState(A1, "working")` and
   `postTurnState(A2, "ready")`, root B's `.resource-row-counts` reads an agent
   segment containing BOTH `1 working` and `1 ready`.
   *Mutation:* collapse the two halves in `formatOpenSurfaceCounts`
   (`resourcePresentation.tsx`, step 4) into a single total → fails on this
   assertion's text. Holding both states live simultaneously is what makes this
   non-vacuous; a single-state fixture would not be.
2. **`no double count`** — the same row simultaneously reports `2 agents` and
   exactly `1 terminal` while three terminal tabs are mounted (assert the
   mounted tab count is 3 in the same step, so the `1` is proven to be an
   exclusion rather than a stale read).
   *Mutation:* drop the `profileId == null` filter from `terminalCountByRoot`
   (`App.tsx`, step 3) → the row reads `3 terminals` → fails.
3. **`badge on a non-selected work root`** — select root A; assert root A is the
   selected row AND root B still carries `data-row-attention="ready"` with its
   agent counts intact, with no Dockview pane for root B mounted.
   *Mutation:* gate the aggregation on the selected root (compute
   `agentAttentionByRoot` from `isSelectedRoot` panes only, `App.tsx` step 2) →
   root B's attribute reads `none` → fails. It also fails if the aggregate is
   read from `attentionByKey` keyed by terminal id without the per-root pane
   join.
4. **`acknowledging the last pending tab clears the row badge`** — bring both
   agents to `ready`, re-select root B and make `T0` the ACTIVE tab (so
   selection itself cannot ack an agent tab), assert `data-row-attention="ready"`;
   click `A1`'s tab → row still `ready`, agent segment reads `1 ready`; click
   `A2`'s tab → row `data-row-attention="none"` and the agent segment reads
   `0 working, 0 ready` — with NO nav-row click of any kind in between.
   *Mutation:* have the aggregator ignore `attention.acknowledgements` (call
   `pendingAttentionStateFor(entry, undefined, status)`, `agentAttention.ts`
   step 1) → the row badge survives both tab acks → fails. Using TWO agents is
   what makes this prove "clears when NO child is still pending" rather than
   merely "clears on any ack".
5. **`server-row aggregation`** (pinned as contract even though the ticket's
   verification sentence does not name it) — with `A1` `working` and `A2`
   `ready` under the same server, `.server-row` reads
   `data-row-attention="ready"`.
   *Mutation:* swap the priority order in
   `navAttentionTone`/`aggregateNavAttentionTone` (`agentAttention.ts`, step 1)
   → reads `working` → fails.

Regression: `npx playwright test agent-spawn-profile.spec.ts` (must stay 1
passed) and `npx playwright test dashboard-acceptance.spec.ts` — judged by
failure SITE: only `:3779` (fitNow short-viewport) may fail, with `:4020`
skipped behind it by serial mode. The nav-row step at `:2855-:2935` must pass;
its `formatOpenSurfaceCounts` call sites need the new argument.

Non-vacuity discipline: run each mutation above against production source,
record the observed failure SITE, revert, and confirm a clean tree — the standard
every prior phase in this ticket met.

## Escalations

- None. Confidence: high. The one finding requiring a decision (map-derived vs
  pane-derived counting) is resolved above against fresh source evidence, and the
  rejected daemon-side option is priced rather than deferred.
