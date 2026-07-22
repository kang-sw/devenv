# Plan: 260722-bug-e2e-open-work-root-locator-ambiguity — Phase 1: Disambiguate the openWorkRoot locator

## Relevant Ticket Contract
- `openWorkRootInBrowser`'s strict-mode locator `[data-command-id="rootPicker.open"]` must resolve to exactly one element.
- Design-reviewer guidance (binding): prefer narrowing the test helper's selector (scope to the primary control, or exclude `.open-work-root-empty-cta`) or adding a distinct `data-testid` to one button.
- Avoid changing either button's `data-command-id` value — `commands.ts` and `hotkeys.ts` key off `rootPicker.open` for dispatch/hotkey binding; retargeting it risks non-test regressions.
- Completion boundary: locator no longer double-matches AND the full `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` acceptance suite runs green end to end.
- Verification: run the acceptance suite (e.g. `npx playwright test dashboard-acceptance.spec.ts` from `ws-dashboard/frontend/`) and confirm all tests pass, including the first `openWorkRootInBrowser` step.

## Out of Scope
- Any other locator ambiguity or flake not tied to `rootPicker.open`/`openWorkRootInBrowser`.
- The `remoteRow.locator('[data-command-id="rootPicker.open"]')` call sites (spec.ts:3677, 3805, 3820, 3866, 3896, 3914) in the separate `"linked server root picker uses server-scoped local gateway routes"` test (spec.ts:3555) — these are already scoped to a single row and are unaffected by this change; confirmed that test never calls `openWorkRootInBrowser`.
- Multi-server duplication of the "icon" variant control (not exercised by the affected test; the test at spec.ts:912 only ever has a single connected/owner server, so this scenario is not in play here).
- Later phases of the parent ticket 260525 (not part of this ticket).

## Codebase Findings
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L590-L602` — `openWorkRootInBrowser` helper; line 591 builds the unscoped `page.locator('[data-command-id="rootPicker.open"]')` that double-matches. The returned `opener` locator is reused for `.click()` (591, 604) and `.toBeFocused()` (602), so the fix must keep `opener` a valid, singular, reusable locator across the whole helper.
- `ws-dashboard/frontend/src/App.tsx#L2185-L2225` — `OpenWorkRootControl` component definition; single component renders one of three variants (`"icon"`, `"section"`, `"empty"`) based on a prop, each variant rendering its own `<button>` with the same `data-command-id="rootPicker.open"`.
- `ws-dashboard/frontend/src/App.tsx#L2508-L2524` — the `"icon"`/`"section"` variant button (`openerButton`), no distinguishing class beyond `icon-button icon-button-primary`.
- `ws-dashboard/frontend/src/App.tsx#L2548-L2565` — the `"empty"` variant button, carries the distinguishing class `open-work-root-empty-cta` already (from commit `21116b54`) in addition to `data-command-id="rootPicker.open"`.
- `ws-dashboard/frontend/src/App.tsx#L4034-L4043` — usage 1: `OpenWorkRootControl variant="icon"` rendered per connected server row (inside the nav server list) whenever that server's `openRoot` action is enabled.
- `ws-dashboard/frontend/src/App.tsx#L11223-L11257` — usage 2: `EmptyWorkbenchPlaceholder` renders `OpenWorkRootControl variant="empty"` whenever no work root is selected (`.empty-workbench-cta` wrapper).
- Only two `OpenWorkRootControl` call sites exist in the codebase (`grep -n "OpenWorkRootControl" App.tsx` → lines 4037, 11248 only), confirming the double-match has exactly two possible sources and no others.
- The double-match is real and reproducible by inspection: at `spec.ts#L950-L951` (`test.step("open real workRoot")`, the very first call to `openWorkRootInBrowser`), the page has one connected (owner/local) server with `openRoot` enabled — producing one `"icon"` variant button — and no work root is yet selected — producing the `"empty"` variant CTA. Both are simultaneously in the DOM, so the unscoped locator matches 2 elements, matching the ticket's reported symptom exactly.
- `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L2874` — existing precedent in this same spec file for a CSS `:not()` exclusion combined with an attribute selector (`'[data-agent-chat-bubble-kind="user"]:not([data-testid="agent-chat-pending-bubble"])'`), confirming `:not()` on an attribute/class selector is an established, idiomatic pattern in this file's Playwright locators — not a novel technique.
- All 5 call sites of `openWorkRootInBrowser` (spec.ts:951, 987, 1718, 2414, 3408) are within the single serial test `"dashboard workRoot UI browser acceptance"` (spec.ts:912), which never connects a second server — so scoping the helper to exclude the empty-state CTA button is sufficient for every call site; no per-call-site special-casing is needed.

## Implementation Plan
1. In `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts#L591`, narrow the helper's locator to exclude the empty-state CTA button, changing:
   `page.locator('[data-command-id="rootPicker.open"]')`
   to:
   `page.locator('[data-command-id="rootPicker.open"]:not(.open-work-root-empty-cta)')`
   This is a test-only change — no source (`App.tsx`) edit is required, matching the design-reviewer's stated preference and avoiding any risk to `commands.ts`/`hotkeys.ts` dispatch behavior (`data-command-id` values are untouched, no new `data-testid` needed).
2. No other call sites need changes: the `remoteRow.locator('[data-command-id="rootPicker.open"]')` occurrences (spec.ts:3677 etc.) are already row-scoped in a different test and are not exposed to the empty-state CTA element, so they are unaffected and out of scope.
3. If, after running the suite (see Verification Plan), any other strict-mode locator collision or failing step surfaces unrelated to this ambiguity, treat it as a separate finding — do not silently broaden this fix's scope beyond the `rootPicker.open` locator itself; flag it back instead.

## Verification Plan
- From `ws-dashboard/frontend/`, run `npx playwright test dashboard-acceptance.spec.ts` and confirm the full suite passes, including the first `openWorkRootInBrowser` step in `test.step("open real workRoot")` (spec.ts:950).
- Specifically confirm no Playwright strict-mode violation is thrown for the `opener` locator at any of its 5 call sites (spec.ts:951, 987, 1718, 2414, 3408).
- Spot-check that `.toBeFocused()` (spec.ts:602) still resolves against the same singular `opener` locator after the modal-cancel flow, since the fix changes what `opener` refers to only in that it excludes the empty-cta element — the underlying icon-variant button and its focus behavior are unchanged.

## Escalations
- None.
