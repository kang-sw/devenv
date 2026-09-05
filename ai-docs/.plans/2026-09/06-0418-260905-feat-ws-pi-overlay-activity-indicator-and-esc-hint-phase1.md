# Plan: 260905-feat-ws-pi-overlay-activity-indicator-and-esc-hint — Phase 1: Working marker and Esc hint

## Relevant Ticket Contract

- Read `channel.isStreaming()` in `render(width)`; draw one line `working…` in
  the streaming-tail slot when it is true and the streaming buffer is empty;
  the first text delta replaces it, settle clears it.
- The state is read at render time from `ForkChannel.isStreaming()` (backed by
  the registry's `record.streaming` flag), NOT derived from `agent_start` /
  `agent_settled` events the component itself receives — two of the paths
  (attach-mid-turn, dormant-relaunch first message) never deliver a start
  event to the component.
- Move the key hint from the footer to the header, directly after the
  `opened <time>` line when present, as exactly:
  `Esc: close view (thread stays open) · /done: end thread`. Wrap it with
  `wrapLine` the same way the title line wraps.
- Remove the footer line entirely (`/done closes the thread · Esc closes this
  view (the thread keeps running)`). After this ticket exactly one line in the
  overlay states what `Esc` does.
- Both additions stay inside the existing `render(width)` bound
  (`visibleWidth(line) <= width`).
- The marker must never appear in the persisted transcript — `onTranscriptChange`
  must never report it.
- No behavior change to `Esc` / `/done` semantics — visibility only.
- Tests required (from the phase text): marker present when streaming with an
  empty tail; replaced by the first delta; absent once the channel reports not
  streaming after settle; marker present on the very first `render()` call of a
  component whose channel already reports streaming (attach-mid-turn /
  dormant-relaunch); header hint present exactly once and width-bounded at
  40/80/120; footer line gone; transcript persistence unchanged.
- Spec impact: amend the "Overlay chat" bullet
  (`ai-docs/spec/pi-adapter-runtime.md#260905-pi-side-thread-owner-question-surface`)
  with the working marker and the header hint, replacing any mention of the
  footer hint.
- Live check (owner-run, not part of this survey/plan's automated verification):
  open a fork-raised thread mid-turn and confirm the marker shows immediately;
  send a message that makes the fork run a tool and confirm the marker shows
  before any text arrives.

## Out of Scope

- `ForkChannel.isStreaming()`'s own implementation, `record.streaming`
  lifecycle, and `sendToAgent`'s prompt/steer branching in
  `agents-plugin-pi/src/ask.ts` / `agents-plugin-pi/src/spawner.ts` — Phase 1
  only reads the existing accessor from `overlay-chat.ts`; the flag's producer
  side is unchanged and out of scope.
- Any spinner/animation approach — explicitly rejected in the ticket's
  Decisions section.
- A toast on `Esc` — explicitly rejected.
- Any later phase or adjacent concern not named in Phase 1's text.
- `agents-plugin-tool/` and `agents-plugin/skills/` — never touched (repo
  constraint for this work).

## Codebase Findings

- `agents-plugin-pi/src/overlay-chat.ts#L71-78` — `ForkChannel` interface
  already declares `isStreaming(): boolean`; the component holds this via
  `this.options.channel` (constructor at `#L301-314`), so no interface or
  wiring change is needed — only a new read site in the render path.
- `agents-plugin-pi/src/overlay-chat.ts#L536-582` — `render(width)`: header
  block is built at `#L558-564` (title via `wrapLine` at `#L561`, then the
  `opened <time>` line at `#L562-563` gated on `formatSpawnTime`, then one
  blank separator row at `#L564`). The transcript loop is `#L566`. The footer
  to remove is `#L568-575`: a blank separator row (`#L568`), the `> <input>`
  line (`#L569`), and the dim-styled footer line built with
  `wrapLine(...).map(line => row(line, "dim"))` (`#L573-575`). The header hint
  must be inserted between the existing `opened` block (`#L562-563`) and the
  blank separator (`#L564`), using the same `wrapLine(text, inner)` +
  `row(line)` pattern the title line uses — this satisfies the "stays inside
  `render(width)`'s bound" constraint for free, since `row()` already pads to
  exactly `w` columns and is what every other width-bounded assertion checks.
- `agents-plugin-pi/src/overlay-chat.ts#L591-619` — `transcriptRows(width)`:
  builds `all` from `this.entries` plus, at `#L594-596`, the live streaming
  tail (`if (this.streaming.trim().length > 0) all.push({ who: "thread", text: this.streaming.trim() })`).
  This is the one spot that is "the streaming tail slot" the ticket refers to
  and is render-time-only — nothing here ever reaches `append()` /
  `onTranscriptChange`, so a marker row added in an `else` branch here
  automatically satisfies "never appears in the persisted transcript" with no
  extra guard. The natural change: add `else if (this.options.channel.isStreaming()) all.push({ who: "thread"-shaped marker row, text: "working…" })` — pushed as a
  `TranscriptRow` directly (bypassing `renderThreadText`/Markdown, since
  `working…` is not thread content and the ticket names it as a literal
  fixed string, not something the host Markdown renderer should touch).
- `agents-plugin-pi/src/overlay-chat.ts#L270-273` — `TranscriptRow` shape
  (`{ text: string; ownerBlock?: boolean }`) is exactly what a plain marker
  row needs; no new type required.
- `agents-plugin-pi/src/overlay-chat.ts#L536-541` — the `cachedLines` /
  `cachedWidth` render cache is invalidated only by `refresh()` (`#L370-374`,
  called from every event handler that changes streaming/entries/input) and
  `invalidate()`. Reading `channel.isStreaming()` fresh inside `render()`
  itself is safe against this cache ONLY if something calls `refresh()` (or
  `invalidate()`) whenever the streaming flag flips outside of an event the
  component already observes. Risk: on the attach-mid-turn / dormant-relaunch
  paths, `isStreaming()` is already true before any event reaches the
  component, so the marker must show on the very FIRST `render(width)` call —
  which trivially bypasses the cache (nothing cached yet) — but if the owner's
  terminal does not repaint again before the fork's first `text_delta` arrives
  (which does call `refresh()` via `handleEvent`), the marker will correctly
  disappear once that delta lands. No dead-marker case exists because every
  transition away from "streaming with empty tail" is already covered by an
  existing `refresh()` call site (`text_delta` → `#L343-346`; `agent_settled`
  → `#L360`). No new invalidation path is needed.
- `agents-plugin-pi/test/overlay-chat.test.ts#L43-128` — the fake `ForkChannel`
  harness already exposes exactly what this phase's tests need with NO
  changes required to the harness itself: `isStreaming: () => streaming`
  (`#L70`) reads a closure variable seeded from `options.streaming ?? false`
  (`#L60`), and `setStreaming(value)` (`#L115-117`) flips it after
  construction. This means:
  - "marker present on the very first render… (attach-mid-turn /
    dormant-relaunch)" is `harness({ streaming: true })` then
    `h.component.render(width)` with **no** prior `emit`/`delta`/`settle` call
    — the harness already supports constructing a channel that reports
    streaming before any event reaches the component.
  - "absent once the channel reports not streaming after settle" needs BOTH
    `h.settle()` (clears `this.streaming` buffer / fires `agent_settled`,
    calling `refresh()`) AND `h.setStreaming(false)` (flips the fake
    channel's own `isStreaming()` return value) — the fake channel's
    `streaming` var is NOT auto-tied to `settle()`; a test that calls only
    `h.settle()` without `h.setStreaming(false)` would still see
    `isStreaming() === true` and the marker would legitimately persist. This
    is a test-authoring detail worth getting right, not a product bug: in the
    real `createForkChannel` (`agents-plugin-pi/src/ask.ts#L1007-1009`,
    confirmed by `agents-plugin-pi/src/spawner.ts#L1846-1847`,
    `applyRpcEvent` sets `record.streaming = false` on `agent_settled`), the
    two do move together.
  - Existing helper `flush()` (`#L131`) is unrelated (microtask flushing for
    `channel.send` rejections) and not needed for these marker tests.
- `agents-plugin-pi/test/overlay-chat.test.ts#L183-209` — "the view is a
  rounded box…" test asserts `fgCalls.includes("dim")` (`#L208`), sourced
  today from the footer's `row(line, "dim")` call. Once the footer is removed,
  this assertion breaks unless the header hint keeps the same `"dim"` styling
  (recommended: reuse `row(line, "dim")` for the new header hint line, so this
  existing width/border test keeps passing with no assertion change beyond
  what naturally follows from the footer text moving, not disappearing
  stylistically). This is a minor implementation choice (not specified by the
  ticket beyond "wraps like the title line"); reusing "dim" preserves the
  existing visual de-emphasis and keeps this test's `dim`-color coverage
  meaningful without inventing new styling semantics.
- `agents-plugin-pi/test/overlay-chat.test.ts#L251-256` — "the footer names
  /done and says Esc leaves the thread running" test reads the footer text
  from `render(80)`; must be rewritten to check the header hint's location and
  wording instead (and to assert `DONE_COMMAND`'s footer phrasing —
  `/done closes the thread`/`keeps running` — is gone), since the ticket's
  Constraints line requires exactly one `Esc`-behavior line to remain.
- `ai-docs/spec/pi-adapter-runtime.md#L926-934` — the exact "Overlay chat"
  bullet to amend: currently documents prompt-vs-steer routing, transcript
  persistence, owner-line styling, Markdown rendering, and `Esc closes the
  view only…, reattachable at any time` — but never quotes the literal footer
  string, so the amendment is additive (working marker + new header-hint
  location) rather than a find-and-replace of quoted text. No other spec line
  matches `keeps running` / `working…` / `streaming tail` (confirmed by
  search), so this is the only spec location needing a change.

## Implementation Plan

1. `agents-plugin-pi/src/overlay-chat.ts` — in `transcriptRows(width)`
   (`#L591-619`), change the streaming-tail branch (`#L594-596`) to: push the
   live streamed text when `this.streaming.trim().length > 0` (unchanged);
   otherwise, when `this.options.channel.isStreaming()` is true, push one
   plain `TranscriptRow` with `text: "working…"` (no `ownerBlock`, not routed
   through `renderThreadText`). Leave the `MAX_TRANSCRIPT_LINES` tail
   truncation (`#L616-618`) untouched — the marker row participates in it
   exactly like the existing tail row did.
2. `agents-plugin-pi/src/overlay-chat.ts` — in `render(width)` (`#L536-582`),
   insert the header hint immediately after the existing `opened <time>`
   block (`#L562-563`) and before the blank separator (`#L564`):
   `for (const line of wrapLine("Esc: close view (thread stays open) · /done: end thread", inner)) lines.push(row(line, "dim"));`
   — placed so it appears whether or not `opened` is present (right after the
   title/opened header block, before the transcript starts).
3. `agents-plugin-pi/src/overlay-chat.ts` — delete the footer block at
   `#L573-575` (the `wrapLine(...).map(row(line, "dim"))` for the
   `/done closes the thread · Esc closes this view…` string), leaving the
   blank separator (`#L568`) and the `> <input>` line (`#L569`) as the last
   two rows before the closing border.
4. `agents-plugin-pi/test/overlay-chat.test.ts` — update the render-box test
   at `#L183-209`: no change needed to the `dim` assertion itself if step 2
   reuses `"dim"` styling (verify after the edit); if it does not, adjust the
   assertion to match wherever `"dim"` is actually now applied.
5. `agents-plugin-pi/test/overlay-chat.test.ts` — rewrite the test at
   `#L251-256` ("the footer names /done…") to assert the header hint text
   (`Esc: close view` / `/done: end thread`) is present and that the old
   footer phrasing (`closes the thread`, `keeps running`) is gone from
   `render(80)`'s joined output.
6. `agents-plugin-pi/test/overlay-chat.test.ts` — add new tests (new
   `describe` block, e.g. "working marker") using the existing `harness()`
   helper, no harness changes needed:
   - streaming with empty tail → `render(...)` includes `"working…"`.
   - `h.delta("...")` after that → marker text is replaced (assert
     `"working…"` is gone and the delta text is present).
   - `h.settle()` **and** `h.setStreaming(false)` → marker absent.
   - `harness({ streaming: true })` (no prior emit/delta/settle) →
     `h.component.render(width)`'s very first call already includes
     `"working…"` (covers attach-mid-turn / dormant-relaunch).
   - marker text never appears in any `h.transcripts` entry (assert across
     the sequence above that `onTranscriptChange` payloads never contain
     `"working…"`).
7. `agents-plugin-pi/test/overlay-chat.test.ts` — add a width-bound test for
   the header hint at widths 40/80/120: hint text (or its wrapped pieces)
   present exactly once in `render(width)`'s joined output, and every line
   still satisfies `visibleWidth(line) <= width` (reuse the existing
   `visibleWidth` import).
8. `ai-docs/spec/pi-adapter-runtime.md` — amend the "Overlay chat" bullet
   (`#L926-934`, under `{#260905-pi-side-thread-owner-question-surface}`) to
   state: the overlay shows a `working…` line in the streaming-tail slot,
   driven by `ForkChannel.isStreaming()` read at render time (not from
   `agent_start`/`agent_settled` events the component itself receives), while
   the respondent's turn is running and no text has streamed yet; and that the
   `Esc`/`/done` key hint is stated once, in the header (not a footer), as
   `Esc: close view (thread stays open) · /done: end thread`.

## Verification Plan

- `cd /Users/kang-sw/orca/workspaces/devenv/track-pi-agent/agents-plugin-pi && node --test test/overlay-chat.test.ts` — the ticket's own named unit tier; must cover all new/changed assertions above with no regressions to the existing 60+ assertions in this file (owner-line painting, `/done` round-trip, paste handling, escape-key matching, transcript persistence).
- Manual/no-tooling check: re-read `render(width)`'s output structure after the edit to confirm exactly one line matching `/Esc/` remains overlay-wide (constraint: "exactly one line... states what Esc does").
- Live check (owner-run, out of this survey's automated scope per the ticket's Phase 1 text): open a fork-raised thread mid-turn and confirm the marker shows immediately; send a tool-invoking message and confirm the marker shows before any text arrives.

## Escalations

- None.
