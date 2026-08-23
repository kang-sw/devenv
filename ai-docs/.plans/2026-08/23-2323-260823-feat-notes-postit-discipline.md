# Plan: 260823-feat-notes-postit-discipline — Phase 1: Notes post-it discipline (write nudge + always-render standing hint)

## Relevant Ticket Contract

- Facet A: `note.write` appends a confirmed relocate/erase challenge to its
  response when any note's `value` has `len(value) >= threshold` (named,
  trivially-tunable constant, start `~300`); the write itself is
  unconditional (note is written regardless); batch calls append the
  challenge once per call, not once per oversized note; remediation verbs are
  relocate/erase, never mute.
- Confirmed challenge text: `Large note — keep only if volatile AND homeless
  AND must-always-stay-in-context; otherwise move it to a
  ticket/spec/mental-model, or erase. Not mute.`
- Facet B: always render `# Notes`, including the empty state (deliberate
  divergence from sibling injections' silent-when-empty contract). Three
  render states: no-notes (empty-state hint), all-muted (existing heading +
  muted-count line + standing hint), has-visible (notes list + standing
  hint). The empty-state hint is keyed off the existing "no notes on any
  layer" empty-skip predicate — never off zero-visible.
- Confirmed empty-state text: `_No notes. Notes are your short,
  always-in-context post-it reminders — `note.write` to pin one, `note.erase`
  when it's no longer needed._`
- Confirmed standing (non-empty) one-line text: `_Post-it reminders:
  `note.write` to pin, `note.erase` when done._`
- Facet B keeps today's placement (appended after `## Session State`, not a
  prepended banner); only empty-render and hint behavior change.
- Update `ai-docs/spec/mcp-tools.md` note-tools (`#260810-note-tools`) and
  note-injection (`#260810-note-injection`) anchors per the ticket's `## Spec
  Impact`.
- Verification boundary: oversize/sub-threshold write behavior, all three
  render states, spec anchors updated, `go test ./...` and `go vet ./...`
  pass in `agents-plugin-tool/`. (The two absorbed idea tickets are already
  moved to `.dropped/` — confirmed on disk, no action needed this phase.)

## Out of Scope

- Any latch/per-session-armed-flag mechanism for facet A (explicitly
  rejected in ticket decisions).
- Firing the challenge on every write regardless of size (explicitly
  rejected).
- Offering `note.mute` as remediation phrasing (explicitly rejected).
- Any change to `note.mute`/`note.unmute`/`note.search`/`note.erase`
  behavior beyond what's needed to keep existing contracts intact.
- Any change to `wsnote.CompareRecords`, the priority cap value, or the
  muted-exclusion-from-cap-budget behavior — unrelated to this phase.
- Updating the `note.write` MCP tool-schema `description` string in
  `server.go` (ticket's Spec Impact names only `mcp-tools.md`; the schema
  description is a separate advisory surface not called out).

## Codebase Findings

- `agents-plugin-tool/internal/mcp/note_tools.go#L288-L308` (`handleNoteWrite`) —
  write handler: resolves store, parses `notes` array, writes unconditionally,
  then branches on `wantsJSON(args)` for JSON vs text response. Facet A hooks
  in after the `store.Write` success, before the response branch.
- `agents-plugin-tool/internal/mcp/note_tools.go#L454-L461` (`formatNoteWrite`) —
  builds the "wrote N note(s):" text; the challenge should append after this
  text, not inside it (keep formatter single-purpose).
- **Reuse pattern for JSON-vs-text nudge asymmetry**:
  `agents-plugin-tool/internal/mcp/server.go#L976-L991` (the `git.commit`
  dispatch case) — `wantsJSON` branch returns `toolJSONResponse(...)` with
  **no** appended reminder; only the text-mode branch appends
  `appendSessionKeyTip`/todo-summary reminders. This is the existing
  convention for "clean structured JSON, prose nudges only in text mode" —
  facet A's challenge should follow it: append only to the text-format
  response, leave `toolJSONResponse(id, records, nil)` at
  `note_tools.go#L305` untouched.
- `agents-plugin-tool/internal/mcp/note_announcement.go#L1-L23` — existing
  `notesInjectionCap = 20` named-constant pattern the ticket cites as the
  precedent to mirror for facet A's threshold constant. `computeNotes` is a
  thin wrapper over `wsnote.Compute`.
- `agents-plugin-tool/internal/wsnote/inject.go#L31-L114` (`Compute`) — the
  function to change for facet B. The "no notes on any layer" empty-skip
  predicate is `if len(all) == 0 { return "" }` at **L74-L76**, keyed on the
  unfiltered `all` collection (before the muted/visible split at L78-L86) —
  this is exactly the predicate the ticket says the empty-state hint must key
  off, confirmed distinct from `len(visible) == 0` (the all-muted case, which
  already renders heading + muted-count line with zero bullet lines, no
  change needed there beyond appending the standing hint).
- `agents-plugin-tool/internal/wsnote/inject.go#L102-L113` — the render tail:
  heading, bullet lines, elision line, muted line,
  `strings.TrimRight(sb.String(), "\n")`. The standing one-line hint appends
  here (has-visible and all-muted cases share this path since both have
  `len(all) > 0`); the empty-state hint instead replaces the whole body when
  `len(all) == 0`.
