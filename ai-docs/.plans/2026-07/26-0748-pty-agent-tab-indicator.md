# Plan: PTY-agent attention notification — Phase 6: tab-label indicator — end-to-end slice closes here

## Relevant Ticket Contract

- `DockviewWorkbenchTab` (`workbench/dockviewLayout.tsx:343-401`) renders only
  icon, title, and close today. Phase 6 adds a state affordance driven by the
  Phase 5 `attentionByKey` stream, with acknowledgement clearing it — reuse the
  ack-watermark PATTERN, not the `attention` field.
- Verification is explicitly split: (1) automated — drive the Phase 4 callback
  endpoint (`POST /api/dashboard/terminals/{terminal_id}/turn-state`) directly
  to synthesize a turn boundary, then assert the indicator appears and clears
  on acknowledgement; (2) recorded manual — one real run with an actual agent
  CLI, captured in the Phase 6 Result, the only step proving hook injection +
  browser path together.
- Binding constraints (lead-supplied, beyond ticket text): reuse the
  ack-watermark pattern; do NOT add an `attention` field to the pane/session
  model (`TerminalPaneState.session` / `TerminalSessionView` /
  `TerminalRegistryEntry`); the callback token must never appear in helper
  argv, `TerminalRegistryEntry`, a URL, or a log line; do not remove
  `terminal-notify`'s silent exit-0.
- ws-web-dashboard domain rule: any change to visible browser UI requires
  browser-level verification (Playwright/DOM automation); `tsc`/build/curl
  alone do not close UI-facing work (`ai-docs/mental-model/ws-web-dashboard/index.md:23`).
- Known baselines entering this phase: `cargo test -p ws-dashboard-daemon --lib`
  201 passed / 0 failed / 2 ignored; `cargo test -p ws-dashboard-daemon --test
  routes` has 2 pre-existing failures
  (`dashboard_resources_refresh_prunes_workspace_without_available_work_roots`,
  `online_missing_work_root_returns_bounded_unavailable_without_path_leak`);
  `dashboard-acceptance.spec.ts` has one known unrelated failure at ~:3779
  (fitNow short-viewport) — judge by failure site, do not fix either.

## Out of Scope

- Phase 7 (nav-row split counter, aggregation, orange-flash overlay) and
  Phase 8 (browser-level notification) — do not design either.
- Widening the `EventSource` subscription-lifecycle test coverage beyond what
  this phase's own browser step happens to exercise incidentally (Phase 5's
  known gap: only the pure `shouldReplaceAttentionSourceOnError` predicate is
  unit-tested, not the open/close/dedup wiring end to end — no jsdom harness
  exists). If the new spec below happens to cover part of it, say so in the
  Result; do not build new coverage for it on purpose.
- Any daemon-side `AttentionHub` reaper/expiry work beyond what the chosen
  stale-indicator fix requires (see Codebase Findings + Implementation Plan
  below). The callback-token half of the session-eviction gap is explicitly
  Phase 4's inherited debt (already recorded in Phase 5's Result) and stays
  untouched.
- Changing `DUMMY_ECHO_PROFILE`'s existing `hook_config: None` — Phase 2's own
  test (`agent_profile_registry.rs:145`,
  `dummy_echo_profile_has_no_hook_config`) asserts the opposite on purpose; add
  a new profile instead (see below), don't repurpose this one.
- Codex profile, MCP injection, Web Push — already out of scope per ticket
  `## Deferred scope`.

## Codebase Findings

