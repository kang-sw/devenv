# Plan: 260908-feat-ws-pi-conversation-view-component — Phase 2: migrate the ask overlay

## Relevant Ticket Contract

- Rebind `ask.ts` to `ConversationViewComponent`: `createForkChannel` becomes a
  `ConversationChannel` (`liveness()` = `streaming ? "running" : "settled"`),
  `openOverlayChat` is replaced by opening the component in `"interactive"`
  mode with `onEscape` = close view and `onDone` = `closeThreadOnDone`,
  `summarizeOnDone` semantics preserved.
- Persisted thread transcripts move to `ConversationItem[]`; a record holding
  the legacy `TranscriptEntry[]` shape is converted on hydrate (`you`→`user`,
  `thread`→`assistant`, `note`→`note`).
- Child tool calls/results seen on the fork's event stream become
  `tool-call`/`tool-result` items.
- Delete `overlay-chat.ts` and migrate its tests.
- Amend `ai-docs/spec/pi-adapter-runtime.md` per the ticket's Spec Impact
  section (the "Overlay chat" bullet under
  `{#260905-pi-side-thread-owner-question-surface}`; the
  `{#260905-pi-live-agent-widget}` import-source line is already accurate —
  confirm, do not assume a rewrite is needed there).
- Constraints: golden rule (`agents-plugin-tool/` untouched, all work in
  `agents-plugin-pi/`); behaviour-preserving for `/answer` (260904 runbook
  item 1, the four 260905 activity-indicator checks — both owner-run, NOT
  agent-doable); no change to `spawner.ts` record semantics
  (`streaming`/`threadBound`/`overlayAttached`).
- Verification (ticket text): `cd agents-plugin-pi && npm test` green with
  migrated overlay cases; `overlay-chat.ts` gone, no remaining importer; the
  two owner-run live checks packaged as a one-shot runbook at closeout, plus a
  live check that a tool call/result on an answered fork renders as a
  collapsed item and expands.

## Out of Scope

- Phase 1's own contract (message model, modes, key-precedence, pi-tui
  dependency, `pi-tui.ts` resolution) — already landed (`124598df`..`c45c3a0d`),
  not re-planned.
- Child B (audit window, owner-steering ownership rule, Esc modal) — a later
  sibling ticket that builds on this component; nothing here anticipates it
  beyond leaving the component's public surface reusable.
- `260906-feat-ws-pi-tool-result-yaml-tui-rendering` Phase 2 — open, does not
  block this ticket (its Phase 1 preview helpers are reused as-is).
- `Editor`'s own chrome (`IDENTITY_EDITOR_THEME`, border coloring) — the
  ticket's rendering requirements only name `assistant` items (host Markdown
  theme) and `user` items (host background); the input box's own theming is
  not required and is left untouched.
- Multi-content-block tool results (image content, mixed content arrays) —
  render via the same narrow "single text content" precedent
  `tool-result-render.ts` already uses; anything else falls back to a plain
  stringified rendering, not a new multi-modal renderer.
- The two owner-run live checks named in Constraints — packaged as a runbook,
  not executed here.

## Codebase Findings

- `agents-plugin-pi/src/ask.ts#L80` — the only non-comment import of
  `overlay-chat.ts`: `openOverlayChat`, `ForkChannel`, `OverlayHandle`,
  `TranscriptEntry`. `test/overlay-chat.test.ts#L40` and
  `test/ask.test.ts#L78` (`import type { OverlayHandle } from
  "../src/overlay-chat.ts"`) are the only other importers. `src/index.ts`,
  `src/text-width.ts`, `src/pi-tui.ts`, `src/agent-widget.ts`,
  `src/conversation-view.ts` only mention the filename in prose comments —
  confirmed via `grep -rl "overlay-chat"` — nothing else to repoint.
- `agents-plugin-pi/src/agent-widget.ts#L38` — already imports `visibleWidth`
  from `./text-width.ts` (Phase 1 already migrated this importer); nothing to
  do here.
