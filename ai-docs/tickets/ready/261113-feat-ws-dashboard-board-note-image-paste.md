---
related:
  261107-feat-ws-dashboard-composer-image-attach: "the composer image pipeline this reuses"
title: "Dashboard board notes: paste images into the note pen"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c510d46b47d38e43
sage-review-completeness-reviewed: c510d46b47d38e43
---

# Dashboard board notes: paste images into the note pen

## Background

Owner request (2026-11-13): allow pasting images into the whiteboard note pen (item notes and topic notes), working like the conversation composer, via an `[Image #N]` index in the note text. The owner expects it to be simple.

## Context

- Note pen: per-item `textarea.note` and per-topic `textarea.tnote` (`agents-plugin-tool/internal/wswhiteboard/assets/form.js:62`, `:85`, `:96`); `form.js:157-182` collects them and builds the reply line through `window.wbAnswers.block(items, topics)` (`form.js:182`), but the dashboard send does not post that line: `sendMessage` (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:1910-1937`) posts `chips` (item notes, read by `readItem` at `dashboard.js:1191-1198`) and `topics` (`topicNotes()`, `dashboard.js:1903-1907`) as separate fields, and the server builds the block (`agents-plugin-tool/internal/wsinbox/answers.go:50`, `AnswerBlock`). `form.js` is shared by the static board page and the dashboard board (`wswhiteboard/render.go:324`).
- `<wb:answers>` grammar and note escaping (`^<note>`, newline as `\n`): `agents-plugin-tool/internal/wswhiteboard/assets/answers.js:4-11`; item notes are emitted at `answers.js:38-39`, topic notes at `answers.js:46-47`.
- Composer image paste: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:1349-1350` (chip held in the page, uploaded only with the send); marker grammar `[Image #N]` at `dashboard.js:1365-1367`; chip creation at `dashboard.js:1414` (the counter `nextNum` is defined at `dashboard.js:1376`, `imgSeq` at `dashboard.js:1362`); the paste handler at `dashboard.js:1499-1504`; `rail.js:69-75` strips markers from display text.
- Server side: session attachments dir exercised by `agents-plugin-tool/internal/wsdashboard/attachments_test.go:72-92` (images written into the session's attachments dir under the attachment grammar).

## Decisions

- Decision 1 (owner, 2026-11-13): the note pen accepts pasted images; each image becomes an attachment delivered with the outgoing dashboard message, and the note text carries `[Image #N]` at the caret, like the composer.
- Decision 2 (owner, 2026-11-13): One image counter is shared across the composer text and every board note in the same outgoing message, so each [Image #N] in one message is unique. Rejected: a separate per-note counter, which repeats [Image #1] within one message.

- Decision 3 (technical, EXEC 2026-11-13, from fact population): `wsinbox.Merge` (`agents-plugin-tool/internal/wsinbox/inbox.go`) also renumbers `[Image #N]` markers inside chip notes and topic notes when a send merges into a held message, so Decision 2's uniqueness holds after a merge. The entry shape is unchanged (T0). Rejected: a separate numbering rule for notes (two numbering schemes in one message).
- Decision 4 (announced default, EXEC 2026-11-13; revised from design review): paste is dashboard-only, wired in `dashboard.js` (a delegated `paste` listener on the board region, or an optional `form.js` hook if that proves simpler; the implementer picks). The static board page pastes nothing, because it has no attachment channel (`wswhiteboard/render.go`). Note images join the one image list of the outgoing message, but the image state is today bound to the composer text (`ctext`), so every routine that reads markers covers the composer text plus every item note and topic note: `syncMarkers()` (it would otherwise drop note images on the next composer keystroke), `nextNum()`, the fresh-draft `imgSeq` reset after send, and `restoreHeld` renumbering and missing-marker detection. Chips are per note: a note's image chips sit with that note's pen, and x on a chip removes that note's marker. The image caps (`IMG_MAX`, `IMG_BYTES`) are shared across the composer and all notes, with the composer's existing over-cap behaviour. Rejected: moving the image pipeline into `form.js`; pushing note images into `imgs` without widening those routines (images silently lost).
- Decision 5 (technical, EXEC 2026-11-13): a cancel or board swap that rebuilds note text (`restoreTopics`, `setItem`) restores the note's image chips with it, so no `[Image #N]` marker is left without its attachment. The image data comes from where the composer already gets it on the same path (the page's image list, or the held message's images that `restoreHeld` restores); an image whose data is not available is handled like the reload case below. Rejected: dropping images on rebuild (orphaned markers). A draft restored on reload follows the composer precedent: images held only as page data are lost, so their markers are stripped from item and topic notes too (`WsRail.stripMarkers`) and counted in the `js.draft.imgslost` warning.

## Constraints

- Compatibility: T0. No change to the `<wb:answers>` line grammar (the note stays text; `[Image #N]` is plain text in it). No version bump.
- The UI track session is retired; the implementer is a frontend Claude worker with a LOOK capture.
- Manual: `ai-docs/manuals/shipped-surface-boundary.md` (Implementation Conventions row for `agents-plugin-tool/`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)

## Prior Decisions

- 261107-feat-ws-dashboard-composer-image-attach (2026-11-07, Decisions): "The browser holds a pasted or dropped image (a removable thumbnail chip in the composer) and uploads it only when the message is sent, so an abandoned paste never touches disk." — bearing: supports
- f73f009a (2026-11-07, commit): "Merge offsets the later send's numbers by the base's highest number (not its count) so gaps never collide, and rewrites only markers that name one of the later send's own images" — bearing: constrains
- 562bfeb9 (2026-11-12, commit): "Message route accepts `topics`; cancel/interrupt answers return `topics` (never null), and the composer's `restoreTopics` puts them back on the board's topic notes" — bearing: constrains
- 3ddae0e1 (2026-11-10, commit): "User whiteboard (598) ok: topic notes get a composer chip with x-clear and click-to-focus; send format unchanged." — bearing: constrains
- 40e8f438 (2026-11-08, commit): "the topic notes also go into the dashboard composer send (appended after the typed text) and survive a board swap (saved with the item answers)" — bearing: constrains
- 261109-feat-ws-tracker-shared-markdown-editor (2026-11-09, Decisions): "Accepted images: png, jpeg, gif, webp, each capped by the store's `attachments.max_bytes` (the composer's `IMG_TYPES` precedent)" — bearing: supports
- ee3097e5 (2026-11-13, commit): "A per-note counter would repeat [Image #1] within one outgoing message, so one shared counter was chosen." — bearing: supports
- 045763b7 (2026-11-13, commit): "Owner expects a small change; T0 because the note stays plain text and images ride the existing message attachments." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wswhiteboard/assets/form.js, agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, likely agents-plugin-tool/internal/wsinbox/inbox.go |
| scope.surface | cross-module | wswhiteboard asset, wsdashboard asset, and the wsinbox merge path; no exported Go symbol named |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | images ride the existing message images/refs; `wsinbox` Normalize (inbox.go ~L434-475) accepts images whose markers sit only in notes; notes stay text (Decisions 3-4, T0) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/attachments_test.go, agents-plugin-tool/internal/wswhiteboard/answers_script_test.go, agents-plugin-tool/internal/wswhiteboard/topic_header_test.go; no automated browser paste test |
| complexity.reuse_points | confirmed | composer image pipeline in dashboard.js:1362-1504 (imgs, nextNum, marker, paste handler) read; it lives in the dashboard.js closure, form.js is a separate shared script |
| complexity.side_effect_risk | moderate | form.js also runs on the static board page, which has no attachment channel; the shared counter touches the composer's draft numbering and clear-after-send |
| risk.correctness | moderate | wsinbox.Merge renumbers markers in Parts only (inbox.go:561-608), not chip or topic notes, so a send merged into a held message could leave note markers colliding or pointing at the wrong image |
| risk.fit | moderate | the shared-counter decision crosses two scripts and note textareas are rebuilt by board swaps and cancel restore (restoreTopics, setItem) |
| risk.test | moderate | verification is a manual LOOK capture; no existing automated test covers paste into notes |
| risk.security_or_contract | moderate | uploads reuse the image/attachment routes and caps (IMG_MAX 8, IMG_BYTES 10 MiB) with a wire-contract question on note markers; T0 as recorded |

## Phases

### Phase 1: Paste images into board notes

Let item notes and topic notes accept pasted images, reusing the composer's attachment pipeline through the `form.js` paste hook (Decision 4), with `[Image #N]` inserted at the caret and each image sent as an attachment of the outgoing message. Extend `wsinbox.Merge` per Decision 3 and restore chips on rebuild per Decision 5.

Place topic-note chips so they work inside the sticky `.tophead` of `261112-feat-ws-whiteboard-topic-tickets-sticky-head` when that lands first (the stuck head grows by the chip row; LOOK-check it).

Tests: a Go test that `wsinbox.Merge` renumbers markers in chip and topic notes; node-run `dashboard.js` (and `form.js` if touched) tests that a composer keystroke keeps note images, numbering spans composer and notes, cancel restore renumbers note markers without duplicating them in the composer, chips are restored on rebuild, and reload strips orphan note markers.

Verification: paste two images into an item note and one into a topic note, plus one in the composer; send; check the lead receives four attachments with unique `[Image #N]` references matching the note text. Dark and light LOOK capture of the note pen with an image chip.
