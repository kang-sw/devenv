# Plan: whiteboard answers go first as a `<wb:answers>` block, shown as capsules in chat

Source: user request on the ws whiteboard (2026-11-12), items (334)-(338) confirmed.

## Problem

A dashboard send today delivers `Entry.Text()` (`agents-plugin-tool/internal/wsinbox/inbox.go`)
as: user text paragraphs, image lines, then the answer line
(`(341) a; (123) b, note: ...; (124), (125) ok`), with topic-note lines
(`[topic] text`) appended to the user text by `dashboard.js` `sendMessage`.
In the chat the whiteboard answers therefore trail the user's words, and the
delivered `user` entry (`dashboard.tmpl` `entry`) shows them as raw text.

## Target

1. **Order and wrapper ((334)).** The delivered text starts with the answer
   block, then one blank line, then the user's message paragraphs and image
   lines, unquoted:

   ```
   <wb:answers>
   341=a
   123,567,691=b
   124..129,181=ok
   130=ok^user note
   131=^multi-line note\nsecond line
   [ship-gate]^topic note
   </wb:answers>

   user message here
   ```

   No block when there are no answers and no topic notes.
2. **Grammar ((335)).** One line per entry: `<ids>=<value>[^<note>]`.
   - Items with the same value and no note are grouped into one line, IDs
     comma-separated, and a run of consecutive IDs is written `a..b`
     (a run of two may stay `a,b`; pick one rule and test it).
   - The note follows the first `^`; a note-only item is `id=^note`.
   - In a note, newline is written `\n` and backslash `\\`, so every
     entry stays on one line and a literal `\n` in a note survives.
   - Values are the existing chip answers (option letter, `ok`, `no`,
     short text matching `answerPattern`).
3. **Topic notes inside the block ((336)).** A topic note is a line
   `[<topic stem or name>]^<note>`, same escaping, instead of a
   `[topic] text` line appended to the user's text. This means the inbox
   entry must carry topic notes structurally (today `sendMessage` folds them
   into `text`); add a field or a chip form for them — your call, keep the
   inbox schema change minimal and versioned per the file's existing
   `SchemaVersion` practice if it needs one.
4. **One grammar everywhere ((337)).**
   - `wsinbox.AnswerBlock` (Go) builds the block; `dashboard.js`
     `answerBlock` (the composer's reply-line preview / copy) and the static
     whiteboard page's copy-answers (`agents-plugin-tool/internal/wswhiteboard/assets/form.js`)
     build the same block. Keep a shared golden-style test that the Go and
     JS builders agree on representative cases if the repo already has a
     pattern for Go-runs-JS tests (look at `wsdashboard` `*script_test.go`);
     otherwise test each side.
   - The whiteboard playbook `#### Answers` section
     (`agents-plugin/rsrc/whiteboard/whiteboard.md`, mirrored in
     `agents-plugin-wsflow/rsrc/whiteboard/whiteboard.md`) documents the
     block: how to read each line, that each item's `wb:answer` body is that
     item's line (or the whole block line for a group), and that a typed
     reply in the old `(62) b; (64) a, note: <text>; (44), (45) ok` form is
     still accepted. Read `ai-docs/manuals/skill-authoring.md`,
     `ai-docs/manuals/wsflow-mirroring.md`, and
     `ai-docs/manuals/shipped-surface-boundary.md` before editing it. Check
     other shipped text that quotes the old reply grammar (grep for
     `note: <text>` / `ok` grouping under `agents-plugin*/`) and the MCP
     strings in `agents-plugin-tool/internal/mcp/dashboard_inbox.go`.
5. **Capsules in chat ((337)).**
   - Delivered `user` entries: when the text starts with a `<wb:answers>`
     block, the page parses it and renders the entries as capsules at the
     top of the bubble, matching the held bubble's `.hchips .achip` look
     (and the composer chips: `(id) value` with the note beside it; a
     topic note as the composer's topic-note chip style), then the rest of
     the text as markdown. Parsing can be server-side in the entry
     rendering (Go, preferred: the entry is server-rendered through
     `dashboard.tmpl` `entry`) — find where `Kind == "user"` entries get
     `.Text` and split there. Ranges expand to one capsule per ID or stay
     grouped as one capsule `(124..129) ok` — grouped is fine and shorter.
   - Held bubbles (`dashboard.tmpl` `held`): move the chips row above the
     parts so held and delivered messages look the same.
   - A malformed block renders as plain text (never drop content).

## Verification

- Go unit tests for the builder (grouping, ranges, notes with `^`, `\n`,
  `\\`, note-only, topic notes, empty) and for the parser used by the chat
  view (round trip of the builder's output; malformed falls back).
- Existing wsinbox / wsdashboard / wswhiteboard tests updated, run through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m <min> -- test ./internal/wsinbox/... ./internal/wsdashboard/... ./internal/wswhiteboard/... ./internal/mcp/...`
  (never run heavy go build/test locally).
- Headless browser capture of a delivered message with a block and of a
  held message, saved under the lead's scratchpad
  `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/shots-chat/`.
- wsflow mirror drift tests pass.

## Boundaries

- Do not change how the agent records answers beyond the playbook text.
- Do not touch the tracker files (another delegate is in `tracker.js`).
- Return to the lead if the inbox schema change turns out to need a
  migration of stored entries beyond a version bump.