- `agents-plugin-pi/src/conversation-view.ts#L300-L392` — `handleEvent` only
  handles `message_update`/`agent_start`/`agent_settled`. It does **not**
  handle `tool_execution_start`/`tool_execution_end` at all — the
  `tool-call`/`tool-result` item kinds exist in the type (`#L137-L143`) but
  nothing yet appends one. Confirmed via `pi-coding-agent`'s
  `core/extensions/types.d.ts#L607-L625`: `ToolExecutionStartEvent
  {type,toolCallId,toolName,args}` / `ToolExecutionEndEvent
  {type,toolCallId,toolName,result,isError}`, and these ARE forwarded over the
  RPC wire (`JsonAgentSessionEvent = Exclude<AgentEvent,{message_update}> |
  …` in `modes/json-event.d.ts`); `spawner.ts#L2238,L2273` already
  pattern-matches `tool_execution_start` off the same `client.onEvent` stream
  `ForkChannel`/`ConversationChannel` reads. This wiring is Phase 2's job per
  the ticket text, not a Phase 1 gap.
- `agents-plugin-pi/src/conversation-view.ts#L229-L250` (`ConversationViewOptions`)
  and `#L185-L190` (`ConversationViewPrimitives`) — no theme plumbing at all:
  `markdownLines()` (`#L589-L591`) hardcodes `IDENTITY_MARKDOWN_THEME`, and
  `renderItem`'s `"user"` case (`#L557-L558`) calls plain `textLines` with no
  background. The ticket's Decisions require "assistant items = Markdown with
  the host theme" and "user lines keep the host user-message background as
  today" — neither is wired. This is additive, not a redesign:
  `node_modules/@earendil-works/pi-tui/dist/components/text.d.ts` shows `Text`
  already takes a 4th `customBgFn?: (text:string)=>string` constructor arg
  (applied at the padding stage, matching `overlay-chat.ts`'s own
  after-padding `bg()` discipline), and `Markdown`'s constructor already takes
  a `theme: MarkdownTheme` 4th arg — `ConversationViewPrimitives.Markdown`'s
  type already includes it. `pi-coding-agent`'s `getMarkdownTheme(): MarkdownTheme`
  (no args) and `Theme.bg(color: ThemeBg, text): string` /
  `Theme.fg(color: ThemeColor, text): string` (`modes/interactive/theme/theme.d.ts#L9-L26`,
  `"userMessageBg"` is a valid `ThemeBg`) are the exact values needed.
  `getMarkdownTheme`/`RpcClient`/`parseFrontmatter` are already **static**
  (non-type) imports elsewhere (`spawner.ts#L90`, `lead-skills.ts#L80`) — no
  guarded dynamic import is needed for this, unlike the old
  `overlay-chat.ts#L738-L755` `loadMarkdownRenderer` (that guard existed for a
  different reason: pi-tui's own resolvability, already solved by Phase 1's
  `pi-tui.ts`).
