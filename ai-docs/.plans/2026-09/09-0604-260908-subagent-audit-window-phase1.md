# Plan: 260908-feat-ws-pi-subagent-audit-window-and-owner-steering — Phase 1: `/audit` picker and read-only viewer

## Relevant Ticket Contract

- Add `agents-plugin-pi/src/audit.ts`: the `/audit` command, a picker modal
  and its shortcut, the session-file parser to `ConversationItem`s, the
  `ConversationChannel` over a registry record (history + live events;
  `liveness()` = `running`/`settled` only this phase), and the `view`-mode
  overlay. Register from `index.ts` on `session_start` for TUI leads only.
- Picker: one row per registry child (running AND dormant), widget naming
  (`alias > title > short uuid`) and three-state ordering (awaiting-owner,
  awaiting-approval, running by elapsed) plus a **fourth, picker-only tier**
  — dormant children by last activity, most recent first (the widget never
  rows a dormant child). `/audit <id-or-alias>` opens one directly. The
  picker is its own focused `ctx.ui.custom` modal (arrow keys move, Enter
  opens, Esc cancels), also bound to a keyboard shortcut. Selection opens the
  viewer as a `ctx.ui.custom` overlay with the ask overlay's geometry
  (`{ overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } }`).
  Never auto-pops; one-overlay-at-a-time (a second `/audit` closes the
  first; the child is unaffected).
- History source: the child's own Pi session file (`record.sessionPath`),
  parsed into `ConversationItem`s. Live tail: the child's RPC event stream
  through the same `ConversationChannel` shape `ask.ts`'s binding already
  uses. **Phase 1 attribution**: every user-side session-file entry renders
  as `lead-message` (no `ownerSends` log exists yet — that is Phase 2).
  Opening a dormant child resumes nothing.
- `view` mode: read-only; follows the stream; Esc closes the viewer directly
  (no modal); reopen shows history so far; nothing in the viewer touches the
  record. Enter in `view` mode raises the overlay to `interactive` via the
  component's own `setMode` (the send path itself is Phase 2 — typing in the
  now-visible editor and submitting must not throw, but there is no
  `channel.send` to actually deliver it).
- Constraints: `agents-plugin-tool/` untouched (Go/ws-mcp is develop-authored
  only); human-only surfaces only (`ctx.ui.custom`, `setWidget`/`setStatus`,
  tool-result `details`) — never `pi.sendMessage` from the viewer; the
  viewer never resumes a dormant child on its own; **lead-only, TUI-only**:
  "nothing is registered in child processes or headless leads
  (`ctx.mode !== "tui"`)".
- Phase 1 test list (ticket): parser maps assistant/tool-call/tool-result and
  user-side entries (all `lead-message`) from a fixture session file; picker
  rows follow widget naming/ordering; the shortcut opens the same picker;
  opening a dormant child resumes nothing; live events append to the tail;
  Esc closes without a modal; Enter in `view` mode switches to `interactive`
  without touching the record; the second `/audit` closes the first overlay.
  Live check (owner-run, isolated tmux socket) is listed as post-build,
  human-only acceptance.

## Out of Scope

- Phase 2 in full: `lastWriter`/`ownerSends` on `RpcAgentRecord`, the
  `idle-awaiting-owner` liveness state, settle-routing-while-owner-held,
  owner-held exemptions (park/fan-in/alias-reuse/cap-eviction), `owner_held`
  on `ws-agent-list`/widget rows, the `[hold]/[finish]/[interrupt]` modal,
  the actual send path (`ConversationChannel.send`), and retiring
  `ThreadRecord.transcript` in favor of the record-based source. None of
  this is touched.
