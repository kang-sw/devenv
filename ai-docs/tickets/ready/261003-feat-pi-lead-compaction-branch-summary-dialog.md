---
title: Carry Pi branch summaries in the lead compaction dialog
related:
  261003-feat-pi-lead-compaction-dialog-transcript: amended; adds branch_summary entries to the `## Dialog` item kinds it defined
  261002-feat-pi-lead-ws-owned-compaction: origin of the lead-owned compaction summary
sage-review-design: skipped
sage-review-completeness: completed
sage-review-completeness-reviewed: 1dbe4ccd02d5739f
---

# Carry Pi branch summaries in the lead compaction dialog

## Background

Pi's `/tree` navigation can rewind the session to an earlier entry and,
when the user asks, summarize the abandoned branch. Pi appends that summary
as a `branch_summary` entry on the new path (`SessionManager.branchWithSummary`
in `@earendil-works/pi-coding-agent` `dist/core/session-manager.js`), and the
model sees it as a user-role message wrapped in Pi's branch-summary prefix.

The lead compaction summary's `## Dialog` section is built by
`collectDialogItems` (`agents-plugin-pi/src/lead-compaction.ts`), which reads
only `type: "message"` entries. A `branch_summary` entry on the current path
is ignored. On the lever path (`ws-compact`) the branch summary therefore
survives the next lead compaction only if the lead happens to restate it in
its prose. The fallback path already sees it through Pi's
`messagesToSummarize`.

Abandoned-branch messages themselves are correctly excluded: compaction reads
`branchEntries`, which is Pi's `getBranch()` root-to-leaf path.

## Decisions

- **A `branch_summary` entry on the current path becomes one `## Dialog` item**,
  in chronological position with the other items.
  - It renders with its own label line, `--- branch summary (<timestamp>) ---`,
    followed by the entry's `summary` text. The timestamp is formatted the
    same way as the existing user and assistant labels in
    `renderDialogItem`.
  - For tool-run folding it counts as a dialog message, so it ends a tool run
    like a user or assistant message does.
- **Same byte budget, no elision.**
  - The item competes in the same `pi.compaction_dialog_budget_bytes` budget
    and newest-first contiguous selection as every other item. When it does
    not fit whole, it is dropped whole and selection stops there, as for any
    item. So repeated rewinds never accumulate summaries without bound.
  - The head/tail long-message elision (over 2560 bytes) does not apply to
    it. It is already a compressed summary, and Pi caps it (2048 max tokens
    in `generateBranchSummary`), so cutting its middle would lose most of it.
  - Rejected: applying the user/assistant elision rule, which keeps only
    about 2 KiB of a summary that is typically 6-8 KB.
- **Source of the summary does not matter.** A hook-provided summary
  (`fromHook: true`) is carried the same way as Pi's built-in one.

## Constraints

- `compaction` entries on the path stay excluded from `## Dialog`; the
  lead's prose carries earlier compaction content, as today.
- No change to Pi tree navigation itself: the adapter adds no
  `session_before_tree` or `session_tree` behavior. Re-syncing ws state
  (agenda, todos, tickets, commits made on an abandoned branch) after a
  rewind is out of scope and deferred.
- Spawned worker, explore, and fork sessions keep Pi's native compaction,
  as in 261002.
- If `lead-compact-guide.md` or the `ws-compact` tool description enumerates
  what `## Dialog` contains, update it to match.

## Prior Decisions

- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, Decisions): "A user or assistant message over 2560 bytes keeps its first 1024 bytes and its last 1024 bytes, with `[... N bytes skipped ...]` between them" — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, Decisions): "Selection is newest-first within the byte budget. The section is always a contiguous newest run." — bearing: supports
- 261003-feat-pi-lead-compaction-dialog-transcript (2026-10-03, Decisions): "At most 8 tool lines between two dialog messages. A run of more consecutive tool calls keeps its last 8 lines." — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (4e7e8ad5e, Result): "Wording updated in the `ws-compact` tool description, `lead-compact-guide.md`, and the manifest key description." — bearing: constrains
- 261003-feat-pi-lead-compaction-dialog-transcript (4e7e8ad5e, Result): "the fallback summary now also reads Pi's would-be kept tail (`keptTailMessages`), because that tail is no longer kept raw" — bearing: supports
- 505cd037 (2026-10-03, commit): "Fixed elision constants (1 KiB head and 1 KiB tail over 2560 bytes; tool args 150+150 over 300 bytes) per the user's call that they need not be settings" — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (94cdf8bd, commit): "The prose is also passed as customInstructions so a degraded native compaction (summary-build failure) is still steered by it." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/lead-compaction.ts, agents-plugin-pi/test/lead-compaction.test.ts, possibly agents-plugin-pi/lead-compact-guide.md |
| scope.surface | cross-module | DialogItem is an exported union and collectDialogItems, renderDialogItem, foldToolRuns are exported from lead-compaction.ts; the ws-compact summary format changes |
| scope.new_public_symbol | no | none named; a new DialogItem variant extends an existing exported type |
| scope.new_type_contract | yes | DialogItem union in agents-plugin-pi/src/lead-compaction.ts#L195-L200 gains a branch_summary variant |
| scope.test_surface | existing | agents-plugin-pi/test/lead-compaction.test.ts |
| complexity.reuse_points | confirmed | collectDialogItems, foldToolRuns, renderDialogItem, selectDialogItems in agents-plugin-pi/src/lead-compaction.ts |
| complexity.side_effect_risk | low | pure functions feeding buildDialogSection; no config key or Pi hook changes |
| risk.correctness | moderate | the unelided branch must be a new switch case, and foldToolRuns must treat the new kind as a run-ender (it already does for any non-tool kind) |
| risk.fit | low | follows the existing item-kind pattern and the byte-budget selection |
| risk.test | moderate | new cases must be added to a large existing test file; tool-run ending is only indirectly covered today |
| risk.security_or_contract | moderate | changes the compaction summary text the lead reads; a summary larger than the budget drops whole and ends selection |

## Phases

### Phase 1: Branch summary dialog item

- Extend `collectDialogItems` and the dialog item rendering and selection in
  `agents-plugin-pi/src/lead-compaction.ts` per Decisions.

Done when:

- `npm test` in `agents-plugin-pi` passes.
- Tests pin:
  - a `branch_summary` entry renders as a labeled item in chronological
    order between dialog messages;
  - a branch summary over 2560 bytes is carried unelided;
  - it counts against the byte budget and is dropped whole, ending
    selection, when it does not fit;
  - it ends a tool run for folding purposes.
- `lead-compact-guide.md` and the `ws-compact` tool description were
  checked; the phase Result states whether either enumerates `## Dialog`
  item kinds and, if so, that it was updated.
