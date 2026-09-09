---
title: "Pi adapter: shared conversation-view component on pi-tui, with the ask overlay migrated onto it"
parent: 260908-epic-ws-pi-subagent-conversation-view
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: owns `overlay-chat.ts` / the `ask.ts` overlay binding this ticket replaces; its owner runbook item 1 is the migration's regression check
  260905-feat-ws-pi-overlay-activity-indicator-and-esc-hint: the `working…` marker and header Esc hint the component must keep, re-expressed on the liveness input
  260905-feat-ws-pi-live-agent-widget: imports `visibleWidth` from `overlay-chat.ts`; moves to the shared text helper this ticket leaves behind
  260906-feat-ws-pi-tool-result-yaml-tui-rendering: Phase 1 landed `tool-result-render.ts`, whose pure preview/display helpers the component's tool items reuse (its Phase 2 is still open and does not block this ticket)
spec:
  - pi-adapter-runtime
related-mental-model:
  - plugin-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: db5e498da6ef333a
sage-review-completeness-reviewed: db5e498da6ef333a
---

# Pi adapter: shared conversation-view component on pi-tui, with the ask overlay migrated onto it

## Background

Child A of `260908-epic-ws-pi-subagent-conversation-view`. The epic wants one
conversation UI for two consumers — the existing owner-question overlay
(`/answer`) and the new subagent audit window (`/audit`, child B). Today the
only conversation UI is `agents-plugin-pi/src/overlay-chat.ts`
(`OverlayChatComponent`, 780 lines) bound to one fork by `ask.ts`'s
`ForkChannel`. It hand-rolls everything a chat view needs: `visibleWidth`,
`wrapLine`, `stripAnsi`, bracketed-paste unwrapping, a width-keyed render
cache, a 24-line transcript tail cut (`MAX_TRANSCRIPT_LINES`), a one-line
input, `/done` interception. Its transcript model is text-only
(`TranscriptEntry {who: "you" | "thread" | "note"; text}`) — a tool call or
tool result the child makes is invisible in it.

The hand-rolling has one root cause, recorded in the file header of
`overlay-chat.ts`: `@earendil-works/pi-tui` is not resolvable from
`ws-pi-bridge` under `node --test`. It is nested under
`@earendil-works/pi-coding-agent`'s own `node_modules` (0.84.4) and the
package's `dependencies` list only `@earendil-works/pi-coding-agent ^0.84.4`
and `yaml 2.9.0`, so the overlay reaches pi-tui's `Markdown` only through a
guarded dynamic `import()` at runtime. Pi's extension docs list pi-tui as an
available import for extensions, and it ships `ScrollView`, `Text`,
`Markdown` and `Editor`.

This ticket makes pi-tui a direct dependency, builds the shared component on
it, and moves the ask overlay onto the component. It does not add the audit
window, the owner-steering ownership rule, or the Esc modal — those are child
B, which builds on this component.

## Decisions

- **Module and name.** `agents-plugin-pi/src/conversation-view.ts` exports
  `ConversationViewComponent` (a pi-tui `Component`: `render(width)`,
  `handleInput(data)`) and the types below. `overlay-chat.ts` is deleted at
  the end of Phase 2; nothing else keeps a rendering path of its own.
