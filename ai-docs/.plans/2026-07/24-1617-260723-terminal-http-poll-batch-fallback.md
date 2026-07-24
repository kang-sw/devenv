# Plan: 260723-bug-dashboard-terminal-http-poll-oN-fallback — Phase 1: Batch fallback polls and bound frontend output growth

## Relevant Ticket Contract

- Step 1: new daemon route pair `POST /api/dashboard/terminals/output/batch`
  (unscoped) and `POST /api/dashboard/servers/{serverRoute}/terminals/output/batch`
  (server-scoped), mirroring the existing single-ID output route's
  scoped/unscoped split. Request: `{"cursors": [{"terminalId": "<id>", "after": <u64>}, ...]}`.
  Response: `{"results": {"<terminalId>": <TerminalOutputView>, ...}}`. Unknown
  or inaccessible terminal IDs are omitted from `results`, not per-ID errored
  and not a whole-batch failure. Auth/work-root gating stays per-terminal
  (same `resolve_online_available_work_root` check as the single-ID handler).
  Extends the `Remote Terminal HTTP Lifecycle` spec anchor
  (`ai-docs/spec/ws-web-dashboard/index.md#remote-terminal-http-lifecycle`,
  under `260703-ws-dashboard-server-route-scoped-operation-endpoints`).
- Step 2: replace the per-pane `fetchTerminalOutput` loop in `poll()` with one
  batched POST per tick carrying every `livePollPanesRef.current` pane's
  cursor; keep the in-flight guard (now one in-flight batch, not N); apply
  the batched response via a single `setTerminalPanes`, preserving
  `canApplyTerminalOutputPoll` / `terminalOutputPollChangedState` per-pane
  skip logic so a quiet pane still causes no re-render.
- Step 3 (secondary guard): back off `terminalOutputPollIntervalMs` once the
  number of concurrently fallback-polling panes exceeds a small threshold
  (exact curve/threshold is an implementation-time choice), and recover once
  the count drops back at/below threshold.
- Step 4 (load-bearing correctness fix): add `outputTrimOffset: number` to
  pane state, incremented by the trimmed character count on every front-trim
  in `appendTerminalOutput`. In the delta-write effect, replace the raw-length
  `writtenLengthRef` comparison with an absolute-position comparison
  (`writtenAbsoluteRef`, tracking `outputTrimOffset + writtenLength-at-last-write`):
  compute `currentEnd = pane.outputTrimOffset + pane.output.length`; if
  greater than the ref, write `pane.output.slice(localStart)` where
  `localStart = Math.max(0, writtenAbsoluteRef.current - pane.outputTrimOffset)`
  (clear+redump only if clamped); if less, unchanged clear+redump; if equal,
  no-op. Confined to the delta-write effect's own bookkeeping and the new
  field; the live-socket direct-write path and `appendTerminalWebSocketMessage`
  are untouched (never trim; offset stays 0 unless the pane previously spent
  time on the fallback path, in which case the same offset-aware formula still
  applies correctly there too).
- Verification boundary (mandated, do not weaken): (1) N-pane single-tick
  batching test — exactly one HTTP call per tick, applied correctly to all N
  panes (cursor advance, quiet-pane skip, per-pane error); (2) bounded-trim
  test for `appendTerminalOutput` — bound honored, trims from front not back;
  (3) the load-bearing trim-boundary regression test — drive a pane through
  enough appends to trim at least once, continue appending past that point,
  assert every appended character is written to the emulator exactly once
  (no silent gap, no spurious full-buffer clear+redump from a trim alone);
  (4) adaptive-interval test — backs off above threshold AND recovers below
  it, both directions; (5) existing single-ID route tests keep passing
  unchanged; (6) daemon batch test — mixed valid/unknown/inaccessible IDs,
  valid ones still returned, others silently omitted.
- Out of scope (ticket-stated): WS-drop root cause; reducing how often
  terminals land on the fallback path; the live-socket path's own unbounded
  output growth (`appendTerminalWebSocketMessage`).

## Out of Scope