- `frontend/src/App.tsx:478-487` — `attentionByKey: Record<string,
  AgentAttentionEntry>` is keyed by `serverScopedIdentity(serverRoute,
  terminalId)` and lives at the top App level, populated but never read
  (Phase 5's own Result note). This is the sole read source for Phase 6.
- `frontend/src/App.tsx:1864-1886` — both the snapshot and single-entry
  handlers write keys as `serverScopedIdentity(serverRoute, entry.terminalId)`;
  `frontend/src/resourceModel.ts:81-85` (`serverScopedIdentity`) is the exact
  key-builder to reuse when reading, not a hand-rolled string join.
- `frontend/src/agentAttention.ts:14-21` — `AgentAttentionEntry = {
  terminalId, workRootId, state: "working"|"ready"|"idle", updatedAtMs }`.
  `updatedAtMs` is the natural ack-watermark revision token (mirrors
  `activityItemRevisionToken`'s role).
- `frontend/src/workbench/terminalWorkbenchPane.tsx:46-71` —
  `terminalWorkbenchPane(pane, actions)` is the exact per-terminal
  presentation-object construction site: it already has `pane.session.serverRoute`,
  `pane.session.terminalId`, and `pane.session.status` in scope. This is the
  correct injection point for a derived indicator; its caller
  `terminalWorkbenchPanesByGroup` (same file, `:10-44`) is the batching point
  where `attentionByKey` + ack-watermark state must be threaded in from
  `buildWorkbenchEditorGroups` (`workbench/editorGroups.ts:53`) <-
  `buildEditorGroupsForRoot` (`App.tsx:4492-4591`).
- `frontend/src/workbench/editorGroups.ts:30-40` (`WorkbenchPane`) and
  `frontend/src/workbench/dockviewLayout.tsx:23-31`
  (`DockviewWorkbenchPane`)/`:85-92` (`DockviewWorkbenchPanelParams`) are
  purely presentational, rebuilt every render from source state — distinct
  from `TerminalPaneState`/`TerminalSessionView`. Adding a derived
  `attentionState` field here does NOT violate the "no attention field on the
  pane/session model" constraint, which is about the durable session objects,
  not this per-render view type.
- **RISK (non-obvious constraint).**
  `frontend/src/workbench/dockviewLayoutModel.ts:33-57`
  (`shouldUpdateDockviewWorkbenchPanelParams`) has a `persistentTerminal`
  early-return branch that compares ONLY `meta[1]` (socketStatus), deliberately
  to avoid Dockview panel-param churn fighting the mounted xterm instance. A
  new attention field that is not added to this comparison will silently never
  trigger a Dockview `updateParams` call — the tab would never visually update
  on a working->ready->idle transition even though `attentionByKey` changed
  correctly. This line MUST be touched.
- `frontend/src/workRootActivity.ts:508` (`ActivityAcknowledgements = Record<string,
  string>`), `:568-587` (`initializeActivityDirtyItems`), `:589-598`
  (`acknowledgeActivityItem`) — the ack-watermark PATTERN to reuse: an
  acknowledgements map from id to the last-seen revision token, "dirty" is
  "no ack, or ack token !== current token."
- `frontend/src/ActivityConsole.tsx:96-105,162-165` — the concrete usage
  precedent: ack state is a component-local `useState`, and acknowledgement
  fires from a callback invoked on item selection (`acknowledgeSelected`).
- `frontend/src/App.tsx:6324-6334` (`selectPane`) — the tab-click entry point
  already wired to `onSelectPane={selectPane}`
  (`App.tsx:6528`) and already special-cases `pane?.kind === "persistentTerminal"`.
  This is the natural acknowledgement trigger for Phase 6, mirroring
  `acknowledgeSelected`'s role in `ActivityConsole.tsx`.
- `frontend/src/terminals.ts:9-14` — `TerminalSessionView.status: "running" |
  "exited" | "terminated" | "error" | string`. This is the render-time signal
  the stale-indicator fix (below) keys off.
- **Stale-indicator gap — daemon-side evidence, confirming the lead's
  framing.** `ws-dashboard/crates/daemon/src/terminal.rs:1679-1701`
  (`is_live`/`admits_attach`), `:440-448` (`list_for_work_root` filters by
  `admits_attach()`, not `is_live()`), `:46` (`DAEMON_GRACE_WINDOW_MS =
  30_000`), `:1825-1852` (`apply_helper_status`/`mark_ipc_closed` both open a
  30s grace window the instant status leaves `Running`), `:532-587`
  (`insert`'s eviction `retain` is the ONLY one of four `attention.forget`
  call sites that fires for a helper that died without a browser `DELETE` —
  the other three are `:652` `remove` and `:687`
  `remove_for_work_roots`, neither reachable from an IPC-only death). Net
  effect: the daemon still lists a dead-but-in-grace-window terminal for up to
  30s with a non-`running` status, and its `sessions`-map entry (plus its
  `AttentionHub` snapshot entry) survives indefinitely past that until some
  UNRELATED `insert()` call runs its retain step.
- `ws-dashboard/crates/daemon/src/agent_profile_registry.rs:94-100`
  (`DUMMY_ECHO_PROFILE`) has `hook_config: None`, and `terminal.rs:1444`
  (`if let (Some(hook_config), Some((_, args))) = ...`) gates callback-token
  generation on `hook_config.is_some()`. **This means the existing `dummy-echo`
  test profile used by Phase 2's browser spec cannot be used to drive Phase
  6's automated callback-endpoint test — it never gets a token.** A new
  test-only profile with `hook_config: Some(...)` is needed, following the
  exact `DUMMY_ECHO_PROFILE` precedent (`:86-93`'s CONTRACT comment: always
  compiled in, invisible on every user-facing surface, id known only to this
  module + its own Playwright spec, never a vendor CLI). An empty
  `HookConfigShape { events: &[] }` is sufficient — `materialize_hook_config`
  (`agent_hook_config.rs:32-52`) loops over `shape.events` and is a no-op on
  an empty slice, and the dummy command never actually runs a CLI that would
  fire a hook — only `hook_config.is_some()` needs to be true to make `spawn`
  generate a real `callback_token`/`terminal-tokens/<id>.json`.
- `ws-dashboard/crates/daemon/src/agent_token_store.rs:34-65`
  (`token_store_dir`/`token_store_path`/`read_token`) — the on-disk token
  layout a test harness needs to replicate path construction for, to read a
  spawned terminal's token directly off disk (no HTTP hop, keeping the token
  off any URL/log per the identity-privacy constraint).
- `frontend/e2e/daemonHarness.ts` has no `--state-dir`/`WS_DASHBOARD_STATE_DIR`
  override plumbed to the spawned daemon (confirmed by search — no match), and
  `persistent_state.rs:495-510` documents no override flag exists at all. The
  acceptance daemon therefore uses the real per-user default state dir (Phase
  3's own Result, finding 1, independently observed this same fact when a test
  polluted `~/.local/state/ws-dashboard/`). The new Playwright spec (below)
  must compute that same OS-default path itself to read the token file — it
  cannot rely on an isolated per-test state dir.
- Precedent for the new spec's shape: Phase 2's
  `frontend/e2e/agent-spawn-profile.spec.ts` (own daemon/workRoot, a dedicated
  sibling spec rather than a `test.step` inside
  `dashboard-acceptance.spec.ts` — see that file's own CONTRACT comment for
  why) is the pattern to copy for Phase 6's new spec.

## Implementation Plan

1. **Backend — new test-only profile**
   (`ws-dashboard/crates/daemon/src/agent_profile_registry.rs`): add a second
   dummy `AgentProfile` (e.g. id `"dummy-echo-hooked"`) reusing
   `DUMMY_ECHO_COMMAND`/`DUMMY_ECHO_ARGS`/`agent_env_profile::NONE`, but with
   `hook_config: Some(HookConfigShape { events: &[] })` so `spawn` generates a
   real `callback_token` (`terminal.rs:1444`) without ever invoking a vendor
   CLI. Register it in `PROFILES` (`:102`) and add unit tests mirroring
   `resolve_finds_the_dummy_echo_profile_with_a_no_op_scrub` /
   `dummy_echo_profile_has_no_hook_config`, asserting the INVERSE
   (`hook_config.is_some()`) for the new id. Keep it out of the profile
   registry's user-facing surface, exactly like `dummy-echo`.
2. **Stale-indicator fix — render-layer suppression (chosen; see decision
   below).** In `terminalWorkbenchPane` (`workbench/terminalWorkbenchPane.tsx:46-71`),
   only surface a non-null `attentionState` when `pane.session.status ===
   "running"`. A session whose helper died is reported as
   `"exited"/"terminated"/"error"` on the very next `listTerminals`
   reconciliation (bounded by the 30s daemon grace window,
   `terminal.rs:46`), so a pure render-time gate hides the stale entry as soon
   as the frontend's normal reconciliation cycle runs — no daemon change.
3. **Presentational model.** Add an optional `attentionState?: AgentAttentionState`
   field to `WorkbenchPane` (`workbench/editorGroups.ts:30-40`) and
   `DockviewWorkbenchPane` (`workbench/dockviewLayout.tsx:23-31`) — a
   render-derived field, NOT a new field on `TerminalPaneState.session` /
   `TerminalSessionView` / `TerminalRegistryEntry`. Thread it through
   `toDockviewWorkbenchPanelParams` (`dockviewLayout.tsx:522-538`) into
   `DockviewWorkbenchPanelParams` (`:85-92`) and
   `DockviewWorkbenchPanelParamsForSync` (`dockviewLayoutModel.ts:4-15`).
4. **Ack-watermark state (App.tsx).** Add a new
   `attentionAcknowledgements: Record<string, number>` `useState` beside
   `attentionByKey` (`App.tsx:485-487`), keyed by the same
   `serverScopedIdentity(serverRoute, terminalId)` string, valued by the last
   acknowledged `entry.updatedAtMs` — mirroring
   `ActivityAcknowledgements`/`initializeActivityDirtyItems`
   (`workRootActivity.ts:508,568-587`). A terminal's indicator is "dirty" (show
   it) when `attentionByKey[key]?.state !== "idle"` AND
   (`attentionAcknowledgements[key]` is undefined OR `!==
   attentionByKey[key].updatedAtMs`).