- Spec Impact edits to `pi-adapter-runtime` — the ticket assigns the spec
  amendment to Phase 2 ("Amend the spec per Spec Impact" appears only in
  Phase 2's phase text); Phase 1 makes no spec changes.
- Sharing the binding between `/answer` and `/audit` — the ticket says this
  merge happens "in Phase 2 when the two share one binding." Phase 1's
  active-overlay singleton for `/audit` is therefore independent of
  `ask.ts`'s own `activeOverlay`; opening `/audit` while `/answer` is open
  (or vice versa) is not covered by this phase's test list (only "the second
  `/audit` closes the first overlay" is required) and is left unaddressed.
- The picker-only dormant tier's synthetic pending-thread rows: the ticket
  scopes the picker to "the registry's children," not `ask.ts`'s
  `ThreadRecord`s — unlike the live-agent widget, the picker does not add a
  synthetic row for a pending thread with no respondent yet.
- The owner-run isolated-tmux live check ("`/audit` a worker mid-task, watch
  a tool call appear and expand it, Esc, reopen, and confirm the lead
  transcript received nothing") is POST-BUILD acceptance, owner-run only —
  not part of this build slice or its automated verification.
- Non-`message` / non-chat session-file entry kinds (`model_change`,
  `thinking_level_change`, `compaction`, `branch_summary`, `custom`,
  `custom_message`, `label`, `session_info`) and non-chat `AgentMessage`
  roles (`bashExecution`, `custom`, `branchSummary`, `compactionSummary`):
  the ticket's Phase 1 test list only requires mapping
  assistant/tool-call/tool-result/user-side entries. These are out of the
  ticket's explicit contract; see Escalations for how the plan treats them.

## Codebase Findings

- `agents-plugin-pi/src/conversation-view.ts` (already merged, Phase 1+2 of
  the prerequisite ticket) — exports exactly what this ticket assumes:
  `ConversationViewComponent`, `ConversationViewMode`, `ChildLiveness`
  (`"running"|"idle-awaiting-owner"|"settled"`), `ConversationChannel`
  (`onEvent`, `liveness()`, optional `send`), six-kind `ConversationItem`
  union, raise-only `setMode`. `onEnter` (view-mode Enter, no default
  meaning) and `onEscape` are exactly the two hooks the viewer needs; `send`
  being optional means the Phase-1 channel can omit it entirely (`view`-only
  consumer, per this file's own header comment: "a `'view'`-only consumer
  (e.g. child B's audit window) is not forced to implement it").
- `agents-plugin-pi/src/ask.ts:1512-1636` (`openThread`) — near-identical
  template for the viewer: `ctx.mode !== "tui"` guard, module-level
  `activeOverlay` token singleton closed before a new overlay opens (exactly
  what "second `/audit` closes the first overlay" needs, reimplemented as
  audit.ts's own singleton per Out of Scope), `ctx.ui.custom(factory, {
  overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor:
  "center" } })` — the literal geometry to reuse, `AskCustomUiCtx`'s
  duck-typed `ctx.ui.custom` signature to mirror, and `getMarkdownTheme()`
  from `@earendil-works/pi-coding-agent` wrapped in a best-effort try/catch
  (line ~1576-1581) as the pattern for a parallel `getSelectListTheme()`
  call (see next finding).
- `agents-plugin-pi/src/ask.ts:1101-1143` — `resolveChildLiveness(streaming)`
  (exported) is byte-for-byte the Phase-1 liveness rule this ticket wants
  ("`liveness()` = `running`/`settled` only this phase"); `createForkChannel`
  is the template for the audit `ConversationChannel`: `sync()` attaches to
  `record.client.onEvent` only when a client exists (a dormant record with
  `client === undefined` never attaches — this is what makes "opening a
  dormant child resumes nothing" true for free), re-syncs listeners, detaches
  on last unsubscribe. The audit channel drops the `send` method entirely
  (no `cwd`/`sendToAgent` needed) and reuses `resolveChildLiveness` directly
  by importing it from `ask.ts`.
- `agents-plugin-pi/src/agent-widget.ts:94-97,158-202` — `rowName(record)`
  (`alias ?? title ?? agentId.slice(0,8)`, currently **not exported**) is the
  exact naming rule the ticket asks for; `buildAgentRows`'s row-inclusion
  predicate (`record.threadBound === true || record.pendingApproval !==
  undefined || record.client !== undefined`) and `STATE_RANK`
  (`awaiting-owner` < `awaiting-approval` < `running`, then `elapsedMs`
  descending) are the three-state ordering to reuse verbatim for the
  picker's non-dormant tiers. The complement of that inclusion predicate
  (`client === undefined && !threadBound && pendingApproval === undefined`)
  is exactly "dormant" — the widget's own doc comment says as much ("An
  idle, non-`threadBound` record is auto-parked ... before it would ever
  read this way").
- `agents-plugin-pi/src/spawner.ts:2393-2417` (`evictForCapacity`) — the
  "last activity" formula the picker's dormant tier needs is already
  written, just inlined: `Math.max(record.lastLeadPromptAt ?? 0,
  record.reportLog.at(-1)?.at ?? (record.lastReportAtOverride ?
  Date.parse(record.lastReportAtOverride) : 0))`. Not currently exported as
  its own function.
- `agents-plugin-pi/src/spawner.ts:1098-1104` — `resolveAgentId(registry,
  idOrAlias)` (exported) resolves an alias-or-uuid to the registry key —
  exactly what `/audit <id-or-alias>` needs, no new lookup logic required.
- `agents-plugin-pi/src/pi-tui.ts` — the package's one resolution point for
  `@earendil-works/pi-tui`, per the ticket's "one resolution point" rule
  established by the prerequisite ticket. Currently re-exports `Box, Editor,
  Markdown, ScrollView, Text, TuiAltScreen, TuiMainScreen,
  stripTerminalSequences, truncateToWidth, visibleWidth` plus type-only
  `SelectListTheme` — **it does not re-export `SelectList`/`SelectItem`**,
  which pi-tui's own package does export
  (`node_modules/@earendil-works/pi-tui/dist/components/select-list.d.ts`:
  `class SelectList implements Component` with `onSelect`/`onCancel`,
  `handleInput` already handling up/down/Enter/Escape via
  `getKeybindings().matches(...)`). `SelectList`'s constructor takes only
  `(items, maxVisible, theme, layout?)` — no `tui` reference at all, so
  (like `ScrollView`/`Markdown`/`Text`, per `conversation-view.ts`'s own
  header comment) it constructs and renders under `node --test` against the
  real class with no injectable-fake seam needed.
- `node_modules/@earendil-works/pi-coding-agent/dist/index.d.ts:29` —
  `getSelectListTheme` is exported alongside `getMarkdownTheme`, from the
  same `theme.ts` module `ask.ts` already imports `getMarkdownTheme` from.
  Direct parallel for a best-effort host-theme read in the picker factory.
- `node_modules/@earendil-works/pi-coding-agent/docs/session-format.md` —
  authoritative session-file schema. Header line
  `{"type":"session",...}` (skip). Chat-relevant lines are
  `{"type":"message","message":{"role":...}}` with roles `user` (`content:
  string | (TextContent|ImageContent)[]`), `assistant` (`content:
  (TextContent|ThinkingContent|ToolCall)[]`; `ToolCall = {type:"toolCall",
  id, name, arguments}`), `toolResult` (`{toolCallId, toolName, content:
  (TextContent|ImageContent)[], isError}`). Non-chat entry types
  (`model_change`, `thinking_level_change`, `compaction`, `branch_summary`,
  `custom`, `custom_message`, `label`, `session_info`) and non-chat
  `AgentMessage` roles (`bashExecution`, `custom`, `branchSummary`,
  `compactionSummary`) exist but are not named in the ticket's Phase 1
  mapping requirement or test list.
- `agents-plugin-pi/src/conversation-view.ts` (private helpers,
  `toolResultContentText`/`isSingleTextContent`, ~line 300) — the exact
  single-text-content-or-JSON-stringify rule the parser needs for a
  `toolResult` message's `content` array (identical shape to the live
  `tool_execution_end` event's `result.content` this function already
  converts). Not exported today.
- `agents-plugin-pi/src/index.ts:393-620` — `session_start` handler:
  `registerAsk`/`registerThreadCommands` (line 563-564) are called
  **unconditionally** every `session_start` (worker/explore/fork/lead,
  headless or TUI), gating instead at handler-invoke time inside
  `openThread` (`ctx.mode !== "tui"` → `notify` + return). The live-agent
  widget's arm gate is different and closer to this ticket's ask:
  `shouldArmAgentWidget(role, mode) = isLeadOrFork(role) && mode === "tui"`
  (agent-widget.ts:299-301), called at the index.ts call site (line 556) to
  decide whether to even construct the controller. **This ticket's
  constraint is stricter than `isLeadOrFork`**: "Lead-only, TUI-only:
  nothing is registered in child processes or headless leads" — a `fork`
  process is a child process too, so the gate must be `readSpawnRole(...)
  === undefined` (true lead, no role marker — `process-role.ts`'s own doc:
  "the host lead process carries no marker at all"), not
  `isLeadOrFork`. `pi.registerCommand`/`pi.registerShortcut` themselves must
  not be called outside that gate (not merely internally guarded), per the
  ticket's literal "nothing is registered" wording.
- `agents-plugin-pi/test/native-tool-registration.test.ts:1-38` — the
  fake-`pi`-harness pattern (`registerTool`/`sendMessage`/`sendUserMessage`/
  `on` stubbed on a plain object cast to `ExtensionAPI`) to mirror for a
  fake `pi` exposing `registerCommand`/`registerShortcut` as `Map`-capturing
  stubs, for testing `/audit`'s command/shortcut handlers directly.
- `agents-plugin-pi/test/conversation-view.test.ts:29-45` — `fakeTui()`
  (`requestRender` only) / `fakeChannel()` (mutable `liveness()`, `fire()`
  for events) are the exact fakes the audit-channel and viewer-open tests
  need; no new fake-primitive infrastructure required since `SelectList`
  needs no `tui` and `ConversationViewComponent` already has its own test
  fakes.
- Risk signal (none blocking, but worth flagging): Phase 1's `onEnter` path
  raises the overlay to `interactive` with no `channel.send`. If the owner
  then types and presses Enter, `ConversationViewComponent.handleSubmit`
  appends a local `"user"` item and calls `channel.send?.(trimmed)`, which
  is a silent no-op since `send` is undefined — the owner would see their
  line appear in the transcript with no indication it was never delivered.
  This is the ticket's own explicit design ("the send path itself is Phase
  2"), not a defect to fix here, but the executor should not add any
  additional affordance that makes this look more "sent" than it is (no
  optimistic sent-confirmation copy).

## Implementation Plan

1. `agents-plugin-pi/src/pi-tui.ts` — add `SelectList` to the destructured
   static re-export tuple and `type { SelectItem, SelectListTheme }` (the
   type is already imported/re-exported; add the class + `SelectItem`
   type). No change to `loadHostPiTui()`'s shape needed beyond the wider
   `PiTuiModule` type already covering it (`typeof piTuiStatic`).
2. `agents-plugin-pi/src/spawner.ts` — extract the inline last-activity
   formula out of `evictForCapacity` (~line 2400) into an exported
   `lastActivityAt(record: RpcAgentRecord): number`, and call it from
   `evictForCapacity` in place of the inline expression. Pure refactor, no
   behavior change; existing `evictForCapacity` tests must still pass
   unchanged.
3. `agents-plugin-pi/src/agent-widget.ts` — export `rowName` (used
   unchanged by `buildAgentRows`); extract the row-inclusion/state
   classification (`threadBound → "awaiting-owner"`, else
   `pendingApproval !== undefined → "awaiting-approval"`, else
   `client !== undefined → "running"`, else not included) into a small
   exported pure helper, e.g. `classifyRegistryRowState(record):
   AgentRowState | undefined` (`undefined` = not included by the widget,
   i.e. dormant), and refactor `buildAgentRows` to call it. Existing
   `agent-widget.test.ts` assertions on rows/sorting must be unaffected
   (same output, same inputs).
4. `agents-plugin-pi/src/conversation-view.ts` — export
   `toolResultContentText` (rename not required) so the session-file parser
   converts a `toolResult` message's content array with the identical rule
   the live event path already uses.
5. `agents-plugin-pi/src/ask.ts` — export nothing new here;
   `resolveChildLiveness` is already exported and importable as-is.
6. Add `agents-plugin-pi/src/audit.ts`:
   - `parseSessionFile(path: string): ConversationItem[]` — reads the file
     (best-effort: a missing/unreadable file yields `[]`, never throws, per
     the codebase's established best-effort-read convention, e.g.
     `fork.ts`'s `tailLines(readFileSync(...))` catch), splits lines,
     `JSON.parse`s each (skip a malformed line rather than aborting the
     whole parse — mirrors `spawner.ts:357`'s per-line `JSON.parse` and
     `agent-sidecar.ts`'s "a corrupt entry must not poison the rest"
     convention). For each `{type:"message", message}`: `role:"user"` →
     `{kind:"lead-message", text: <joined text content>}` (Phase 1: always
     `lead-message`, never `user` — no `ownerSends` log exists yet);
     `role:"assistant"` → one `{kind:"assistant", text}` item per `text`
     content block (thinking blocks dropped) plus one
     `{kind:"tool-call", id, name, args: arguments}` item per `toolCall`
     block, in original order; `role:"toolResult"` →
     `{kind:"tool-result", id: toolCallId, name: toolName, content:
     toolResultContentText(message), isError}` using the imported helper
     from step 4. Every other `type`/`role` (session header,
     `model_change`, `thinking_level_change`, `compaction`,
     `branch_summary`, `custom`, `custom_message`, `label`,
     `session_info`, and `bashExecution`/`custom`/`branchSummary`/
     `compactionSummary` message roles) is skipped — not part of the
     ticket's Phase 1 mapping contract (see Escalations for confidence on
     this).
   - `createAuditChannel(rpcRegistry, agentId): ConversationChannel` —
     `createForkChannel`'s `sync()`/`onEvent` shape minus `send` (`send` is
     omitted entirely, matching the interface's optional field), `liveness`
     delegating to `resolveChildLiveness(record?.streaming === true)`
     imported from `ask.ts`.
   - `buildAuditPickerItems(registry: RpcAgentRegistry, now: number):
     SelectItem[]` — for each record, `classifyRegistryRowState(record)`
     (step 3); when defined, use it as the state (tiers 1-3, `rowName` +
     `runStartedAt`-based elapsed for sort, matching `buildAgentRows`);
     when `undefined` (dormant), tier 4, sorted by `lastActivityAt(record)`
     descending, most-recent-first. Concatenate in tier order into
     `SelectItem[]` (`value: agentId`, `label: "<name> · <state-or-dormant-label>"`
     mirroring `agent-widget.ts`'s `formatRow` style, `description`
     optional).
   - `registerAuditCommands(pi, ctx-scoped deps, rpcRegistry, role, mode)`
     (exact signature per what `index.ts`'s call site can supply) — internal
     gate `role === undefined && mode === "tui"` (exported as a small pure
     predicate, e.g. `shouldRegisterAudit(role, mode)`, mirroring
     `shouldArmAgentWidget`'s existing precedent) returns early with no
     `pi.registerCommand`/`pi.registerShortcut` calls when false. When true:
     registers `pi.registerCommand("audit", { description, handler(args,
     ctx) })` — with an id/alias arg, resolve via `resolveAgentId` and open
     the viewer directly; with no arg, open the picker — and
     `pi.registerShortcut(<key TBD, e.g. "ctrl+shift+u">, { description,
     handler(ctx) })` opening the same picker. Both route through one
     `openPicker`/`openViewer` pair.
   - `openPicker` — `ctx.ui.custom` factory constructing a `SelectList`
     (via `loadHostPiTui()`'s `SelectList`, matching the runtime-instance
     convention `ask.ts`/`conversation-view.ts` already follow for host
     render primitives) over `buildAuditPickerItems(...)`, theme from a
     best-effort `getSelectListTheme()` call (step's parallel to
     `getMarkdownTheme()`); `onSelect` resolves `done(item.value)` then the
     caller opens the viewer for that `agentId`; `onCancel` resolves
     `done(undefined)`.
   - `openViewer(pi, ctx, rpcRegistry, agentId, sessionCtx)` — mirrors
     `openThread`'s shape: `ctx.mode !== "tui"` guard (defensive; should be
     unreachable given the registration gate, but matches the established
     per-call defensive style); a module-level `activeAuditOverlay` token
     singleton (own to `audit.ts`, per Out of Scope) closed before opening a
     new one; builds `initialItems` via `parseSessionFile(record.sessionPath)`;
     constructs `ConversationViewComponent` in `"view"` mode with
     `channel: createAuditChannel(...)`, `onEscape: () => done(undefined)`
     (no modal, per Phase 1's "Esc closes the viewer directly"), `onEnter:
     () => component.setMode("interactive")` (no `onDone`/`send` wiring
     beyond what the component already no-ops); `ctx.ui.custom(factory, {
     overlay: true, overlayOptions: { width: "80%", maxHeight: "80%",
     anchor: "center" } })` (ask.ts's exact geometry).
7. `agents-plugin-pi/src/index.ts` — import `registerAuditCommands` (or
   `shouldRegisterAudit` + the register function, per how step 6 shapes the
   export) and call it once per `session_start`, near the
   `registerAsk`/`registerThreadCommands` call site (~line 563-564), passing
   `readSpawnRole(process.env)` and `ctx.mode` for the internal gate.

## Verification Plan

- `cd agents-plugin-pi && npm test` (full suite) — zero new failures beyond
  the existing baseline (prior phase results record ~130 pre-existing
  `WS_PI_SPAWN_ROLE`/fork-prefix environment failures; the executor should
  confirm the failing-test-name set is unchanged, not just the count).
- `cd agents-plugin-pi && env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE node --test test/audit.test.ts test/agent-widget.test.ts test/ask.test.ts test/conversation-view.test.ts test/spawner.test.ts` —
  targeted run covering the new file plus every module this plan edits
  (`agent-widget.ts`'s extracted classifier, `spawner.ts`'s
  `lastActivityAt`, `conversation-view.ts`'s newly-exported helper,
  `ask.ts`'s reused `resolveChildLiveness`).
- New `test/audit.test.ts` covers the ticket's Phase 1 test list directly:
  parser mapping (assistant/tool-call/tool-result/user-side-as-lead-message)
  from a fixture JSONL built with `mkdtempSync`/`writeFileSync` (pattern:
  `test/native-tool-registration.test.ts`'s fixture-file setup); picker row
  naming/ordering against a hand-built `RpcAgentRegistry` `Map` fixture
  (running, awaiting-approval, threadBound, and dormant records, following
  `test/agent-widget.test.ts`'s duck-typed fake-record convention);
  `createAuditChannel`'s liveness/no-resume/event-append behavior via
  `fakeChannel`-style fakes (`test/conversation-view.test.ts`'s pattern) —
  no `client.onEvent` attach for a dormant record proves "resumes nothing";
  Esc-closes-without-modal and Enter-raises-to-interactive-without-touching-the-record
  via a constructed `ConversationViewComponent` + fake `tui`
  (`fakeTui()`/`fakeChannel()` reused directly); the second-`/audit`-closes-
  the-first-overlay singleton behavior via a fake `ctx.ui.custom` that
  records open/close calls (pattern: `native-tool-registration.test.ts`'s
  fake-`pi` harness, extended with a `ui.custom` stub); `shouldRegisterAudit`
  gate truth table (`undefined`/`"worker"`/`"fork"`/`"explore"` ×
  `"tui"`/other mode).
- Manual-only / owner-run (not part of this build's automated verification,
  per ticket): the isolated-tmux live check — `/audit` a worker mid-task,
  watch a tool call appear and expand it, Esc, reopen, confirm the lead
  transcript received nothing from the viewer. Record as pending owner-run
  acceptance, same convention the prerequisite ticket's own `## Blocked`
  section already uses.

## Escalations

- None. Confidence: high — every referenced type/export
  (`ConversationViewComponent`, `ConversationChannel`, `resolveChildLiveness`,
  `resolveAgentId`, `rowName`, `evictForCapacity`'s formula,
  `getSelectListTheme`, `SelectList`) was read directly from source, and the
  session-file schema comes from the vendored package's own
  `docs/session-format.md`, not inference.
- One implementation judgment call worth surfacing (not a scope reduction —
  the ticket's own test list does not require it, so this is filling an
  unspecified gap conservatively rather than dropping a specified
  requirement): non-chat session-file entries and non-chat `AgentMessage`
  roles (`bashExecution`, `compaction`, `branch_summary`, etc.) are skipped
  by the parser rather than mapped to any `ConversationItem` kind. If the
  lead wants `bashExecution` surfaced (it is arguably tool-call-shaped), the
  executor can add a `tool-call`/`tool-result` synthetic pair for it without
  changing this plan's other steps.
