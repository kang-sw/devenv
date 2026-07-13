# Plan: 260711-feat-ws-dashboard-agent-activity-chat-ui — Phase 2: Streaming conversation rendering

## Relevant Ticket Contract

- Phase 2 scope (ticket lines 241-250): "Implement the messenger-style bubble
  layout, per-line streaming markdown rendering, collapsible thinking blocks,
  per-turn/per-tool-use bubble separation with summary/expand, and the
  copy-button affordance on every bubble."
- Verification boundary: "frontend component tests for streaming markdown
  rendering and collapsible-block default state; browser-level acceptance
  evidence for a live streamed turn rendering incrementally."
- Decisions (owner, 2026-07-11) that govern this phase specifically:
  - "standard messenger layout — user messages right-aligned, agent messages
    left-aligned, markdown rendering with real-time per-line streaming
    formatting (Obsidian-flavored markdown as the target dialect)."
  - "Where a harness exposes extractable thinking/reasoning content, render
    it as a collapsible block (default collapsed) interleaved between the
    surrounding agent content."
  - "One chat bubble per agent turn; each tool invocation is its own separate
    bubble. Bubbles show a summary by default with click-to-expand for
    detail; the exact summarization depth is implemented at a reasonable
    common-sense default for the first pass and left as an explicit TBD for
    refinement, not a blocking design question."
  - "Every chat bubble (user, agent-turn, and tool-use) has a copy button."
- Constraint: per-harness-gated affordances must reflect the Cross-Harness
  Feature Matrix — not applicable to plain rendering, only to
  resume/fork/queue affordances explicitly deferred to Phase 3.
- No real backend exists yet (`260620`/`260624` still not landed); Phase 1's
  in-memory `activitySessionStub.ts` is the only data source. Ticket allows
  building/testing Phase 2 against the stub.

## Out of Scope

- "resume from here" / "fork from here" buttons, mid-turn submission
  queuing, prompt-box history traversal, pending/queued badge — all Phase 3.
- Cross-Harness Feature Matrix gating logic — Phase 3.
- `serverId` threading — Phase 4.
- Real `activity.session.*` adapters (`260620`) and real cross-harness
  history (`260624`) — out of scope; keep building against the stub.
- Resume popover entry-size display, hardcoded `"0"` cursor cleanup — Phase 1
  forward-noted Minor findings, not required by Phase 2 unless the new bubble
  work touches the same lines incidentally.

## Codebase Findings

- `ws-dashboard/frontend/src/App.tsx#L6859-L6884` — current minimal
  `AgentChatPaneBody` rendering: a flat `.map` over `session.transcript.blocks`
  into `agent-chat-transcript-block` divs (title + raw text, no markdown, no
  bubbles, no role/alignment, no collapse, no copy button). This is the
  section Phase 2 replaces.
- `ws-dashboard/frontend/src/workRootActivity.ts#L80-L95` — `TranscriptBlock`
  type: `{ cursor, timestamp, renderKind, title, text, data, degraded }`. No
  `role` (user/agent), no turn id, no tool-invocation grouping field exists
  today. `renderKind` already has a `"thinking"` variant (added Phase 1
  specifically anticipating this phase, per the inline CONTRACT comment at
  L75-79) but nothing else needed for messenger alignment or turn/tool
  bubble separation. **Risk signal**: this type is described as mirroring the
  real daemon projection API (`260620`), so widening it is technically a
  shared-contract surface, not purely local UI state. Given the `thinking`
  precedent was added the same way for this exact ticket, treat this as the
  established pattern: add narrowly-scoped, additive, optional fields (not a
  breaking reshape) so `260620`'s real adapter can populate them later
  without forcing a redesign. Do not block Phase 2 on `260620` for this.
- `ws-dashboard/frontend/src/activitySessionStub.ts#L56-L128` — stub only
  ever emits single-shot, fully-formed blocks (one block per call). No
  existing incremental/streaming emission exists anywhere in the frontend
  (`grep stream` across `src/*.ts(x)` turns up unrelated hits only). Phase 2
  must add stub-side incremental emission (e.g. a block that grows its
  `text` over several ticks, or a small chunk-append helper) to make
  streaming behavior testable/demoable without a live daemon.
