# Pi compact tool presentation

## Relevant Ticket Contract

Inline authority; no ticket creation or updates. Owner explicitly requests one implementer delegate and one independent reviewer. That overrides separate survey/research/documentation agents: the same implementer owns focused survey, implementation, verification, and later documentation payloads.

For the existing bridged YAML tool presentation:
- Keep a bold tool-title row and existing separated input/output backgrounds.
- Input text is white rather than gray; output stays gray. Use white on the active dark presentation; preserve theme-aware readability rather than making light-theme text invisible.
- Exactly one blank row between title and input and between input and output, unaffected by outer whitespace in displayed input. Trim only outer whitespace of the prepared input display, preserving internal whitespace and original model data.
- Input logical-line starts are indented four columns relative to the title. Automatically wrapped continuation rows are indented three columns relative to the title (one-column outdent).
- Use the accepted cheap width approximation: printable ASCII code points cost one column; non-ASCII code points cost two. Wrap long lines into the available content width, counting displayed content rows, not merely newline-separated logical lines. Account for indent and existing shell padding. No exact grapheme-width requirement; early wrapping of combining/complex emoji is accepted.
- Collapsed input and output previews show at most ten content rows. When more content exists append a separate `...` row (not part of the ten-content-row budget). Preserve expanded full output and existing input-capped behavior.
- Preserve original tool arguments/results/model-facing content, error/partial/unsupported-result fallbacks, title identity, dispatch and lifecycle.
- Normalize/sanitize controls and tabs for safe display before approximate width accounting. Preserve native terminal-safe final fitting and caching. Do not revive repeated whole-document grapheme segmentation/wrapping on every redraw.

## Out of Scope

No subagent push renderer changes: their ten-logical-line gray presentation was captured in a separate idea ticket, not requested for this implementation. No dispatch-tool headers/model-resolution plumbing, ws-mcp or shared rsrc edits, dependency redesign, ticket changes, automatic merge, or release.

## Codebase Findings

Recon evidence (implementer verifies current source): `agents-plugin-pi/src/tool-result-render.ts` owns YAML preparation, logicalPreview, native layout/clipping caches and styles. `src/bridge.ts` centrally attaches hooks. Tests in `test/tool-result-render.test.ts` include installed Pi composition. Input currently uses gray `toolOutput`, `toolPendingBg`, one additional left column and a top blank row; parent Box adds its own padding. Output uses gray `toolOutput` and `toolSuccessBg`.

History: `22987b75` rolled back expensive custom width rendering; `bbf9a29a` / `2c8c50a` replaced it with logical previews and native caches; `4d278c13` added bold titles/backgrounds; later input padding landed. Owner confirmed responsiveness restored, but exact costly call-site attribution is unavailable. Read relevant commit AI Context and current source rather than treating history as implementation authority.

## Implementation Plan

1. Check status and create `impl/track/pi-agent/pi-compact-tool-presentation` from `track/pi-agent` (base `27dda46aefb7d0c735c7a65e7c680994dd32372a`) before source changes. Do not touch unrelated untracked files.
2. Load relevant Pi extension/themes/TUI docs fully and necessary linked docs; read project instructions and focused spec. Complete a focused survey on this plan and commit the plan before source edits.
3. Implement the bounded display-only contract in the existing renderer/helper and tests. Prefer a linear, cached codepoint walk bounded by preview needs; preserve full original data for expanded output.
4. Run focused tests, full adapter suite/build checks available in package scripts, and installed-host composition probes. Commit source/tests as a logical unit with AI Context.
5. Return for a single independent review; receive any relay in this same implementer session. Documentation updates will follow review on lead instruction, without another delegate.

## Verification Plan

Pure wrapping/trim logic: tests first. Cover printable ASCII, CJK, surrogate-pair emoji, combining/ZWJ sequences (conservative counts), tabs/control sanitization, zero/narrow widths and widths smaller than indentation, input logical starts versus continuations, exactly ten versus eleven rows, marker presence only on truncation, outer-whitespace trimming and exact blank separators, white input/gray output, theme changes, full output expansion, payload identity, streaming/error/native fallback, and repeated-render cache reuse. Include real Pi parent-shell/native-component composition to check actual padding and rows. Run full tests and inspect complete output. Owner-live appearance/performance acceptance cannot be claimed from tests alone.

## Owner follow-up: native backgrounds and input-owned separator

After live inspection, the owner explicitly replaces the earlier separated
background requirement: remove custom input/output background overrides and
inherit Pi's native parent lifecycle background uniformly. Preserve the accepted
input/output foregrounds, indentation, approximate wrapping, markers, caches,
model payloads and native fallback behavior.

Correct the missing input-to-output blank row for raw fallback results such as
`todo.check`: the input/call component owns one trailing separator independently
of result shape; remove the YAML result's leading separator to avoid duplication.
Retain one title-to-input separator and trim outer input display whitespace.
Verify real installed parent composition for YAML success, raw-text fallback,
error fallback and pending/no-result states, distinguishing the one owned
separator from unchanged outer native shell padding. Do not redefine native shell
padding, lifecycle, or unsupported-result rendering to meet an artificial total
blank-row count at the end of a pending tool.

This is a same-slice follow-up on the retained implementation branch, with the
same implementer and independent reviewer; no tickets, new agents, or merge.
Commit this contract update before source edits, then code/tests; lead updates
spec after review. Preserve unrelated untracked files.

## Owner hotfix: input truncation marker

The owner approves a same-slice input-only marker correction. When collapsed
input reaches its ten-content-row budget, render the marker at the four-column
input-start indent and style it with `toolOutput`, independently from the
normal input `text` foreground. Do not count omitted rows by scanning or
wrapping the remainder: use gray `...`, not a count-bearing marker. Output
markers retain their current formatting.

The marker must still fit a single physical row at narrow widths by reducing
indentation and marker length as needed. Preserve the existing content budget,
physical-layout cache behavior, native backgrounds, call-owned separators, and
payload/fallback contracts. Add focused fake and installed-Pi composition tests
for input marker indent/color, narrow fitting, and unchanged redraw cost.

## Escalations

No open product decision. Stop and ask the lead if existing renderer constraints prevent this contract or implementation requires changes outside Pi adapter/spec scope. No merge without owner approval. Record any source/history discrepancy and unverified live behavior.