- **Message model** (plain data; persisted where the consumer persists):

  ```ts
  export type ConversationItem =
    | { kind: "user"; text: string }                         // owner line typed in the view
    | { kind: "lead-message"; text: string }                 // a message the lead sent the child
    | { kind: "assistant"; text: string }                    // child text, rendered as Markdown
    | { kind: "tool-call"; id: string; name: string; args: unknown }
    | { kind: "tool-result"; id: string; name: string; content: string; isError?: boolean }
    | { kind: "note"; text: string };                        // adapter note (question seed, status)
  ```

  `tool-call` / `tool-result` items are collapsed by default to one head line
  and expand per item. Head line and expanded body both come from
  `tool-result-render.ts` (`260906`): the head is that module's one-line
  preview (`yamlInputPreview` for a call, `completedTextPreview` /
  `logicalPreview` for a result), the expanded body is its full
  `yamlContainerDisplay` rendering — the component adds only the
  collapse/expand chrome, never a preview of its own. Keys: `Tab` /
  `Shift+Tab` move a selection highlight across the collapsible items
  (newest first), `Space` toggles the selected one, `Ctrl+O` toggles all of
  them at once. Precedence against the editor: in `view` mode these keys
  are always the component's; in `interactive` mode `Tab` / `Shift+Tab`
  act only while the editor is empty, `Space` acts only while a selection
  is active (a selection starts with `Tab`), any other typed character
  clears the selection and goes to the editor, and `Ctrl+O` acts in both
  modes regardless of editor state. Everything else renders in full: `user` lines keep
  the host user-message background as today, `lead-message` lines carry a
  `lead ›` label in the header colour so a lead-sent message is never
  mistaken for the owner's own line or the child's text, `note` lines are
  dim. The streaming tail (partial `assistant` text) is render-time state,
  never an item, exactly as today.
- **Two modes.** `mode: "view" | "interactive"`. `view` has no input line and
  never sends; `interactive` has the editor and delivers owner lines through
  the channel. Mode is set at construction and may be **raised** from `view`
  to `interactive` on the same instance via `setMode("interactive")` (history,
  scroll position, expand state and the event subscription survive; only the
  editor appears); it is never lowered. Enter in `view` mode routes to an
  `onEnter` callback (child B uses it for the switch); the component itself
  gives Enter no other meaning in `view` mode.
- **Liveness is a 3-state input read at render time**:

  ```ts
  export type ChildLiveness = "running" | "idle-awaiting-owner" | "settled";
  export interface ConversationChannel {
    onEvent(listener: (evt: unknown) => void): () => void;  // child RPC event stream
    liveness(): ChildLiveness;                              // read fresh on every render
    send?(text: string): Promise<void>;                     // interactive mode only
  }
  ```

  `ConversationChannel` replaces `ForkChannel`; `liveness()` replaces
  `isStreaming()` and is read at render time for the same reason the
  `260905-…-activity-indicator` ticket recorded (attach mid-turn and a
  dormant relaunch deliver no start event to the component). The
  `working…` marker shows when `liveness() === "running"` and the tail is
  empty. `idle-awaiting-owner` renders prominently — visibly louder than the
  other two states, in the header and at the transcript foot — so the owner
  notices; `settled` renders quietly. In Phase 2 the ask binding maps the
  registry's `streaming` flag to `running` and everything else to `settled`;
  producing `idle-awaiting-owner` is child B's ownership rule.
