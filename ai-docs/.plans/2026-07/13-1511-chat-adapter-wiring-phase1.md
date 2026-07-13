# Plan: 260713-feat-ws-dashboard-agent-chat-real-adapter-wiring — Phase 1: Real fetch client, swap stub call sites

## Relevant Ticket Contract

- Write a real fetch client for the REST-nested paths the existing backend routes actually use (`codex_routes.rs`/`claude_routes.rs`), covering create/prompt/control/transcript operations.
- Swap the stub call sites in `App.tsx` to call the real client behind matching function signatures — the seam `activitySessionStub.ts` documents ("shapes stay conformant to `activitySessionApi.ts` so a later real handler can replace call sites here without reshaping callers").
- Harness selection must route Codex/Claude sessions to the real client and leave any other harness (OpenCode, or anything without a real adapter) on the stub — do not regress stub-backed flows for unadapted harnesses.
- For the fork request shape, read `260713-feat-ws-dashboard-activity-session-fork-cursor` first; build the real client's fork request against its guidance rather than reinventing it, and use this work to help close that idea ticket's Phase 1.
- Verification: typecheck/build passes; existing frontend unit tests pass against the new client; a targeted test confirms each swapped call site issues the correct REST-nested request shape for a Codex and a Claude session.
- Non-goal (per Goal section / later phases): real streaming/polling delivery is Phase 2; new `CodexControlRequest` variants (incl. a live fork control action) are Phase 3; Claude fork-from-here stays out of scope entirely (Hack-tier).

## Out of Scope

- Phase 2 streaming/polling implementation (only needs a call site that Phase 2 can upgrade).
- Phase 3 `CodexControlRequest` fork variant and any new daemon route for fork (no backend fork route/handler exists yet at all — not even the enum variant).
- Any Claude `/control` route or `ClaudeControlRequest` type — none exists today, and no Phase 1 call site needs it (see finding below).
- OpenCode wiring (stays on stub per ticket).
- Rewind/resume-from-here (already scaffolded-disabled, untouched here).

## Codebase Findings