- `ws-dashboard/frontend/src/documentViewer.tsx#L1-L120,#L434-L595` —
  existing Obsidian-flavored markdown pipeline: `unified().use(remarkParse).
  use(remarkGfm)` parser, `deriveMarkdownDocumentModel(markdown, {path})` ->
  blocks/renderBlocks, `groupedMarkdownRenderUnits`, and node-render helpers
  (`renderBlockNode`, `renderNode`, `renderTranslatedMarkdown`). This is the
  dialect the ticket wants ("Obsidian-flavored markdown as the target
  dialect") and should be reused rather than reinventing a second markdown
  renderer. `renderNode` is currently module-private (not exported); the
  exported surface (`deriveMarkdownDocumentModel`, `groupedMarkdownRenderUnits`)
  is sufficient for block-splitting, but a chat bubble needs a lighter
  render entry point than the full `DocumentViewer` component, which carries
  block-selection rails and a translation-overlay toolbar that don't apply to
  a chat bubble.
- `ws-dashboard/frontend/src/workRootActivity.ts#L822-L905` — existing
  `transcriptBlockView` / `transcriptCompactSummary` / `transcriptToolCallSummary`
  / `transcriptToolResultSummary` (all exported except the last three helper
  functions feeding the exported `transcriptBlockView`) already implement a
  reasonable-default compact-summary heuristic for tool-call/tool-result
  blocks, keyed off `renderKind`/`title`/`data`. This directly matches the
  ticket's "reasonable common-sense default" ask for tool-use bubble
  summaries — reuse `transcriptBlockView`'s summary/tone output rather than
  writing new summarization logic.
- `ws-dashboard/frontend/src/ActivityConsole.tsx#L640-L678` — existing
  `ActivityTranscriptBlock` component: a compact/expand pattern (`expanded`
  boolean prop, `Set<string>` of expanded cursors lifted by the caller,
  "More"/"Less" toggle button) for the read-only Activity console. Same
  interaction shape the ticket wants for collapsible thinking blocks and
  tool-use bubble expand, but this component renders plain `<pre>` text (no
  markdown, no messenger alignment, no copy button) and is scoped to the
  read-only projection tab — do not import/reuse the component directly, but
  mirror its state-management pattern (locally-owned `Set<string>` of
  expanded ids, default collapsed) in a new chat-specific component.
- `ws-dashboard/frontend/src/documentViewer.tsx#L449-L451` — existing copy
  pattern: `navigator.clipboard?.writeText(text)`, wired to plain buttons.
  Reuse this exact call for the per-bubble copy button (no new clipboard
  abstraction needed).
- `ws-dashboard/frontend/src/agentChatSessions.ts#L30-L38` —
  `AgentChatSessionView.transcript: ActivityTranscript` is the only session
  shape; no separate "chat message" model exists. Phase 2 will likely add
  a derived bubble-grouping function (transcript blocks -> bubbles) rather
  than changing `AgentChatSessionView` itself.
- `ws-dashboard/frontend/src/documentViewer.test.ts#L1-L70` and
  `package.json` `test:document-viewer` / `test:agent-chat-tabs` scripts —
  established component-test convention: plain TS test files using
  `react-dom/server`'s `renderToStaticMarkup` plus small local `assert`/
  `assertEqual` helpers, compiled via `tsconfig.route-tests.json` and run as
  a `node ./node_modules/.tmp/route-tests/<name>.test.js` npm script. No
  jsdom/RTL in this repo — follow this exact pattern for new bubble/markdown/
  collapsible-block tests rather than introducing a new test framework.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2533-L2589` —
  Phase 1's Playwright step for the agent-chat tab, ending right after the
  Codex tile click renders the stub transcript. Phase 2's browser-level
  acceptance evidence should extend this same `test.step` block (or add an
  adjacent one) to assert incremental rendering of a streamed turn, reusing
  the existing `pane`/`transcript` locators.
- `ws-dashboard/frontend/src/styles.css#L3793-L3798` — current
  `.agent-chat-transcript-block` / `-title` styles to be replaced/extended
  with bubble-layout CSS (alignment, thinking-block collapse, copy-button
  affordance).

## Implementation Plan

1. **Extend the shared transcript block shape additively** in
   `ws-dashboard/frontend/src/workRootActivity.ts` (near `TranscriptBlock`,
   `#L87-L95`): add optional fields needed for bubble grouping — e.g.
   `role?: "user" | "agent" | "tool" | string` and `turnId?: string | null`.
   Keep `renderKind` as the existing tool/thinking signal (already present).
   Follow the same "open string union, additive, tolerant" style as the
   existing `thinking` addition (see the CONTRACT comment at `#L75-79`) so
   `260620`'s real adapter can populate these later without a reshape.

2. **Add a bubble-grouping helper**, likely in a new
   `ws-dashboard/frontend/src/agentChatBubbles.ts(x)` module: a pure function
   `groupTranscriptIntoBubbles(blocks: TranscriptBlock[]): ChatBubble[]` that
   partitions the flat block list into per-turn agent bubbles, per-tool-use
   bubbles (one bubble per tool invocation, matching `260620`'s
   fixture-verified tool-call/tool-result block boundaries), user bubbles,
   and thinking blocks interleaved/attached to their surrounding agent
   bubble. Reuse `transcriptBlockView`/`transcriptCompactSummary` from
   `workRootActivity.ts#L822-905` for each bubble's default summary/tone
   rather than writing new heuristics.

3. **Add a lightweight markdown render entry point** in
   `ws-dashboard/frontend/src/documentViewer.tsx`: export a small function
   (e.g. `renderMarkdownFragment(markdown: string): ReactNode`) built on the
   existing `deriveMarkdownDocumentModel`/`renderBlockNode` pipeline (module-
   private `renderNode` may need exporting or wrapping) so chat bubbles reuse
   the same Obsidian-flavored `unified`/`remark-parse`/`remark-gfm` parser
   instead of a second implementation. Do not reuse the full `DocumentViewer`
   component (it carries block-selection rails/translation toolbar that
   don't apply here).

4. **Build the bubble UI components** (new file, e.g.
   `agentChatBubbles.tsx`): `UserBubble`, `AgentTurnBubble`,
   `ToolUseBubble`, `ThinkingBlock` (default collapsed, mirroring the
   `expanded`/`Set<string>`-of-ids pattern from
   `ActivityConsole.tsx#L640-678`'s `ActivityTranscriptBlock`, but rendering
   markdown via step 3 and adding the copy button from step 5). Right-align
   user bubbles, left-align agent/tool/thinking content, per the ticket's
   messenger-layout decision.

5. **Add the copy-button affordance** to every bubble type, reusing the
   existing `navigator.clipboard?.writeText(text)` call already used in
   `documentViewer.tsx#L449-451` — no new clipboard abstraction.

6. **Wire the new components into `App.tsx`**, replacing the flat block
   `.map` at `#L6869-6882` (`AgentChatPaneBody`'s active-session branch) with
   the bubble-grouping helper (step 2) feeding the new bubble components
   (step 4). Preserve the existing `data-testid="agent-chat-transcript"`
   container so Phase 1's e2e assertions keep passing; add new
   `data-testid`/`data-*` attributes as needed for bubble-level test hooks
   (e.g. `data-agent-chat-bubble-kind`).

7. **Add stub-side incremental/streaming emission** in
   `ws-dashboard/frontend/src/activitySessionStub.ts`: extend the synthetic
   session so at least one agent-turn block's `text` grows over several
   ticks after `stubStartActivitySession`/`stubStartNewAgentChatSession`
   resolves (e.g. a small interval-driven chunk-append helper local to the
   stub, or a returned subscribe/update callback the pane wires into local
   state), so both the new component tests and the Playwright acceptance
   step can observe genuinely incremental rendering without a live daemon.
   Keep this additive and clearly stub-scoped per the file's existing
   CONTRACT header (`#L1-19`) — do not let it leak into `260620`/`260624`
   scope.

8. **Update `ws-dashboard/frontend/src/styles.css`** (`#L3793-3798` area):
   replace/extend `.agent-chat-transcript-block` styles with bubble-layout
   CSS — left/right alignment, collapsed/expanded thinking-block styling,
   copy-button placement — following the visual conventions already used by
   `.activity-transcript-block` in the same file for tone/mode classes.

## Verification Plan

- New component test file (e.g.
  `ws-dashboard/frontend/src/agentChatBubbles.test.ts`), following the
  `documentViewer.test.ts` convention (`renderToStaticMarkup` + local
  `assert`/`assertEqual` helpers, no jsdom/RTL): assert (a) streaming
  markdown renders correctly at intermediate and final text lengths (feed
  partial/growing markdown strings through the bubble-grouping + render
  pipeline and check the resulting static HTML), and (b) a thinking block's
  default rendered state is collapsed (detail not present/hidden in the
  static markup) until toggled. Register a new npm script (e.g.
  `test:agent-chat-bubbles`) matching the existing
  `test:document-viewer`/`test:agent-chat-tabs` pattern in `package.json`.
- Extend `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts`'s existing
  `260711 Phase 1` `test.step` (`#L2533-2589`) or add an adjacent Phase-2
  step: after the Codex tile click, assert the transcript's rendered content
  changes incrementally over time (e.g. poll/wait for a growing text length
  or a sequence of DOM mutations) to provide the "live streamed turn
  rendering incrementally" browser-level acceptance evidence, using the stub
  streaming emission added in Implementation step 7.
- Run: `npm run test:document-viewer`, the new bubble test script, and
  `npm run test:agent-chat-tabs` (regression on Phase 1's pane-state tests)
  before `npm run test:browser` for the full acceptance pass.

## Escalations

- None.
