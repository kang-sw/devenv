# Plan: 260620-feat-ws-dashboard-agent-client-activity-sources — Phase 1: Agent-client provider and Activity source contract

## Relevant Ticket Contract

- Define the dashboard-owned ACP-shaped `AgentClientProvider` subset and the
  Activity source projection that consumes it; update the ws web dashboard
  spec and mental model so the intended source split is recoverable: legacy
  ws-mercenary/named-agent is a compatibility source, Codex app-server and
  OpenCode ACP are interactive provider sources, OpenCode serve is an
  optional observation source, browser sees only the dashboard Activity
  model.
- Frontend interaction-API draft (illustrative names, not a final route
  contract) to record as the Phase-1 method-shape decision: common subset
  `activity.history.list`, `activity.session.start/create/send`,
  `activity.session.usage` (read-only usage display); per-harness-gated
  `activity.session.compact/steer/goal.set|get|clear/rewind/fork/skills`
  (hidden/disabled unless the active harness's adapter reports the
  capability — Codex-native for all of these today per the Cross-Harness
  Feature Matrix).
- New `thinking` `TranscriptBlock.renderKind` value for extractable
  reasoning/thinking content (Claude `assistant` stream + Codex reasoning
  item stream), separate from `assistant` text blocks.
- `activityId` may gain a new dashboard-owned source prefix; provider thread
  ids, turn ids, session ids, raw event ids, process ids, cache/transcript
  paths stay daemon-private, never browser identity.
- `ActivitySourceDisplay.kind`, item `kind`, transcript `renderKind` may gain
  new string values; parsers/tests must keep unknown-value tolerance.
- New Codex/OpenCode activity must flow through `ActivityFeed.items`, never
  forced into the legacy `agents` compatibility projection.
- Event vocabulary should not grow until a concrete Codex/OpenCode behavior
  cannot be represented by the existing `ActivityConsoleEvent` variants
  (item upsert/removal, transcript updated, snapshot invalidated, mode
  changed, heartbeat).
- New write routes back `activity.session.create/start/send` and the
  per-harness-gated methods, under the existing
  `/api/dashboard/servers/{serverRoute}/...`-scoped, `workRootId`/
  `activityId` identity model — same Server Route pattern every other
  dashboard resource already follows, not a new identity/routing scheme.
- Verification boundary (Phase 1 only): documentation and type-level
  tests/fixtures are enough; no provider process needs to run. Route and
  TypeScript contract tests must assert mixed-source rows keep using the
  existing Activity routes, and that browser payloads tolerate unknown
  future source/provider kinds without treating them as named agents.
- Ticket-wide constraint: no direct harness/runtime development (no new
  model loop, edit engine, permission runtime, or ws MCP agent runtime);
  Hack-tier capabilities (Cross-Harness Feature Matrix) stay out of these
  normal phases entirely.

## Out of Scope

- Phase 2 (Codex app-server adapter spawn/JSON-RPC wiring), Phase 3
  (OpenCode ACP adapter), Phase 4 (Claude CLI stream-json duplex adapter),
  Phase 5 (Activity UI/UX lift) — implementing any adapter, its
  spawn/subprocess lifecycle, or live event parsing is not this phase's
  work; Phase 1 only defines the contract shapes they will conform to.
- ws/wsflow plugin-presence spawn precondition enforcement — this is
  per-adapter runtime behavior owned by Phases 2-4; Phase 1 has no spawn
  path to gate.
- The interactive chat UI/UX (tab entry points, conversation view,
  resume/fork affordances, composer, mid-turn queuing) — split into
  `260711-feat-ws-dashboard-agent-activity-chat-ui`.
- Calling any harness process or capturing live fixture event sequences —
  out of scope per this phase's stated verification boundary.
- Deciding whether the dashboard should maintain its own skill/capability
  layer instead of relaying `activity.session.skills` passthrough as-is —
  `260711-feat-ws-dashboard-agent-activity-chat-ui` raises this as an open,
  not-yet-decided question; Phase 1 only records the method shape and its
  per-harness gating, not the resolution.
- Modeling subagents as nested Activity/session rows — deliberately
  deferred pending an OpenCode fixture check; no bespoke type for this now.

## Codebase Findings