- **Byte-equality risk signal (source-verified, not stated in the ticket
  text)**: `agents-plugin-tool/internal/mcp/workflow_manual.go#L172-L194`
  (`handleWorkflowState`) currently does **not** call `computeNotes` at all —
  it returns only `renderSessionState(rec)+"\n"`. Meanwhile
  `agents-plugin-tool/internal/mcp/workflow_manual.go#L295-L303` (the
  CONTINUE branch of `handleWorkflowManual`) appends `computeNotes(rec.Root)`
  after `renderSessionState(rec)` whenever notes exist. Today this divergence
  is silent because `computeNotes` returns `""` whenever there are no notes
  on any layer, and no existing test exercises `workflow_state` with notes
  present. Once facet B makes `Compute` (and therefore `computeNotes`)
  **always** return non-empty text — even the empty-state hint — the
  divergence becomes unconditional: `workflow_manual`'s `## Session State`
  suffix will always carry the notes block while `workflow_state`'s output
  never will. This directly breaks
  `agents-plugin-tool/internal/mcp/session_state_test.go#L3701-L3749`
  (`TestWorkflowStateReturnsSessionStateOnly`), which asserts `workflow_state`'s
  output equals `workflow_manual`'s response sliced from `"## Session State"`
  onward, byte for byte, for *every* resolved session (not just ones with
  notes). The mental-model doc `ai-docs/mental-model/mcp-runtime.md`
  (`{#260702-workflow-state-tool}`, `{#260810-note-tools}`, around L109-L127)
  documents this exact-byte-equality contract as a standing invariant. Facet B
  must add a matching `computeNotes(rec.Root)` append to `handleWorkflowState`
  (mirroring the CONTINUE branch's exact concat shape) or this test breaks on
  every run, not just notes-present ones.
- `agents-plugin-tool/internal/mcp/workflow_manual.go#L264-L270` (FRESH-with-root
  branch) and `#L295-L303` (CONTINUE branch) — the two `if notes :=
  computeNotes(...); notes != "" { body += "\n\n" + notes }` call sites named
  in the ticket's Prior Art. Once `Compute` never returns `""`, the `!= ""`
  guard becomes permanently true (dead condition, not a bug) — simplify both
  to an unconditional `body += "\n\n" + computeNotes(...)` for clarity, mirroring
  the always-rendered `# Manuals` block model the ticket cites.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go#L88-L96`
  (`injectBootstrapStalenessWarning`) — confirms the prepend-warning helper is
  a real no-op on empty input (`if warning == "" { return body }`), unlike the
  notes append; this is the pattern the ticket explicitly says NOT to use for
  `# Notes` (Prior Art: "Prepend-warning pattern... NOT used here").
- **Existing tests that will need updates to match facet B's contract change**
  (risk signal — these currently assert the *opposite* of the new behavior):
  - `agents-plugin-tool/internal/wsnote/inject_test.go#L10-L17`
    (`TestComputeReturnsEmptyWhenNoNotesExist`) — asserts `Compute(...) ==
    ""`; must become an assertion on the empty-state hint text instead.
  - `agents-plugin-tool/internal/mcp/note_workflow_manual_test.go#L193-L212`
    (`TestWorkflowManualNotesBlockAbsentWhenNoNotesExist`) — asserts
    `notesBlockIndex(resp) < 0` (block absent) for both FRESH-with-root and
    CONTINUE with no notes; must become an assertion the block **is** present
    with the empty-state hint.
- `ai-docs/spec/mcp-tools.md#L416-L547` (Note Tools, `#260810-note-tools`) —
  facet A's target anchor; the `note.write` bullet is at **L482-L488**.
- `ai-docs/spec/mcp-tools.md#L753-L790` (Note Injection,
  `#260810-note-injection`) — facet B's target anchor; the empty-skip
  paragraph to replace with the always-render + three-states description is
  at **L772-L776** ("The append is skipped entirely... matching the
  silent-when-empty contract of every sibling injection.").
- `ai-docs/mental-model/mcp-runtime.md` around **L109-L127** documents the
  guarded-concat invariant and the byte-equality contract; this passage
  becomes stale once the guard is simplified to unconditional — flag for the
  doc-pass todo (mental-model update), not required by this ticket's `## Spec
  Impact` (which names only `mcp-tools.md`), but leaving it unedited will
  describe removed code ("The guard is required, not optional") — the
  implementer/doc-pass should update it on contact per repo doc-freshness
  norms, not necessarily in this same commit.

## Implementation Plan

1. **Facet A — `agents-plugin-tool/internal/mcp/note_tools.go`**: add a named
   constant near `handleNoteWrite` (e.g. `noteOversizeThreshold = 300`, doc
   comment mirroring `notesInjectionCap`'s style) and a small helper
   `noteWriteExceedsOversizeThreshold(records []wsnote.Record) bool` that
   returns true if any record's `len(rec.Value) >= noteOversizeThreshold`. Add
   the confirmed challenge text as a named string constant. In
   `handleNoteWrite` (`L288-L308`), after `store.Write` succeeds, leave the
   `wantsJSON` branch (`L304-L306`) untouched; in the text branch, build
   `formatNoteWrite(records)` and, when `noteWriteExceedsOversizeThreshold(records)`
   is true, append `"\n" + <challenge constant>` before returning via
   `toolTextResponse`.
2. **Facet B — `agents-plugin-tool/internal/wsnote/inject.go`**: restructure
   `Compute` (`L31-L114`) so the `len(all) == 0` branch (`L74-L76`) writes the
   `# Notes` heading plus the confirmed empty-state hint line instead of
   returning `""`, then returns early (trimmed). Leave the muted/visible
   split, sort, cap/elision logic (`L78-L100`) and existing bullet/elision/
   muted-line rendering (`L102-L112`) unchanged. After that existing render
   block (still inside the `len(all) > 0` path, covering both all-muted and
   has-visible), append the confirmed one-line standing hint before the final
   `strings.TrimRight`.
3. **Facet B — `agents-plugin-tool/internal/mcp/workflow_manual.go`**:
   simplify the two `if notes := computeNotes(...); notes != "" { body +=
   "\n\n" + notes }` guards (`L264-L270`, `L295-L303`) to unconditional `body
   += "\n\n" + computeNotes(...)`, since `Compute` no longer returns `""`.
   Add a matching unconditional `renderSessionState(rec) + "\n\n" +
   computeNotes(rec.Root)` (mirroring the CONTINUE branch's exact shape) to
   `handleWorkflowState`'s resolved-record return (`L193`, currently
   `renderSessionState(rec)+"\n"`) so the byte-equality contract with
   `workflow_manual`'s `## Session State` suffix holds — this is required to
   keep `TestWorkflowStateReturnsSessionStateOnly` passing (see Codebase
   Findings). Do not touch the two fail-loud "no restorable state" branches
   (`workflow_manual.go#L222-L225`, `#L184-L188`) — those have no root and
   stay notes-free on both sides, matching `TestWorkflowStateUnknownKeySameFailLoudAsWorkflowManual`.
4. **Update existing tests that assert the old contract**:
   - `agents-plugin-tool/internal/wsnote/inject_test.go#L10-L17` — change
     `TestComputeReturnsEmptyWhenNoNotesExist` to assert the empty-state hint
     text is present instead of `got == ""`.
   - `agents-plugin-tool/internal/mcp/note_workflow_manual_test.go#L193-L212`
     — change `TestWorkflowManualNotesBlockAbsentWhenNoNotesExist` to assert
     the block **is** present (empty-state hint) for both FRESH-with-root and
     CONTINUE.
5. **Add new tests** (facet A): sub-threshold `note.write` does not append the
   challenge; oversize (`>= 300`) single write appends it exactly once; a
   batch write with one oversized note among several sub-threshold ones
   appends it exactly once (not per-note); the note is written in both
   oversize and sub-threshold cases; JSON-mode (`wantsJSON`) response stays
   clean (no challenge text) even when oversize. Facet B: no-notes state
   renders the empty-state hint (not the old absent-block behavior);
   all-muted state renders heading + muted-count line + standing hint, never
   the "No notes." empty hint; has-visible state renders notes list +
   standing hint; `workflow_state` output stays byte-identical to
   `workflow_manual`'s `## Session State` suffix when notes are present
   (extend `TestWorkflowStateReturnsSessionStateOnly` or add a sibling test
   that writes a note before calling both tools).
6. **Update `ai-docs/spec/mcp-tools.md`**:
   - `#260810-note-tools` anchor (`L482-L488`, `note.write` bullet): document
     the oversize challenge — threshold, that it's a named tunable constant,
     that the write is unconditional, and the confirmed challenge text/verbs
     (relocate/erase, not mute).
   - `#260810-note-injection` anchor (`L772-L776`): replace the skipped-when-
     empty description with always-render + the three render states
     (no-notes with empty-state hint keyed off the no-notes-on-any-layer
     predicate, all-muted with heading+muted-count+standing hint,
     has-visible with notes list+standing hint), and note the deliberate
     divergence from sibling injections' silent-when-empty contract with the
     user-facing-affordance rationale.
7. Run `gofmt -l` / `go vet ./...` / `go test ./...` in
   `agents-plugin-tool/` (see Verification Plan).

## Verification Plan

- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go vet ./...`
- `cd agents-plugin-tool && go test ./...` — must include the updated
  `internal/wsnote` and `internal/mcp` note/workflow-state test files above,
  and the new facet A/B tests from Implementation Plan step 5.
- Manual/spec check: confirm `ai-docs/spec/mcp-tools.md`
  `#260810-note-tools` and `#260810-note-injection` anchors read consistent
  with the ticket's `## Spec Impact` and the confirmed phrasing.
- Confirm the two absorbed idea tickets are already under `.dropped/`
  (verified on disk during survey — `260823-feat-note-write-oversize-relocate-nudge.md`
  and `260823-feat-notes-block-standing-postit-hint.md`); no action needed.

## Escalations

- None.
