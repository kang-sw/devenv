---
title: "Dashboard board notes: paste images into the note pen"
---

# Dashboard board notes: paste images into the note pen

## Background

Owner request (2026-11-13): allow pasting images into the whiteboard note pen (item notes and topic notes), working like the conversation composer, via an `[Image #N]` index in the note text. The owner expects it to be simple.

## Context

- Note pen: per-item `textarea.note` and per-topic `textarea.tnote` (`agents-plugin-tool/internal/wswhiteboard/assets/form.js:62`, `:85`, `:96`); the send path collects them at `form.js:157-182` and builds the block through `window.wbAnswers.block(items, topics)` (`form.js:182`).
- `<wb:answers>` grammar and note escaping (`^<note>`, newline as `\n`): `agents-plugin-tool/internal/wswhiteboard/assets/answers.js:4-11`; item notes are emitted at `answers.js:38-39`, topic notes at `answers.js:46-47`.
- Composer image paste: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:1349-1350` (chip held in the page, uploaded only with the send); marker grammar `[Image #N]` at `dashboard.js:1365-1367`; chip creation and counter (`nextNum`) at `dashboard.js:1414`; the paste handler at `dashboard.js:1499-1504`; `rail.js:69-75` strips markers from display text.
- Server side: session attachments dir exercised by `agents-plugin-tool/internal/wsdashboard/attachments_test.go:72-92` (images written into the session's attachments dir under the attachment grammar).

## Decisions

- Decision 1 (owner, 2026-11-13): the note pen accepts pasted images; each image becomes an attachment delivered with the outgoing dashboard message, and the note text carries `[Image #N]` at the caret, like the composer.
- Decision 2 (owner, 2026-11-13; was 412): One image counter is shared across the composer text and every board note in the same outgoing message, so each [Image #N] in one message is unique. Rejected: a separate per-note counter, which repeats [Image #1] within one message.

## Constraints

- Compatibility: T0. No change to the `<wb:answers>` line grammar (the note stays text; `[Image #N]` is plain text in it). No version bump.
- The UI track session is retired; the implementer is a frontend Claude worker with a LOOK capture.
- Manual: `ai-docs/manuals/shipped-surface-boundary.md` (Implementation Conventions row for `agents-plugin-tool/`).

## Phases

### Phase 1: Paste images into board notes

Let item notes and topic notes accept pasted images, reusing the composer's attachment pipeline, with `[Image #N]` inserted at the caret and each image sent as an attachment of the outgoing message.

Verification: paste two images into an item note and one into a topic note, plus one in the composer; send; check the lead receives four attachments with unique `[Image #N]` references matching the note text. Dark and light LOOK capture of the note pen with an image chip.
