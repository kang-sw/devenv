# Review794 UI-C1: bounded static Web popover repair

## Authority

MAIN assigned UI-C1 at 2026-11-14T14:36:29Z: static fix after H, parallel-worktree diagnosis allowed, mail the fix commit; tests run only in a later allocated slot. H stopped at its first a1 failure and released the heavy slot. This assignment consumes no heavy slot and authorizes no execution-based verification or daemon action.

## Outcome and evidence

Read the complete finding at `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/c8591ce0-01-sweep794-ui-correctness.md`, but fix ONLY UI-C1. Its pinned source computes viewport coordinates then applies them as absolute offsets inside a relative toolbar slot. Keep the Web popover inside the existing slot and convert the clamped viewport position into slot-relative offsets, following the terminal popover pattern. Preserve column/viewport containment, width/available-height treatment, opener placement and existing input/focus behavior. Do not change unrelated UI-C2 or redesign popovers.

## Scope

- Worktree: `/Users/kang-sw/.ws-worktrees/devenv/corporate-predator-shrug`, branch `fix/review794-web-popover`.
- Product edit: `agents-plugin-tool/internal/wsdashboard/assets/webcard.js`.
- Regression coverage authoring: `agents-plugin-tool/internal/wsdashboard/webcard_scripts_test.go`; if the existing suite cannot express the case without unrelated changes, report the gap rather than expanding scope.
- Include this immutable plan in the implementation commit. No ticket creation, ticket state changes, source changes elsewhere, new API, shared store, dependencies or broader architecture.

## Verification boundary

Read-only source/Git/manual inspection and static reasoning only. Author a focused regression for nonzero slot origins and relevant width/height bounds, preserving the same visible behavior. DO NOT RUN tests, builds, syntax checks, browser, benchmarks, helper/SDK imports, network probes, npm, installers or daemon/runtime operations. Do not label authored tests as passed. Record exact deferred test commands and limits for a later MAIN-allocated slot.

Read every matching project implementation manual before edits. Commit one logical unit with AI Context explaining the coordinate-space correction and explicitly unexecuted verification. Do not merge, rebase, reset, push, release the worktree, or chain review/test work. Return the fix commit, patch summary, static evidence, deferred commands, and any blocker. Source control integration and subsequent verification remain lead-owned.