- `ws-dashboard/crates/core/src/activity.rs#L8-L104` — `ActivityFeed`,
  `ActivityItem`, `ActivitySourceDisplay`, `ActivityTranscript`,
  `TranscriptBlock`, `ActivityConsoleEvent` are the existing source-neutral
  read-model types (re-exported via `crates/core/src/lib.rs#L8-L10`).
  Critically, `kind` (`ActivityItem`, `ActivitySourceDisplay`) and
  `render_kind` (`TranscriptBlock`) are plain `String` fields, not closed
  enums — adding new source/render kinds (e.g. `agent.codex`, `thinking`)
  is additive, doc-comment-only, no serde/schema change needed.
- `ws-dashboard/frontend/src/workRootActivity.ts#L28-L67` — hand-mirrored
  (not generated/codegen'd) TypeScript types for the same shapes. Unions
  already use `"namedAgent" | "exec" | string` / `"markdown" | "text" |
  "json" | string` — the open-string-union pattern already satisfies
  "tolerate unknown future source/provider kinds"; Phase 1 adds new known
  literals alongside the existing `| string` fallback and must keep
  Rust/TS hand-synchronized (no codegen exists — confirmed by the mental
  model's own coupling note that field changes require updating Rust
  serde tests, fixtures, and TS types "together").
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L624,644-651` — renders by
  passing `source.kind` through as an opaque `sourceKind: string`, not a
  switch/enum — the render path already degrades unknown kinds gracefully.
- No existing Rust provider/adapter trait anywhere in `ws-dashboard/`: a
  repo-wide grep for `AgentClientProvider`, `agent-client`, `OpenCode ACP`,
  `app-server` returned zero hits. The current named-agent/wsstate
  compatibility source lives inline in
  `ws-dashboard/crates/daemon/src/work_root_activity.rs` (2943 lines) and
  `work_root_activity_registry.rs` (189 lines) as concrete structs
  (`WorkRootActivityProjector`, `WorkRootActivityProjectionConfig`), not
  behind a pluggable trait — Phase 1's `AgentClientProvider` is genuinely
  new vocabulary, not an extension of something that exists.
- `ws-dashboard/crates/daemon/src/terminal.rs#L138-L327` — closest existing
  per-backend adapter *shape* precedent: a registry struct
  (`TerminalRegistry`) plus request/response view structs
  (`CreateTerminalRequest`, `TerminalSessionView`, etc.) plus route
  handlers — not a trait-object abstraction. Useful module-layout
  precedent for Phase 2-4's later adapter modules; Phase 1 itself needs no
  registry (no live process yet), only the shared request/response
  vocabulary and inert type/trait signatures sufficient for type-level
  tests.
- `ws-dashboard/crates/daemon/src/router.rs#L44-L55,159-168,325-334` — the
  three existing read routes are dual-registered: server-scoped under
  `/api/dashboard/servers/{server_route}/work-roots/{work_root_id}/activity...`
  and again as the bare local-gateway compatibility alias
  `/api/dashboard/work-roots/{work_root_id}/activity...`. Any Phase-1
  method-shape draft for the new write methods should stay compatible with
  this same dual-registration pattern, which Phase 2+ will actually wire.
- `ai-docs/spec/ws-web-dashboard/index.md` — single consolidated spec file;
  the ticket's `spec:` frontmatter entries
  (`260521-ws-dashboard-activity-console-read-model`,
  `260521-ws-dashboard-activity-console-ui-shell`) are anchor ids inside
  this one file, not separate files. Relevant sections:
  - `## Activity Console Read Model {#260521-ws-dashboard-activity-console-read-model}`
    at `#L821-L911`, containing an existing
    `> [!note] Implementation Gap · 2026-06-20` callout at `#L840-L849`
    that already states almost verbatim what Phase 1 must formalize
    ("the dashboard does not yet expose dashboard-owned Activity source
    adapters for host-owned agent-client surfaces such as Codex app-server
    or OpenCode ACP... OpenCode serve may later supplement observation/
    discovery, but it is not the primary interactive provider
    counterpart"). This note should be superseded/expanded in place, not
    duplicated elsewhere.
  - `### SQLite-Backed Named-Agent Compatibility Source {#260525-ws-dashboard-sqlite-agent-activity-source}`
    at `#L877-L911` — the existing compatibility-source subsection whose
    H3-nested-under-H2 pattern should be mirrored for the new source
    split (e.g. a subsection enumerating Codex/OpenCode/OpenCode-serve).
  - `## Activity Console Transcript Expansion {#260522-ws-dashboard-activity-console-transcript-expansion}`
    (not fully quoted above but present in the file) already states Claude
    and Gemini native transcript parsing stay unsupported until
    fixture-backed formats exist — this is a still-accurate statement for
    Phase 1 (Claude fixture work is Phase 4's job) but the doc edit should
    make clear Phase 1 only adds the `thinking` render-kind vocabulary, not
    a Claude parser.
  - `## 🚧 Server Route Identity And Scoped Operation Endpoints {#260703-ws-dashboard-server-route-scoped-operation-endpoints}`
    at line 245, with `### Remote Activity, Git, And Workspace Operations {#remote-activity-git-workspace-operations}`
    at line 388 explicitly listing the current registered Activity
    server-scoped routes (`.../activity`, `.../activity/items/{activityId}/transcript`,
    `.../activity/events`) and the `server-local` in-process /
    forward-over-bearer-helper dispatch rule — the exact pattern any
    Phase-1-drafted new write-route names must stay consistent with.
- `ai-docs/mental-model/ws-dashboard-agent-harness.md` (141 lines) —
  already exists and already documents the Passthrough/Overlay/Hack/
  Unavailable tiering and a frontend-interaction-API coupling bullet list
  (`## Coupling`, `#L117-L127`). This file is largely ahead of the spec
  already; Phase 1's mental-model work is smaller than the ticket text
  implies — mainly cross-referencing the new spec anchor, naming the
  concrete `AgentClientProvider` module once created, and upgrading the
  `## Coupling` method-name bullet list into a coupling reference table
  (common subset vs. per-harness-gated).
- `ai-docs/mental-model/ws-web-dashboard.md#L114-L118` — explicit hand-off:
  interactive agent-harness provider/session work is deliberately kept out
  of this file and lives in `ws-dashboard-agent-harness` instead. Line 80
  already says "Future Codex app-server, OpenCode ACP, and managed CLI
  sources must add source adapters feeding `items`/transcripts rather than
  treating SQLite/wsstate named-agent records as provider authority" —
  Phase 1 doc edits should respect the existing file split rather than
  duplicating tiering language here.
- Test conventions to follow:
  - Rust: `#[cfg(test)] mod tests` colocated in `activity.rs#L188-L394`
    (serde round-trip + forbidden-substring privacy assertions), plus
    broader route/integration tests in
    `ws-dashboard/crates/daemon/tests/routes.rs` (13,785 lines;
    Activity-specific cases currently around lines 767-873, 1088-1102,
    1257-1267, 2872-2874, 3630-3784).
  - TypeScript: no vitest/jest — plain Node-assert test files compiled via
    `tsconfig.route-tests.json` and run through a dedicated
    `npm run test:<name>` script in `frontend/package.json`
    (`"test:work-root-activity": "tsc -p tsconfig.route-tests.json &&
    node ./node_modules/.tmp/route-tests/workRootActivity.test.js"`, line
    17). A new Phase-1 TypeScript contract-test file must be added to
    `tsconfig.route-tests.json`'s include list and either extend the
    existing `test:work-root-activity` script or get its own `test:*`
    entry, or it will silently never run.

## Implementation Plan

1. **Draft the `AgentClientProvider` contract as Rust types**, new module
   `ws-dashboard/crates/core/src/agent_client_provider.rs` (new file,
   following the one-file-per-concern convention `activity.rs` already
   uses), re-exported from `crates/core/src/lib.rs` alongside the existing
   `activity` re-export. Define inert, doc-comment-heavy method signatures
   or a non-runtime trait covering the ticket's common interactive subset:
   initialization/capabilities, session list/create/resume, prompt/send,
   assistant/user message shapes, tool activity, permission/blocked
   states, interruption/cancellation, file-change summaries, transcript
   backfill, provider metadata. Keep Codex/OpenCode-specific fields out;
   only promote a field into the shared contract when a browser-facing
   need already exists per the ticket.
2. **Add a capability-flag type** (e.g.
   `AgentClientCapabilities { compact: bool, steer: bool, goal: bool, fork: bool, rewind: bool, skills: bool }`)
   in the same module, so later adapters (Phase 2-4) can report which of
   `compact/steer/goal/rewind/fork/skills` they support and a future
   frontend can hide/disable per-harness-gated controls. No rewind/fork
   logic — capability-flag shape and method-signature stubs only.
3. **Extend documented `kind`/`render_kind` values** in
   `ws-dashboard/crates/core/src/activity.rs` doc comments (no enum change
   needed — already `String`): add `agent.codex`, `agent.opencode`,
   `agent.claude` (or the ticket's illustrative discriminators) beside the
   existing `namedAgent`/`exec` source kinds, and add `thinking` beside
   `markdown`/`text`/`json` for `TranscriptBlock.render_kind`. Mirror the
   same additions into
   `ws-dashboard/frontend/src/workRootActivity.ts#L28-L35,59-67` union
   literals (before the `| string` fallback) to keep Rust/TS
   hand-synchronized.
4. **Draft the Phase-1 route/method-shape vocabulary as inert TypeScript
   types**, either appended to `workRootActivity.ts` or a new sibling file
   (e.g. `activitySessionApi.ts`) for `activity.history.list`,
   `activity.session.start/create/send/usage`, and the per-harness-gated
   `activity.session.compact/steer/goal.*/rewind/fork/skills` —
   request/response shape only, no fetch/handler implementation (no route
   is registered this phase). Doc-comment that Phase 2+ will register
   these under the dual server-scoped/local-gateway pattern from
   `router.rs#L159-L168,325-334` and the
   `#remote-activity-git-workspace-operations` spec section.
5. **Update the spec** `ai-docs/spec/ws-web-dashboard/index.md`: expand or
   supersede the `> [!note] Implementation Gap · 2026-06-20` callout at
   `#L840-L849` with prose describing the new source split (legacy
   ws-mercenary compatibility source / Codex app-server interactive source
   / OpenCode ACP interactive source / OpenCode-serve observation-only
   source / dashboard Activity model as the only browser-visible shape),
   and add a new H3 subsection mirroring `### SQLite-Backed Named-Agent
   Compatibility Source` (`#L877`) that names the `AgentClientProvider`
   contract module and its relationship to `items`/`agents`. Record the
   new `thinking` render kind and new source `kind` strings in the same
   edit, using a new stable anchor id following the `260620-...` ticket
   numbering convention already used throughout the file.
6. **Update the mental model**
   `ai-docs/mental-model/ws-dashboard-agent-harness.md`: refresh `##
   Coupling` (`#L117-L127`) to name the concrete `agent_client_provider.rs`
   module path once created, and convert the current prose bullet list of
   frontend interaction-API method names into a small reference table
   (common subset vs. per-harness-gated), cross-referencing the new spec
   anchor from step 5.
7. **Add contract tests**:
   - Rust: extend `activity.rs` `mod tests` with a case serializing an
     `ActivityItem`/`ActivitySourceDisplay` using a new `kind` (e.g.
     `"agent.codex"`) and a `TranscriptBlock` with `render_kind:
     "thinking"`, asserting values round-trip and the existing
     forbidden-substring assertions (`#L312-L336`) still hold — proves new
     kinds are additive, no schema break.
   - Rust: add a small serde round-trip test for the new
     `agent_client_provider.rs` types (capability flags, method
     request/response shapes) so the module has at least type-level
     coverage even with no runtime behavior.
   - TypeScript: extend `workRootActivity.test.ts` (or add a test file for
     the new sibling types file, added to `tsconfig.route-tests.json` and
     given a `test:*` script entry) asserting a value using a new source
     `kind` literal still type-checks against the `| string`-tolerant
     union, and that existing route helpers
     (`workRootActivityEndpoint`, etc.) are unchanged — proving mixed-source
     rows keep using the existing Activity routes without a parallel
     identity scheme.

## Verification Plan

- `cd ws-dashboard && cargo test -p ws-dashboard-core` (confirm the exact
  crate name via `crates/core/Cargo.toml` before running) to exercise the
  new/updated `activity.rs` and `agent_client_provider.rs` serialization
  tests.
- `cd ws-dashboard/frontend && npm run test:work-root-activity` (extend
  this script, or add a new `test:*` entry if a new sibling types file is
  created per step 4) to run the updated/added TypeScript contract test.
- Manual check: re-read the edited
  `ai-docs/spec/ws-web-dashboard/index.md` section and
  `ai-docs/mental-model/ws-dashboard-agent-harness.md` to confirm the
  Implementation Gap note is resolved/superseded rather than duplicated,
  and confirm no route registration or handler code was added anywhere
  (`router.rs`, `servers.rs`) — this phase is documentation/type-only; any
  new route wiring would indicate scope creep into Phase 2+.
- No provider process needs to run this phase (ticket's own verification
  boundary); do not attempt a live Codex/OpenCode/Claude smoke test here.

## Escalations

- None.
