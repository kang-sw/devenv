---
title: Inline skill and topic capsules with fuzzy composer pickers
related:
  261115-feat-ws-dashboard-conversation-rendering-consolidation: landing order; urgent user-role/i18n correction heads first, deferred Phase 2 is not a dependency
  261114-refactor-ws-dashboard-whiteboard-asset-split: related; preserve landed split ownership
  261115-feat-ws-dashboard-search-drawer-answer-agent: related; existing palette tone and keyboard boundaries
  261115-feat-ws-whiteboard-topic-close-request: related; preserve reason/note capsules
  261109-feat-ws-dashboard-composer-file-mention: related; reusable picker and file mention behavior
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f73fc6103600879c
sage-review-completeness-reviewed: f73fc6103600879c
completed: 2026-11-15
---

# Inline skill and topic capsules with fuzzy composer pickers

## Background

MAIN relayed the owner's composer-ux request on 2026-11-15 after conversation-rendering Phase 1. The existing prefix-only slash selection splits the composer rather than remaining inline. The owner wants compact atomic capsules and pickers consistent with the existing search palette.

## Decisions

- The slash picker opens at a token before the caret that begins with `/`, anywhere in the text, when the suffix after the caret is empty or whitespace. Do not restrict it to the beginning of the editor.
- A selected skill is an inline capsule sized to its text, with a slightly smaller font; one Backspace removes the whole capsule. Skill search is fuzzy and its popup follows the existing search-palette visual tone.
- `#` opens fuzzy search over topic labels. Confirmation inserts an atomic label capsule; sending substitutes `[topic-stem]`. One Backspace removes it as a unit.
- **(991)=c, confirmed through MAIN at 06:40:34Z:** a skill capsule invokes only when it is at the very start of the session message. A mid-text capsule sends plain text naming the skill without invocation. Rejected: consuming all surrounding text as arguments of a mid-text skill; the owner chose position-dependent invocation instead.
- **(992)=b, confirmed through MAIN at 06:40:34Z:** topic-label `#` pickers work in the session composer, memos and whiteboard answer-note fields; skill execution is session-composer-only. MAIN explicitly permitted omitting the slash picker in other fields or inserting names only. Choose no slash picker in memos/answer notes: it is the smaller supported surface and avoids wiring command behavior into non-command editors. Rejected: composer-only topic references; name-only slash selection outside the composer adds unnecessary picker behavior for this permitted simpler variant.
- **Topic candidate scope, MAIN technical call at 06:49:49Z:** memo `#` search uses the currently selected session's topics, including closed topics through existing pagination. With no selected session there are no candidates. Use the same session-local topic boundary for the session composer and its whiteboard answer notes. Rejected: a workspace-wide topic source contract or silently omitting unopened closed topics.
- Expected compatibility tier: T0 client-only presentation, without a new cross-version surface. Preserve the actual existing structured `{command,args}` command-entry wire form; do not replace it with slash-prefixed request text. MAIN confirmed at 06:42:07Z that (991)=c fits that existing boundary. A start-position capsule maps to a command entry as today, while mid-text capsules remain ordinary text and never invoke. Existing command-only guards remain: answers, topic notes, mentions and attachments cannot accompany a command invocation. If any selected behavior requires a protocol/API or persisted-contract change, stop and report to MAIN.

## Constraints

- Composer product implementation starts only after the urgent 781 user-role correction's unused-locale-key rehead is delivered to MAIN. Its owning ticket remains ready with Phase 1 recorded and Phase 2 deferred; that deferred work is not a dependency. (991), (992) and (996) are settled. MAIN owns owner discussion, all decision IDs, integration and shared-runtime actions.
- Preserve queue/claim/acknowledgment, producers, stored contracts and command-entry wire semantics. Do not assume that a skill capsule can be implemented by changing the command parser.
- Preserve existing attachments, selection bundles, drafts, input-mode preferences, IME behavior, keyboard ownership and send guards on the selected editors.
- Follow UI-fast execution: implement, focused relevant tests and wsi18n for strings, one bounded logic review, then head. Owner-screen visual acceptance follows MAIN's merge and swap. No screenshot matrix or browser-proof gate.
- Before product implementation, read every matching repository Implementation Conventions manual, including shipped-surface boundary and release compatibility; this draft does not assert source facts not yet inspected.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Visual Reference

