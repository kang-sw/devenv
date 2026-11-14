---
title: "Dashboard: write an inline note beside a selected-text mention and attach both to the composer"
related:
  261110-feat-ws-dashboard-session-rail: "preserve existing per-session composer draft restoration"
  261112-feat-ws-dashboard-slash-commands: "preserve existing composer dispatch and separate send behavior"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5999eed10b13ab32
sage-review-completeness-reviewed: 5999eed10b13ab32
---

# Inline notes on selected-text mentions

## Background

The owner showed a selected passage with the floating Mention button and no adjacent input. They requested a small input bubble next to that action so their message accompanies the selected excerpt. On 2026-11-14, whiteboard (664)=a selected attachment to the existing composer, followed by its separate send action, rather than immediate transmission.

The existing selection action adds a mention to the composer (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2385-L2393`); it does not send until `sendMessage` posts the message (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2437-L2458`). The implementation must preserve that flow, the quoted source and its provenance, while enabling one user note for the selection bundle. This is independent of shared-card framework work. No approved new visual mockup exists; the source screenshot demonstrated the missing input, not an approved new layout.

## Decisions

1. **Add a note input beside the floating selection action.** Capture the selected quote and provenance before the input takes focus, so typing does not lose the selection. The note is optional; an empty note retains existing mention behavior. Authority: the owner's original request and (664).
2. **Attach, then send separately.** The action attaches quote plus note to the current session's composer. The user may revise/add text and use the existing send button. Rejected: immediate transmission from the floating action. Authority: (664)=a.
3. **Keep quote and user message distinct.** The note is not appended inside the selected-source `text` (`wsinbox.Mention.Text`), or substituted for the source text; there is no `quotedText` field in the current mention contract (`agents-plugin-tool/internal/wsinbox/mentions.go#L34-L41`). Preserve topic, item and immutable entry provenance. For a selection spanning multiple entries, retain the ordered excerpts and one note belonging to that complete selection bundle, not an invented note on every excerpt.
4. **Compatibility is T0.** Preserve accepted `wb:mention` syntax and existing readers. A new strict wire attribute or API semantic change is not authorized. Internal draft/UI representation may be extended compatibly.
5. **Use ordinary editable composer text for the attached note.** Keep the selected bundle together while capturing and attaching it, then use the existing `{text, mentions}` message contract. The quote remains structured provenance and the note remains user text; no machine-readable per-note bundle relation is added after send/merge. Preserve a clear human-readable association between notes and their selected quote/provenance, including multiple selection bundles in one draft, edits, draft restore and the delivered message. Rejected: a new note/bundle wire field, because the requested adjacent input and separate send do not require it and it would enlarge the compatibility surface. This is an auto-confirmed fact-populator technical resolution, not a separate explicit owner policy choice or permission to alter the quote. If the actual accepted parser/message path prevents this human-readable contract, return the concrete failing case to the lead rather than adding fields or claiming success.

## Constraints

- Preserve quote/provenance when focusing the input, composing text with an IME, clicking the floating action, changing the DOM selection or receiving a normal source update. Do not retarget an already captured quote to another passage.
- Preserve existing per-session composer draft restoration and user control over when to send. Switching sessions cannot attach the selection/note to the wrong session or lose an already attached note.
- Preserve plain mention behavior and current keyboard/click dismissal where no note is being entered. IME composition must not trigger accidental attachment or send. The worker may use conventional focus/dismiss handling without adding an immediate-send shortcut.
- No change to workflow whiteboard answer parsing or tracker mention semantics; this feature is selected-text mention input only.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Applicable implementation manuals: `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md`. If the MCP package or shared authored skill/protocol guidance must change, read the matching declared manuals first and return any newly required public contract decision to the lead.

## Prior Art

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`: selection capture, floating mention action, composer attachment and draft persistence.
- `agents-plugin-tool/internal/wswhiteboard/assets/doc-frame.js`: sandbox document selection bridge (`#L120-L141`); provenance is taken from host-owned iframe attributes, not the bridge message (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2397-L2406`).
- `agents-plugin-tool/internal/wsinbox/mentions.go`: strict `wb:mention` parsing.
- `agents-plugin-tool/internal/wsinbox/inbox.go`, `agents-plugin-tool/internal/wsdashboard/pageview_inbox.go`, and `agents-plugin-tool/internal/wsagent/runner.go`: existing inbox representation, rendering and delivery.

Current accepted path and verification boundary:

- Selection snapshots are already plain `{topic, id?, entry?, text}` objects, split in document order across entries (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2270-L2347`). The floating button currently prevents mousedown focus changes; selectionchange and frame outside-mousedown handlers can clear the pending snapshot (`#L2370-L2372`, `#L2379-L2384`, `#L2409-L2415`). The new input must preserve a captured bundle independently of these live-selection handlers.
- Existing send accepts `{text, mentions}` in the message request; `text` becomes `Entry.Parts`, while `mentions` remains separate (`agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L82-L88`, `#L414`). `Entry.Text()` emits the escaped mention block before ordinary text paragraphs (`agents-plugin-tool/internal/wsinbox/inbox.go#L332-L353`). Hosted delivery consumes that same text (`agents-plugin-tool/internal/wsagent/runner.go#L1813-L1825`); the conversation renderer separates mentions back out from user text (`agents-plugin-tool/internal/wsdashboard/pageview_inbox.go#L103-L115`). Thus ordinary composer text can carry a distinct user note without a new wire field or immediate send.
- Strict syntax is `<wb:mention topic="…" id="…" entry="eN">escaped source text</wb:mention>`, with optional `id` then optional `entry` in that order, and a newline or end after the closing tag. Unknown attributes are not accepted; no note or bundle attribute exists (`agents-plugin-tool/internal/wsinbox/mentions.go#L28-L41`, `#L70-L93`). Draft restoration also reconstructs only the existing mention fields (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L2492-L2502`, `#L8651-L8662`, `#L8718-L8721`).
- The current path distinguishes all quotes from message text but has no machine-readable note-to-bundle relation after multiple attachments or merged sends (`agents-plugin-tool/internal/wsinbox/inbox.go#L595`, `agents-plugin-tool/internal/wsinbox/mentions.go#L34-L41`). Decision 5 deliberately retains that existing delivery representation. Input placement, optional note, snapshot-before-focus and separate send are settled; no shared-card dependency or approved mockup is implied.
- Existing targeted tests include `TestBoardMentionFromSelection` in `agents-plugin-tool/internal/wsdashboard/wbmention_test.go`, `TestMessageCarriesMentionsAsCapsules` and `TestMessageEntryMentionsSurviveRouteLifecycle` in `agents-plugin-tool/internal/wsdashboard/inbox_routes_test.go`, and grammar/round-trip/lifecycle cases in `agents-plugin-tool/internal/wsinbox/mentions_test.go`. These are source-inspected, not run here; new note/focus/IME/browser acceptance remains deferred to implementation.
- Relations are preservation-only: `261110-feat-ws-dashboard-session-rail` and `261112-feat-ws-dashboard-slash-commands` are both done (their `.done/` tickets carry Results); neither is an outstanding blocker or landing-order constraint.

## Prior Decisions

- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, Decisions): "a selection crossing several entries splits into ordered entry-specific mentions, each retaining its selected text" — bearing: constrains
- e0964b73 (2026-11-12, commit): "Mentions are a structured wsinbox.Entry.Mentions field (topic, id, text) rendered into the message by Entry.Text(), not markup the browser writes into the typed text" — bearing: constrains
- 261112-feat-ws-dashboard-slash-commands (2026-11-12, Result): "While a command is selected, chips, notes and mentions stay in the composer and images block the send with a toast." — bearing: constrains
- 5695371f (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | dashboard.js selection/composer/draft changes plus UI styling and affected tests; doc-frame.js and inbox/render/delivery files are named preservation surfaces |
| scope.surface | cross-module | dashboard composer, wsinbox serialization and wsagent delivery must preserve the existing message contract |
| scope.new_public_symbol | no | Decision 4 authorizes no new wire attribute or API semantic change |
| scope.new_type_contract | yes | Internal selection-bundle draft/UI representation must retain one optional note across ordered excerpts; no new wire contract authorized |
| scope.test_surface | existing | wsdashboard/wbmention_test.go, wsdashboard/inbox_routes_test.go, wsinbox/mentions_test.go; additional browser cases required |
| complexity.reuse_points | confirmed | Read selectionMentions, addMention, sendMessage, validMentions, readDraft/restoreDraft, MentionBlock/ParseMentions and Entry.Text |
| complexity.side_effect_risk | moderate | Focus and selection handlers interact with async source updates, draft persistence and session-bound attachment |
| risk.correctness | high | Captured quote/provenance must not retarget or duplicate; one bundle note must survive focus, updates, session switches and send |
| risk.fit | moderate | Adjacent input must fit the existing floating action and separate composer send; Decision 5 preserves ordinary user text rather than adding a structured note/bundle relation |
| risk.test | high | Existing Node/Go cases do not establish new real-browser focus, IME, sandbox and session-switch behavior; no tests run during grounding |
| risk.security_or_contract | high | Strict mention grammar and host-owned immutable provenance must survive; unsupported note attributes would break parsing |

## Phases

### Phase 1: Capture and attach a selection note without changing send semantics

Implement the adjacent note input and selection snapshot, preserve the existing attach-to-composer action, and carry the quote bundle and distinct user note through draft, display and existing message delivery. Keep the existing strict mention parser compatible; do not hide the note inside quoted source text.

Verification:

- Browser tests for a single passage, multiple immutable entries and sandboxed document selection, with note input focus after the original selection.
- Empty note behaves like the current mention; nonempty note is attached with the correct ordered quote/provenance and is visibly distinct from the quoted source. No network send occurs until the existing composer send action.
- Cover pointer selection, keyboard selection, focus/dismiss, IME composition, source update/removal, session switch and composer draft restore without quote retargeting, duplicates or wrong-session attachment.
- Verify old `wb:mention` payloads still parse and render, new UI output uses accepted syntax, and the delivered user message contains the note as editable ordinary user text separately from the quote. Cover multiple selection bundles/attachments in one draft, ordinary user edits, draft restore and existing message merges: human-readable note-to-quote/provenance association must remain clear without a new machine-readable field. No note may become quoted source text or silently lose its intended association.
- Run focused parser/inbox/renderer tests and the affected dashboard selection/composer browser tests. Include wide/narrow widths and accessible labels/focus. Record actual commands, output and any untested surface; no live daemon replacement or provider invocation is required.

## Blocked (2026-11-14)

The xlarge worker stopped under non-convergence rule (f) after two failures of
the keyboard/empty-note capsule-count assertion. Owner question (670) asks
whether to authorize one additional bounded diagnosis/fix/verification round
or retain this checkpoint without further execution. Do not automatically
retry, weaken assertions, claim completion or merge before that answer.
The existing implementation and failure evidence remain preserved on the
retained implementation branch.

## Interrupted implementation - 2026-11-14

- Implementation remains incomplete on `impl/docs/settled-common-contracts-660-665/flap-brink-slang`; no phase Result or closure is claimed. Worker stopped under rule (f) after the keyboard/empty-note browser case repeated its capsule-count failure.
- Delegate commit `ba9f765bc` adds all locale labels and exact merge/cancel/resend/strict-parser/ring-render coverage. Delegate verification passed: `go test ./internal/wsdashboard -run 'Test(Message.*Mention|.*SelectionNote.*)$' -count=1` and `go test ./internal/wsinbox ./internal/wsi18n -count=1`, from `agents-plugin-tool`.
- The remaining checkpoint adds the adjacent input, input-held copied selection/provenance, IME guards, session-bound attachment and self-contained editable note context. Context contains each complete selected excerpt and its topic/item/entry, rather than an ordinal that could change after removal or merged sends. No wire contract or Go implementation was changed.
- Worker verification passed before browser work: `go test ./internal/wsdashboard -run 'Test(BoardMentionFromSelection|SelectionNoteIsEditableTextWithSelfContainedContext)$' -count=1` and `node --check internal/wsdashboard/assets/dashboard.js`, from `agents-plugin-tool`.
- Failing browser command, from `agents-plugin-tool`: `WS_PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright go test ./internal/wsdashboard -run '^TestSelectionNoteBrowser$' -count=1 -v -timeout=3m`. Latest output: `selection-note.browser.cjs:118`, expected five capsules, actual six, after keyboard selection plus empty-note attachment. The same capsule-count failure recurred after adjusting the native keyboard gesture; no further attempt was made.
- Failure diagnosis: direct `?frame=1` navigation first redirected into the shell, so the fixture was corrected to use the real shell iframe. Programmatic collapse of a non-editable paragraph was not a native keyboard caret. A later diagnostic run showed the native keyboard-selected range included `Second immutable quote` and the following `Plan` heading, with corresponding source capsules. That implicated test gesture/expectation semantics. The latest repeated count mismatch is not resolved; do not claim product correctness or weaken the expected source/provenance assertions without inspecting the actual selected range and pending snapshot.
- The browser test has reached pointer selection, input focus/Tab, synthetic IME guards, real SSE source update, multi-entry notes, sandbox selection dispatch and source removal on the first width. These are partial observations only: complete 1440/420 acceptance, session/draft/send completion, full focused suite/build and independent lite review remain pending. No native OS IME candidate UI, live/shared daemon replacement, provider, installer, merge or push was used.