- Any Phase beyond Phase 1 (none currently defined for this ticket).
- The daemon-side blocking-PTY-write fix (`260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`, already `.done/`).
- Anything about *why* WebSockets drop (Windows/EDR hypothesis) — independent follow-up per Constraints.
- `appendTerminalWebSocketMessage`'s own unbounded growth on the live-socket path.
- The sibling ticket's own scope (`260723-bug-dashboard-terminal-frontend-output-oN-rerender`, already merged, `fa4e9355`) — not re-touched except where this phase must integrate with what it shipped (see findings below).

## Codebase Findings

### Frontend — poll loop (App.tsx), post-merge locations differ from the ticket's stale citations

- `ws-dashboard/frontend/src/App.tsx#L4918-5019` — the poll loop, now at these
  lines (ticket cites stale `4859-4916`). `livePollPanesRef.current`
  (4921-4944) is rebuilt every render, filtered to the active work root AND
  its `resourcePath.serverId` — **all panes in one poll tick already share one
  `serverRoute`**, so one batched POST per tick needs no per-server splitting.
  `poll()` itself (4955-5012) is the per-pane `fetchTerminalOutput` loop to
  replace with one batch call; the `inFlight` `Set<string>` (4952) becomes a
  single in-flight boolean/flag; the per-pane `.then`/`.catch` bodies
  (4966-4989 success, 4991-5006 error) are the per-pane apply/error logic to
  reuse against the batch response.
- `ws-dashboard/frontend/src/App.tsx#L443` — `const terminalOutputPollIntervalMs = 120;` (module-private, not exported). Step 3's adaptive backoff needs to read `livePollPanesRef.current.length` and vary the effective interval; since `window.setInterval` is fixed at creation, this either needs the interval effect to re-derive/reschedule its timer when pane-count crosses the threshold, or an internal higher-frequency tick that self-throttles — an implementation-time choice per the ticket, not fixed here.
- **Risk signal — no App.tsx-level test harness exists** (no `App.test.tsx`; confirmed via `find` across `ws-dashboard/frontend` for `*.test.*`). The ticket's mandated unit tests for (a) exactly-one-batched-request-per-tick and (b) the adaptive-interval back-off/recovery cannot be written against inline `poll()`/interval-effect code in `App.tsx` directly. **Directly relevant precedent from the just-merged sibling ticket** (`fa4e9355` merge commit AI Context): its own fix-cycle 1 finding was "the rAF-lifecycle test originally exercised a hand-written mirror rather than the shipped closures — fixed by extracting the scheduler into terminals.ts and rewriting the test against the shipped factory" (`createOutputCursorFlushScheduler`, `terminals.ts:657-703`). The same shape of problem applies here: the batch-tick orchestration (build cursor list from panes, apply batch response to `Record<string, TerminalPaneState>`, decide the next poll interval from pane count) should be extracted as pure/testable functions in `terminals.ts` (mirroring `flushPendingOutputCursors`'s "takes current state + a batch, returns next state, same-reference no-op contract" shape), with `App.tsx`'s `poll()`/interval-effect reduced to wiring only. This is not scope creep — it is required to satisfy the ticket's own mandated tests given the established codebase pattern, and matches AGENTS.md's Testability standard (pure logic over side effects).

### Frontend — terminals.ts, merged sibling additions and exact reusable shapes

