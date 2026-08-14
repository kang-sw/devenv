---
title: "# Notes workflow_manual tests false-fail on a # Notes substring collision"
related-mental-model:
  - mcp-runtime
---

# `# Notes` workflow_manual tests false-fail on a `# Notes` substring collision

## Problem

Two `internal/mcp` tests fail today, and have failed since before the 260814
manuals-anchor work — the failure is unrelated to the manuals surface:

- `TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState`
- `TestWorkflowManualNotesBlockAbsentWhenNoNotesExist`

Both live in `agents-plugin-tool/internal/mcp/note_workflow_manual_test.go` and
locate the ambient `# Notes` block with a naive
`strings.Index(body, "# Notes")` (see `assertNotesAfterSessionState`, ~L60, and
the absence assertion, ~L179). That substring also matches the prose heading
`### Notes / durable memory` in the lead workflow-manual rsrc
(`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:149`, added in
`fbec365f`): `"### Notes"` contains `"# Notes"`. So:

- the position test finds the prose match (before `## Session State`) and fails
  the "block must come after Session State" ordering assertion; and
- the absence test finds the prose match and fails the "must stay silent when no
  notes exist" assertion.

Confirmed pre-existing: both tests already fail at commit `6e12c9a2` (the commit
immediately before the Phase 1 manuals feat `525064f4`), where the prose heading
is present but no manuals change had landed.

## Impact

Low correctness risk — the product `# Notes` block behaves correctly; only the
test's locator is wrong. But the two red tests are persistent noise in every
`internal/mcp` run and train readers to ignore failures in that package, which
erodes the signal.

## Fix direction (not yet decided)

Make the test locator match the real block boundary instead of any `# Notes`
substring. Options: anchor on a line-start `\n# Notes\n` (the block is a
top-level `#` heading, the prose is `###`), search only the post–`## Session
State` region for the positive case, or assert on the block's full rendered
header line. Whatever is chosen must keep both the position test and the absence
test honest (the absence test must still fail if a real block is wrongly
emitted). Verify no other `strings.Index`/`Contains` probe in the mcp tests has
the same `###`-vs-`#` collision.

## Notes

Surfaced during 260814 Phase 2 (retire manuals.list/find) verification, where
the diagnostic dump showed a leading `# Manuals` block and briefly looked
manuals-related; it is not.