- **Built on pi-tui, not local helpers.** Transcript = `ScrollView`
  (`follow: "end"` while streaming; the owner can scroll back through the
  whole history — the 24-line tail cut goes away, the persisted cap stays
  the consumer's business), `assistant` items = `Markdown` with the host
  theme, other items = `Text`, input = `Editor`. The local `visibleWidth`,
  `wrapLine`, `stripAnsi`, render cache and paste unwrapping are removed in
  favour of pi-tui's exported equivalents; a helper pi-tui does not export is
  moved to a small `agents-plugin-pi/src/text-width.ts` shared with
  `agent-widget.ts` (which imports `visibleWidth` from `overlay-chat.ts`
  today), never re-implemented inside the component.
- **Dependency.** `package.json` gains `"@earendil-works/pi-tui": "0.84.4"`
  — an **exact** pin equal to the version `pi-coding-agent` nests, following
  the in-package precedent of `yaml: 2.9.0` (pinned to Pi's shipped
  version); the two pins are bumped together. `npm ls @earendil-works/pi-tui`
  must report one deduped copy; if it reports two, the outcome is to stop
  and align the pins, never to ship two copies.
  **One resolution point.** A new `agents-plugin-pi/src/pi-tui.ts` re-exports
  what the adapter uses from pi-tui through one static import; the component
  and the three existing guarded dynamic importers (`push-render.ts`,
  `tool-result-render.ts`, `index.ts`) all import from it, and their
  "pi-tui unavailable" fallback branches — dead once the package resolves —
  are deleted along with their tests' unavailable cases.
  **Runtime instance.** The epic requires the runtime to use the host's
  pi-tui instance (Pi loads the extension through its own loader). Phase 1
  verifies instance identity, not just version: inside a live lead, the
  `tui` the host hands to a widget/overlay factory must satisfy
  `tui instanceof TUI` with `TUI` taken from `pi-tui.ts`. If it does not
  (the loader resolved the adapter tree's copy), Phase 1 stops and records
  it; the only acceptable fallback is `pi-tui.ts` resolving through the
  host at runtime (the shim shape `loadMarkdownRenderer` uses today) while
  tests keep the static path — not a duplicated instance.
- **Key handling.** Esc is detected with the existing kitty-protocol-safe
  `isEscapeKey` logic and routed to an `onEscape` callback the consumer
  supplies (Phase 2's ask binding closes the view, as today; child B replaces
  it with the modal in interactive mode). `\x03` (Ctrl+C) is swallowed in
  both modes and given no meaning — Pi's editor exits the process on a
  double Ctrl+C within 500 ms and a focused overlay owns the raw input, so
  the component must never forward it. `/done` typed in interactive mode is
  intercepted and routed to an `onDone` callback (kept as an alias per the
  epic). Scrolling uses `ScrollView`'s own bindings.
- **Overlay geometry unchanged.** Opened through `ctx.ui.custom` with
  `{ overlay: true, overlayOptions: { width: "80%", maxHeight: "80%", anchor: "center" } }`;
  the header keeps `title · opened <time>` and one hint line supplied by the
  consumer as `headerHint: string` (the ask binding passes today's
  `Esc: close view (thread stays open) · /done: end thread`; the audit
  viewer passes its own), rendered exactly once and wrapped like the title
  (`260905-…-activity-indicator`: exactly one line states what Esc does).
- **Rejected.** Wrapping `OverlayChatComponent` in an adapter layer (its
  text-only model is the problem); keeping per-file guarded dynamic
  `import()`s with dead fallbacks after the dependency lands (hides a
  version or instance mismatch behind a silent degrade); a second component
  for the audit window.

## Constraints

- Golden rule: `agents-plugin-tool/` untouched; all work in `agents-plugin-pi/`.
- Every rendered line respects `visibleWidth(line) <= width` across
  40/80/120 columns (the existing overlay and widget test discipline).
- The migration is behaviour-preserving for `/answer`: `260904`'s owner
  runbook item 1 (lead-ask round trip: open, two turns, Esc, reopen onto the
  same fork, `/done`, injection) and the four `260905-…-activity-indicator`
  checks (marker on attach mid-turn, marker before tool text, header hint
  once, no footer) must still pass; `thread.transcript` files written by
  today's code must still open.
- No change to `spawner.ts` record semantics in this ticket: `streaming`,
  `threadBound` and `overlayAttached` keep their meaning until child B.

## Prior Art

- `overlay-chat.ts` — the behaviours to carry over (paste as one message,
  `/done` interception, header hint, working marker, render on
  `agent_start`), and the tests in `test/overlay-chat.test.ts` (67 cases) to
  migrate rather than drop.
- `agent-widget.ts` — `setWidget` factory receiving `(tui, theme)`; the same
  shape the component's factory takes.
- pi-tui package README: `ScrollView` usage with `follow: "end"`; Pi bundled
  examples `doom-overlay`, `overlay-test.ts` for overlay components.

## Spec Impact

`pi-adapter-runtime`:

- `{#260905-pi-side-thread-owner-question-surface}` "Overlay chat" bullet:
  the overlay is the shared conversation-view component; the working marker
  reads `ConversationChannel.liveness()`; the transcript scrolls (no 24-line
  tail cut); transcript items carry the `ConversationItem` model (tool calls
  and results visible); Ctrl+C is swallowed.
- New anchor for the component itself: message model, modes, liveness input,
  key contract, and the direct pi-tui dependency.
- `{#260905-pi-live-agent-widget}`: width helper now imported from the shared
  text helper, not `overlay-chat.ts`.

## Phases

### Phase 1: pi-tui dependency and the shared component

Add the exact-pinned dependency and verify one deduped copy. Add
`pi-tui.ts` and move the three dynamic importers onto it (deleting their
unavailable fallbacks and the tests that exercised them; the `260906`
rendering tests must stay green). Build `conversation-view.ts` with the
model, modes, liveness input and key contract above, on
`ScrollView`/`Markdown`/`Text`/`Editor`. First establish that each of the
four primitives constructs and renders under `node --test` with a fake
`tui` (`requestRender` only) and the host theme absent; a primitive that
does not is reached through an injectable `primitives` factory on the
component (defaulting to the real classes) so the test tier injects a
minimal fake for that one primitive only — the offline tests stay the
ticket's evidence. Move `visibleWidth` (and any other helper pi-tui lacks)
to `text-width.ts` and point `agent-widget.ts` at it; leave
`overlay-chat.ts` running untouched for now. Run the live instance-identity
check (`tui instanceof TUI`) in a lead session and record the result.

Tests (`node --test`, pure `render(width)` — no TTY): every item kind renders
and stays width-bounded at 40/80/120; tool items collapsed by default with
the `tool-result-render.ts` head line, `Tab` selects and `Space` expands
one in `view` mode, `Ctrl+O` expands all in both modes, `Space` in
`interactive` mode with no selection reaches the editor; the header hint
renders exactly once at 40/80/120; `lead-message` renders with its label; `working…` shown when `liveness()` is `running`
with an empty tail, replaced by the first delta, absent when `settled`;
`idle-awaiting-owner` produces the prominent header/foot rendering and
`settled` does not; `view` mode has no editor and ignores typed text;
`interactive` mode delivers a line and a bracketed paste as one `send`;
`/done` routes to `onDone`; Esc routes to `onEscape`; Enter in `view` mode
routes to `onEnter`; `setMode("interactive")` keeps items, scroll and expand
state and shows the editor; `\x03` changes nothing and is not forwarded;
`agent_start` requests a render.

### Result (124598df) - 2026-09-09

Landed the pi-tui dependency and the shared `ConversationViewComponent` as one
slice; `overlay-chat.ts` left running (Phase 2 owns its deletion),
`spawner.ts` record semantics and `agents-plugin-tool/` untouched. Range
`1d1352b3..c45c3a0d` (feat pi-tui.ts `6df13053`, refactor importers
`243c507a`, drop push-render unavailable tests `31c342c1`, component
`124598df`, overlay-chat assertion `27388d81`, review-fix `c45c3a0d`).

Component: `ConversationItem` model (`user`/`lead-message`/`assistant`/
`tool-call`/`tool-result`/`note`, tool items carrying `id`), `view`/`interactive`
modes with raise-only `setMode`, render-time `ChildLiveness` (`running`/
`idle-awaiting-owner`/`settled`) via `ConversationChannel`, collapsible tool
items reusing `tool-result-render.ts` previews (no new preview logic), the full
key-precedence contract (Tab/Shift+Tab newest-first, Space, Ctrl+O; interactive
editor-empty/selection-active gating; Esc via isEscapeKey; \x03 swallowed;
/done; Enter-in-view), consumer-supplied `headerHint`, built on
`ScrollView`/`Markdown`/`Text`/`Editor` through an injectable `primitives`
factory. `visibleWidth` moved to `text-width.ts`; `agent-widget.ts` repointed.
The three guarded dynamic importers now route through `pi-tui.ts`.

Deviation 1 (dual-package, lead-approved — plan addendum + commit `1d1352b3`):
`pi-coding-agent`'s npm-shrinkwrap nests its own `pi-tui`, so a top-level dep
yields two physical copies that no pin alignment can dedupe. Per the ticket's
sage-settled Runtime-instance fallback, `pi-tui.ts` resolves pi-tui through the
host at runtime (`loadHostPiTui()` shim) with a static import for
types/tests/defaults — no duplicated instance in use at runtime. Recorded as a
repo gotcha note (`gotcha.pi-tui-dual-package`).

Deviation 2 (forced test edit, lead-approved): the top-level pi-tui dep makes
`overlay-chat.ts`'s own `loadMarkdownRenderer` resolvable under `node --test`, so
`test/overlay-chat.test.ts`'s one "resolves to undefined" assertion was updated
to expect a renderer (source untouched); the plan's "unmodified" expectation for
that file is superseded by this recorded decision.

Review (partitioned, correctness=opus/fit+test=sonnet): review #1 returned 2
Critical + several Important. Critical C1 (missing `assistant` kind; child text
mislabeled as `lead-message`) and C2 (interactive key-precedence unimplemented)
were relayed and fixed in `c45c3a0d`; Critical re-review #2 returned both
[resolved], no new issues, clean. Five Important (working… empty-tail guard, Tab
newest-first, idle-at-foot rendering, consumer headerHint, send optional, plus
paste-as-one-send and tool-preview-pinning test coverage) self-reported [fixed]
in the same commit (best-effort, not re-reviewed). Minors recorded only.

Verification: `node --test test/conversation-view.test.ts` 44/44; targeted run
(conversation-view, push-render, tool-result-render, agent-widget, overlay-chat,
native-tool-registration) 182/182; full `npm test` 1240/1370 with the same 130
pre-existing environment failures (127 fork-prefix "missing installed Pi SDK
chunks" + 3 others), zero introduced.

Pending owner-run (not agent-cleared): the live instance-identity check inside a
lead session — `tui instanceof TuiMainScreen || tui instanceof TuiAltScreen`
(classes re-exported from `pi-tui.ts`) — expected to pass given the host-runtime
resolution. Phase 2 (ask overlay migration, overlay-chat deletion, transcript
hydration, the {#260905-pi-side-thread-owner-question-surface} spec-bullet
amendment) remains; ticket stays in `ready/`. No merge to the parent track
implied.

### Phase 2: migrate the ask overlay

Rebind `ask.ts` to the component: `createForkChannel` becomes a
`ConversationChannel` (`liveness()` = `streaming ? "running" : "settled"`),
`openOverlayChat` is replaced by opening the component in `interactive`
mode with `onEscape` = close view and `onDone` = `closeThreadOnDone`,
`summarizeOnDone` semantics preserved. Persisted thread transcripts move to
`ConversationItem[]`; a record holding the legacy `TranscriptEntry[]` shape
is converted on hydrate (`you` → `user`, `thread` → `assistant`, `note` →
`note`). Child tool calls and results seen on the fork's event stream become
`tool-call`/`tool-result` items. Delete `overlay-chat.ts` and migrate its
tests. Amend the spec per Spec Impact.

Verification: `cd agents-plugin-pi && npm test` green with the migrated
overlay cases; `overlay-chat.ts` gone and no remaining importer; the two
owner-run live checks named in Constraints (`260904` runbook item 1 and the
four activity-indicator checks) packaged as a one-shot owner runbook at
closeout, plus: open `/answer` on a fork that ran a tool and confirm the
tool call and result appear as collapsed items and expand.

### Result (770d8fc3) - 2026-09-09

Migrated the `/answer` overlay onto the Phase-1 `ConversationViewComponent`;
`overlay-chat.ts` and `test/overlay-chat.test.ts` are deleted with no remaining
importer. Range `c0120804..5b0de95c` (feat `770d8fc3`, test migration
`22def4f0`, spec `5e2b8c18` + lead completion `d35aee6b`, relay fix
`5b0de95c`). Golden rule held: `agents-plugin-tool/` untouched;
`spawner.ts` record semantics (`streaming`/`threadBound`/`overlayAttached`)
unchanged.

`ask.ts`: `openThread` rebuilt on `ctx.ui.custom` + the component in
`interactive` mode; `createForkChannel` returns a `ConversationChannel`
(`liveness()` via the extracted `resolveChildLiveness`); live render primitives
are host-resolved through `pi-tui.ts`'s `loadHostPiTui()` (the dual-package
rule applied to render classes, not `DEFAULT_PRIMITIVES`). The
summarize-then-close state machine, `OverlayHandle`, `EMPTY_SUMMARY_TEXT`,
`buildDoneSummaryPrompt`, and `formatSpawnTime` were ported in unchanged before
deletion; `summarizeOnDone` semantics preserved. `ThreadRecord.transcript` is
now `ConversationItem[]`; `normalizeTranscript` accepts both the native shape
(per-kind validation) and the legacy `{who,text}` shape (`you`→`user`,
`thread`→`assistant`, `note`→`note`), capped newest-first. `conversation-view.ts`
gained additive options only (`markdownTheme`, `userLineBg`, `onItemsChange`)
plus `tool_execution_start`/`_end` → `tool-call`/`tool-result` item wiring.

Deviation (lead-owned split): the spec "Overlay chat" bullet was finished by
the lead (`d35aee6b`) per lead-update-spec, not in the implementer relay — the
implementer's `5e2b8c18` did only the `liveness()` rename.

Review (partitioned correctness=opus/fit+test=sonnet): review #1 returned 1
Critical (the fork's summary turn double-appended to view+transcript on the
`/done` happy path, from two listeners on one channel) + several Important
(leaked summarize listener / lost at-most-once idempotency; `summarizeOnDone
=== false` untested; dropped "no-op once finished" regression test; missing
malformed native `tool-call`/`tool-result` normalization coverage; spec-bullet
completeness). Relay #1 fixed all in `5b0de95c` via an `alreadyRendered` flag
on `closeWithSummary`, a shared `finished` guard on `buildOverlayHandle` with a
pending-listener teardown hook, routing `Esc` through the guarded `close()`,
and an extracted `resolveDoneAction`; the spec Important was resolved lead-side.
Critical-scoped review #2 returned [resolved]/clean, no new defect. Minors
recorded only (two doc-comment typos fixed in passing; one cosmetic
`JSON.stringify` assertion deferred).

Verification: `env -u WS_PI_SPAWN_ROLE node --test test/ask.test.ts
test/conversation-view.test.ts` 173/173; full `npm test` 1209/1339 with the
same ~130 pre-existing WS_PI_SPAWN_ROLE/fork-prefix env failures (byte-identical
failing-test-name set to baseline), zero regressions introduced.

## Blocked (2026-09-09)

Both phases are implemented and review-clean, but this ticket cannot close to
`.done/` until the **owner-run acceptance runbook** passes — these checks are
human-only and were never agent-cleared:

1. Phase 1 live instance-identity check: in a lead session, confirm the host
   `tui` satisfies `tui instanceof TuiMainScreen || tui instanceof TuiAltScreen`
   (classes re-exported from `pi-tui.ts`).
2. `260904` runbook item 1: open `/answer`, two turns, `Esc`, reopen onto the
   same fork, `/done`, confirm the summary injection into the lead.
3. The four `260905` activity-indicator checks (working… / idle-awaiting-owner
   / settled states and the header Esc hint).
4. Open `/answer` on a fork that ran a tool and confirm the tool call and its
   result appear as collapsed items and expand.

The Phase 2 code is merged into the goal track (impl→goal per-cycle merge); this
blocker gates only the ticket's `.done/` transition, not the merge. Keep this
note and the ticket in `ready/` until every remaining runbook item is green.

### Owner-live acceptance attempt (2026-09-09)

The owner completed a live `/answer` conversation with two turns, `Esc`, reopen,
a tool call, and `/done`. Observed results:

- The overlay rendered without breakage. No live debug line exposed the exact
  `instanceof` expression, so this is only the agreed functional identity proxy,
  not direct instrumentation of item 1.
- The single header `Esc` hint rendered correctly.
- The tool call and result appeared collapsed and expanded to their full content,
  although the owner reported very poor overall visibility.
- `/done` closed the overlay but did **not** inject a summary into the lead
  transcript.
- The `working…` marker existed in the wrong location rather than at the end of
  the agent dialogue, making it easy to miss, and it was not visible immediately
  before tool output.
- The idle-awaiting-owner and settled state rendering remains **unverified**.

Item 1 has only proxy evidence, and items 2 and 3 remain failed or unverified,
so the owner-live gate is still blocked and the ticket must not move to `.done/`.