- Approved tracked reference (owner (996)=ok, relayed by MAIN at 06:48:14Z on 2026-11-15): [composer-proposal.html](../assets/261115-feat-ws-composer-inline-skill-and-topic-capsules/composer-proposal.html), committed as `ddb84c95e` with the immutable preparation plan.
- MAIN rendered the updated HTML to PNG; owner (996)=ok approves the updated mockup, per MAIN's 06:48:14Z relay. The tracked HTML at `ddb84c95e` is the approved capsule/popup visual reference. Side-by-side and visual comparison belong to the owner's on-screen check after MAIN's merge/swap under the explicit UI-fast direction, not a pre-head screenshot, browser-proof or matrix gate. Composer product implementation waits for the separate urgent 781 rehead; no additional owner decision is needed for this visual approval.

## Current Source Evidence

Preparation and fact population inspected the existing source boundaries:

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js:744-754` and `inbox_routes.go:279-320` carry structured command requests. `agents-plugin-tool/internal/wsinbox/inbox.go:522-542` rejects other message components on command entries.
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:22` and `dashboard-composer.js:677-693` use a textarea, separate command capsule and prefix-only picker. Atomic inline capsule editing is new implementation work, not a claim that textarea DOM already supports it.
- `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L50-L50` separates topic labels from stable stems. `agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js#L104-L119` and `agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js#L177-L186` own composer draft save/restore. The memo transport is `agents-plugin-tool/internal/wsdashboard/assets/memo.js#L107-L128`, not `agents-plugin-tool/internal/wswhiteboard/assets/memo.js` (that path does not exist); `agents-plugin-tool/internal/wsdashboard/memos.go#L153-L162` trims and requires 1–8000 bytes of memo text for non-delete writes.
- Local draft boundaries: composer state is per-session browser localStorage, not a daemon store (`agents-plugin-tool/internal/wsdashboard/assets/rail.js#L12-L20`, `agents-plugin-tool/internal/wsdashboard/assets/rail.js#L35-L79`). It currently saves plain text, one separate command, mention ranges and answer strings, not inline skill/topic identity. Memo add/edit uses textarea strings; workspace changes clear the add input, and sessionStorage records panel resume only, not memo draft text (`agents-plugin-tool/internal/wsdashboard/assets/memo.js#L57-L72`, `agents-plugin-tool/internal/wsdashboard/assets/memo.js#L129-L136`, `agents-plugin-tool/internal/wsdashboard/assets/memo.js#L169-L173`). No new persistent memo-draft contract is implied by preserving existing drafts.
- Topic candidate reuse is partial, not an existing complete `#` source: `paletteRows` reads rendered session-board labels/stems and `paletteClosed` incrementally loads older closed topics (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-tracker.js#L265-L325`); the closed-topic route serves paginated HTML (`agents-plugin-tool/internal/wsdashboard/pages.go#L1398-L1435`). MAIN's 06:49:49Z technical call settles the missing scope: current selected session only, closed topics included through that pagination, and no candidates without a session. Reuse these existing sources without adding a workspace-wide route or silently restricting search to mounted DOM.
- Reusable popup machinery is textarea-based token/source/render/choose and listbox state (`agents-plugin-tool/internal/wsdashboard/assets/mention.js#L94-L168`), not an atomic inline editor. Existing focused neighbors include `agents-plugin-tool/internal/wsdashboard/slash_command_test.go`, `inbox_command_test.go`, `memos_test.go`, `testdata/memo_test.cjs`, `search_palette_test.go` and `agents-plugin-tool/internal/wsi18n/keys_test.go`. These are inspection pointers, not claims that the new behavior is tested.

## Prior Decisions

- ddb84c95 (2026-11-15, commit): "owner confirmed 991=c (only a skill at the very start of a session message invokes; mid-text capsules serialize as a literal skill name, never invoke) and 992=b" — bearing: supports
- 261114-refactor-ws-dashboard-whiteboard-asset-split (2026-11-15, commit): "Concatenate contiguous JavaScript source fragments without separators into the original single IIFE" — bearing: constrains
- 261115-feat-ws-dashboard-search-drawer-answer-agent (2026-11-15, commit): "Keep layout A, 1600-code-point successful-only optional context, keyboard/citation/auth boundaries, the 26px input and full scope/strip deletion. T0 compatibility remains unchanged." — bearing: constrains
- 0bc7fcf2 (2026-11-15, commit): "Keep existing localStorage preference keys and synchronize layout/input with native same-origin storage events to every cached frame." — bearing: constrains
- 261115-feat-ws-whiteboard-topic-close-request (2026-11-15, Decisions): "each close-request capsule carries its own optional reason field, like a topic note" — bearing: constrains
- 261109-feat-ws-dashboard-composer-file-mention (2026-11-09, Decisions): "the popup is a standalone module that takes the input element (and a candidate source) as arguments" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js, dashboard-lifecycle.js, dashboard.tmpl, memo.js and agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl; adjacent tests/i18n. |
| scope.surface | cross-module | Dashboard composer, workspace memos and whiteboard answer notes; existing HTTP/inbox boundaries preserved. |
| scope.new_public_symbol | no | No new route, MCP symbol or exported daemon API authorized. |
| scope.new_type_contract | no | Preserve structured command/args and plain string memo/note contracts; inline identity remains client-local. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/slash_command_test.go, inbox_command_test.go, memos_test.go, testdata/memo_test.cjs, search_palette_test.go; agents-plugin-tool/internal/wsi18n/keys_test.go. |
| complexity.reuse_points | confirmed | Read mention.js token/source/render/choose, composer commandList/sendCommand, lifecycle draft restoration, rail localStorage drafts, memo change and paletteRows/paletteClosed. Atomic inline editing is new work. |
| complexity.side_effect_risk | high | Replacing textarea editing affects attachment markers, selection ranges, IME/key ownership and local draft restoration across three editor surfaces. |
| risk.correctness | high | Start-position classification must never invoke mid-text capsules; atomic edits and topic-stem substitution must preserve surrounding text, images and command-only guards. |
| risk.fit | moderate | Existing popup and draft owners support reuse but do not implement rich-inline identity. Candidate scope is now settled to the selected session, with existing closed pagination and no-session empty results. |
| risk.test | high | Existing textarea/source fixtures do not establish atomic deletion, caret/IME and cross-surface restore; focused offline coverage is required, with owner visual acceptance after MAIN integration rather than browser-proof gates. |
| risk.security_or_contract | high | Command invocation crosses local-viewer/advertised-command authorization; preserve agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L309-L320 and agents-plugin-tool/internal/wsinbox/inbox.go#L522-L542 without parser/store changes. |

## Phases

### Phase 1: Inline capsules and fuzzy pickers without wire changes

Implement the confirmed capsule and picker behavior in the session composer and topic references in memos/whiteboard answer notes. Preserve the existing execution path for a skill capsule at the very start of the session message; serialize a mid-text skill capsule as ordinary skill-name text, never as a command entry. Replace topic label capsules with `[topic-stem]` at the relevant text submission boundary. Keep capsule identity in local editing/draft behavior without changing daemon stores or producer contracts. Preserve the existing non-command text, attachments and input-mode behavior; command-only submission guards continue to apply.

Use the tracked mockup for visual direction. UI-fast owner-screen review after MAIN's merge/swap is the visual acceptance route, not a screenshot matrix or pre-head browser gate. MAIN's 06:43:35Z direction explicitly permits starting without waiting for owner check (996).

Verification must cover caret-local slash activation and suffix guards, fuzzy ordering, labels versus wire stems, atomic deletion, start versus mid-text command classification, literal mid-text submission, cursor and keyboard/IME behavior, draft save/restore, existing attachments, all selected editor surfaces, selected-session switching, no-session empty candidates, unopened closed-topic pagination, and preservation of structured command requests and command-only guards. Run focused relevant offline tests and wsi18n for new strings, with one bounded logic review; do not run full suites or shared-runtime actions. Any newly discovered preference-sensitive behavior goes to MAIN; technical source facts must be cited rather than guessed.

### Result (a8c077482) - 2026-11-15

- Implemented compact atomic skill/topic capsules over the existing textarea owners, with caret-local fuzzy pickers, whole-capsule deletion, canonical text serialization, IME/keyboard ownership, plain-text clipboard boundaries and local draft identity. Session-composer skill picks invoke through the existing structured `{command,args}` request only at offset zero; mid-text picks send the literal skill name without a slash. Command-only held components and image refusal remain intact.
- Added topic-only pickers to memo add/edit and whiteboard answer/topic notes. Candidates belong only to the selected session, including every unopened closed page through the existing pagination route; no session yields none, and stale memo callbacks are discarded. Closed pages are parsed inertly instead of changing history folds or scroll ownership. Existing note/image/form owners and legacy fixed-argument returned-command selections remain supported.
- Landed implementation in `92bf09b9b` and `44d01be77`. The one independent lite review identified a Critical blank-line serialization loss; `b35bb43d0` preserves interior/leading/consecutive empty native blocks and caret positions, with deterministic regressions. No Critical or Important review finding remains; no second review was allocated.
- MAIN performed the sole integration of exact `ce22992e6fcb98bbd2250566742504c1e04a4248` into the implementation branch as `a8c077482`. Inspection found no capsule interaction requiring further product edits. Upstream search drawer, TASK3 history, `wbHistory.place`, answer-chip markup and history-chip layout are retained.
- Verification: all Go commands ran from the absolute `agents-plugin-tool` directory with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. The pre-review focused dashboard selection covering capsules, slash/command guards, memos, drafts, note images, check answers, search and closed pagination passed; full `go test ./internal/wsi18n -count=1 -timeout=3m` passed. After the review fix, affected `Test(InlineCapsules|Slash|Memo|RailDraftRestore|BoardNoteImages)` passed. After integration, focused note/check/pagination tests plus `TestHistoryAnswersUseSingleLineChipsAndCompleteDisclosure`, `TestHistoryRichDetailIsContentDrivenAndStatic` and `TestViewKitPlaceUnderNode` passed; `Test(InlineCapsules|OverviewPureContracts|OverviewFilterContracts|OverviewStateFilterLifecycle)` passed; full wsi18n passed again after the incoming template change. The standalone capsule Node contract and whitespace checks also passed.
- Encountered upstream test-contract failure: `TestOverviewPageIntegration` expects `maximize(c)`, while `assets/cards.js` uses `maximize(c, floating)`. Both that test and asset are byte-identical to exact incoming `ce22992e6`; MAIN explicitly retains the separate test-contract repair. No production change or weakened assertion was made for it, and no all-green overview/package claim is made.
- Decisions: retain plain-string note/memo and structured command boundaries (client-only T0; no daemon API, producer or store change); use the permitted no-slash variant outside the session composer; keep capsule identity only in browser editing/drafts. The hidden backing textarea caches its native value for board clones, and note identity follows the existing answer snapshot/restore path.
- Omitted by the approved UI-fast contract: full package/repository suites, additional review, browser/screenshot matrices, provider/download actions, runtime changes and further synchronization. Comparison with `ai-docs/tickets/assets/261115-feat-ws-composer-inline-skill-and-topic-capsules/composer-proposal.html` remains the owner's on-screen acceptance after MAIN's integration/runtime swap; no capture or visual-match claim is made here. Future queue-agent removal and the separate upstream test-contract repair remain outside this ticket.