- `ws-dashboard/frontend/src/terminals.ts#L53-69` — `TerminalPaneState` type. Add `outputTrimOffset: number` here.
- `ws-dashboard/frontend/src/terminals.ts#L328-353` — `terminalPaneFromSession`, the **only** production constructor of `TerminalPaneState`; add `outputTrimOffset: 0` alongside the existing `output: ""` initialization (line 341).
- `ws-dashboard/frontend/src/terminals.ts#L539-551` — `appendTerminalOutput` (line numbers happen to be unchanged from the ticket's stale citation despite the merge shifting later functions — confirmed by direct read, not assumed). Currently `pane.output + output.chunks.map(...).join("")` with no bound. This is the front-trim site: apply the character-budget cap here and increment the new `outputTrimOffset` by the trimmed count K.
- `ws-dashboard/frontend/src/terminals.ts#L577-609` — merged `markTerminalOutputCursor` / `flushPendingOutputCursors`: the exact "pure state-transform, same-reference no-op contract" pattern step 2's batch-apply helper and step 1's client fetch/apply should mirror.
- `ws-dashboard/frontend/src/terminals.ts#L657-703` — merged `createOutputCursorFlushScheduler`: precedent for extracting a stateful-but-pure, dependency-injected (fake `requestAnimationFrame`/timers) scheduler factory instead of inlining timer logic in `App.tsx`. Step 3's adaptive interval, if it needs its own scheduling state, should follow this factory shape for the same testability reason noted above.
- `ws-dashboard/frontend/src/terminals.ts#L705-733` — `appendTerminalWebSocketMessage`: confirmed untouched by this ticket; never trims; `pane.output +=` unbounded growth stays out of scope here exactly as the ticket states.
- `ws-dashboard/frontend/src/terminals.ts#L735-742` — `shouldPollTerminalOutput` (ticket cites stale `616-623`; real location shifted because the merge inserted ~140 new lines above it). Reused unchanged as the batch cursor-list filter.
- `ws-dashboard/frontend/src/terminals.ts#L744-769` — `canApplyTerminalOutputPoll` / `terminalOutputPollChangedState`: both pure, both reused unchanged per-pane inside the batch-apply helper.
- `ws-dashboard/frontend/src/terminals.ts#L118-125` — `terminalOutputEndpoint` (single-ID, GET, `?after=`) and `#L71-79` (`resourceModel.ts`) `localCompatibleDashboardApiRoute(serverRoute, segments)` — the batch client endpoint builder should call `localCompatibleDashboardApiRoute(serverRoute, ["terminals", "output", "batch"])` (unscoped: `/api/dashboard/terminals/output/batch`; scoped: `/api/dashboard/servers/{route}/terminals/output/batch`), matching the ticket's chosen paths exactly with no new URL-building code needed.
- `ws-dashboard/frontend/src/terminals.test.ts` — plain top-level-assertion style (`assertEqual`/`assertDeepEqual`, no describe/it), run via `npm run test:terminals`. New `appendTerminalOutput` bound tests and the new batch-apply-helper tests belong here, alongside the existing `appendTerminalOutput` test near line 333.

### Frontend — terminalPaneBody.tsx, delta-write effect and the mount-effect interaction step 4 must also cover

- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L674-687` — the delta-write effect (ticket cites stale `664-679`; shifted ~10 lines by the merged live-socket cursor-increment addition above it at line 593). This is the effect step 4 rewrites: `writtenLengthRef` (declared line 92) → an absolute-position ref, comparisons at 679/682 → the offset-aware formula from the ticket contract.
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L591-593` — the live-socket direct-write path (ticket cites stale `583-585`): `terminal.write(message.chunk.data); writtenLengthRef.current += message.chunk.data.length;`. Per the ticket, this increment must also become offset-aware (`+= data.length` still correct in absolute terms as long as the ref itself is redefined as an absolute position — no trim ever happens here, so this line's arithmetic is unchanged, only the ref's *meaning* changes uniformly with the delta-write effect).
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L142-205` (mount effect, deps `[]` — confirmed via the closing `}, []);` at line 339) — sets `writtenLengthRef.current = 0` at mount (line 167) and `= mountWrite.text.length` on the replay branch (line 204). Both must convert to the absolute scheme: `writtenAbsoluteRef.current = pane.outputTrimOffset` for the restore/none branches, `= pane.outputTrimOffset + pane.output.length` for the replay branch (mountWrite.text === pane.output per `resolveTerminalMountWrite`). In practice `pane.outputTrimOffset` is always `0` at genuine mount time (component only unmounts/remounts on real terminal close/reopen — see the comment at `terminalPaneBody.tsx:76-83` confirming visibility toggles do NOT unmount it — and `terminalPaneFromSession` always seeds a fresh `0`), so this is a defensive-correctness edit, not a live bug path, but must still be made for consistency with the new field's meaning.
- **Established precedent to reuse for step 4's extraction**: `ws-dashboard/frontend/src/workbench/terminalVisualRestore.ts#L138-164` — `resolveTerminalMountWrite`, a pure function extracted specifically so "the branch selection... is unit testable without a React/xterm harness," with side effects (`terminal.write`/`writtenLengthRef`) staying at the call site. The ticket's step-4 regression test needs the same treatment: add a sibling pure function (e.g. `resolveTerminalDeltaWrite(pane: {output, outputTrimOffset}, writtenAbsolute: number): {kind: "noop"|"tail"|"reset", text?, nextWrittenAbsolute, clampedTail?: boolean}`) next to `resolveTerminalMountWrite` in the same file, tested in `workbench/terminalVisualRestore.test.ts` (already the home of `resolveTerminalMountWrite`'s own tests), with `terminalPaneBody.tsx`'s effect reduced to calling it and applying the `terminal.write`/`terminal.clear`/ref-update side effects. This directly enables the ticket's mandated "assert every appended character written exactly once, no gap, no spurious clear" test as a pure-function unit test instead of requiring an xterm-backed component test (which does not exist anywhere in this codebase today).
- `ws-dashboard/frontend/src/terminalPaneBody.tsx#L700-725` — the debounced visual-capture effect (also keyed on `[pane.output]`) reads via `serializeAddon.serialize(...)` (xterm's own buffer), not raw `pane.output` slicing — confirmed unaffected by the trim/offset change, no edit needed here.

### Daemon — Rust-side line numbers are NOT stale (frontend-only sibling merge did not touch these files)

- `ws-dashboard/crates/daemon/src/terminal.rs#L33` — `const MAX_OUTPUT_CHUNKS: usize = 1024;`, confirmed as cited.
- `ws-dashboard/crates/daemon/src/terminal.rs#L655-667` — `terminal_output` handler: `state.terminals.get(&terminal_id)` → 404 if missing → `resolve_online_available_work_root` gate → `session.output_after(query.after)`. The batch handler should loop this exact three-step pattern per cursor, pushing into a `HashMap`/`BTreeMap<String, TerminalOutputView>` and simply `continue`-ing (never erroring) on a missing/inaccessible ID, per the ticket's contract.
- `ws-dashboard/crates/daemon/src/terminal.rs#L895-911` — `TerminalSession::output_after(&self, after: u64) -> TerminalOutputView`, directly reusable per-cursor, no changes needed.
- `ws-dashboard/crates/daemon/src/terminal.rs#L282` — `TerminalRegistry::get(&self, terminal_id: &str) -> Option<Arc<TerminalSession>>`.
- `ws-dashboard/crates/daemon/src/terminal.rs#L478-483`, `#L586-589` — `TerminalOutputView` (Serialize) and `TerminalOutputQuery` (Deserialize, single `after: u64`) structs; the new batch request/response types (`TerminalOutputBatchRequest { cursors: Vec<TerminalOutputCursor> }`, `TerminalOutputBatchResponse { results: HashMap<String, TerminalOutputView> }`, `TerminalOutputCursor { terminal_id: String, after: u64 }`) follow the same `#[serde(rename_all = "camelCase")]` convention next to these.
- `ws-dashboard/crates/daemon/src/router.rs#L358-359` (unscoped) and `#L260-261` (scoped) — existing single-ID route registrations, confirmed as cited; the new routes register as `post(terminal_output_batch)` at `/api/dashboard/terminals/output/batch` and `post(server_scoped_terminal_output_batch)` at `/api/dashboard/servers/{server_route}/terminals/output/batch`.
- `ws-dashboard/crates/daemon/src/servers.rs#L1479-1491` — `server_scoped_terminal_output` (GET, forwards via `ServerScopedForwardOperation::terminal_output`) — reference shape, but the batch route is POST-with-body, so the closer templates are `server_scoped_terminal_input`/`server_scoped_terminal_resize` (`servers.rs#L1493-1525`), which parse the local-alias body via `parse_json_alias_body::<T>(&headers, &body)` (`servers.rs#L2134-2151`) before dispatching to the unscoped handler when `server_route == LOCAL_SERVER_ID`, else forward `headers`/`body` verbatim via `forward_server_scoped_operation`.
- `ws-dashboard/crates/daemon/src/servers.rs#L823-864` — `ServerScopedForwardOperation` constructors (`terminals`, `terminal_output`, `terminal_input`, `terminal_resize`, `terminal_close`); add `terminal_output_batch()` (`Method::POST`, `legacy_path: "/api/dashboard/terminals/output/batch".to_owned()`, `rewrite: ForwardResponseRewrite::None`) alongside these.
- No existing "batch" request/response pattern exists anywhere in the daemon crate (`grep` for `"cursors"`/`batch` in `src/*.rs` returns nothing) — this is genuinely new envelope shape, not an extension of an existing batching mechanism.
- `ws-dashboard/crates/daemon/tests/routes.rs#L146` (`test_terminal_registry()`), `#L1091`/`#L1302` (single-ID output-route tests), `#L2416-2497` (`server_scoped_one_shot_mutation_routes_dispatch_equivalent_local_aliases`, the closest existing template for a POST-with-body server-scoped-vs-local-alias equivalence test) — reuse these fixtures/helpers (`paired_test_app()`, `request_json_for_test(...)`) for the new batch-route tests.

### Spec

- `ai-docs/spec/ws-web-dashboard/index.md#L482-509` — `Remote Terminal HTTP Lifecycle` anchor; the registered-routes list (`#L490-494`) needs the new batch route line added, matching the existing bullet style (`GET .../terminals/{terminalId}/output` etc.).

## Implementation Plan

1. **Daemon types + handler** (`ws-dashboard/crates/daemon/src/terminal.rs`): add `TerminalOutputCursor { terminal_id: String, after: u64 }`, `TerminalOutputBatchRequest { cursors: Vec<TerminalOutputCursor> }`, `TerminalOutputBatchResponse { results: HashMap<String, TerminalOutputView> }` (camelCase serde) near the existing `TerminalOutputQuery`/`TerminalOutputView` (`#L478-589`). Add `pub async fn terminal_output_batch(State(state), Json(request)) -> Response` near `terminal_output` (`#L655-667`): for each cursor, look up via `state.terminals.get`, skip on `None`; skip on `resolve_online_available_work_root` error; else insert `session.output_after(cursor.after)` into the results map keyed by `terminal_id`. Always return `200` with the (possibly partial/empty) map — never a batch-wide error.
2. **Router registration** (`ws-dashboard/crates/daemon/src/router.rs`): add `.route("/api/dashboard/terminals/output/batch", post(terminal_output_batch))` near `#L358-359`, and the scoped mirror near `#L260-261` pointing at a new `server_scoped_terminal_output_batch`.
3. **Server-scoped forwarding** (`ws-dashboard/crates/daemon/src/servers.rs`): add `ServerScopedForwardOperation::terminal_output_batch()` near `#L823-864` (POST, no terminal-id path segment — id is in the body). Add `server_scoped_terminal_output_batch(State, AxumPath(server_route), headers, body)` near `#L1493-1507`'s pattern: on `LOCAL_SERVER_ID`, `parse_json_alias_body::<TerminalOutputBatchRequest>` then call `terminal_output_batch` directly; else `forward_server_scoped_operation(state, server_route, operation, headers, body)`.
4. **Spec update** (`ai-docs/spec/ws-web-dashboard/index.md`): add the batch route's method/path/envelope to the `Remote Terminal HTTP Lifecycle` anchor's registered-routes bullet list (`#L490-494`) and a short "Key properties" note on the omit-don't-error contract.
5. **Frontend pane-state field** (`ws-dashboard/frontend/src/terminals.ts`): add `outputTrimOffset: number` to `TerminalPaneState` (`#L53-69`); initialize to `0` in `terminalPaneFromSession` (`#L328-353`, alongside `output: ""`).
6. **Frontend bounded trim** (`terminals.ts#L539-551`): in `appendTerminalOutput`, after concatenating, if the result exceeds a chosen character budget (size using `MAX_OUTPUT_CHUNKS = 1024` as the daemon-side sizing reference, translated to a frontend char budget — implementation-time constant choice per the ticket), slice off the front K characters and add `outputTrimOffset: pane.outputTrimOffset + K` to the returned pane; trim from the front only, never the back.
7. **Frontend batch client + apply helper** (`terminals.ts`): add `fetchTerminalOutputBatch(cursors: {terminalId, after}[], serverRoute?)` using `localCompatibleDashboardApiRoute(serverRoute, ["terminals", "output", "batch"])`, `POST`, JSON body `{cursors}`. Add a pure `applyTerminalOutputBatch(panes: Record<string, TerminalPaneState>, requests: {logicalKey, terminalId, nextSequence}[], results: Record<string, TerminalOutputView>): Record<string, TerminalPaneState>` mirroring `flushPendingOutputCursors`'s same-reference-if-no-op contract, reusing `canApplyTerminalOutputPoll`/`terminalOutputPollChangedState`/`appendTerminalOutput` per pane exactly as `poll()`'s current `.then` body does.
8. **Frontend poll-tick wiring** (`App.tsx#L4918-5019`): replace the per-pane loop in `poll()` with: build `cursors` from `livePollPanesRef.current` (skip if an in-flight batch is already pending), call `fetchTerminalOutputBatch`, on success `setTerminalPanes((current) => applyTerminalOutputBatch(current, ...))` from step 7, on failure fall back to the existing per-pane error-setting shape (batch-wide network failure marks every requested pane's `error`, matching current per-pane failure semantics as closely as possible). Keep the single in-flight guard.
9. **Adaptive interval (step 3)**: extract the "next interval given current fallback-poll pane count" decision as a small pure function in `terminals.ts` (e.g. `nextTerminalOutputPollIntervalMs(paneCount: number, baseMs: number): number`), and have the `App.tsx` interval effect either re-derive the timer on a pane-count threshold crossing or read the function each tick to self-throttle — concrete mechanism is an implementation-time choice; the function itself must be independently unit-testable (see risk-signal finding above).
10. **Delta-write pure extraction (step 4, load-bearing)**: add `resolveTerminalDeltaWrite` to `workbench/terminalVisualRestore.ts` next to `resolveTerminalMountWrite` (`#L138-164`), implementing exactly the ticket's `currentEnd`/`localStart`/clamp formula, returning enough for the call site to perform `terminal.clear()` conditionally, `terminal.write(text)`, and update the ref. Wire it into `terminalPaneBody.tsx`'s delta-write effect (`#L674-687`), replacing the raw-length `writtenLengthRef` with an absolute-position ref fed by this function's `nextWrittenAbsolute`. Update the mount effect's two `writtenLengthRef` seed sites (`#L167`, `#L204`) and the live-socket increment (`#L591-593`) to the same absolute-ref meaning per the findings above.

## Verification Plan

- `cd ws-dashboard/frontend && npm run test:terminals` — add: (a) `appendTerminalOutput` bound test (repeated over-budget appends stay at/under bound, trims front not back, `outputTrimOffset` advances by trimmed count); (b) `applyTerminalOutputBatch`/batch-client test (N panes → one request, correct per-pane apply including quiet-pane skip and per-pane error); (c) `resolveTerminalDeltaWrite` regression test in `workbench/terminalVisualRestore.test.ts` — drive enough `appendTerminalOutput` calls to trim at least once, continue past it, assert the accumulated writes cover every character exactly once with no gap and no spurious clamp-triggered clear when not actually falling behind; (d) `nextTerminalOutputPollIntervalMs` (or equivalent) back-off-and-recovery test, both directions.
- `cd ws-dashboard/frontend && npm run build` — type-check the new `TerminalPaneState.outputTrimOffset` field and its one production construction site, plus the new `terminals.ts`/`terminalVisualRestore.ts` exports and their `terminalPaneBody.tsx`/`App.tsx` call sites.
- `cd ws-dashboard/crates/daemon && cargo test` (or targeted `cargo test --test routes -- terminal_output_batch`) — new tests: mixed valid/unknown/inaccessible-ID batch request returns only the valid entries, `200`, no whole-batch failure; scoped-vs-unscoped local-alias equivalence (following `server_scoped_one_shot_mutation_routes_dispatch_equivalent_local_aliases` shape); existing single-ID `terminal_output` tests (`tests/routes.rs#L1091`, `#L1302`) continue to pass unchanged.
- Manual/e2e sanity (no App.tsx-level harness exists): open 2+ terminals, force fallback polling (or drive `shouldPollTerminalOutput` conditions), confirm via DevTools Network tab that exactly one `POST .../terminals/output/batch` fires per ~120ms tick regardless of pane count, and that a terminal driven well past the trim bound while polling never visibly loses output in the emulator.

## Escalations

- None.
