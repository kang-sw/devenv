---
title: "whiteboard.update refuses a wb:option holding single-quoted words"
---

# whiteboard.update refuses a wb:option holding single-quoted words

## Observation (2026-11-10)

During a lead's `ws/whiteboard.update`, a batch whose `wb:option` body
contained Korean words wrapped in single quotes (for example `'열기'`) was
refused with "no closing </wb:option>". Removing the quotes made the same
batch pass. The command syntax says a body is raw text up to its exact
closing tag, so quotes inside a body should not matter.

Cause unconfirmed: possibly the tag scanner treats a quote inside a body as
the start of an attribute value and scans past the closing tag.

## Directions (not decided)

- Reproduce with ASCII words in single quotes and with double quotes, inside
  `wb:option`, `wb:why`, and `wb:body`, to see whether the scanner or a
  field-specific path is at fault.
- Fix so body text is never quote-scanned; add a parser test.

## Constraints

- Parser under `agents-plugin-tool/internal/wswhiteboard/`; Go tests via
  `~/.local/bin/ws-rgo`.