- `ws-dashboard/frontend/src/activitySessionStub.ts:1-19` — header states the seam contract explicitly: shapes track `activitySessionApi.ts` so callers don't need reshaping when a real handler lands.
- `ws-dashboard/frontend/src/activitySessionStub.ts:239,339,362,414,432,491` — the 6 stub functions imported by `App.tsx`: `stubBeginStreamingTurn`, `stubStartNewAgentChatSession`, `stubResumeAgentChatSession`, `stubSteerActivitySession`, `stubForkActivitySession`, `stubActivityHistoryList`.
- `ws-dashboard/frontend/src/App.tsx:4035,4943,4978,5065,7024,7084,7147` — actual call sites (7 lines, not the ticket-text's stated 12; several stub functions are called from more than one place, e.g. `stubBeginStreamingTurn` at both 7024 and 7084). Each call site's containing function: `onLoadHistory` (~4034), `startAgentChatHarness` (~4925), `resumeAgentChatHistoryItem` (~4964), `forkAgentChatFromBubble` (~5039), a `useEffect` demo ticker (~7019), `beginSimulatedTurn` (~7074), and the send-message steer branch (~7106-7154).
- **Risk signal**: `sendAgentChatMessage` (App.tsx:5021) currently calls only the local `appendUserTranscriptBlock` — there is no existing stub call site for "send/prompt" at all. Phase 1 must *add* a new real-client call here (POST `.../prompt`), not merely swap an existing one; the ticket's "swap 6 stub call sites" framing does not cover this gap on its own.
- `ws-dashboard/frontend/src/activitySessionApi.ts:144-152` — current `ActivitySessionForkRequest` is `{ workRootId, activityId, serverRoute? }` and `ActivitySessionForkResponse` is `{ activityId }`; no cursor/cut-point field exists yet.
- `ai-docs/tickets/idea/260713-feat-ws-dashboard-activity-session-fork-cursor.md:34-45` — **does not commit a literal field name/type** for the cursor. It defers to the stub's `cutBlocks: readonly TranscriptBlock[]` parameter (`activitySessionStub.ts:421-431`) as the reference shape and leaves reconciliation ("adopt the same shape, or thread the stub's parameter into the now-real request type") as an open decision for this ticket to make.
- **Risk signal**: no backend fork route or `CodexControlRequest::Fork` variant exists anywhere (`codex_routes.rs:88-93` enum has only `Compact`/`Steer`/`Skills`; no `/fork` path in `router.rs`). Adding that variant is explicitly Phase 3 scope. So a Phase 1 fork call site can define the frontend request shape and call a client function, but it will 404 against the real daemon until Phase 3 lands — consistent with Phase 1's verification bar being "correct request shape," not "succeeds end-to-end."
- `ws-dashboard/crates/daemon/src/codex_routes.rs:104-232`, `router.rs:387-422` — Codex REST paths: `.../codex-sessions` (list/create), `.../codex-sessions/{activityId}/transcript` (GET), `/prompt` (POST), `/interrupt` (POST), `/control` (POST, dispatches `CodexControlRequest`). Server-scoped variants exist at `router.rs:186-219` under `/servers/{serverRoute}/...`, forwarding via `server_scoped_codex_*` wrappers.
- `ws-dashboard/crates/daemon/src/claude_routes.rs:94-182` — Claude REST paths mirror Codex for list/create/transcript/prompt/interrupt, but **there is no `/control` route and no `ClaudeControlRequest` enum** (only an unused `ClaudeControlResponse` struct). This is not a blocker for Phase 1: `stubSteerActivitySession`'s capability table (`activitySessionStub.test.ts`) only enables `steer` for Codex, not Claude, so no Phase 1 call site needs a Claude control endpoint.
- `ws-dashboard/frontend/src/gitToolbar.ts:64-160` — reusable fetch-client pattern to follow: a `*Base(workRootId, serverRoute)` URL helper on `localCompatibleDashboardApiRoute`, a shared `readJson<T>(response, fallback)` helper that throws `Error(body?.error ?? "HTTP <status>: <fallback>")` on non-ok responses, one exported async function per REST operation. Same idiom repeated in `gitWorktreeAdd.ts`, `linkedServers.ts`, `openWorkRoot.ts`, `rootPicker.ts`, `workRootActivity.ts:427,443`, `terminals.ts:187-285`.
- `ws-dashboard/frontend/src/agentChatSessions.test.ts` and `activitySessionStub.test.ts` — both use a no-framework, plain top-level `await` + custom `assertEqual`/`assert` (throw-based) style; a new real-client test should mirror this, not introduce a test-runner DSL.
- `ai-docs/mental-model/ws-dashboard-agent-harness.md:17-47` — confirms tiering: Codex `thread/fork` is Passthrough (real, MVP-eligible); Claude's only fork/rewind path is Hack-tier (transcript truncation) and out of scope here; Codex `thread/rollback` (rewind) is deprecated/coarse — do not build new functionality around it.

## Implementation Plan

1. Add a new module `ws-dashboard/frontend/src/activitySessionClient.ts` mirroring `gitToolbar.ts`'s pattern: a shared `readJson<T>` helper plus one exported async function per operation, targeting the REST-nested paths from `router.rs:186-219,387-422` (both non-server-scoped and `/servers/{serverRoute}/...` forms), parameterized by harness (`codex-sessions` vs `claude-sessions`).
2. Implement client functions with signatures matching the 6 stub functions plus a new prompt/send function:
   - `startNewAgentChatSession` (POST create) — real counterpart to `stubStartNewAgentChatSession`.
   - `resumeAgentChatSession` (GET transcript / hydrate) — counterpart to `stubResumeAgentChatSession`.
   - `activityHistoryList` (GET list, per-harness) — counterpart to `stubActivityHistoryList`.
   - `steerActivitySession` (POST control, `{action: "steer", text}`) — counterpart to `stubSteerActivitySession`; Codex-only per capability gating already in place.
   - `forkActivitySession` — extend `ActivitySessionForkRequest` (activitySessionApi.ts:144-148) with a cursor field derived from the stub's `cutBlocks` reference shape (e.g. last-kept transcript block identifier); wire the client call against the future `/control` `Fork` action even though the backend variant doesn't exist until Phase 3 (call will 404 until then — acceptable per Phase 1's verification bar).
   - `sendAgentChatPrompt` (POST prompt) — new call, added into `sendAgentChatMessage` (App.tsx:5021), since no stub call site exists there today.
   - `beginRealStreamingTurn` — a minimal real counterpart to `stubBeginStreamingTurn` that sends the prompt and performs a single transcript fetch to update state (no polling loop yet; Phase 2 upgrades this to continuous diff-polling).
3. In `App.tsx`, branch each of the 7 call sites (4035, 4943, 4978, 5065, 7024, 7084, 7147) plus the new prompt call in `sendAgentChatMessage` on harness type: call the new real client functions for `codex`/`claude` sessions, keep calling the existing stub functions for any other harness (e.g. OpenCode).
4. Update `activitySessionApi.ts` to add the cursor field to `ActivitySessionForkRequest`/`Response` per step 2; leave other types unchanged (out of scope).
5. Add a new test file `ws-dashboard/frontend/src/activitySessionClient.test.ts` mirroring the plain-assert style of `agentChatSessions.test.ts`/`activitySessionStub.test.ts`: mock/stub `fetch`, assert each client function issues the correct method/URL/body for a Codex and a Claude session (server-scoped and non-server-scoped `serverRoute` variants, per `workRootActivity.test.ts:1365-1422`'s existing `serverRoute`-threading pattern).

## Verification Plan

- `npm run typecheck` / project build in `ws-dashboard/frontend` passes.
- Existing tests `agentChatSessions.test.ts` and `activitySessionStub.test.ts` continue to pass unmodified (stub path for non-adapted harnesses untouched).
- New `activitySessionClient.test.ts` passes, covering per-operation REST-nested request shape for both Codex and Claude sessions.
- Manual/automated confirmation is deferred to Phase 4 (real end-to-end walkthrough); Phase 1 verification is request-shape-level only, consistent with the ticket's own phase split.

## Escalations

- None.