- `agents-plugin-pi/src/conversation-view.ts#L1-L92` header — explicitly says
  "`primitives` is therefore an injectable option (default: `./pi-tui.ts`'s
  static classes) … **live wiring (Phase 2 / child B) can later feed
  host-resolved classes through the very same seam without this component's
  own code changing**". This is a direct instruction: the live factory in
  `ask.ts` must call `pi-tui.ts`'s `loadHostPiTui()` and pass its
  `{ScrollView,Markdown,Text,Editor}` as `primitives`, not rely on
  `DEFAULT_PRIMITIVES` (the package's own static-import copy) — this is the
  gotcha-note's "resolve through the host at runtime" rule applied to the
  render primitives, not just the `TUI` identity check.
- `agents-plugin-pi/src/conversation-view.ts#L342-L345` (`appendItem`) — the
  single internal write path (every `"user"`/`"assistant"` append in
  `handleEvent`/`handleSubmit` goes through it), but there is no callback
  fired on append — no `onTranscriptChange`/`onItemsChange` equivalent to
  `overlay-chat.ts`'s own `append()` (`#L329-L332`, "the only transcript write
  path… reported to `onTranscriptChange`"). Without one, `ask.ts` cannot
  persist the transcript as it grows (needed for the Esc/reopen and
  lead-restart persistence constraint) — this is a required additive option,
  not optional polish.
- `agents-plugin-pi/src/conversation-view.ts#L300-L321` (constructor) — no
  auto-seeding of a `note` item from a `question`, unlike
  `overlay-chat.ts#L318-L324` (`else if (options.question) { this.append({who:
  "note", text: options.question}); }`). `ask.ts` must build `initialItems`
  itself (existing transcript, else a single seeded `note` from
  `thread.question` when present, else empty) before constructing the
  component.
- `agents-plugin-pi/src/conversation-view.ts#L229-L250`,`#L450-L463` — `onDone`
  is a **zero-arg** `() => void`, fired **synchronously** the instant `/done`
  is submitted (`handleSubmit`), with no "ask the fork for a summary and wait
  for its settle" state machine at all — unlike `overlay-chat.ts`'s
  `donePending`/`finish(summary)` machinery (`#L290,#L379-L392,#L417-L448`).
  The ticket's "onDone = closeThreadOnDone, summarizeOnDone semantics
  preserved" cannot be a literal direct function reference — `closeThreadOnDone`
  needs a `summary: string` (`ask.ts#L875-L887`). **The summarize-then-close
  state machine must be re-implemented in `ask.ts`**, ported from
  `overlay-chat.ts`'s exact logic before it is deleted: on `/done`, if
  `summarizeOnDone` (`thread.origin === "lead-ask"`), append a `note` ("ending
  the thread — asking for a summary…"), send `buildDoneSummaryPrompt()` via
  the channel, subscribe once more to `channel.onEvent` to accumulate the next
  `text_delta`s and fire on the next `agent_settled` (settled text, or
  `EMPTY_SUMMARY_TEXT` if empty), unsubscribe, then call `closeThreadOnDone`
  and resolve the `ctx.ui.custom` `done(undefined)`. The component's own
  internal subscription will independently append that same settled turn as
  an `"assistant"` item — expected and harmless (two independent listeners on
  one channel), matches the old behavior where the settled text landed in the
  transcript before the close decision was made. `EMPTY_SUMMARY_TEXT` and
  `buildDoneSummaryPrompt()` (`overlay-chat.ts#L190-L195`) must move into
  `ask.ts` before deletion.
- `agents-plugin-pi/src/overlay-chat.ts#L714-L720` (`OverlayHandle`) — shape
  (`close()`, `closeWithSummary(summary)`) is unchanged by the ticket and is
  reused by `ask.ts#L1018-L1025` (`activeOverlay`), `#L1055-L1087`
  (`handleRespondentFinalReport`), and `test/ask.test.ts#L1107-L1153`'s fake.
  Move the interface verbatim into `ask.ts` and export it there (no shape
  change); build one from the `ctx.ui.custom` `done` callback + the
  summarize-then-close helper from the previous finding, so
  `handleRespondentFinalReport`'s `overlay.closeWithSummary(message)` path
  keeps working unchanged.
- `agents-plugin-pi/src/ask.ts#L971-L1008` (`createForkChannel`) — only
  `isStreaming()` needs to become `liveness()`; `onEvent`/`send` are unchanged.
  Not exported/unit-tested today (live-glue only, matches the file's own
  "genuinely live glue … left to the plan's tmux/owner-runbook gates" header
  claim, `#L57-L59`). Pull the ternary out as a small exported pure helper
  (`resolveChildLiveness(streaming): ChildLiveness`), matching the file's own
  precedent (`resolveOwnerSendInterrupt`, `#L578-L580`) — cheap, testable,
  consistent with the "Testability" code standard.
- `agents-plugin-pi/src/ask.ts#L146-L236,L311-L339` (`ThreadRecord.transcript`,
  `normalizeTranscript`, `parseThreadRegistry`) — `transcript` type changes to
  `ConversationItem[] | undefined`. `normalizeTranscript` must accept BOTH
  shapes per entry: legacy `{who,text}` (`who: "you"→{kind:"user",text}`,
  `"thread"→{kind:"assistant",text}`, `"note"→{kind:"note",text}`) and native
  `{kind,...}` (validate per-kind required fields: `tool-call` needs
  `id,name,args`; `tool-result` needs `id,name,content`, optional `isError`;
  the rest need `text: string`). Cap to newest `THREAD_TRANSCRIPT_CAP`
  unchanged. `test/ask.test.ts#L199-L231` (existing round-trip/malformed/cap
  tests) assert the OLD shape and must be rewritten for the new one, plus a
  new legacy-hydration case.
- `agents-plugin-pi/src/ask.ts#L1238-L1314` (`openThread`) — the whole
  `openOverlayChat` call and its `renderMarkdown`/`initialEntries`/
  `onTranscriptChange`/`onOpened` options are replaced by a `ctx.ui.custom`
  factory building the component directly (see Implementation Plan). Header
  text: the ticket's "the header keeps `title · opened <time>` and one hint
  line" is NOT a second rendering slot on the component — `ConversationViewOptions.headerHint`
  is a single string rendered once (`hintText()`, `#L526-L531`) through
  `Text`, and `Text`'s wrapping (`pi-tui/dist/utils.js#L757-L772`,
  `wrapTextWithAnsi`) already splits on `\n` before wrapping each line. So
  `ask.ts` composes ONE multi-line `headerHint` string —
  `` `ws thread ${threadId} · ${title}\n  opened ${formatSpawnTime(...)}\nEsc: close view (thread stays open) · /done: end thread` `` —
  ported from `overlay-chat.ts#L601-L616`'s existing header-building lines
  (move `formatSpawnTime`, `#L178-L183`, into `ask.ts` too).
- `agents-plugin-pi/ai-docs/spec/pi-adapter-runtime.md#L1259-L1270` (inside
  `{#260905-pi-side-thread-owner-question-surface}`, "Overlay chat" bullet) —
  the only spec text needing a rewrite (`ForkChannel.isStreaming()` →
  `ConversationChannel.liveness()`, plus the ticket's Spec Impact wording:
  shared component, tool calls/results visible, scrolling transcript with no
  24-line tail cut, Ctrl+C swallowed). Confirmed via
  `grep -n "overlay-chat|ForkChannel|TranscriptEntry|isStreaming"` over the
  whole spec file — this is the only hit. The
  `{#260905-pi-live-agent-widget}` section never names an import source for
  `visibleWidth` today, so the ticket's Spec Impact line for it is already
  satisfied — no edit needed there (double-check on contact, do not silently
  skip if a later read disagrees).
- `test/overlay-chat.test.ts` (67 cases, line-numbered `describe` blocks
  listed via `grep -n "^describe|test(\""`) vs `test/conversation-view.test.ts`
  (44 cases, Phase 1) — categorized:
  - **Already superseded, drop, no migration**: `visibleWidth`/`wrapLine`
    (obsolete hand-rolled helpers), the rounded-box `render(width)` tests,
    `working marker` describe block, `isEscapeKey` describe block, basic owner
    input routing (submit/backspace/paste-as-one-send), "closing without
    /done" (Escape) — all already re-covered by `conversation-view.test.ts`'s
    existing 44 cases (width-bounded rendering, headerHint-once, `working…`
    banner semantics, `isEscapeKey` kitty/modifyOtherKeys cases, paste-as-one-send,
    Esc→`onEscape`).
  - **Obsolete, drop**: `loadMarkdownRenderer resolves gracefully`,
    `openOverlayChat falls back … hands back an OverlayHandle` — both test the
    deleted guarded-dynamic-import glue; no longer applicable now that
    `getMarkdownTheme`/pi-tui are static/host-resolved.
  - **Migrate to `test/conversation-view.test.ts`** (need the new options):
    `owner lines (dogfood: visually distinct from thread text)` describe block
    (`#L701-L771`, 4 cases — bg-block padding, long-turn wrap, fg-only/no-theme
    fallback, ANSI-safe width) → cases for the new `userLineBg` option;
    `markdown rendering for thread text` describe block (`#L772-L830` minus
    the two obsolete cases above) → cases for the new `markdownTheme` option;
    the "every append is reported… in order, streaming tail never is" /
    "reported array is a copy" cases from `transcript persistence`
    (`#L657-L700`) → cases for the new `onItemsChange` option; a new case for
    `tool_execution_start`/`tool_execution_end` → `tool-call`/`tool-result`
    item appends (not present in either file today — net-new, mirrors the
    existing `agent_start`/`agent_settled` event tests at `#L739-L757`).
  - **Migrate to `test/ask.test.ts`** (behavior now lives in `ask.ts`): the
    entire `/done (the single fixed round-trip)` describe block (`#L497-L600`,
    summarizeOnDone true/false, `EMPTY_SUMMARY_TEXT` placeholder, M11
    half-streamed-leak guard, at-most-once, fixed prompt text) → tests against
    the new summarize-then-close helper; `closeWithSummary (the fork ended the
    thread itself)` describe block (`#L881-L910`) → already exercised in
    spirit by `test/ask.test.ts#L1107-L1153`'s `handleRespondentFinalReport`
    fake-`OverlayHandle` tests, extend rather than duplicate; the
    question-seeding half of `transcript persistence` ("restored transcript …
    question note NOT seeded twice", "empty initial transcript … question
    seeded") → tests on `openThread`'s new `initialItems`-building logic (pure
    enough to extract and test directly, e.g. as an exported
    `buildInitialConversationItems(thread)` helper).
  - `test/ask.test.ts#L78` import path (`OverlayHandle`) moves from
    `../src/overlay-chat.ts` to `../src/ask.ts`.

## Implementation Plan

1. `agents-plugin-pi/src/conversation-view.ts` — additive options only, no
   behavior change to existing consumers:
   - Widen `ConversationViewPrimitives.Text` (`#L188`) to accept the optional
     4th `customBgFn?: (text: string) => string` constructor arg pi-tui's real
     `Text` already supports.
   - Add `markdownTheme?: MarkdownTheme` and `userLineBg?: (text: string) =>
     string` to `ConversationViewOptions` (`#L229-L250`); add
     `onItemsChange?: (items: readonly ConversationItem[]) => void`.
   - `markdownLines()` (`#L589-L591`) uses `this.options.markdownTheme ??
     IDENTITY_MARKDOWN_THEME` instead of the hardcoded identity theme.
   - `textLines()` (`#L585-L587`) gains an optional 3rd `bg?:
     (text:string)=>string` param, forwarded as `Text`'s 4th constructor arg;
     `renderItem`'s `"user"` case (`#L557-L558`) passes
     `this.options.userLineBg`.
   - `appendItem()` (`#L342-L345`) calls `this.options.onItemsChange?.(this.items)`
     after pushing (copy semantics matches `overlay-chat.ts`'s own
     `[...this.entries]` convention — pass `[...this.items]`, never the live
     array).
   - `handleEvent()` (`#L373-L392`) adds two branches before the existing
     `agent_settled` check: `tool_execution_start` → `this.appendItem({kind:
     "tool-call", id: e.toolCallId, name: e.toolName, args: e.args})`;
     `tool_execution_end` → `this.appendItem({kind: "tool-result", id:
     e.toolCallId, name: e.toolName, content: <derived text>, isError:
     e.isError})`, where the content derivation mirrors
     `tool-result-render.ts`'s own single-text-content precedent (`content[0].text`
     when `content` is a one-element `type:"text"` array, else a best-effort
     stringification — do not import the private `isSingleTextContent`,
     duplicate the same 2-line check locally, matching the file's existing
     "copied rather than imported from a file this component must not couple
     to" convention already used for `isEscapeKey`).
2. `agents-plugin-pi/src/ask.ts`:
   - Swap the import at `#L80`: drop `overlay-chat.ts`; import
     `ConversationViewComponent`, `type ConversationChannel`, `type
     ConversationItem`, `type ChildLiveness` from `./conversation-view.ts`;
     import `loadHostPiTui` from `./pi-tui.ts`; import `getMarkdownTheme`
     (value) from `@earendil-works/pi-coding-agent` (static, matching
     `spawner.ts#L90`'s `RpcClient` precedent).
   - Define `export interface OverlayHandle { close(): void;
     closeWithSummary(summary: string): void; }` locally (moved verbatim from
     `overlay-chat.ts#L715-L720`).
   - Move `formatSpawnTime`, `buildDoneSummaryPrompt`, `EMPTY_SUMMARY_TEXT`
     from `overlay-chat.ts` into `ask.ts` (unchanged bodies).
   - `ThreadRecord.transcript` (`#L177`) → `ConversationItem[] | undefined`.
   - Rewrite `normalizeTranscript` (`#L227-L236`) per the Codebase Findings
     entry above (legacy `who`→`kind` conversion + native-shape validation +
     cap); `parseThreadRegistry`'s transcript destructure (`#L334-L338`) is
     unaffected (still calls `normalizeTranscript`).
   - Add `export function resolveChildLiveness(streaming: boolean):
     ChildLiveness { return streaming ? "running" : "settled"; }`; use it in
     `createForkChannel` (`#L971-L1008`): rename `isStreaming()` to
     `liveness()`, body `return resolveChildLiveness(rpcRegistry.get(agentId)?.streaming
     === true);`, return type `ConversationChannel`.
   - Add `buildInitialConversationItems(thread: Pick<ThreadRecord,
     "transcript"|"question">): ConversationItem[]` — returns `thread.transcript`
     when non-empty, else `[{kind:"note", text: thread.question}]` when
     `question` is set, else `[]`. Export for direct testing.
   - Rewrite `openThread` (`#L1238-L1314`)'s body from `await openOverlayChat(...)`
     onward: build `channel = createForkChannel(...)`; build `initialItems =
     buildInitialConversationItems(thread)`; build the header string per the
     Codebase Findings entry; best-effort `let markdownTheme:
     MarkdownTheme|undefined; try { markdownTheme = getMarkdownTheme(); }
     catch {}`; call
     `await ctx.ui.custom(async (tui, theme, _keybindings, done) => { const hostPiTui
     = await loadHostPiTui(); const component = new ConversationViewComponent(tui,
     {channel, initialItems, headerHint, markdownTheme, userLineBg: (text) =>
     theme.bg("userMessageBg", text), primitives: {ScrollView: hostPiTui.ScrollView,
     Markdown: hostPiTui.Markdown, Text: hostPiTui.Text, Editor: hostPiTui.Editor},
     onEscape: () => done(undefined), onDone: () => { /* summarize-then-close, see
     below */ }, onItemsChange: (items) => { thread.transcript = items.length >
     THREAD_TRANSCRIPT_CAP ? items.slice(-THREAD_TRANSCRIPT_CAP) : [...items];
     persistThreads(handle); }}); component.setMode("interactive");
     options.onOpened?.(buildOverlayHandle(component, channel, done)); return
     component; }, {overlay: true, overlayOptions: {width: "80%", maxHeight: "80%",
     anchor: "center"}})`. Keep `activeOverlay`/`overlayToken` bookkeeping
     (`#L1018-L1025,L1266-L1313`) unchanged around this call.
   - Add a helper (name it `buildOverlayHandle` or inline) that returns an
     `OverlayHandle` wrapping: `close()` → `done(undefined)`; `closeWithSummary(summary)`
     → append the summary text as a `"note"`/`"assistant"` item when non-empty
     (matches `overlay-chat.ts#L340-L346`'s `closeWithSummary`), call
     `closeThreadOnDone(pi, handle, rpcRegistry, thread, summary)`, then
     `done(undefined)`.
   - Implement the summarize-then-close logic (ticket-required "summarizeOnDone
     semantics preserved") as a small helper invoked from `onDone` above:
     given `summarizeOnDone = thread.origin === "lead-ask"` (same flag
     `openOverlayChat` used to receive), if false call the `OverlayHandle`'s
     `closeWithSummary("")` immediately; if true, subscribe once more to
     `channel.onEvent`, accumulate `text_delta`s, send
     `buildDoneSummaryPrompt()` via `channel.send`, and on the next
     `agent_settled` unsubscribe and call `closeWithSummary(settled ||
     EMPTY_SUMMARY_TEXT)` — ported from `overlay-chat.ts#L379-L392,L417-L448`'s
     exact state machine before that file is deleted.
   - `handleRespondentFinalReport` (`#L1055-L1087`) needs no signature change
     — it already takes `overlay: OverlayHandle | undefined`, now satisfied by
     the new `buildOverlayHandle` wrapper.
3. Delete `agents-plugin-pi/src/overlay-chat.ts`.
4. `agents-plugin-pi/test/ask.test.ts`:
   - `#L78` import path → `../src/ask.ts`.
   - Rewrite the transcript tests at `#L199-L231` for the new
     `ConversationItem[]` shape (round-trip, malformed-degrades, cap) plus a
     new legacy-hydration case (`{who:"you"/"thread"/"note",text}` arrays
     converting to `{kind:"user"/"assistant"/"note",text}`).
   - Add tests for `resolveChildLiveness` (streaming→"running",
     not-streaming→"settled").
   - Add tests for `buildInitialConversationItems` (existing transcript wins;
     question seeds one `note` when transcript empty; both empty → `[]`).
   - Migrate the `/done` round-trip describe block and the `closeWithSummary`
     describe block from `test/overlay-chat.test.ts` (see Codebase Findings)
     onto the new summarize-then-close helper / `buildOverlayHandle`, reusing
     the existing fake-`OverlayHandle` convention already at `#L1107-L1153`.
5. `agents-plugin-pi/test/conversation-view.test.ts` — add cases for
   `userLineBg` (background applied only to `"user"` items, matches
   `overlay-chat.ts`'s 4 "owner lines" cases), `markdownTheme` (passed through
   to `Markdown` for `"assistant"`/`"lead-message"` items, matches
   `overlay-chat.ts`'s "markdown rendering" cases minus the two obsolete
   dynamic-import ones), `onItemsChange` (fires with a copy of the full array
   on every append, streaming tail never included), and
   `tool_execution_start`/`tool_execution_end` → `tool-call`/`tool-result`
   item appends (id/name/args/content/isError wiring, error marker).
6. Delete `agents-plugin-pi/test/overlay-chat.test.ts` in full (every case is
   either already superseded by `conversation-view.test.ts`, migrated per
   step 4/5, or obsolete per the Codebase Findings categorization).
7. `ai-docs/spec/pi-adapter-runtime.md` — rewrite the "Overlay chat" bullet
   under `{#260905-pi-side-thread-owner-question-surface}` (around line
   1259-1270): the overlay is now the shared conversation-view component;
   `ForkChannel.isStreaming()` → `ConversationChannel.liveness()`; note the
   transcript scrolls (no 24-line tail cut); transcript items carry the
   `ConversationItem` model (tool calls/results visible); Ctrl+C is swallowed.
   Confirm (do not silently skip) that the `{#260905-pi-live-agent-widget}`
   section needs no edit, per the Codebase Findings note.

## Verification Plan

- `cd agents-plugin-pi && npm test` (== `node --test`) green, including the
  new/migrated cases in `conversation-view.test.ts` and `ask.test.ts`.
- `grep -rl "overlay-chat" agents-plugin-pi/src agents-plugin-pi/test` returns
  nothing (file deleted, no remaining importer).
- `grep -n "overlay-chat\|ForkChannel\|TranscriptEntry\|isStreaming"
  ai-docs/spec/pi-adapter-runtime.md` returns nothing.
- Manual-only (packaged as the owner runbook per ticket text, NOT
  agent-doable): 260904 runbook item 1 (open, two turns, Esc, reopen onto the
  same fork, `/done`, injection); the four 260905 activity-indicator checks;
  opening `/answer` on a fork that ran a tool and confirming the tool call/
  result render as a collapsed item and expand.

## Escalations

- None. The mechanisms needed to close every gap found (theme plumbing via
  pi-tui's existing `Text`/`Markdown` constructor args, host-resolved
  primitives via `pi-tui.ts`'s `loadHostPiTui()`, the `tool_execution_start`/
  `tool_execution_end` wire shape, and the summarize-then-close state machine)
  are all confirmed against real type declarations and existing precedent in
  this package, not open design questions — see Codebase Findings for the
  exact evidence.