5. **Wire ack + derive per-pane state.** Thread `attentionByKey` and
   `attentionAcknowledgements` down through `buildEditorGroupsForRoot`
   (`App.tsx:4492-4591`) -> `buildWorkbenchEditorGroups`
   (`workbench/editorGroups.ts:53`) -> `terminalWorkbenchPanesByGroup` ->
   `terminalWorkbenchPane` (both `workbench/terminalWorkbenchPane.tsx`), which
   computes the final `attentionState` per pane per step 2's suppression rule
   plus the dirty check from step 4.
6. **Acknowledge on select.** In `selectPane` (`App.tsx:6324-6334`, already
   branches on `pane?.kind === "persistentTerminal"`), when the selected pane
   has a dirty attention entry, update `attentionAcknowledgements` with that
   entry's `updatedAtMs` — the click-to-acknowledge trigger, mirroring
   `ActivityConsole.tsx`'s `acknowledgeSelected` (`:162-165`).
7. **Fix the Dockview param-diff blind spot (REQUIRED).** Widen the
   `persistentTerminal` branch in `shouldUpdateDockviewWorkbenchPanelParams`
   (`workbench/dockviewLayoutModel.ts:54-57`) to also compare the new
   `attentionState` field (not only `meta[1]`/socketStatus), or a
   working->ready->idle transition will never repaint the tab.
8. **Render the affordance.** In `DockviewWorkbenchTab`
   (`workbench/dockviewLayout.tsx:343-401`), render a small state
   badge/icon next to the existing icon/title, keyed off
   `params.attentionState`, and expose it via a stable
   `data-attention-state` attribute (matching this component's existing
   `data-workbench-*` attribute convention) so it is Playwright-assertable
   without relying on CSS/visual inspection.
9. **New browser spec**
   (`frontend/e2e/agent-attention-indicator.spec.ts`, own daemon/workRoot,
   sibling to `dashboard-acceptance.spec.ts` per the
   `agent-spawn-profile.spec.ts` precedent): spawn a terminal using the new
   `"dummy-echo-hooked"` profile (production route, not user-facing), compute
   the daemon's default per-user state-dir path from the harness's own
   knowledge of the platform (mirroring `agent_token_store.rs:34-38`'s path
   construction) and read `terminal-tokens/<terminal_id>.json` directly with
   Node `fs` (never over HTTP, never logged), POST `{ token, state: "ready" }`
   to `/api/dashboard/terminals/{terminal_id}/turn-state`, assert
   `data-attention-state="ready"` appears on the tab, click the tab, assert it
   clears.
10. **Manual verification (required, not optional).** Spawn a real `"claude"`
    profile terminal from the toolbar (Phase 2's existing button), complete
    one prompt/reply turn, observe the tab indicator appear at `Stop` and clear
    on selecting the tab. Record what was observed (screenshots or a precise
    written description of the before/after DOM state) directly in the Phase 6
    `### Result`.

**Stale-indicator decision (required by lead directive):** choose **(a)
render-layer suppression** (step 2 above), not (b) daemon-side
`attention.forget` on the IPC-death path. Reason: `pane.session.status` is
already carried on every `listTerminals` reconciliation and flips away from
`"running"` the instant `apply_helper_status`/`mark_ipc_closed` fire
(`terminal.rs:1825-1852`), so a pure render-time gate closes the user-visible
bug with zero daemon change; option (b) would require threading `AttentionHub`
access into `spawn_ipc_reader_task` (`terminal.rs:1855`, which today only holds
`Arc<TerminalSession>`, not the registry), a real Phase-4/5 seam change this
phase does not own, and would still leave the reattach question live (no code
path re-lives a session already marked non-`Running` within one daemon
process — `grace-reattach` per the `terminal.rs:1689` CONTRACT comment is a
boot-restart concept, and a full restart already wipes the in-memory
`AttentionHub` regardless — so (b) is not incorrect, just disproportionate
scope for this phase). The underlying daemon-side `AttentionHub`/`sessions`
map leak past the 30s grace window is unaffected by this choice and stays
exactly as Phase 5's Result already recorded it: real, bounded-impact
(memory/snapshot-payload only, no longer user-visible once step 2 lands), and
explicitly inherited debt, not fixed here.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --lib` — expect 201 + N (new profile-registry
  tests), 0 failed.
- `cargo check -p ws-dashboard-daemon --tests` — exit 0.
- `npm run build` (tsc -b + vite build) — clean.
- `npx playwright test --grep "agent attention indicator"` (new dedicated spec)
  — asserts indicator appears on a synthesized callback POST and clears on tab
  click, with no dependency on a vendor binary, credentials, or network.
- `npx playwright test dashboard-acceptance.spec.ts` (full regression) —
  judge by failure site: only the pre-existing ~:3779 fitNow short-viewport
  failure is acceptable; do not fix it, do not let a new failure hide behind
  it.
- `cargo test -p ws-dashboard-daemon --test routes` — expect the same 2
  pre-existing failures, no new ones.
- Manual real-`claude`-CLI run, recorded in the Phase 6 Result per the
  ticket's split-verification requirement (see Implementation Plan step 10).

## Escalations

- None.
